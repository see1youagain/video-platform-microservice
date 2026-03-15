package minio

import (
"context"
"fmt"
"io"
"os"
"strings"
"time"

"github.com/minio/minio-go/v7"
"github.com/minio/minio-go/v7/pkg/credentials"
)

var (
Core       *minio.Core
Client     *minio.Client
BucketName string
)

type Config struct {
Endpoint        string
AccessKeyID     string
SecretAccessKey string
BucketName      string
UseSSL          bool
}

func InitMinIO() error {
cfg := Config{
Endpoint:        getEnv("MINIO_ENDPOINT", "127.0.0.1:9000"),
AccessKeyID:     getEnv("MINIO_ACCESS_KEY", "minioadmin"),
SecretAccessKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
BucketName:      getEnv("MINIO_BUCKET", "video-platform"),
UseSSL:          strings.EqualFold(getEnv("MINIO_USE_SSL", "false"), "true"),
}
return InitMinIOWithConfig(cfg)
}

func InitMinIOWithConfig(config Config) error {
var err error
Client, err = minio.New(config.Endpoint, &minio.Options{
Creds:  credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
Secure: config.UseSSL,
})
if err != nil {
return fmt.Errorf("init minio client failed: %w", err)
}

Core, err = minio.NewCore(config.Endpoint, &minio.Options{
Creds:  credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
Secure: config.UseSSL,
})
if err != nil {
return fmt.Errorf("init minio core failed: %w", err)
}

BucketName = config.BucketName
if BucketName == "" {
BucketName = "video-platform"
}

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

exists, err := Client.BucketExists(ctx, BucketName)
if err != nil {
return fmt.Errorf("check bucket failed: %w", err)
}
if !exists {
if err := Client.MakeBucket(ctx, BucketName, minio.MakeBucketOptions{}); err != nil {
return fmt.Errorf("create bucket failed: %w", err)
}
}

return nil
}

func UploadFile(ctx context.Context, objectName, filePath, contentType string) (string, error) {
f, err := os.Open(filePath)
if err != nil {
return "", fmt.Errorf("open file failed: %w", err)
}
defer f.Close()

fi, err := f.Stat()
if err != nil {
return "", fmt.Errorf("stat file failed: %w", err)
}

if _, err := Client.PutObject(ctx, BucketName, objectName, f, fi.Size(), minio.PutObjectOptions{
ContentType: contentType,
}); err != nil {
return "", fmt.Errorf("put object failed: %w", err)
}

return GetObjectURL(ctx, objectName, 7*24*time.Hour)
}

func UploadStream(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) (string, error) {
if _, err := Client.PutObject(ctx, BucketName, objectName, reader, size, minio.PutObjectOptions{
ContentType: contentType,
}); err != nil {
return "", fmt.Errorf("put object stream failed: %w", err)
}

return GetObjectURL(ctx, objectName, 7*24*time.Hour)
}

func DownloadFile(ctx context.Context, objectName, filePath string) error {
obj, err := Client.GetObject(ctx, BucketName, objectName, minio.GetObjectOptions{})
if err != nil {
return fmt.Errorf("get object failed: %w", err)
}
defer obj.Close()

dst, err := os.Create(filePath)
if err != nil {
return fmt.Errorf("create file failed: %w", err)
}
defer dst.Close()

if _, err = io.Copy(dst, obj); err != nil {
return fmt.Errorf("copy object failed: %w", err)
}
return nil
}

func GetObject(ctx context.Context, objectName string) (*minio.Object, error) {
obj, err := Client.GetObject(ctx, BucketName, objectName, minio.GetObjectOptions{})
if err != nil {
return nil, fmt.Errorf("get object failed: %w", err)
}
return obj, nil
}

func StatObject(ctx context.Context, objectName string) (minio.ObjectInfo, error) {
info, err := Client.StatObject(ctx, BucketName, objectName, minio.StatObjectOptions{})
if err != nil {
return minio.ObjectInfo{}, fmt.Errorf("stat object failed: %w", err)
}
return info, nil
}

func ComposeObjects(ctx context.Context, dstObject string, srcObjects []string) (string, error) {
if len(srcObjects) == 0 {
return "", fmt.Errorf("srcObjects is empty")
}

sources := make([]minio.CopySrcOptions, 0, len(srcObjects))
for _, src := range srcObjects {
sources = append(sources, minio.CopySrcOptions{Bucket: BucketName, Object: src})
}

if _, err := Client.ComposeObject(ctx, minio.CopyDestOptions{Bucket: BucketName, Object: dstObject}, sources...); err != nil {
return "", fmt.Errorf("compose object failed: %w", err)
}

return GetObjectURL(ctx, dstObject, 7*24*time.Hour)
}

func GetObjectURL(ctx context.Context, objectName string, expiry time.Duration) (string, error) {
url, err := Client.PresignedGetObject(ctx, BucketName, objectName, expiry, nil)
if err != nil {
return "", fmt.Errorf("presign url failed: %w", err)
}
return url.String(), nil
}

func DeleteObject(ctx context.Context, objectName string) error {
if err := Client.RemoveObject(ctx, BucketName, objectName, minio.RemoveObjectOptions{}); err != nil {
return fmt.Errorf("remove object failed: %w", err)
}
return nil
}

func getEnv(key, defaultValue string) string {
if value := os.Getenv(key); value != "" {
return value
}
return defaultValue
}
