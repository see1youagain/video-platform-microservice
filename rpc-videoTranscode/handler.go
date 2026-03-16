package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"time"

	transcodeDb "video-platform-microservice/rpc-videoTranscode/internal/db"
	"video-platform-microservice/rpc-videoTranscode/kitex_gen/videotranscode"

	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	commonEvents "github.com/see1youagain/video-platform-microservice/common/events"
	commonKafka "github.com/see1youagain/video-platform-microservice/common/kafka"
	commonMinio "github.com/see1youagain/video-platform-microservice/common/minio"
	commonOutbox "github.com/see1youagain/video-platform-microservice/common/outbox"
	"gorm.io/gorm"
)

type VideoTranscodeServiceImpl struct{}

func (s *VideoTranscodeServiceImpl) CreateTask(ctx context.Context, req *videotranscode.TranscodeTaskReq) (*videotranscode.TranscodeTaskResp, error) {
	resp := &videotranscode.TranscodeTaskResp{}

	if req.FileHash == "" {
		resp.Code = 400
		resp.Msg = "file_hash 不能为空"
		return resp, nil
	}
	if len(req.ResolutionList) == 0 {
		resp.Code = 400
		resp.Msg = "resolution_list 不能为空"
		return resp, nil
	}

	var existing transcodeDb.Job
	err := commondb.GetDB().Where("file_hash = ?", req.FileHash).Order("id desc").First(&existing).Error
	if err == nil {
		taskID := existing.TaskID
		resp.Code = 200
		resp.Msg = "任务已存在（幂等）"
		resp.TaskId = &taskID
		return resp, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		resp.Code = 500
		resp.Msg = "查询任务失败"
		return resp, nil
	}

	taskID := fmt.Sprintf("tc-%s-%d", req.FileHash[:min(8, len(req.FileHash))], time.Now().UnixMilli())
	payloadObj := commonEvents.TranscodeTaskEvent{
		TaskID:      taskID,
		FileHash:    req.FileHash,
		UserID:      "",
		Resolutions: req.ResolutionList,
	}
	payload, _ := json.Marshal(payloadObj)

	repo := commonOutbox.NewRepository(commondb.GetDB())
	txErr := commondb.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&transcodeDb.Job{
			TaskID:    taskID,
			FileHash:  req.FileHash,
			UserID:    "",
			Status:    "pending",
			Progress:  0,
			ResultURL: "[]",
		}).Error; err != nil {
			return err
		}
		return repo.EnqueueTx(tx, commonOutbox.Message{
			AggregateType: "transcode_job",
			AggregateID:   taskID,
			Topic:         commonEvents.TopicTranscodeRequested,
			MessageKey:    "tc:" + taskID,
			Payload:       string(payload),
		})
	})
	if txErr != nil {
		resp.Code = 500
		resp.Msg = "创建任务失败"
		return resp, nil
	}

	resp.Code = 200
	resp.Msg = "转码任务已创建"
	resp.TaskId = &taskID
	return resp, nil
}

func (s *VideoTranscodeServiceImpl) GetStatus(ctx context.Context, req *videotranscode.TranscodeStatusReq) (*videotranscode.TranscodeStatusResp, error) {
	resp := &videotranscode.TranscodeStatusResp{}
	if req.TaskId == "" {
		resp.Code = 400
		resp.Msg = "task_id 不能为空"
		return resp, nil
	}

	var job transcodeDb.Job
	err := commondb.GetDB().Where("task_id = ?", req.TaskId).First(&job).Error
	if err == gorm.ErrRecordNotFound {
		resp.Code = 404
		resp.Msg = "任务不存在"
		return resp, nil
	}
	if err != nil {
		resp.Code = 500
		resp.Msg = "查询任务失败"
		return resp, nil
	}

	status := job.Status
	progress := job.Progress
	resp.Code = 200
	resp.Msg = "ok"
	resp.Status = &status
	resp.Progress = &progress
	if job.ResultURL != "" {
		var urls []string
		_ = json.Unmarshal([]byte(job.ResultURL), &urls)
		resp.Urls = urls
	}
	return resp, nil
}

