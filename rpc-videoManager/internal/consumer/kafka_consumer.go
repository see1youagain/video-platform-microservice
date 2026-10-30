// Package consumer 提供 rpc-video 服务的 Kafka 事件消费逻辑：
//   - 订阅 video.file.uploaded → 在数据库中创建"转码中"视频记录
//   - 订阅 video.transcode.finished → 更新视频记录状态为"已发布"
//   - 补偿任务：每 5 分钟扫描状态为"转码中"超过 1 小时的记录，主动查询 videoTranscode
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"video-platform-microservice/rpc-videoManager/internal/db"

	commonEvents "github.com/see1youagain/video-platform-microservice/common/events"
	commonKafka "github.com/see1youagain/video-platform-microservice/common/kafka"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StartConsumers 启动所有 Kafka 消费者和补偿任务
func StartConsumers(ctx context.Context) {
	go consumeFileUploaded(ctx)
	go consumeTranscodeFinished(ctx)
	go compensationTask(ctx)
}

// ─── FileUploaded 消费者 ───────────────────────────────────────────────────

func consumeFileUploaded(ctx context.Context) {
	reader := commonKafka.NewReader(commonEvents.TopicFileUploaded, "video-service")

	log.Printf("[video-consumer] 订阅 %s (group=video-service)", commonEvents.TopicFileUploaded)
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[video-consumer] FileUploaded 读取失败: %v", err)
			time.Sleep(time.Second)
			continue
		}

		var event commonEvents.FileUploadedEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			log.Printf("[video-consumer] 解析 FileUploaded 失败: %v", err)
			continue
		}

		log.Printf("[video-consumer] 收到 FileUploaded fileHash=%s user=%s", event.FileHash, event.UserID)

		// 幂等：若记录已存在则跳过
		exists, _, err := db.FileExistsByHashAndUser(event.FileHash, event.UserID)
		if err != nil {
			log.Printf("[video-consumer] DB 检查失败: %v", err)
			continue
		}
		if exists {
			log.Printf("[video-consumer] 幂等：fileHash=%s 已存在，跳过", event.FileHash)
			continue
		}

		// 创建"转码中"视频记录
		file := &db.File{
			FileHash:        event.FileHash,
			UserID:          event.UserID,
			Filename:        event.Filename,
			FileSize:        event.FileSize,
			URL:             event.MinioURL,
			Status:          "finished",
			TranscodeStatus: "pending",
			Width:           event.Width,
			Height:          event.Height,
			RequestID:       event.RequestID,
		}
		if createErr := db.CreateFileRecord(file); createErr != nil {
			log.Printf("[video-consumer] 创建视频记录失败 fileHash=%s: %v", event.FileHash, createErr)
		} else {
			log.Printf("[video-consumer] 视频记录已创建（转码中）fileHash=%s", event.FileHash)
		}
	}
}

// ─── TranscodeFinished 消费者 ─────────────────────────────────────────────

func consumeTranscodeFinished(ctx context.Context) {
	reader := commonKafka.NewReader(commonEvents.TopicTranscodeFinished, "video-service")
	defer reader.Close()
	for ctx.Err() == nil {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if !retryWait(ctx) {
				return
			}
			continue
		}
		for ctx.Err() == nil {
			var event commonEvents.TranscodeFinishedEvent
			err = json.Unmarshal(msg.Value, &event)
			if err == nil {
				err = ApplyTranscodeFinished(ctx, event)
			} else {
				log.Printf("[video-consumer] invalid finished JSON: %v", err)
				err = nil
			}
			if errors.Is(err, ErrInvalidFinishedEvent) {
				log.Printf("[video-consumer] rejected finished event: %v", err)
				err = nil
			}
			if err == nil {
				err = reader.CommitMessage(ctx, msg)
			}
			if err == nil {
				break
			}
			log.Printf("[video-consumer] finished event apply/commit: %v", err)
			if !retryWait(ctx) {
				return
			}
		}
	}
}

var ErrInvalidFinishedEvent = errors.New("invalid transcode finished event")

// ApplyTranscodeFinished atomically updates the task and its owned video.
// Duplicate terminal events and results from older tasks cannot regress state.
func ApplyTranscodeFinished(ctx context.Context, event commonEvents.TranscodeFinishedEvent) error {
	if !commonEvents.ValidTaskID(event.TaskID) || event.UserID == "" ||
		(event.Status != "completed" && event.Status != "failed") ||
		(event.Status == "completed" && len(event.URLs) == 0) {
		return ErrInvalidFinishedEvent
	}
	return db.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task db.TranscodeTask
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("task_id = ?", event.TaskID).First(&task).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Direct videoTranscode RPC jobs need not have a manager task.
			return nil
		}
		if err != nil {
			return err
		}
		if task.FileHash != event.FileHash || task.UserID != event.UserID {
			return fmt.Errorf("%w: task identity mismatch", ErrInvalidFinishedEvent)
		}
		if task.Status == "completed" || task.Status == "failed" {
			return nil
		}
		progress := int32(100)
		videoStatus := "published"
		urls := event.URLs
		if event.Status == "failed" {
			progress, videoStatus, urls = 0, "transcode_failed", []string{}
		}
		encoded, _ := json.Marshal(urls)
		errorMsg := event.ErrorMsg
		if len(errorMsg) > 4000 {
			errorMsg = errorMsg[:4000]
		}
		if err := tx.Model(&task).Updates(map[string]any{
			"status": event.Status, "progress": progress,
			"result_urls": string(encoded), "error_msg": errorMsg,
		}).Error; err != nil {
			return err
		}
		var latest db.TranscodeTask
		if err := tx.Where("file_hash = ? AND user_id = ?", event.FileHash, event.UserID).
			Order("id DESC").First(&latest).Error; err != nil {
			return err
		}
		if latest.TaskID != task.TaskID {
			return nil
		}
		return tx.Model(&db.File{}).Where("file_hash = ? AND user_id = ? AND status = ?",
			event.FileHash, event.UserID, "finished").Updates(map[string]any{
			"transcode_status": videoStatus, "transcode_urls": string(encoded),
		}).Error
	})
}

func retryWait(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(time.Second):
		return true
	}
}

// ─── 最终一致性补偿任务 ────────────────────────────────────────────────────

// compensationTask 每 5 分钟扫描"转码中"状态超过 1 小时的记录。
// 在生产环境中应调用 videoTranscode 的 GetStatus RPC 来主动查询任务状态。
func compensationTask(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runCompensation(ctx)
		}
	}
}

func runCompensation(ctx context.Context) {
	threshold := time.Now().Add(-1 * time.Hour)
	staleFiles, err := db.FindStaleTranscoding(threshold)
	if err != nil {
		log.Printf("[compensation] 查询滞留记录失败: %v", err)
		return
	}
	if len(staleFiles) == 0 {
		return
	}

	log.Printf("[compensation] 发现 %d 条滞留'转码中'记录（超过1小时）", len(staleFiles))
	for _, f := range staleFiles {
		// TODO（生产级）：通过 RPC 调用 videoTranscode.GetStatus 查询实际状态
		// 此处简化为标记为失败，避免永久滞留
		log.Printf("[compensation] 标记超时记录为 transcode_failed fileHash=%s", f.FileHash)
		if updateErr := db.UpdateTranscodeStatus(f.FileHash, f.UserID, "transcode_failed", "[]"); updateErr != nil {
			log.Printf("[compensation] 更新失败: %v", updateErr)
		}
	}
}
