package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/bytedance/gopkg/cloud/metainfo"
	"github.com/cloudwego/kitex/client"
	etcd "github.com/kitex-contrib/registry-etcd"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	commonOutbox "github.com/see1youagain/video-platform-microservice/common/outbox"
	"gorm.io/gorm"

	managerDb "video-platform-microservice/rpc-videoManager/internal/db"
	videomanager "video-platform-microservice/rpc-videoManager/kitex_gen/videomanager"
	"video-platform-microservice/rpc-videoManager/kitex_gen/videoupload"
	uploadSvc "video-platform-microservice/rpc-videoManager/kitex_gen/videoupload/videouploadservice"
)

// ─── 转码事件负载（写入 Outbox payload）─────────────────────────────────────
type StartTranscodePayload struct {
	TaskID      string   `json:"task_id"`
	FileHash    string   `json:"file_hash"`
	UserID      string   `json:"user_id"`
	MinioURL    string   `json:"minio_url"`
	Resolutions []string `json:"resolutions"`
	RequestID   string   `json:"request_id,omitempty"`
}

// ─── videoUpload Kitex RPC 客户端（由 main.go 初始化后注入）────────────────
var uploadClient uploadSvc.Client

// InitVideoUploadClient 初始化跨服务 RPC 客户端（通过 etcd 服务发现）
func InitVideoUploadClient(etcdEndpoints []string) {
	r, err := etcd.NewEtcdResolver(etcdEndpoints)
	if err != nil {
		log.Printf("⚠️  初始化 etcd resolver 失败: %v，进度查询/中止功能将降级", err)
		return
	}
	c, err := uploadSvc.NewClient("videoupload", client.WithResolver(r), client.WithRPCTimeout(10*time.Minute), client.WithConnectTimeout(10*time.Second))
	if err != nil {
		log.Printf("⚠️  初始化 videoUpload 客户端失败: %v", err)
		return
	}
	uploadClient = c
	log.Println("✅ videoUpload RPC 客户端初始化成功")
}

// ─── User ID 工具 ────────────────────────────────────────────────────────
func getUserIDFromContext(ctx context.Context, reqUserID string) string {
	if uid, ok := metainfo.GetPersistentValue(ctx, "user_id"); ok && uid != "" {
		return uid
	}
	if reqUserID != "" {
		return reqUserID
	}
	return "anonymous"
}

// ─── 服务实现 ─────────────────────────────────────────────────────────────
type VideoManagerServiceImpl struct{}

const minMultipartSize = 5 * 1024 * 1024

// InitUpload 秒传检测 → 委托 videoUpload 开启分片上传
func (s *VideoManagerServiceImpl) InitUpload(ctx context.Context, req *videomanager.InitUploadReq) (*videomanager.InitUploadResp, error) {
	resp := &videomanager.InitUploadResp{}
	if req.FileHash == "" || req.Filename == "" || req.UserId == "" {
		resp.Code = 400
		resp.Msg = "缺少必要字段"
		return resp, nil
	}

	if req.FileSize > 0 && req.FileSize < minMultipartSize {
		status := "single_shot"
		resp.Code = 200
		resp.Msg = "文件小于5MB，请使用单次上传"
		resp.Status = &status
		return resp, nil
	}

	// 本地 DB 秒传检测
	var existing managerDb.File
	if err := commondb.GetDB().Where("file_hash = ? AND status = 'finished'", req.FileHash).First(&existing).Error; err == nil {
		status := "finished"
		resp.Code = 200
		resp.Msg = "秒传成功"
		resp.Status = &status
		resp.Url = &existing.URL
		return resp, nil
	}

	if uploadClient == nil {
		resp.Code = 503
		resp.Msg = "videoUpload 服务不可用"
		return resp, nil
	}
	upReq := &videoupload.InitUploadReq{
		FileHash:  req.FileHash,
		Filename:  &req.Filename,
		FileSize:  &req.FileSize,
		UserId:    &req.UserId,
		Width:     req.Width,
		Height:    req.Height,
		RequestId: req.RequestId,
	}
	upResp, err := uploadClient.InitUpload(ctx, upReq)
	if err != nil {
		resp.Code = 502
		resp.Msg = fmt.Sprintf("调用 videoUpload 失败: %v", err)
		return resp, nil
	}
	resp.Code = upResp.Code
	resp.Msg = upResp.Msg
	resp.Status = upResp.Status
	resp.Url = upResp.Url
	resp.FinishedChunks = upResp.FinishedChunks
	resp.UploadId = upResp.UploadId
	return resp, nil
}

