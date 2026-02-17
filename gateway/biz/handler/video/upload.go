package video

import (
	"context"

	"video-platform-microservice/gateway/internal/logger"
	"video-platform-microservice/gateway/internal/validator"
	"video-platform-microservice/gateway/rpc"
	video "video-platform-microservice/rpc-video/kitex_gen/video"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.uber.org/zap"
)

// UploadChunkHandler 处理上传分片请求
func UploadChunkHandler(ctx context.Context, c *app.RequestContext) {
	var req struct {
		FileHash  string `json:"file_hash" binding:"required"`
		Index     string `json:"index" binding:"required"`
		Data      string `json:"data" binding:"required"`
	}

	traceID, _ := c.Get("trace_id")

	if err := c.BindAndValidate(&req); err != nil {
		logger.Logger.Warn("上传分片参数绑定失败",
			zap.Any("trace_id", traceID),
			zap.Error(err),
		)
		c.JSON(consts.StatusBadRequest, map[string]interface{}{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	if err := validator.ValidateFileHash(req.FileHash); err != nil {
		logger.Logger.Warn("文件哈希验证失败",
			zap.Any("trace_id", traceID),
			zap.String("file_hash", req.FileHash),
			zap.Error(err),
		)
		c.JSON(consts.StatusBadRequest, map[string]interface{}{
			"code": 400,
			"msg":  err.Error(),
		})
		return
	}

	logger.Logger.Info("调用 RPC UploadChunk",
		zap.Any("trace_id", traceID),
		zap.String("file_hash", req.FileHash),
		zap.String("index", req.Index),
	)

	resp, err := rpc.VideoClient.UploadChunk(ctx, &video.UploadChunkReq{
		FileHash: req.FileHash,
		Index:    req.Index,
		Data:     []byte(req.Data),
	})

	if err != nil {
		logger.Logger.Error("RPC 调用失败",
			zap.Any("trace_id", traceID),
			zap.Error(err),
		)
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"code": 500,
			"msg":  "服务暂时不可用，请稍后重试",
		})
		return
	}

	var httpStatus int
	switch resp.Code {
	case 200:
		httpStatus = consts.StatusOK
	case 400:
		httpStatus = consts.StatusBadRequest
	default:
		httpStatus = consts.StatusInternalServerError
	}

	logger.Logger.Info("上传分片成功",
		zap.Any("trace_id", traceID),
		zap.String("file_hash", req.FileHash),
		zap.String("index", req.Index),
		zap.Int32("code", resp.Code),
	)

	c.JSON(httpStatus, map[string]interface{}{
		"code": resp.Code,
		"msg":  resp.Msg,
	})
}