package outbox

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func mockRepository(t *testing.T) (*Repository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		sqlDB.Close()
	})
	return NewRepository(db), mock
}

type nearTime struct{ offset time.Duration }

func (a nearTime) Match(value driver.Value) bool {
	v, ok := value.(time.Time)
	return ok && time.Since(v.Add(-a.offset)) < 5*time.Second && time.Since(v.Add(-a.offset)) > -5*time.Second
}

func expectClaim(mock sqlmock.Sqlmock, rows *sqlmock.Rows) {
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM outbox_events WHERE \\(status = \\? AND available_at <= \\?\\) OR \\(status = \\? AND \\(locked_at IS NULL OR locked_at <= \\?\\)\\).*FOR UPDATE SKIP LOCKED").WithArgs(StatusPending, nearTime{}, StatusProcessing, nearTime{offset: -5 * time.Minute}, 100).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(9))
	mock.ExpectExec("UPDATE `outbox_events` SET").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT .* FROM `outbox_events`").WithArgs(9).WillReturnRows(rows)
	mock.ExpectCommit()
}
func TestClaimBatchIncludesExpiredProcessingEvents(t *testing.T) {
	repo, mock := mockRepository(t)
	expectClaim(mock, sqlmock.NewRows([]string{"id", "topic", "payload", "status", "locked_by"}).AddRow(9, "video.transcode.tasks", "{}", StatusProcessing, "new-lease"))
	rows, err := repo.ClaimBatch("test-worker", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Status != StatusProcessing {
		t.Fatalf("events=%v", rows)
	}
}
func TestClaimBatchReturnsEmptyWithoutWriting(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM outbox_events").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()
	rows, err := repo.ClaimBatch("worker", 50)
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
}
func TestClaimBatchRollsBackOnDatabaseFailure(t *testing.T) {
	repo, mock := mockRepository(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM outbox_events").WillReturnError(errors.New("database unavailable"))
	mock.ExpectRollback()
	if _, err := repo.ClaimBatch("worker", 50); err == nil {
		t.Fatal("claim error ignored")
	}
}

func TestMarkRetryUsesBoundedBackoffAndTruncatesError(t *testing.T) {
	for _, attempt := range []int32{0, 12} {
		t.Run(map[int32]string{0: "first retry", 12: "bounded retry"}[attempt], func(t *testing.T) {
			repo, mock := mockRepository(t)
			seconds := 2 * time.Second
			if attempt == 12 {
				seconds = 300 * time.Second
			}
			mock.ExpectBegin()
			mock.ExpectExec("UPDATE `outbox_events` SET").WithArgs(nearTime{offset: seconds}, strings.Repeat("x", 2000), nil, "", attempt+1, StatusPending, sqlmock.AnyArg(), 9).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			if err := repo.MarkRetry(9, strings.Repeat("x", 3000), attempt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDispatcherPublishFailureRemainsRetryable(t *testing.T) {
	repo, mock := mockRepository(t)
	expectClaim(mock, sqlmock.NewRows([]string{"id", "topic", "message_key", "payload", "status", "retry_count", "locked_by"}).AddRow(9, "video.transcode.tasks", "task-key", "payload", StatusProcessing, 0, "lease-owner"))
	// The publisher first renews ownership, then records a retry under that same lease.
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `outbox_events` SET.*WHERE id = \\? AND status = \\? AND locked_by = \\?").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), 9, StatusProcessing, "lease-owner").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `outbox_events` SET.*locked_by = \\? AND status = \\?").WithArgs(nearTime{offset: 2 * time.Second}, "publish failed: broker unavailable", nil, "", 1, StatusPending, sqlmock.AnyArg(), "lease-owner", StatusProcessing, 9).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	called := false
	d := NewDispatcher(repo, "worker", 100, time.Second, func(_ context.Context, topic, key, payload string) error {
		called = true
		if topic != "video.transcode.tasks" || key != "task-key" || payload != "payload" {
			t.Fatalf("unexpected publication %s %s %s", topic, key, payload)
		}
		return errors.New("broker unavailable")
	})
	d.dispatchOnce(context.Background())
	if !called {
		t.Fatal("event never published")
	}
}
