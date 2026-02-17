package video

import (
	"context"

	"video-platform-microservice/gateway/internal/logger"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.uber.org/zap"
)

// CalculateFileHashHandler 处理文件哈希计算请求
func CalculateFileHashHandler(ctx context.Context, c *app.RequestContext) {
	traceID, _ := c.Get("trace_id")

	logger.Logger.Warn("文件哈希计算功能暂未实现",
		zap.Any("trace_id", traceID),
	)

	c.JSON(consts.StatusNotImplemented, map[string]interface{}{
		"code": 501,
		"msg":  "文件哈希计算功能暂未实现，请在客户端计算哈希",
	})
}