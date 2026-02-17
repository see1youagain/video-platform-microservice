package redis

import (
	"context"
	"time"

	redisLib "github.com/redis/go-redis/v9"
	"github.com/see1youagain/video-platform-microservice/common/redis"
)

const (
	UploadStatusPrefix    = "upload:status:"
	UploadChunksPrefix    = "upload:chunks:"
	TombstonePrefix       = "file:tombstone:"
	UploadStatusTTL       = 24 * time.Hour
	TombstoneTTL          = 30 * 24 * time.Hour // 30 days
)

// GetClient returns the Redis client from common
func GetClient() *redisLib.Client {
	return redis.GetClient()
}

// SetUploadStatus sets upload status for a file
func SetUploadStatus(ctx context.Context, fileHash string, status string) error {
	key := UploadStatusPrefix + fileHash
	return GetClient().Set(ctx, key, status, UploadStatusTTL).Err()
}

// GetUploadStatus gets upload status for a file
func GetUploadStatus(ctx context.Context, fileHash string) (string, error) {
	key := UploadStatusPrefix + fileHash
	return GetClient().Get(ctx, key).Result()
}

// AddFinishedChunk adds a finished chunk index to Redis set
func AddFinishedChunk(ctx context.Context, fileHash string, chunkIndex string) error {
	key := UploadChunksPrefix + fileHash
	if err := GetClient().SAdd(ctx, key, chunkIndex).Err(); err != nil {
		return err
	}
	// Set expiration time
	return GetClient().Expire(ctx, key, UploadStatusTTL).Err()
}

// GetFinishedChunks gets all finished chunk indices for a file
func GetFinishedChunks(ctx context.Context, fileHash string) ([]string, error) {
	key := UploadChunksPrefix + fileHash
	return GetClient().SMembers(ctx, key).Result()
}

// DeleteUploadCache clears upload cache for a file
func DeleteUploadCache(ctx context.Context, fileHash string) error {
	statusKey := UploadStatusPrefix + fileHash
	chunksKey := UploadChunksPrefix + fileHash
	return GetClient().Del(ctx, statusKey, chunksKey).Err()
}

// SetTombstone sets a tombstone for a completed file (permanent cache)
func SetTombstone(ctx context.Context, fileHash string, url string) error {
	key := TombstonePrefix + fileHash
	return GetClient().Set(ctx, key, url, TombstoneTTL).Err()
}

// GetTombstone gets tombstone for a file
func GetTombstone(ctx context.Context, fileHash string) (string, error) {
	key := TombstonePrefix + fileHash
	return GetClient().Get(ctx, key).Result()
}

// IsChunkUploaded checks if a chunk is in the uploaded set
func IsChunkUploaded(ctx context.Context, fileHash string, chunkIndex string) (bool, error) {
	key := UploadChunksPrefix + fileHash
	return GetClient().SIsMember(ctx, key, chunkIndex).Result()
}

// GetUploadedChunkCount gets count of uploaded chunks
func GetUploadedChunkCount(ctx context.Context, fileHash string) (int64, error) {
	key := UploadChunksPrefix + fileHash
	return GetClient().SCard(ctx, key).Result()
}

// ClearUploadCache clears all upload-related data for a file
func ClearUploadCache(ctx context.Context, fileHash string) error {
	return DeleteUploadCache(ctx, fileHash)
}
