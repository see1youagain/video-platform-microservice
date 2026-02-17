package video

import (
	"context"

	"video-platform-microservice/gateway/internal/logger"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.uber.org/zap"
)

// SimpleUploadHandler 处理简单上传请求（单个请求上传整个文件）
func SimpleUploadHandler(ctx context.Context, c *app.RequestContext) {
	traceID, _ := c.Get("trace_id")

	logger.Logger.Warn("简单上传功能暂未实现",
		zap.Any("trace_id", traceID),
	)

	c.JSON(consts.StatusNotImplemented, map[string]interface{}{
		"code": 501,
		"msg":  "简单上传功能暂未实现，请使用分片上传",
	})
}