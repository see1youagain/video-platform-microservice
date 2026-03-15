package middleware

import (
	"context"
	"time"

	"github.com/bytedance/gopkg/cloud/metainfo"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"video-platform-microservice/gateway/internal/logger"
)

// TraceIDMiddleware 添加请求追踪 ID
func TraceIDMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		traceID := c.GetHeader("X-Trace-ID")
		if len(traceID) == 0 {
			traceID = []byte(uuid.New().String())
		}
		traceIDStr := string(traceID)

		ctx = metainfo.WithPersistentValue(ctx, "trace_id", traceIDStr)
		ctx = metainfo.WithPersistentValue(ctx, "request_id", traceIDStr)
		c.Set("trace_id", traceIDStr)
		c.Header("X-Trace-ID", traceIDStr)

		start := time.Now()
		path := string(c.Path())
		method := string(c.Method())

		logger.Logger.Info("请求开始",
			zap.String("trace_id", traceIDStr),
			zap.String("method", method),
			zap.String("path", path),
			zap.String("client_ip", c.ClientIP()),
		)

		c.Next(ctx)

		duration := time.Since(start)
		statusCode := c.Response.StatusCode()
		logger.Logger.Info("请求完成",
			zap.String("trace_id", traceIDStr),
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status_code", statusCode),
			zap.Duration("duration", duration),
		)
	}
}
