package main

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/bytedance/gopkg/cloud/metainfo"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	"github.com/see1youagain/video-platform-microservice/common/events"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"video-platform-microservice/rpc-videoManager/kitex_gen/videomanager"
)

const testHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func managerMock(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	previous := commondb.GetDB()
	commondb.SetDB(db)
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		commondb.SetDB(previous)
		_ = sqlDB.Close()
	})
	return mock
}

func ownedVideo() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "file_hash", "user_id", "status"}).AddRow(1, testHash, "7", "finished")
}

func taskRow(status, resolutions string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "task_id", "file_hash", "user_id", "resolutions", "status", "progress", "result_urls", "error_msg"}).
		AddRow(11, "tc-existing", testHash, "7", resolutions, status, 100, `["https://store/result.mp4"]`, "invalid video")
}

func expectOwnedVideo(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `video_files`.*FOR UPDATE").WithArgs(testHash, "7", 1).WillReturnRows(ownedVideo())
}

func TestTranscodeRejectsInvalidInputWithoutDatabase(t *testing.T) {
	longID := strings.Repeat("x", 65)
	for _, req := range []*videomanager.TranscodeReq{
		nil, {}, {FileHash: "short", UserId: "7", Resolutions: []string{"360p"}},
		{FileHash: testHash, UserId: "7", Resolutions: []string{"4k"}},
		{FileHash: testHash, UserId: "7", Resolutions: []string{"360p"}, RequestId: &longID},
	} {
		resp, err := new(VideoManagerServiceImpl).Transcode(context.Background(), req)
		if err != nil || resp.Code != 400 || resp.TaskId != nil {
			t.Fatalf("resp=%+v err=%v", resp, err)
		}
	}
}

func TestTranscodeLocksOnlyAuthenticatedUsersVideo(t *testing.T) {
	mock := managerMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `video_files`.*FOR UPDATE").WithArgs(testHash, "8", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()
	ctx := metainfo.WithPersistentValue(context.Background(), "user_id", "8")
	resp, err := new(VideoManagerServiceImpl).Transcode(ctx, &videomanager.TranscodeReq{
		FileHash: testHash, UserId: "7", Resolutions: []string{"360p"},
	})
	if err != nil || resp.Code != 404 {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestTranscodeRequestReplayAndParameterConflict(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed replay", true: "parameter conflict"}[conflict], func(t *testing.T) {
			mock := managerMock(t)
			expectOwnedVideo(mock)
			resolutions := `["360p"]`
			if conflict {
				resolutions = `["720p"]`
			}
			mock.ExpectQuery("SELECT .* FROM `transcode_tasks`.*idempotency_key").
				WithArgs(sqlmock.AnyArg(), 1).WillReturnRows(taskRow("completed", resolutions))
			mock.ExpectCommit()
			id := "same-request"
			resp, err := new(VideoManagerServiceImpl).Transcode(context.Background(), &videomanager.TranscodeReq{
				FileHash: testHash, UserId: "7", Resolutions: []string{"360p", "360p"}, RequestId: &id,
			})
			if err != nil {
				t.Fatal(err)
			}
			if conflict {
				if resp.Code != 409 || resp.TaskId != nil {
					t.Fatalf("conflict resp=%+v", resp)
				}
			} else if resp.Code != 200 || resp.GetTaskId() != "tc-existing" {
				t.Fatalf("replay resp=%+v", resp)
			}
		})
	}
}

