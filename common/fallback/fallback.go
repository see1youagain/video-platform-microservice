package fallback

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

// CheckRedisHealth 检查Redis健康状态
func CheckRedisHealth(ctx context.Context, client *redis.Client) bool {
	if client == nil {
		return false
	}

	err := client.Ping(ctx).Err()
	return err == nil
}

// CheckServiceHealth 检查服务健康状态（通过RPC调用）
func CheckServiceHealth(serviceName string, checkFunc func() error) bool {
	err := checkFunc()
	if err != nil {
		log.Printf("[Fallback] Service %s health check failed: %v", serviceName, err)
		return false
	}
	return true
}

// ServiceFallbackStrategy 服务降级策略
type ServiceFallbackStrategy struct {
	ServiceName    string
	HealthChecker  func() error
	FallbackAction func() error
	isHealthy      bool
}

// NewServiceFallback 创建服务降级策略
func NewServiceFallback(name string, checker func() error, fallback func() error) *ServiceFallbackStrategy {
	return &ServiceFallbackStrategy{
		ServiceName:    name,
		HealthChecker:  checker,
		FallbackAction: fallback,
		isHealthy:      true,
	}
}

// Execute 执行服务调用，失败时使用降级策略
func (s *ServiceFallbackStrategy) Execute(action func() error) error {
	// 检查服务健康状态
	if !CheckServiceHealth(s.ServiceName, s.HealthChecker) {
		s.isHealthy = false
		log.Printf("[Fallback] %s is unhealthy, using fallback strategy", s.ServiceName)
		return s.FallbackAction()
	}

	s.isHealthy = true

	// 尝试执行主逻辑
	err := action()
	if err != nil {
		log.Printf("[Fallback] %s action failed: %v, using fallback", s.ServiceName, err)
		return s.FallbackAction()
	}

	return nil
}

// IsHealthy 返回服务健康状态
func (s *ServiceFallbackStrategy) IsHealthy() bool {
	return s.isHealthy
}

// RedisFallbackConfig Redis降级配置
type RedisFallbackConfig struct {
	EnableFallback bool
	Client         *redis.Client
}

var globalRedisFallbackConfig *RedisFallbackConfig

// SetRedisFallbackConfig 设置全局Redis降级配置
func SetRedisFallbackConfig(config *RedisFallbackConfig) {
	globalRedisFallbackConfig = config
}

// GetRedisFallbackConfig 获取全局Redis降级配置
func GetRedisFallbackConfig() *RedisFallbackConfig {
	return globalRedisFallbackConfig
}

// WithRedis Fallback 执行Redis操作，失败时使用降级策略
func WithRedisFallback(ctx context.Context, redisOp func() error, fallbackOp func() error) error {
	config := GetRedisFallbackConfig()
	if config == nil || !config.EnableFallback {
		return redisOp()
	}

	// 检查Redis健康状态
	if !CheckRedisHealth(ctx, config.Client) {
		log.Println("[Fallback] Redis is unhealthy, using fallback strategy")
		return fallbackOp()
	}

	// 尝试Redis操作
	err := redisOp()
	if err != nil {
		log.Printf("[Fallback] Redis operation failed: %v, using fallback", err)
		return fallbackOp()
	}

	return nil
}

// UploadFallbackHandler 上传服务降级处理器
type UploadFallbackHandler struct {
	UseSimpleUpload bool // 是否启用简单上传降级
}

// Execute 执行上传逻辑，降级到简单上传
func (h *UploadFallbackHandler) Execute(
	normalUpload func() error,
	simpleUpload func() error,
) error {
	if h.UseSimpleUpload {
		log.Println("[Fallback] Using simple upload (fallback mode)")
		return simpleUpload()
	}

	err := normalUpload()
	if err != nil {
		log.Printf("[Fallback] Normal upload failed: %v, fallback to simple upload", err)
		h.UseSimpleUpload = true
		return simpleUpload()
	}

	return nil
}

// ResetFallback 重置降级状态
func (h *UploadFallbackHandler) ResetFallback() {
	h.UseSimpleUpload = false
	log.Println("[Fallback] Upload fallback status reset")
}

// ShouldUseSimpleUpload 是否应该使用简单上传
func (h *UploadFallbackHandler) ShouldUseSimpleUpload() bool {
	return h.UseSimpleUpload
}

// FallbackError 降级错误
type FallbackError struct {
	ServiceName string
	Reason      string
}

func (e *FallbackError) Error() string {
	return fmt.Sprintf("service %s fallback: %s", e.ServiceName, e.Reason)
}

// NewFallbackError 创建降级错误
func NewFallbackError(serviceName, reason string) *FallbackError {
	return &FallbackError{
		ServiceName: serviceName,
		Reason:      reason,
	}
}
