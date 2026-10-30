package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	"github.com/see1youagain/video-platform-microservice/common/events"
	commonMinio "github.com/see1youagain/video-platform-microservice/common/minio"
	"github.com/see1youagain/video-platform-microservice/common/outbox"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	transcodeDb "video-platform-microservice/rpc-videoTranscode/internal/db"
	"video-platform-microservice/rpc-videoTranscode/internal/engine"
)

const jobLease = 90 * time.Second

var errLeaseLost = errors.New("transcode job lease lost")

type minioStore struct{}

func (minioStore) Download(ctx context.Context, object, path string) error {
	return commonMinio.DownloadFile(ctx, object, path)
}

func (minioStore) Upload(ctx context.Context, object, path string) (string, error) {
	return commonMinio.UploadFile(ctx, object, path, "video/mp4")
}

func (minioStore) Delete(ctx context.Context, object string) error {
	return commonMinio.DeleteObject(ctx, object)
}

// StartWorkers uses MySQL as a durable queue. The lease fences crashed/stale
// workers; abandoned processing jobs become eligible after the lease expires.
func StartWorkers(ctx context.Context, encoder *engine.Engine, count int) *sync.WaitGroup {
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				job, err := claimJob(ctx)
				if err != nil {
					if !errors.Is(err, gorm.ErrRecordNotFound) && ctx.Err() == nil {
						log.Printf("[transcode] claim job: %v", err)
					}
					if !waitRetry(ctx) {
						return
					}
					continue
				}
				runJob(ctx, encoder, job)
			}
		}()
	}
	return &wg
}

func claimJob(ctx context.Context) (*transcodeDb.Job, error) {
	var job transcodeDb.Job
	err := commondb.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? OR (status = ? AND (lease_until IS NULL OR lease_until <= ?))", "pending", "processing", now).
			Order("id ASC").First(&job).Error; err != nil {
			return err
		}
		owner := uuid.NewString()
		until := now.Add(jobLease)
		attempt := job.Attempts + 1
		if err := tx.Model(&job).Updates(map[string]any{
			"status": "processing", "lease_owner": owner, "lease_until": until,
			"attempts": attempt, "progress": 0, "result_url": "[]", "error_msg": "",
		}).Error; err != nil {
			return err
		}
		job.LeaseOwner, job.LeaseUntil = owner, &until
		job.Attempts = attempt
		return nil
	})
	return &job, err
}

func ownedJob(tx *gorm.DB, job *transcodeDb.Job) *gorm.DB {
	return tx.Model(&transcodeDb.Job{}).Where("task_id = ? AND status = ? AND lease_owner = ? AND lease_until > ?",
		job.TaskID, "processing", job.LeaseOwner, time.Now())
}

func runJob(parent context.Context, encoder *engine.Engine, job *transcodeDb.Job) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stopHeartbeat := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(jobLease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stopHeartbeat:
				return
			case <-ticker.C:
				result := ownedJob(commondb.GetDB().WithContext(ctx), job).Update("lease_until", time.Now().Add(jobLease))
				if result.Error != nil || result.RowsAffected != 1 {
					cancel()
					return
				}
			}
		}
	}()
	var resolutions []string
	err := json.Unmarshal([]byte(job.Resolutions), &resolutions)
	var urls []string
	if err == nil {
		// Each lease gets separate object keys, so a late stale worker cannot
		// overwrite or clean up the replacement worker's output.
		urls, err = encoder.Transcode(ctx, job.LeaseOwner, job.FileHash, resolutions, func(progress int32) error {
			result := ownedJob(commondb.GetDB().WithContext(ctx), job).Update("progress", progress)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errLeaseLost
			}
			return nil
		})
	}
	close(stopHeartbeat)
	<-heartbeatDone
	if parent.Err() != nil || (ctx.Err() != nil && !errors.Is(err, context.DeadlineExceeded)) {
		// Shutdown or an uncertain heartbeat leaves durable work retryable.
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer releaseCancel()
		_ = ownedJob(commondb.GetDB().WithContext(releaseCtx), job).Updates(map[string]any{
			"status": "pending", "lease_owner": "", "lease_until": nil,
		}).Error
		return
	}
	if finishErr := finishJob(parent, job, urls, err); finishErr != nil {
		// No terminal event was committed. The lease will expire and recover.
		log.Printf("[transcode] persist result task=%s: %v", job.TaskID, finishErr)
	}
}

func finishJob(ctx context.Context, job *transcodeDb.Job, urls []string, encodeErr error) error {
	status, message := "completed", ""
	progress := int32(100)
	if encodeErr != nil {
		status, message, progress = "failed", encodeErr.Error(), 0
		urls = []string{}
		if len(message) > 4000 {
			message = message[:4000]
		}
	}
	encoded, err := json.Marshal(urls)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(events.TranscodeFinishedEvent{
		TaskID: job.TaskID, FileHash: job.FileHash, UserID: job.UserID,
		Status: status, URLs: urls, ErrorMsg: message,
	})
	if err != nil {
		return err
	}
	return commondb.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := ownedJob(tx, job).Updates(map[string]any{
			"status": status, "progress": progress, "result_url": string(encoded),
			"error_msg": message, "lease_owner": "", "lease_until": nil,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errLeaseLost
		}
		return outbox.NewRepository(tx).EnqueueTx(tx, outbox.Message{
			AggregateType: "transcode_job", AggregateID: job.TaskID,
			Topic: events.TopicTranscodeFinished, MessageKey: "tc:" + job.TaskID, Payload: string(payload),
		})
	})
}

func positiveInt(value string, fallback int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.Atoi(value)
	if err != nil || number <= 0 || number > 64 {
		return 0, fmt.Errorf("expected integer between 1 and 64: %q", value)
	}
	return number, nil
}