func StartFileUploadedConsumer(ctx context.Context) {
	reader := commonKafka.NewReader(commonEvents.TopicTranscodeRequested, "transcode-service")
	defer reader.Close()

	sem := make(chan struct{}, runtime.NumCPU()*2)
	log.Printf("[videoTranscode] 开始消费 topic=%s, 最大并发: %d", commonEvents.TopicTranscodeRequested, runtime.NumCPU()*2)
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[videoTranscode] ReadMessage 错误: %v", err)
			time.Sleep(time.Second)
			continue
		}

		var event commonEvents.TranscodeTaskEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			log.Printf("[videoTranscode] 解析消息失败: %v", err)
			continue
		}

		if event.FileHash == "" {
			continue
		}
		taskID := event.TaskID
		if taskID == "" {
			taskID = fmt.Sprintf("tc-%s-%d", event.FileHash[:min(8, len(event.FileHash))], time.Now().UnixMilli())
		}
		if len(event.Resolutions) == 0 {
			event.Resolutions = []string{"360p", "720p", "1080p"}
		}

		var existing transcodeDb.Job
		qErr := commondb.GetDB().Where("task_id = ?", taskID).First(&existing).Error
		if qErr == nil {
			log.Printf("[videoTranscode] task 已存在，跳过 taskID=%s", taskID)
			continue
		}
		if qErr != nil && qErr != gorm.ErrRecordNotFound {
			log.Printf("[videoTranscode] 查询任务失败 taskID=%s qErr=%v", taskID, qErr)
			continue
		}

		if createErr := commondb.GetDB().Create(&transcodeDb.Job{
			TaskID:    taskID,
			FileHash:  event.FileHash,
			UserID:    event.UserID,
			Status:    "pending",
			Progress:  0,
			ResultURL: "[]",
		}).Error; createErr != nil {
			log.Printf("[videoTranscode] 创建任务失败 taskID=%s err=%v", taskID, createErr)
			continue
		}

		sem <- struct{}{}
		go func(t, fh, u string, r []string) {
			defer func() { <-sem }()
			processTranscode(ctx, t, fh, u, r)
		}(taskID, event.FileHash, event.UserID, event.Resolutions)
	}
}

func processTranscode(ctx context.Context, taskID, fileHash, userID string, resolutions []string) {
	_ = commondb.GetDB().Model(&transcodeDb.Job{}).Where("task_id = ?", taskID).Updates(map[string]interface{}{
		"status":   "processing",
		"progress": 0,
	})

	resultURLs := make([]string, 0, len(resolutions))
	for i, res := range resolutions {
		time.Sleep(500 * time.Millisecond)

		objectName := fmt.Sprintf("videos/%s/%s.mp4", fileHash, res)
		dummyData := []byte(fmt.Sprintf("transcoded/%s/%s", fileHash, res))
		url, err := commonMinio.UploadStream(ctx, objectName, bytes.NewReader(dummyData), int64(len(dummyData)), "video/mp4")
		if err != nil {
			url = fmt.Sprintf("/files/video/%s/%s.mp4", fileHash, res)
		}
		resultURLs = append(resultURLs, url)

		progress := int32((i + 1) * 100 / len(resolutions))
		urlsJSON, _ := json.Marshal(resultURLs)
		_ = commondb.GetDB().Model(&transcodeDb.Job{}).Where("task_id = ?", taskID).Updates(map[string]interface{}{
			"status":     "processing",
			"progress":   progress,
			"result_url": string(urlsJSON),
		})
	}

	finishedEvt := commonEvents.TranscodeFinishedEvent{
		TaskID:   taskID,
		FileHash: fileHash,
		UserID:   userID,
		Status:   "completed",
		URLs:     resultURLs,
	}
	payload, _ := json.Marshal(finishedEvt)
	urlsJSON, _ := json.Marshal(resultURLs)
	repo := commonOutbox.NewRepository(commondb.GetDB())
	if txErr := commondb.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&transcodeDb.Job{}).Where("task_id = ?", taskID).Updates(map[string]interface{}{
			"status":     "completed",
			"progress":   100,
			"result_url": string(urlsJSON),
		}).Error; err != nil {
			return err
		}
		return repo.EnqueueTx(tx, commonOutbox.Message{
			AggregateType: "transcode_job",
			AggregateID:   taskID,
			Topic:         commonEvents.TopicTranscodeFinished,
			MessageKey:    "tc:" + taskID,
			Payload:       string(payload),
		})
	}); txErr != nil {
		log.Printf("[Transcode] 写入业务数据/Outbox失败 taskID=%s err=%v", taskID, txErr)
		_ = commondb.GetDB().Model(&transcodeDb.Job{}).Where("task_id = ?", taskID).Updates(map[string]interface{}{
			"status": "failed",
		})
		return
	}

	log.Printf("[Transcode] 完成并写入Outbox taskID=%s", taskID)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
