package video

import (
	"context"
	"strconv"

	"video-platform-microservice/gateway/internal/logger"
	"video-platform-microservice/gateway/internal/validator"
	videomanager "video-platform-microservice/gateway/kitex_gen/videomanager"
	"video-platform-microservice/gateway/rpc"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.uber.org/zap"
)

// MergeFileHandler 合并分片并发布 FileUploaded 事件 → videoUpload 服务
func MergeFileHandler(ctx context.Context, c *app.RequestContext) {
	var req struct {
		FileHash    string   `json:"file_hash"    binding:"required"`
		Filename    string   `json:"filename"     binding:"required"`
		TotalChunks int32    `json:"total_chunks" binding:"required"`
		UploadID    string   `json:"upload_id"    binding:"required"`
		UserID      string   `json:"user_id"`
		Width       int32    `json:"width"`
		Height      int32    `json:"height"`
		RequestID   string   `json:"request_id"`
		Resolutions []string `json:"resolutions"`
	}

	traceID, _ := c.Get("trace_id")
	userID, _ := c.Get("user_id")
	var userIDStr string
	switch v := userID.(type) {
	case string:
		userIDStr = v
	case int64:
		userIDStr = strconv.FormatInt(v, 10)
	case uint:
		userIDStr = strconv.FormatUint(uint64(v), 10)
	}

	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]interface{}{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}
	if err := validator.ValidateFileHash(req.FileHash); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]interface{}{"code": 400, "msg": err.Error()})
		return
	}

	if userIDStr == "" {
		userIDStr = req.UserID
	}

	logger.Logger.Info("FinalizeUpload → videoManager",
		zap.Any("trace_id", traceID),
		zap.String("file_hash", req.FileHash),
		zap.String("upload_id", req.UploadID),
		zap.Int32("total_chunks", req.TotalChunks),
	)

	rpcReq := &videomanager.FinalizeUploadReq{
		FileHash:    req.FileHash,
		Filename:    req.Filename,
		TotalChunks: req.TotalChunks,
		UploadId:    &req.UploadID,
		UserId:      userIDStr,
		RequestId:   &req.RequestID,
		Resolutions: req.Resolutions,
	}
	if req.Width != 0 {
		rpcReq.Width = &req.Width
	}
	if req.Height != 0 {
		rpcReq.Height = &req.Height
	}

	resp, err := rpc.VideoManagerClient.FinalizeUpload(ctx, rpcReq)
	if err != nil {
		logger.Logger.Error("VideoManager FinalizeUpload 失败", zap.Any("trace_id", traceID), zap.Error(err))
		c.JSON(consts.StatusServiceUnavailable, map[string]interface{}{
			"code":     503,
			"msg":      "视频管理服务暂不可用",
			"fallback": "simpleUpload",
		})
		return
	}

	c.JSON(consts.StatusOK, map[string]interface{}{
		"code":    resp.Code,
		"msg":     resp.Msg,
		"url":     resp.GetUrl(),
		"task_id": resp.GetTaskId(),
	})
}