// FinalizeUpload 合并请求 → 委托 videoUpload CompleteMultipartUpload
func (s *VideoManagerServiceImpl) FinalizeUpload(ctx context.Context, req *videomanager.FinalizeUploadReq) (*videomanager.FinalizeUploadResp, error) {
	resp := &videomanager.FinalizeUploadResp{}
	if req.FileHash == "" || req.Filename == "" || req.TotalChunks <= 0 {
		resp.Code = 400
		resp.Msg = "参数不完整"
		return resp, nil
	}
	if uploadClient == nil {
		resp.Code = 503
		resp.Msg = "videoUpload 服务不可用"
		return resp, nil
	}

	userID := getUserIDFromContext(ctx, req.UserId)
	upReq := &videoupload.FinalizeUploadReq{
		FileHash:    req.FileHash,
		Filename:    req.Filename,
		TotalChunks: req.TotalChunks,
		UserId:      &userID,
		Width:       req.Width,
		Height:      req.Height,
		RequestId:   req.RequestId,
		Resolutions: req.Resolutions,
		UploadId:    req.UploadId,
	}
	upResp, err := uploadClient.FinalizeUpload(ctx, upReq)
	if err != nil {
		resp.Code = 502
		resp.Msg = fmt.Sprintf("调用 videoUpload FinalizeUpload 失败: %v", err)
		return resp, nil
	}

	resp.Code = upResp.Code
	resp.Msg = upResp.Msg
	resp.Url = upResp.Url
	resp.TaskId = upResp.TaskId

	if upResp.Code != 200 {
		return resp, nil
	}
	if upResp.Url == nil || *upResp.Url == "" {
		resp.Code = 500
		resp.Msg = "合并成功但未返回有效URL"
		return resp, nil
	}

	requestID := ""
	if req.RequestId != nil {
		requestID = *req.RequestId
	}
	finalURL := *upResp.Url
	width := int32(0)
	height := int32(0)
	if req.Width != nil {
		width = *req.Width
	}
	if req.Height != nil {
		height = *req.Height
	}

	txErr := commondb.GetDB().Transaction(func(tx *gorm.DB) error {
		return tx.Where("file_hash = ?", req.FileHash).
			Assign(managerDb.File{
				FileHash:        req.FileHash,
				UserID:          userID,
				Filename:        req.Filename,
				URL:             finalURL,
				Status:          "finished",
				TranscodeStatus: "pending",
				Width:           width,
				Height:          height,
				RequestID:       requestID,
			}).
			FirstOrCreate(&managerDb.File{}).Error
	})
	if txErr != nil {
		resp.Code = 500
		resp.Msg = fmt.Sprintf("合并成功但本地落库失败: %v", txErr)
		return resp, nil
	}

	return resp, nil
}

// QueryUploadProgress 委托 videoUpload 查询 Redis HLEN
func (s *VideoManagerServiceImpl) QueryUploadProgress(ctx context.Context, req *videomanager.QueryUploadProgressReq) (*videomanager.QueryUploadProgressResp, error) {
	resp := &videomanager.QueryUploadProgressResp{}
	if req.UploadId == "" {
		resp.Code = 400
		resp.Msg = "upload_id 不能为空"
		return resp, nil
	}
	if uploadClient == nil {
		resp.Code = 503
		resp.Msg = "videoUpload 服务不可用"
		return resp, nil
	}
	pResp, err := uploadClient.QueryProgress(ctx, &videoupload.QueryProgressReq{UploadId: req.UploadId})
	if err != nil {
		resp.Code = 502
		resp.Msg = fmt.Sprintf("调用失败: %v", err)
		return resp, nil
	}
	resp.Code = pResp.Code
	resp.Msg = pResp.Msg
	resp.UploadedParts = pResp.UploadedParts
	return resp, nil
}