func TestTranscodeReusesMatchingActiveTask(t *testing.T) {
	mock := managerMock(t)
	expectOwnedVideo(mock)
	mock.ExpectQuery("SELECT .* FROM `transcode_tasks`.*status IN").
		WithArgs(testHash, "7", "pending", "processing", 1).WillReturnRows(taskRow("pending", `["360p"]`))
	mock.ExpectCommit()
	resp, err := new(VideoManagerServiceImpl).Transcode(context.Background(), &videomanager.TranscodeReq{
		FileHash: testHash, UserId: "7", Resolutions: []string{"360p"},
	})
	if err != nil || resp.Code != 200 || resp.GetTaskId() != "tc-existing" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

type capturedTaskID struct{ value *string }

func (a capturedTaskID) Match(value driver.Value) bool {
	s, ok := value.(string)
	if !ok || !events.ValidTaskID(s) {
		return false
	}
	*a.value = s
	return true
}

type taskPayload struct{ taskID *string }

func (a taskPayload) Match(value driver.Value) bool {
	s, ok := value.(string)
	if !ok {
		return false
	}
	var event events.TranscodeTaskEvent
	return json.Unmarshal([]byte(s), &event) == nil && event.TaskID == *a.taskID &&
		event.FileHash == testHash && event.UserID == "7" && reflect.DeepEqual(event.Resolutions, []string{"360p"})
}

func TestTranscodeTransactionIncludesCanonicalTopicAndRollsBackOutboxFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "outbox rollback"}[fail], func(t *testing.T) {
			mock := managerMock(t)
			expectOwnedVideo(mock)
			mock.ExpectQuery("SELECT .* FROM `transcode_tasks`.*status IN").WillReturnRows(sqlmock.NewRows([]string{"id"}))
			var taskID string
			mock.ExpectExec("INSERT INTO `transcode_tasks`").WithArgs(
				capturedTaskID{&taskID}, testHash, "7", `["360p"]`, "pending", 0, "", "", nil, "", sqlmock.AnyArg(), sqlmock.AnyArg(),
			).WillReturnResult(sqlmock.NewResult(11, 1))
			mock.ExpectExec("UPDATE `video_files`").WithArgs("pending", sqlmock.AnyArg(), 1).WillReturnResult(sqlmock.NewResult(0, 1))
			publication := mock.ExpectExec("INSERT INTO `outbox_events`").WithArgs(
				"transcode_task", sqlmock.AnyArg(), events.TopicTranscodeTasks, testHash, taskPayload{&taskID}, "", 0, 0,
				sqlmock.AnyArg(), nil, "", "", nil, sqlmock.AnyArg(), sqlmock.AnyArg(),
			)
			if fail {
				publication.WillReturnError(errors.New("outbox disconnected"))
				mock.ExpectRollback()
			} else {
				publication.WillReturnResult(sqlmock.NewResult(12, 1))
				mock.ExpectCommit()
			}
			resp, err := new(VideoManagerServiceImpl).Transcode(context.Background(), &videomanager.TranscodeReq{
				FileHash: testHash, UserId: "7", Resolutions: []string{"360p", "360p"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if fail {
				if resp.Code != 500 || resp.TaskId != nil {
					t.Fatalf("failed commit exposed task: %+v", resp)
				}
			} else if resp.Code != 200 || resp.GetTaskId() != taskID {
				t.Fatalf("resp=%+v task=%s", resp, taskID)
			}
		})
	}
}

func TestTranscodeStatusRequiresOwnerAndReturnsTerminalResult(t *testing.T) {
	service := new(VideoManagerServiceImpl)
	resp, _ := service.GetTranscodeStatus(context.Background(), &videomanager.GetTranscodeStatusReq{TaskId: "tc-existing"})
	if resp.Code != 401 {
		t.Fatalf("unauthenticated resp=%+v", resp)
	}
	for _, state := range []string{"completed", "failed", "pending"} {
		t.Run(state, func(t *testing.T) {
			mock := managerMock(t)
			mock.ExpectQuery("SELECT .* FROM `transcode_tasks`").WithArgs("tc-existing", "7", 1).WillReturnRows(taskRow(state, `["360p"]`))
			if state == "pending" {
				mock.ExpectQuery("SELECT status, progress FROM `transcode_jobs`").WithArgs("tc-existing", "7", 1).
					WillReturnRows(sqlmock.NewRows([]string{"status", "progress"}).AddRow("processing", 42))
			}
			ctx := metainfo.WithPersistentValue(context.Background(), "user_id", "7")
			resp, err := service.GetTranscodeStatus(ctx, &videomanager.GetTranscodeStatusReq{TaskId: "tc-existing"})
			if err != nil || resp.Code != 200 {
				t.Fatalf("resp=%+v err=%v", resp, err)
			}
			switch state {
			case "completed":
				if resp.GetStatus() != state || resp.GetProgress() != 1 || len(resp.CompletedUrls) != 1 {
					t.Fatalf("resp=%+v", resp)
				}
			case "failed":
				if resp.GetStatus() != state || len(resp.CompletedUrls) != 0 || !strings.Contains(resp.Msg, "invalid video") {
					t.Fatalf("resp=%+v", resp)
				}
			case "pending":
				if resp.GetStatus() != "processing" || resp.GetProgress() != 0.42 || len(resp.CompletedUrls) != 0 {
					t.Fatalf("resp=%+v", resp)
				}
			}
		})
	}
}
