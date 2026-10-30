package consumer

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	"github.com/see1youagain/video-platform-microservice/common/events"
	managerdb "video-platform-microservice/rpc-videoManager/internal/db"
)

func TestMySQLFinishedEventConvergenceIntegration(t *testing.T) {
	if os.Getenv("RUN_TRANSCODE_DB_INTEGRATION") != "1" {
		t.Skip("set RUN_TRANSCODE_DB_INTEGRATION=1 against isolated MySQL 8")
	}
	if !strings.HasSuffix(os.Getenv("DB_NAME"), "_test") {
		t.Fatal("real database test requires DB_NAME ending in _test")
	}
	previous := commondb.GetDB()
	if err := commondb.InitDB(); err != nil {
		t.Fatal(err)
	}
	db := commondb.GetDB()
	t.Cleanup(func() { commondb.Close(); commondb.SetDB(previous) })
	if err := managerdb.Init(); err != nil {
		t.Fatal(err)
	}
	user := "it_" + uuid.NewString()
	hash := strings.Repeat("e", 32)
	oldID := "it_" + uuid.NewString()
	newID := "it_" + uuid.NewString()
	failedID := "it_" + uuid.NewString()
	t.Cleanup(func() {
		db.Where("user_id = ?", user).Delete(&managerdb.TranscodeTask{})
		db.Where("user_id = ?", user).Delete(&managerdb.File{})
	})
	video := managerdb.File{FileHash: hash, UserID: user, Filename: "integration.mp4", Status: "finished", TranscodeStatus: "pending"}
	if err := db.Create(&video).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{oldID, newID} {
		if err := db.Create(&managerdb.TranscodeTask{TaskID: id, FileHash: hash, UserID: user, Status: "pending", Resolutions: `["360p"]`}).Error; err != nil {
			t.Fatal(err)
		}
	}
	event := events.TranscodeFinishedEvent{TaskID: oldID, FileHash: hash, UserID: user, Status: "completed", URLs: []string{"https://old/result.mp4"}}
	if err := ApplyTranscodeFinished(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	db.First(&video, video.ID)
	if video.TranscodeStatus != "pending" {
		t.Fatal("older result overwrote newer task's video state")
	}
	event.TaskID = newID
	event.URLs = []string{"https://new/result.mp4"}
	if err := ApplyTranscodeFinished(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	db.First(&video, video.ID)
	if video.TranscodeStatus != "published" || video.TranscodeURLs != `["https://new/result.mp4"]` {
		t.Fatalf("metadata failed to converge %+v", video)
	}
	event.Status = "failed"
	event.ErrorMsg = "late duplicate failure"
	event.URLs = nil
	if err := ApplyTranscodeFinished(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	var task managerdb.TranscodeTask
	db.Where("task_id = ?", newID).First(&task)
	if task.Status != "completed" || task.Progress != 100 {
		t.Fatalf("duplicate regressed task %+v", task)
	}
	event.UserID = "other"
	if err := ApplyTranscodeFinished(context.Background(), event); !errors.Is(err, ErrInvalidFinishedEvent) {
		t.Fatalf("identity mismatch accepted: %v", err)
	}
	if err := db.Create(&managerdb.TranscodeTask{TaskID: failedID, FileHash: hash, UserID: user, Status: "pending", Resolutions: `["480p"]`}).Error; err != nil {
		t.Fatal(err)
	}
	event.TaskID, event.UserID, event.ErrorMsg = failedID, user, "corrupt source"
	if err := ApplyTranscodeFinished(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	db.First(&video, video.ID)
	if video.TranscodeStatus != "transcode_failed" || video.TranscodeURLs != "[]" {
		t.Fatalf("failure did not converge %+v", video)
	}
}
