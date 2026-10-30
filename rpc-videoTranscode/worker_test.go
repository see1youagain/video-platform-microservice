package main

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	"github.com/see1youagain/video-platform-microservice/common/events"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	transcodeDb "video-platform-microservice/rpc-videoTranscode/internal/db"
)

func mockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	old := commondb.GetDB()
	commondb.SetDB(db)
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		commondb.SetDB(old)
		sqlDB.Close()
	})
	return mock
}
func jobRows(status, owner string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "task_id", "file_hash", "user_id", "status", "resolutions", "lease_owner", "attempts"}).AddRow(1, "tc-test", strings.Repeat("a", 32), "7", status, `["360p"]`, owner, 1)
}

func TestPersistTaskRedeliveryIsIdempotent(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "same identity", true: "conflicting identity"}[conflict], func(t *testing.T) {
			mock := mockDB(t)
			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO `transcode_jobs`").WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
			rows := jobRows("completed", "")
			if conflict {
				rows = sqlmock.NewRows([]string{"task_id", "file_hash", "user_id", "resolutions"}).AddRow("tc-test", strings.Repeat("b", 32), "7", `["360p"]`)
			}
			mock.ExpectQuery("SELECT .* FROM `transcode_jobs`").WithArgs("tc-test", 1).WillReturnRows(rows)
			err := persistTask(context.Background(), events.TranscodeTaskEvent{TaskID: "tc-test", FileHash: strings.Repeat("a", 32), UserID: "7", Resolutions: []string{"360p", "360p"}})
			if conflict && !errors.Is(err, errInvalidTask) {
				t.Fatalf("conflict err=%v", err)
			}
			if !conflict && err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestPersistTaskRejectsInvalidEventBeforeDB(t *testing.T) {
	mockDB(t)
	if err := persistTask(context.Background(), events.TranscodeTaskEvent{TaskID: "../../escape"}); !errors.Is(err, errInvalidTask) {
		t.Fatal(err)
	}
}

func TestClaimJobLocksAndLeasesWork(t *testing.T) {
	for _, status := range []string{"pending", "processing"} {
		t.Run(status, func(t *testing.T) {
			mock := mockDB(t)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT .* FROM `transcode_jobs`.*FOR UPDATE SKIP LOCKED").WithArgs("pending", "processing", sqlmock.AnyArg(), 1).WillReturnRows(jobRows(status, "old-owner"))
			mock.ExpectExec("UPDATE `transcode_jobs` SET .*lease_owner.*lease_until.* WHERE `id` = \\?").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			job, err := claimJob(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if job.LeaseOwner == "" || job.LeaseOwner == "old-owner" || job.LeaseUntil == nil || !job.LeaseUntil.After(time.Now()) || job.Attempts < 2 {
				t.Fatalf("invalid claim %+v", job)
			}
		})
	}
}
func TestClaimJobRollsBackWhenQueueEmpty(t *testing.T) {
	mock := mockDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `transcode_jobs`").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	if _, err := claimJob(context.Background()); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal(err)
	}
}

func ownedTestJob() *transcodeDb.Job {
	until := time.Now().Add(time.Minute)
	return &transcodeDb.Job{TaskID: "tc-test", FileHash: strings.Repeat("a", 32), UserID: "7", Status: "processing", LeaseOwner: "owner", LeaseUntil: &until}
}

type finishedPayload struct{ status string }

func (a finishedPayload) Match(value driver.Value) bool {
	var event events.TranscodeFinishedEvent
	text, ok := value.(string)
	if !ok || json.Unmarshal([]byte(text), &event) != nil {
		return false
	}
	if event.TaskID != "tc-test" || event.UserID != "7" || event.FileHash != strings.Repeat("a", 32) || event.Status != a.status {
		return false
	}
	if a.status == "failed" {
		return len(event.URLs) == 0 && event.ErrorMsg == "invalid media"
	}
	return len(event.URLs) == 1 && event.URLs[0] == "https://store/360p.mp4" && event.ErrorMsg == ""
}
func TestFinishJobCommitsTerminalStateAndEventTogether(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed", true: "failed"}[failed], func(t *testing.T) {
			mock := mockDB(t)
			status, message, progress, urls := "completed", "", int32(100), `["https://store/360p.mp4"]`
			if failed {
				status, message, progress, urls = "failed", "invalid media", 0, "[]"
			}
			mock.ExpectBegin()
			mock.ExpectExec("UPDATE `transcode_jobs` SET .* WHERE task_id = \\? AND status = \\? AND lease_owner = \\? AND lease_until > \\?").WithArgs(message, "", nil, progress, urls, status, sqlmock.AnyArg(), "tc-test", "processing", "owner", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("INSERT INTO `outbox_events`").WithArgs("transcode_job", "tc-test", events.TopicTranscodeFinished, "tc:tc-test", finishedPayload{status: status}, "", 0, 0, sqlmock.AnyArg(), nil, "", "", nil, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
			var failure error
			if failed {
				failure = errors.New("invalid media")
			}
			if err := finishJob(context.Background(), ownedTestJob(), []string{"https://store/360p.mp4"}, failure); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestFinishJobFencesStaleWorker(t *testing.T) {
	mock := mockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `transcode_jobs`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	if err := finishJob(context.Background(), ownedTestJob(), nil, nil); !errors.Is(err, errLeaseLost) {
		t.Fatal(err)
	}
}
func TestFinishJobRollsBackIfOutboxFails(t *testing.T) {
	mock := mockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `transcode_jobs`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO `outbox_events`").WillReturnError(errors.New("outbox unavailable"))
	mock.ExpectRollback()
	if err := finishJob(context.Background(), ownedTestJob(), []string{"url"}, nil); err == nil {
		t.Fatal("outbox error was ignored")
	}
}

func TestPositiveIntConfiguration(t *testing.T) {
	for _, value := range []string{"0", "-1", "65", "garbage", "2oops"} {
		if _, err := positiveInt(value, 2); err == nil {
			t.Fatalf("invalid config accepted: %q", value)
		}
	}
	if n, err := positiveInt("", 2); n != 2 || err != nil {
		t.Fatal(n, err)
	}
}
