package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	"github.com/see1youagain/video-platform-microservice/common/events"
	commonKafka "github.com/see1youagain/video-platform-microservice/common/kafka"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	transcodeDb "video-platform-microservice/rpc-videoTranscode/internal/db"
	"video-platform-microservice/rpc-videoTranscode/kitex_gen/videotranscode"
)

type VideoTranscodeServiceImpl struct{}

// CreateTask persists work before returning. The database workers also execute
// RPC tasks, so Kafka redelivery cannot strand a pre-created job.
func (s *VideoTranscodeServiceImpl) CreateTask(ctx context.Context, req *videotranscode.TranscodeTaskReq) (*videotranscode.TranscodeTaskResp, error) {
	resp := &videotranscode.TranscodeTaskResp{}
	if req == nil || req.UserId <= 0 {
		resp.Code, resp.Msg = 400, "有效 user_id 不能为空"
		return resp, nil
	}
	resolutions, err := events.ValidateTranscodeRequest(req.FileHash, req.ResolutionList)
	if err != nil {
		resp.Code, resp.Msg = 400, err.Error()
		return resp, nil
	}
	userID := strconv.FormatInt(req.UserId, 10)
	encoded, _ := json.Marshal(resolutions)
	var existing transcodeDb.Job
	err = commondb.GetDB().WithContext(ctx).Where("file_hash = ? AND user_id = ? AND resolutions = ? AND status IN ?",
		req.FileHash, userID, string(encoded), []string{"pending", "processing"}).Order("id DESC").First(&existing).Error
	if err == nil {
		resp.Code, resp.Msg, resp.TaskId = 200, "任务处理中", &existing.TaskID
		return resp, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		resp.Code, resp.Msg = 500, "查询任务失败"
		return resp, nil
	}
	taskID := "tc_" + uuid.NewString()
	err = persistTask(ctx, events.TranscodeTaskEvent{TaskID: taskID, FileHash: req.FileHash, UserID: userID, Resolutions: resolutions})
	if err != nil {
		resp.Code, resp.Msg = 500, "创建任务失败"
		return resp, nil
	}
	resp.Code, resp.Msg, resp.TaskId = 200, "转码任务已创建", &taskID
	return resp, nil
}

func (s *VideoTranscodeServiceImpl) GetStatus(ctx context.Context, req *videotranscode.TranscodeStatusReq) (*videotranscode.TranscodeStatusResp, error) {
	resp := &videotranscode.TranscodeStatusResp{}
	if req == nil || !events.ValidTaskID(req.TaskId) {
		resp.Code, resp.Msg = 400, "无效 task_id"
		return resp, nil
	}
	var job transcodeDb.Job
	err := commondb.GetDB().WithContext(ctx).Where("task_id = ?", req.TaskId).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		resp.Code, resp.Msg = 404, "任务不存在"
		return resp, nil
	}
	if err != nil {
		resp.Code, resp.Msg = 500, "查询任务失败"
		return resp, nil
	}
	resp.Code, resp.Msg = 200, "ok"
	resp.Status, resp.Progress = &job.Status, &job.Progress
	if job.ErrorMsg != "" {
		resp.ErrorMsg = &job.ErrorMsg
	}
	_ = json.Unmarshal([]byte(job.ResultURL), &resp.Urls)
	return resp, nil
}

var errInvalidTask = errors.New("invalid transcode event")

func persistTask(ctx context.Context, event events.TranscodeTaskEvent) error {
	resolutions, err := events.ValidateTranscodeRequest(event.FileHash, event.Resolutions)
	if err != nil || !events.ValidTaskID(event.TaskID) || event.UserID == "" || len(event.UserID) > 64 {
		return fmt.Errorf("%w: task=%q hash=%q", errInvalidTask, event.TaskID, event.FileHash)
	}
	encoded, _ := json.Marshal(resolutions)
	job := transcodeDb.Job{TaskID: event.TaskID, FileHash: event.FileHash, UserID: event.UserID,
		Resolutions: string(encoded), Status: "pending", ResultURL: "[]"}
	if err := commondb.GetDB().WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&job).Error; err != nil {
		return err
	}
	var existing transcodeDb.Job
	if err := commondb.GetDB().WithContext(ctx).Where("task_id = ?", event.TaskID).First(&existing).Error; err != nil {
		return err
	}
	if existing.FileHash != event.FileHash || existing.UserID != event.UserID || existing.Resolutions != string(encoded) {
		return fmt.Errorf("%w: task_id conflicts with existing work", errInvalidTask)
	}
	return nil
}

// StartTaskConsumer acknowledges only after work is in the durable queue.
func StartTaskConsumer(ctx context.Context) {
	go consumeTaskTopic(ctx, events.TopicTranscodeRequestedLegacy)
	consumeTaskTopic(ctx, events.TopicTranscodeTasks)
}

func consumeTaskTopic(ctx context.Context, topic string) {
	reader := commonKafka.NewReader(topic, "transcode-service")
	defer reader.Close()
	for ctx.Err() == nil {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if !waitRetry(ctx) {
				return
			}
			continue
		}
		for ctx.Err() == nil {
			var event events.TranscodeTaskEvent
			err = json.Unmarshal(msg.Value, &event)
			if err != nil {
				err = fmt.Errorf("%w: %v", errInvalidTask, err)
			} else {
				err = persistTask(ctx, event)
			}
			if errors.Is(err, errInvalidTask) {
				log.Printf("[transcode] rejected event: %v", err)
				err = nil // Invalid input is not retryable.
			}
			if err == nil {
				err = reader.CommitMessage(ctx, msg)
			}
			if err == nil {
				break
			}
			log.Printf("[transcode] persist/commit failed: %v", err)
			if !waitRetry(ctx) {
				return
			}
		}
	}
}

func waitRetry(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(time.Second):
		return true
	}
}
