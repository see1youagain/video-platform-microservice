package utils

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	currentSecretKey string
	secretKeyMutex   sync.RWMutex
	secretRotateTicker *time.Ticker
)

// InitJWTWithRotation 初始化 JWT 并启动密钥轮换
func InitJWTWithRotation(redisClient *redis.Client, rotateInterval time.Duration) error {
	// 首次初始化
	if err := loadOrGenerateSecret(context.Background(), redisClient); err != nil {
		return err
	}

	// 启动密钥轮换协程
	go startSecretRotation(redisClient, rotateInterval)

	return nil
}

// loadOrGenerateSecret 从 Redis 加载密钥，如果不存在则生成新密钥
func loadOrGenerateSecret(ctx context.Context, redisClient *redis.Client) error {
	const secretKey = "jwt:secret_key"

	// 尝试从 Redis 加载
	secret, err := redisClient.Get(ctx, secretKey).Result()
	if err == redis.Nil {
		// Redis 中没有密钥，生成新密钥
		secret, err = generateRandomSecret(64)
		if err != nil {
			return fmt.Errorf("生成密钥失败: %w", err)
		}

		// 保存到 Redis（设置 TTL）
		if err := redisClient.Set(ctx, secretKey, secret, 24*time.Hour).Err(); err != nil {
			return fmt.Errorf("保存密钥到 Redis 失败: %w", err)
		}

		log.Printf("[JWT] 生成新密钥并保存到 Redis")
	} else if err != nil {
		return fmt.Errorf("从 Redis 加载密钥失败: %w", err)
	} else {
		log.Printf("[JWT] 从 Redis 加载密钥成功")
	}

	// 更新全局密钥
	secretKeyMutex.Lock()
	currentSecretKey = secret
	secretKeyMutex.Unlock()

	// 同时设置到环境变量（兼容旧代码）
	os.Setenv("JWT_SECRET", secret)
	InitJWT() // 重新初始化 JWT

	return nil
}

// generateRandomSecret 生成随机密钥
func generateRandomSecret(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// startSecretRotation 定期轮换密钥
func startSecretRotation(redisClient *redis.Client, interval time.Duration) {
	secretRotateTicker = time.NewTicker(interval)
	defer secretRotateTicker.Stop()

	for range secretRotateTicker.C {
		log.Printf("[JWT] 开始轮换密钥...")
		
		// 生成新密钥
		newSecret, err := generateRandomSecret(64)
		if err != nil {
			log.Printf("[JWT] 生成新密钥失败: %v", err)
			continue
		}

		ctx := context.Background()
		
		// 保存旧密钥到备份 key（用于验证旧 token）
		secretKeyMutex.RLock()
		oldSecret := currentSecretKey
		secretKeyMutex.RUnlock()

		if oldSecret != "" {
			if err := redisClient.Set(ctx, "jwt:old_secret_key", oldSecret, 2*time.Hour).Err(); err != nil {
				log.Printf("[JWT] 保存旧密钥失败: %v", err)
			}
		}

		// 更新密钥
		if err := redisClient.Set(ctx, "jwt:secret_key", newSecret, 24*time.Hour).Err(); err != nil {
			log.Printf("[JWT] 更新密钥失败: %v", err)
			continue
		}

		// 更新全局密钥
		secretKeyMutex.Lock()
		currentSecretKey = newSecret
		secretKeyMutex.Unlock()

		os.Setenv("JWT_SECRET", newSecret)
		InitJWT() // 重新初始化 JWT

		log.Printf("[JWT] 密钥轮换成功")
	}
}

// GetCurrentSecretKey 获取当前密钥
func GetCurrentSecretKey() string {
	secretKeyMutex.RLock()
	defer secretKeyMutex.RUnlock()
	return currentSecretKey
}

// CacheToken 将 token 缓存到 Redis
func CacheToken(ctx context.Context, redisClient *redis.Client, userID string, token string, expiration time.Duration) error {
	key := fmt.Sprintf("token:%s", userID)
	return redisClient.Set(ctx, key, token, expiration).Err()
}

// GetCachedToken 从 Redis 获取缓存的 token
func GetCachedToken(ctx context.Context, redisClient *redis.Client, userID string) (string, error) {
	key := fmt.Sprintf("token:%s", userID)
	return redisClient.Get(ctx, key).Result()
}

// InvalidateToken 使 token 失效
func InvalidateToken(ctx context.Context, redisClient *redis.Client, userID string) error {
	key := fmt.Sprintf("token:%s", userID)
	return redisClient.Del(ctx, key).Err()
}

// ValidateTokenFromCache 从缓存验证 token
func ValidateTokenFromCache(ctx context.Context, redisClient *redis.Client, userID string, token string) (bool, error) {
	cachedToken, err := GetCachedToken(ctx, redisClient, userID)
	if err == redis.Nil {
		return false, nil // token 不在缓存中
	}
	if err != nil {
		return false, err
	}

	return cachedToken == token, nil
}
