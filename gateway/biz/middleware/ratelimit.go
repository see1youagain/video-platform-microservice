package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/redis/go-redis/v9"
)

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Rate       int           // 令牌生成速率（每秒）
	Capacity   int           // 令牌桶容量
	RedisClient *redis.Client // Redis 客户端
}

// TokenBucketMiddleware 基于 Redis 的令牌桶限流中间件
func TokenBucketMiddleware(config RateLimitConfig) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// 获取客户端标识（可以是 IP、用户 ID 等）
		clientID := c.ClientIP()
	
		// 尝试从令牌桶获取令牌
		allowed, err := tryAcquireToken(ctx, config.RedisClient, clientID, config.Rate, config.Capacity)
		if err != nil {
			c.JSON(consts.StatusInternalServerError, map[string]interface{}{
				"code": 500,
				"msg":  "限流服务异常",
			})
			c.Abort()
			return
		}

		if !allowed {
			c.JSON(consts.StatusTooManyRequests, map[string]interface{}{
				"code": 429,
				"msg":  "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}

		c.Next(ctx)
	}
}

// tryAcquireToken 尝试从令牌桶获取令牌
func tryAcquireToken(ctx context.Context, client *redis.Client, clientID string, rate, capacity int) (bool, error) {
	now := time.Now().Unix()
	key := fmt.Sprintf("ratelimit:token_bucket:%s", clientID)
	tokensKey := key + ":tokens"
	timestampKey := key + ":timestamp"

	// 使用 Lua 脚本保证原子性
	script := `
		local tokens_key = KEYS[1]
		local timestamp_key = KEYS[2]
		local rate = tonumber(ARGV[1])
		local capacity = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])
		local requested = 1

		local tokens = tonumber(redis.call('get', tokens_key))
		if not tokens then
			tokens = capacity
		end

		local last_time = tonumber(redis.call('get', timestamp_key))
		if not last_time then
			last_time = now
		end

		local delta = math.max(0, now - last_time)
		local filled_tokens = math.min(capacity, tokens + delta * rate)

		local allowed = 0
		local new_tokens = filled_tokens
		if filled_tokens >= requested then
			new_tokens = filled_tokens - requested
			allowed = 1
		end

		redis.call('setex', tokens_key, 3600, new_tokens)
		redis.call('setex', timestamp_key, 3600, now)

		return allowed
	`

	result, err := client.Eval(ctx, script, []string{tokensKey, timestampKey}, rate, capacity, now).Result()
	if err != nil {
		return false, err
	}

	return result.(int64) == 1, nil
}
