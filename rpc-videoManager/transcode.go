package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bytedance/gopkg/cloud/metainfo"
	"github.com/google/uuid"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	"github.com/see1youagain/video-platform-microservice/common/events"
	"github.com/see1youagain/video-platform-microservice/common/outbox"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	managerDb "video-platform-microservice/rpc-videoManager/internal/db"
	"video-platform-microservice/rpc-videoManager/kitex_gen/videomanager"
)

func (s *VideoManagerServiceImpl) Transcode(ctx context.Context, req *videomanager.TranscodeReq) (*videomanager.TranscodeResp, error) {
	resp := &videomanager.TranscodeResp{}
	if req == nil || req.UserId == "" {
		resp.Code, resp.Msg = 400, "user_id 不能为空"
		return resp, nil
	}
	resolutions, err := events.ValidateTranscodeRequest(req.FileHash, req.Resolutions)
	if err != nil {
		resp.Code, resp.Msg = 400, err.Error()
		return resp, nil
	}
	userID := getUserIDFromContext(ctx, req.UserId)
	resJSON, _ := json.Marshal(resolutions)
	requestID := req.GetRequestId()
	if len(requestID) > 64 {
		resp.Code, resp.Msg = 400, "request_id 不能超过64字节"
		return resp, nil
	}
	var key *string
	if requestID != "" {
		sum := sha256.Sum256([]byte(userID + ":" + requestID))
		value := hex.EncodeToString(sum[:])
		key = &value
	}
	matchExisting := func(task *managerDb.TranscodeTask) {
		if task.FileHash != req.FileHash || task.Resolutions != string(resJSON) {
			resp.Code, resp.Msg = 409, "request_id 已用于其他转码参数"
			return
		}
		resp.Code, resp.Msg, resp.TaskId = 200, "转码任务已存在", &task.TaskID
	}
	err = commondb.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize competing requests for the same owned video.
		var file managerDb.File
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("file_hash = ? AND user_id = ?", req.FileHash, userID).
			First(&file).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			resp.Code, resp.Msg = 404, "视频不存在或无权限"
			return nil
		} else if err != nil {
			return err
		}
		if file.Status != "finished" {
			resp.Code, resp.Msg = 409, "视频尚未完成上传或已删除"
			return nil
		}
		if key != nil {
			var existing managerDb.TranscodeTask
			err := tx.Where("idempotency_key = ?", *key).First(&existing).Error
			if err == nil {
				matchExisting(&existing)
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		// Avoid redundant encoders on retry even if the client has no request_id.
		var active managerDb.TranscodeTask
		qErr := tx.Where("file_hash = ? AND user_id = ? AND status IN ?", req.FileHash, userID,
			[]string{"pending", "processing"}).Order("id DESC").First(&active).Error
		if qErr == nil {
			if active.Resolutions != string(resJSON) {
				resp.Code, resp.Msg = 409, "该视频有不同参数的转码任务正在处理"
			} else {
				// A distinct idempotency key must retain its own durable mapping.
				if key != nil && (active.IdempotencyKey == nil || *active.IdempotencyKey != *key) {
					resp.Code, resp.Msg = 409, "该视频已有转码任务，请查询已有任务"
				} else {
					resp.Code, resp.Msg, resp.TaskId = 200, "转码任务处理中", &active.TaskID
				}
			}
			return nil
		}
		if !errors.Is(qErr, gorm.ErrRecordNotFound) {
			return qErr
		}
		task := managerDb.TranscodeTask{
			TaskID: "tc_" + uuid.NewString(), FileHash: req.FileHash, UserID: userID,
			Resolutions: string(resJSON), Status: "pending", RequestID: requestID, IdempotencyKey: key,
		}
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		if err := tx.Model(&managerDb.File{}).Where("id = ?", file.ID).Update("transcode_status", "pending").Error; err != nil {
			return err
		}
		payload, _ := json.Marshal(events.TranscodeTaskEvent{
			TaskID: task.TaskID, FileHash: req.FileHash, UserID: userID, Resolutions: resolutions, RequestID: requestID,
		})
		if err := outbox.NewRepository(tx).EnqueueTx(tx, outbox.Message{
			AggregateType: "transcode_task", AggregateID: task.TaskID, Topic: events.TopicTranscodeTasks,
			MessageKey: req.FileHash, Payload: string(payload),
		}); err != nil {
			return err
		}
		resp.Code, resp.Msg, resp.TaskId = 200, "转码任务已创建", &task.TaskID
		return nil
	})
	if err != nil {
		// A concurrent request on a different file can race the unique key.
		if key != nil {
			var existing managerDb.TranscodeTask
			if qErr := commondb.GetDB().WithContext(ctx).Where("idempotency_key = ?", *key).First(&existing).Error; qErr == nil {
				matchExisting(&existing)
				return resp, nil
			}
		}
		resp.Code, resp.Msg, resp.TaskId = 500, "创建转码任务失败", nil
	}
	return resp, nil
}

func (s *VideoManagerServiceImpl) GetTranscodeStatus(ctx context.Context, req *videomanager.GetTranscodeStatusReq) (*videomanager.GetTranscodeStatusResp, error) {
	resp := &videomanager.GetTranscodeStatusResp{}
	if req == nil || !events.ValidTaskID(req.TaskId) {
		resp.Code, resp.Msg = 400, "无效 task_id"
		return resp, nil
	}
	userID, _ := metainfo.GetPersistentValue(ctx, "user_id")
	if userID == "" {
		resp.Code, resp.Msg = 401, "缺少用户身份"
		return resp, nil
	}
	var task managerDb.TranscodeTask
	err := commondb.GetDB().WithContext(ctx).Where("task_id = ? AND user_id = ?", req.TaskId, userID).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		resp.Code, resp.Msg = 404, "任务不存在或无权限"
		return resp, nil
	}
	if err != nil {
		resp.Code, resp.Msg = 500, "查询任务失败"
		return resp, nil
	}
	// Progress comes from the executing service's durable job. Completion is
	// authoritative only after its finished event is applied to transcode_tasks.
	progress := task.Progress
	status := task.Status
	if status == "pending" || status == "processing" {
		var running struct {
			Status   string
			Progress int32
		}
		result := commondb.GetDB().WithContext(ctx).Table("transcode_jobs").
			Select("status, progress").Where("task_id = ? AND user_id = ?", task.TaskID, userID).Take(&running)
		if result.Error == nil && running.Status == "processing" {
			status, progress = "processing", running.Progress
		}
	}
	resp.Code, resp.Msg = 200, "ok"
	if task.Status == "failed" {
		resp.Msg = fmt.Sprintf("转码失败: %s", task.ErrorMsg)
	}
	p := float64(progress) / 100
	resp.Status, resp.Progress = &status, &p
	if task.Status == "completed" {
		_ = json.Unmarshal([]byte(task.ResultURLs), &resp.CompletedUrls)
	}
	return resp, nil
}
