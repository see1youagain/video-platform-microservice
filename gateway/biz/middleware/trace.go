package middleware

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"time"
	"video-platform-microservice/gateway/internal/logger"
)

// TraceIDMiddleware 添加请求追踪 ID
func TraceIDMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// 生成或获取 trace ID
		traceID := c.GetHeader("X-Trace-ID")
		if len(traceID) == 0 {
			traceID = []byte(uuid.New().String())
		}

		// 设置到上下文
		c.Set("trace_id", string(traceID))
		c.Header("X-Trace-ID", string(traceID))

		// 记录请求开始
		start := time.Now()
		path := string(c.Path())
		method := string(c.Method())

		logger.Logger.Info("请求开始",
			zap.String("trace_id", string(traceID)),
			zap.String("method", method),
			zap.String("path", path),
			zap.String("client_ip", c.ClientIP()),
		)

		// 继续处理请求
		c.Next(ctx)

		// 记录请求结束
		duration := time.Since(start)
		statusCode := c.Response.StatusCode()

		logger.Logger.Info("请求完成",
			zap.String("trace_id", string(traceID)),
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status_code", statusCode),
			zap.Duration("duration", duration),
		)
	}
}
