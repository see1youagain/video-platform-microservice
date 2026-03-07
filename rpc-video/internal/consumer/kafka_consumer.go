// Package consumer 提供 rpc-video 服务的 Kafka 事件消费逻辑：
//   - 订阅 video.file.uploaded → 在数据库中创建"转码中"视频记录
//   - 订阅 video.transcode.finished → 更新视频记录状态为"已发布"
//   - 补偿任务：每 5 分钟扫描状态为"转码中"超过 1 小时的记录，主动查询 videoTranscode
package consumer

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"video-platform-microservice/rpc-video/internal/db"

	commonEvents "github.com/see1youagain/video-platform-microservice/common/events"
	commonKafka "github.com/see1youagain/video-platform-microservice/common/kafka"
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
			TranscodeStatus: "transcoding",
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

	log.Printf("[video-consumer] 订阅 %s (group=video-service)", commonEvents.TopicTranscodeFinished)
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[video-consumer] TranscodeFinished 读取失败: %v", err)
			time.Sleep(time.Second)
			continue
		}

		var event commonEvents.TranscodeFinishedEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			log.Printf("[video-consumer] 解析 TranscodeFinished 失败: %v", err)
			continue
		}

		log.Printf("[video-consumer] 收到 TranscodeFinished fileHash=%s status=%s", event.FileHash, event.Status)

		// 更新转码状态
		newStatus := "published"
		if event.Status == "failed" {
			newStatus = "transcode_failed"
		}
		urlsJSON, _ := json.Marshal(event.URLs)
		if updateErr := db.UpdateTranscodeStatus(event.FileHash, event.UserID, newStatus, string(urlsJSON)); updateErr != nil {
			log.Printf("[video-consumer] 更新转码状态失败 fileHash=%s: %v", event.FileHash, updateErr)
		} else {
			log.Printf("[video-consumer] 视频状态更新为 %s fileHash=%s", newStatus, event.FileHash)
		}
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
