package consumer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	"github.com/see1youagain/video-platform-microservice/common/events"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func testDB(t *testing.T) sqlmock.Sqlmock {
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
func eventFixture() events.TranscodeFinishedEvent {
	return events.TranscodeFinishedEvent{TaskID: "tc-test", FileHash: strings.Repeat("a", 32), UserID: "7", Status: "completed", URLs: []string{"https://store/360p.mp4"}}
}
func taskRows(status, user string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "task_id", "file_hash", "user_id", "status"}).AddRow(1, "tc-test", strings.Repeat("a", 32), user, status)
}

func TestApplyFinishedValidatesBeforeTransaction(t *testing.T) {
	testDB(t)
	for _, event := range []events.TranscodeFinishedEvent{{}, {TaskID: "../task"}, {TaskID: "tc-test", UserID: "7", Status: "processing"}, {TaskID: "tc-test", UserID: "7", Status: "completed"}} {
		if err := ApplyTranscodeFinished(context.Background(), event); !errors.Is(err, ErrInvalidFinishedEvent) {
			t.Fatalf("event=%+v err=%v", event, err)
		}
	}
}
func TestApplyFinishedDuplicateDoesNotRegressTerminalState(t *testing.T) {
	for _, status := range []string{"completed", "failed"} {
		t.Run(status, func(t *testing.T) {
			mock := testDB(t)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT .* FROM `transcode_tasks`.*FOR UPDATE").WithArgs("tc-test", 1).WillReturnRows(taskRows(status, "7"))
			mock.ExpectCommit()
			if err := ApplyTranscodeFinished(context.Background(), eventFixture()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestApplyFinishedRejectsIdentityMismatch(t *testing.T) {
	mock := testDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `transcode_tasks`").WillReturnRows(taskRows("pending", "other-user"))
	mock.ExpectRollback()
	if err := ApplyTranscodeFinished(context.Background(), eventFixture()); !errors.Is(err, ErrInvalidFinishedEvent) {
		t.Fatal(err)
	}
}
func TestApplyFinishedCommitsTaskAndOwnedVideo(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed", true: "failed"}[failed], func(t *testing.T) {
			mock := testDB(t)
			event := eventFixture()
			progress, results, terminal, videoStatus, errorMsg := int32(100), `["https://store/360p.mp4"]`, "completed", "published", ""
			if failed {
				event.Status = "failed"
				event.ErrorMsg = "unsupported source"
				event.URLs = nil
				progress, results, terminal, videoStatus, errorMsg = 0, "[]", "failed", "transcode_failed", "unsupported source"
			}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT .* FROM `transcode_tasks`.*FOR UPDATE").WillReturnRows(taskRows("pending", "7"))
			mock.ExpectExec("UPDATE `transcode_tasks`").WithArgs(errorMsg, progress, results, terminal, sqlmock.AnyArg(), 1).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery("SELECT .* FROM `transcode_tasks`.*ORDER BY id DESC").WithArgs(event.FileHash, "7", 1).WillReturnRows(taskRows("pending", "7"))
			mock.ExpectExec("UPDATE `video_files`.*WHERE file_hash = \\? AND user_id = \\? AND status = \\?").WithArgs(videoStatus, results, sqlmock.AnyArg(), event.FileHash, "7", "finished").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			if err := ApplyTranscodeFinished(context.Background(), event); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestApplyFinishedOlderTaskDoesNotOverwriteNewVideoResult(t *testing.T) {
	mock := testDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `transcode_tasks`").WillReturnRows(taskRows("pending", "7"))
	mock.ExpectExec("UPDATE `transcode_tasks`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `transcode_tasks`.*ORDER BY id DESC").WillReturnRows(sqlmock.NewRows([]string{"id", "task_id"}).AddRow(2, "tc-newer"))
	mock.ExpectCommit()
	if err := ApplyTranscodeFinished(context.Background(), eventFixture()); err != nil {
		t.Fatal(err)
	}
}
func TestApplyFinishedRollsBackTaskIfVideoWriteFails(t *testing.T) {
	mock := testDB(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .* FROM `transcode_tasks`").WillReturnRows(taskRows("pending", "7"))
	mock.ExpectExec("UPDATE `transcode_tasks`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `transcode_tasks`").WillReturnRows(taskRows("pending", "7"))
	mock.ExpectExec("UPDATE `video_files`").WillReturnError(errors.New("database disconnected"))
	mock.ExpectRollback()
	if err := ApplyTranscodeFinished(context.Background(), eventFixture()); err == nil {
		t.Fatal("write failure was acknowledged")
	}
}
