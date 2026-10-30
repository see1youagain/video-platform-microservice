package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	"github.com/see1youagain/video-platform-microservice/common/events"
	"github.com/see1youagain/video-platform-microservice/common/outbox"
	transcodeDb "video-platform-microservice/rpc-videoTranscode/internal/db"
)

// This test uses a real MySQL 8 server; sqlmock cannot establish SKIP LOCKED,
// concurrent claims or transaction behavior. It only runs in an explicit test DB.
func TestMySQLDurableTaskRecoveryIntegration(t *testing.T) {
	if os.Getenv("RUN_TRANSCODE_DB_INTEGRATION") != "1" {
		t.Skip("set RUN_TRANSCODE_DB_INTEGRATION=1 against isolated MySQL 8")
	}
	if !strings.HasSuffix(os.Getenv("DB_NAME"), "_test") {
		t.Fatal("real database test requires a DB_NAME ending in _test")
	}
	previous := commondb.GetDB()
	if err := commondb.InitDB(); err != nil {
		t.Fatal(err)
	}
	db := commondb.GetDB()
	t.Cleanup(func() { commondb.Close(); commondb.SetDB(previous) })
	if err := transcodeDb.Init(); err != nil {
		t.Fatal(err)
	}
	if err := outbox.NewRepository(db).InitSchema(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	prefix := "it_" + uuid.NewString()
	ids := []string{prefix}
	t.Cleanup(func() {
		db.Where("task_id IN ?", ids).Delete(&transcodeDb.Job{})
		db.Where("aggregate_id IN ?", ids).Delete(&outbox.Event{})
	})
	event := events.TranscodeTaskEvent{TaskID: prefix, FileHash: strings.Repeat("d", 32), UserID: "integration-worker", Resolutions: []string{"360p"}}
	if err := persistTask(ctx, event); err != nil {
		t.Fatal(err)
	}
	if err := persistTask(ctx, event); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&transcodeDb.Job{}).Where("task_id = ?", prefix).Count(&count)
	if count != 1 {
		t.Fatalf("redelivery created %d jobs", count)
	}
	first, err := claimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.TaskID != prefix || first.Attempts != 1 {
		t.Fatalf("claimed %+v", first)
	}
	// Simulate a crash: the first owner never committed, then its lease expires.
	if err := db.Model(&transcodeDb.Job{}).Where("task_id = ?", prefix).Update("lease_until", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	replacement, err := claimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.TaskID != prefix || replacement.LeaseOwner == first.LeaseOwner || replacement.Attempts != 2 {
		t.Fatalf("invalid recovery %+v", replacement)
	}
	if err := finishJob(ctx, first, []string{"https://stale/result.mp4"}, nil); !errors.Is(err, errLeaseLost) {
		t.Fatalf("stale owner committed: %v", err)
	}
	if err := finishJob(ctx, replacement, []string{"https://store/result.mp4"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := persistTask(ctx, event); err != nil {
		t.Fatal(err)
	}
	var done transcodeDb.Job
	if err := db.Where("task_id = ?", prefix).First(&done).Error; err != nil {
		t.Fatal(err)
	}
	if done.Status != "completed" || done.Progress != 100 || done.LeaseOwner != "" || done.LeaseUntil != nil {
		t.Fatalf("terminal state regressed %+v", done)
	}
	var publications []outbox.Event
	if err := db.Where("aggregate_id = ?", prefix).Find(&publications).Error; err != nil {
		t.Fatal(err)
	}
	if len(publications) != 1 || publications[0].Topic != events.TopicTranscodeFinished {
		t.Fatalf("publications=%+v", publications)
	}
	var result events.TranscodeFinishedEvent
	if err := json.Unmarshal([]byte(publications[0].Payload), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || len(result.URLs) != 1 || result.URLs[0] != "https://store/result.mp4" {
		t.Fatalf("result=%+v", result)
	}
	// Four concurrent owners must claim four distinct rows under SKIP LOCKED.
	for i := 0; i < 4; i++ {
		id := "it_" + uuid.NewString()
		ids = append(ids, id)
		copy := event
		copy.TaskID = id
		if err := persistTask(ctx, copy); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	claimed := make(chan *transcodeDb.Job, 4)
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, err := claimJob(ctx)
			if err != nil {
				failures <- err
				return
			}
			claimed <- job
		}()
	}
	wg.Wait()
	close(claimed)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	seen := map[string]bool{}
	for job := range claimed {
		if seen[job.TaskID] {
			t.Errorf("job claimed twice: %s", job.TaskID)
		}
		seen[job.TaskID] = true
	}
	if len(seen) != 4 {
		t.Fatalf("distinct jobs=%d", len(seen))
	}
}
