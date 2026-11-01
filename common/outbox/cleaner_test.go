package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// A string such as "published" is coerced to numeric zero by MySQL and would
// match pending events. Insist on the numeric sent status and retention cutoff.
func TestCleanerDeletesOnlyExpiredSentEvents(t *testing.T) {
	repo, mock := mockRepository(t)
	retention := 24 * time.Hour
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `outbox_events` WHERE status = \\? AND updated_at < \\?").
		WithArgs(StatusSent, nearTime{offset: -retention}).
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()
	repo.cleanOutdatedEvents(retention)
}

func TestCleanerDatabaseFailureRollsBackWithoutBroadeningDelete(t *testing.T) {
	repo, mock := mockRepository(t)
	retention := 7 * 24 * time.Hour
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM `outbox_events` WHERE status = \\? AND updated_at < \\?").
		WithArgs(StatusSent, nearTime{offset: -retention}).
		WillReturnError(errors.New("database unavailable"))
	mock.ExpectRollback()
	repo.cleanOutdatedEvents(retention)
}

func TestCleanerCanceledContextDoesNotDeleteEvents(t *testing.T) {
	repo, _ := mockRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		repo.StartCleaner(ctx, time.Hour, 24*time.Hour)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleaner did not stop after cancellation")
	}
}
