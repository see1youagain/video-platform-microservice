package minio

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var (
	BucketName  string
	storageRoot string
)

type Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	BucketName      string
	UseSSL          bool
}

func InitMinIO() error {
	cfg := Config{BucketName: getEnv("MINIO_BUCKET", "video-platform")}
	return InitMinIOWithConfig(cfg)
}

// InitMinIOWithConfig 本地文件系统模拟 MinIO
func InitMinIOWithConfig(config Config) error {
	BucketName = config.BucketName
	if BucketName == "" {
		BucketName = "video-platform"
	}
	storageRoot = filepath.Join("/tmp", "mock-minio", BucketName)
	if err := os.MkdirAll(storageRoot, 0o755); err != nil {
		return fmt.Errorf("init mock minio failed: %w", err)
	}
	fmt.Printf("✅ Mock MinIO ready at %s\n", storageRoot)
	return nil
}

func UploadFile(ctx context.Context, objectName string, filePath string, contentType string) (string, error) {
	_ = ctx
	_ = contentType
	src, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	target := filepath.Join(storageRoot, objectName)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	dst, err := os.Create(target)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err = io.Copy(dst, src); err != nil {
		return "", err
	}
	return fmt.Sprintf("mock-minio://%s/%s", BucketName, objectName), nil
}

func UploadStream(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) (string, error) {
	_ = ctx
	_ = size
	_ = contentType
	target := filepath.Join(storageRoot, objectName)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	dst, err := os.Create(target)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err = io.Copy(dst, reader); err != nil {
		return "", err
	}
	return fmt.Sprintf("mock-minio://%s/%s", BucketName, objectName), nil
}

func DownloadFile(ctx context.Context, objectName string, filePath string) error {
	_ = ctx
	src, err := os.Open(filepath.Join(storageRoot, objectName))
	if err != nil {
		return err
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	dst, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func GetObjectURL(ctx context.Context, objectName string, expiry time.Duration) (string, error) {
	_ = ctx
	_ = expiry
	return fmt.Sprintf("mock-minio://%s/%s", BucketName, objectName), nil
}

func DeleteObject(ctx context.Context, objectName string) error {
	_ = ctx
	return os.Remove(filepath.Join(storageRoot, objectName))
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