// AbortUpload 更新 MySQL 状态 + 委托 videoUpload 取消 MinIO 分片上传
func (s *VideoManagerServiceImpl) AbortUpload(ctx context.Context, req *videomanager.AbortUploadReq) (*videomanager.AbortUploadResp, error) {
	resp := &videomanager.AbortUploadResp{}
	if req.UploadId == "" || req.FileHash == "" {
		resp.Code = 400
		resp.Msg = "参数不完整"
		return resp, nil
	}

	commondb.GetDB().Model(&managerDb.File{}).
		Where("file_hash = ?", req.FileHash).
		Update("status", "cancelled")

	if uploadClient != nil {
		_, err := uploadClient.AbortUpload(ctx, &videoupload.AbortUploadReq{
			UploadId: req.UploadId,
			FileHash: req.FileHash,
		})
		if err != nil {
			log.Printf("[AbortUpload] 调用 videoUpload 失败: %v", err)
		}
	}
	resp.Code = 200
	resp.Msg = "已取消"
	return resp, nil
}

// GetVideoInfo 从 MySQL 返回视频信息
func (s *VideoManagerServiceImpl) GetVideoInfo(ctx context.Context, req *videomanager.GetVideoInfoReq) (*videomanager.GetVideoInfoResp, error) {
	resp := &videomanager.GetVideoInfoResp{}
	var f managerDb.File
	if err := commondb.GetDB().Where("file_hash = ?", req.FileHash).First(&f).Error; err != nil {
		resp.Code = 404
		resp.Msg = "视频不存在"
		return resp, nil
	}
	resp.Code = 200
	resp.Msg = "ok"
	fh := f.FileHash
	fn := f.Filename
	fs := f.FileSize
	fw := f.Width
	fhgt := f.Height
	resp.FileHash = &fh
	resp.Filename = &fn
	resp.FileSize = &fs
	resp.Width = &fw
	resp.Height = &fhgt
	resp.Url = &f.URL
	resp.TranscodeStatus = &f.TranscodeStatus
	return resp, nil
}

// DeleteVideo 软删除（更新状态为 deleted）
func (s *VideoManagerServiceImpl) DeleteVideo(ctx context.Context, req *videomanager.DeleteVideoReq) (*videomanager.DeleteVideoResp, error) {
	resp := &videomanager.DeleteVideoResp{}
	if req.FileHash == "" || req.UserId == "" {
		resp.Code = 400
		resp.Msg = "参数不完整"
		return resp, nil
	}
	result := commondb.GetDB().Model(&managerDb.File{}).
		Where("file_hash = ? AND user_id = ?", req.FileHash, req.UserId).
		Update("status", "deleted")
	if result.RowsAffected == 0 {
		resp.Code = 404
		resp.Msg = "视频不存在或无权限删除"
		return resp, nil
	}
	resp.Code = 200
	resp.Msg = "删除成功"
	return resp, nil
}

