package redis

import (
"context"
"fmt"
"os"
"strconv"
"time"

"github.com/redis/go-redis/v9"
)

var Client *redis.Client

// InitRedis 初始化 Redis 连接
func InitRedis() error {
host := os.Getenv("REDIS_HOST")
port := os.Getenv("REDIS_PORT")
password := os.Getenv("REDIS_PASSWORD")
dbStr := os.Getenv("REDIS_DB")

db := 0
if dbStr != "" {
var err error
db, err = strconv.Atoi(dbStr)
if err != nil {
return fmt.Errorf("invalid REDIS_DB: %w", err)
}
}

Client = redis.NewClient(&redis.Options{
Addr:         fmt.Sprintf("%s:%s", host, port),
Password:     password,
DB:           db,
DialTimeout:  5 * time.Second,
ReadTimeout:  3 * time.Second,
WriteTimeout: 3 * time.Second,
PoolSize:     10,
MinIdleConns: 5,
})

// 测试连接
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := Client.Ping(ctx).Err(); err != nil {
return fmt.Errorf("redis ping failed: %w", err)
}

fmt.Println("✅ Redis 连接成功")
return nil
}

// Close 关闭 Redis 连接
func Close() error {
if Client != nil {
return Client.Close()
}
return nil
}

// SetUploadStatus 设置上传状态
func SetUploadStatus(ctx context.Context, fileHash string, status string) error {
key := fmt.Sprintf("upload:status:%s", fileHash)
return Client.Set(ctx, key, status, 24*time.Hour).Err()
}

// GetUploadStatus 获取上传状态
func GetUploadStatus(ctx context.Context, fileHash string) (string, error) {
key := fmt.Sprintf("upload:status:%s", fileHash)
return Client.Get(ctx, key).Result()
}

// AddFinishedChunk 添加已完成的分片
func AddFinishedChunk(ctx context.Context, fileHash string, chunkIndex string) error {
key := fmt.Sprintf("upload:chunks:%s", fileHash)
return Client.SAdd(ctx, key, chunkIndex).Err()
}

// GetFinishedChunks 获取已完成的分片列表
func GetFinishedChunks(ctx context.Context, fileHash string) ([]string, error) {
key := fmt.Sprintf("upload:chunks:%s", fileHash)
return Client.SMembers(ctx, key).Result()
}

// DeleteUploadCache 删除上传相关缓存
func DeleteUploadCache(ctx context.Context, fileHash string) error {
statusKey := fmt.Sprintf("upload:status:%s", fileHash)
chunksKey := fmt.Sprintf("upload:chunks:%s", fileHash)
return Client.Del(ctx, statusKey, chunksKey).Err()
}

// SetTombstone 设置墓碑标记（文件已完成上传）
func SetTombstone(ctx context.Context, fileHash string, url string) error {
key := fmt.Sprintf("file:tombstone:%s", fileHash)
return Client.Set(ctx, key, url, 0).Err() // 永久保存
}

// GetTombstone 获取墓碑标记
func GetTombstone(ctx context.Context, fileHash string) (string, error) {
key := fmt.Sprintf("file:tombstone:%s", fileHash)
return Client.Get(ctx, key).Result()
}