// Transcode ── Transactional Outbox 核心 ────────────────────────────────────
//
// 单事务保证：
//  1. video_files.transcode_status → "pending"
//  2. INSERT transcode_tasks
//  3. INSERT outbox_events（topic: video.transcode.requested）
//
// Outbox Dispatcher goroutine 异步轮询 outbox_events，
// 将事件投递至 Kafka → rpc-videoTranscode 消费，执行真正的转码。
func (s *VideoManagerServiceImpl) Transcode(ctx context.Context, req *videomanager.TranscodeReq) (*videomanager.TranscodeResp, error) {
	resp := &videomanager.TranscodeResp{}
	if req.FileHash == "" || req.UserId == "" || len(req.Resolutions) == 0 {
		resp.Code = 400
		resp.Msg = "参数不完整"
		return resp, nil
	}

	userID := getUserIDFromContext(ctx, req.UserId)

	// 校验文件存在且状态为 finished
	var videoFile managerDb.File
	if err := commondb.GetDB().Where("file_hash = ?", req.FileHash).First(&videoFile).Error; err != nil {
		resp.Code = 404
		resp.Msg = "视频文件不存在，请先完成上传"
		return resp, nil
	}
	if videoFile.Status != "finished" {
		resp.Code = 409
		resp.Msg = fmt.Sprintf("视频状态异常，无法转码: %s", videoFile.Status)
		return resp, nil
	}

	taskID := fmt.Sprintf("tc_%s_%d", req.FileHash[:8], time.Now().UnixMilli())
	resJSON, _ := json.Marshal(req.Resolutions)

	var reqID string
	if req.RequestId != nil {
		reqID = *req.RequestId
	}

	payloadBytes, _ := json.Marshal(StartTranscodePayload{
		TaskID:      taskID,
		FileHash:    req.FileHash,
		UserID:      userID,
		MinioURL:    videoFile.URL,
		Resolutions: req.Resolutions,
		RequestID:   reqID,
	})

	// ── 本地事务（Transactional Outbox）────────────────────────────────
	txErr := commondb.GetDB().Transaction(func(tx *gorm.DB) error {
		// Step 1: 更新视频转码状态
		if err := tx.Model(&managerDb.File{}).
			Where("file_hash = ?", req.FileHash).
			Update("transcode_status", "pending").Error; err != nil {
			return fmt.Errorf("更新视频状态失败: %w", err)
		}

		// Step 2: 创建转码任务
		task := managerDb.TranscodeTask{
			TaskID:      taskID,
			FileHash:    req.FileHash,
			UserID:      userID,
			Resolutions: string(resJSON),
			Status:      "pending",
			RequestID:   reqID,
		}
		if err := tx.Create(&task).Error; err != nil {
			return fmt.Errorf("创建转码任务失败: %w", err)
		}

		// Step 3: 写入 Outbox（与业务数据同一事务 = 双写强一致）
		if err := tx.Create(&commonOutbox.Event{
			AggregateType: "TranscodeTask",
			AggregateID:   taskID,
			Topic:         "video.transcode.requested",
			MessageKey:    req.FileHash,
			Payload:       string(payloadBytes),
			Status:        commonOutbox.StatusPending,
			AvailableAt:   time.Now(),
		}).Error; err != nil {
			return fmt.Errorf("写入 Outbox 失败: %w", err)
		}
		return nil
	})

	if txErr != nil {
		log.Printf("[Transcode] 事务回滚: %v", txErr)
		resp.Code = 500
		resp.Msg = "发起转码失败，请重试"
		return resp, nil
	}

	log.Printf("[Transcode] 任务 %s 已入 Outbox，等待 Relay 投递 Kafka", taskID)
	resp.Code = 200
	resp.Msg = "转码任务已创建"
	resp.TaskId = &taskID
	return resp, nil
}

// GetTranscodeStatus 查询 transcode_tasks 表
func (s *VideoManagerServiceImpl) GetTranscodeStatus(ctx context.Context, req *videomanager.GetTranscodeStatusReq) (*videomanager.GetTranscodeStatusResp, error) {
	resp := &videomanager.GetTranscodeStatusResp{}
	var task managerDb.TranscodeTask
	if err := commondb.GetDB().Where("task_id = ?", req.TaskId).First(&task).Error; err != nil {
		resp.Code = 404
		resp.Msg = "任务不存在"
		return resp, nil
	}
	resp.Code = 200
	resp.Msg = "ok"
	s2 := task.Status
	p := float64(task.Progress) / 100.0
	resp.Status = &s2
	resp.Progress = &p
	if task.ResultURLs != "" {
		var urls []string
		if err := json.Unmarshal([]byte(task.ResultURLs), &urls); err == nil && len(urls) > 0 {
			resp.CompletedUrls = urls
		}
	}
	return resp, nil
}
