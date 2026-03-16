package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/bytedance/gopkg/cloud/metainfo"
	"github.com/minio/minio-go/v7"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	commonEvents "github.com/see1youagain/video-platform-microservice/common/events"
	commonMinio "github.com/see1youagain/video-platform-microservice/common/minio"
	commonOutbox "github.com/see1youagain/video-platform-microservice/common/outbox"
	commonredis "github.com/see1youagain/video-platform-microservice/common/redis"
	"gorm.io/gorm"

	uploadDb "video-platform-microservice/rpc-videoUpload/internal/db"
	"video-platform-microservice/rpc-videoUpload/kitex_gen/videoupload"
)

const minChunkSize = 5 * 1024 * 1024

func getUserIDFromContext(ctx context.Context, reqUserID string) string {
	if uid, ok := metainfo.GetPersistentValue(ctx, "user_id"); ok && uid != "" {
		return uid
	}
	if reqUserID != "" {
		return reqUserID
	}
	return "anonymous"
}

func originalObject(hash string) string {
	return "videos/raw/" + hash + ".mp4"
}

type VideoUploadServiceImpl struct{}

func findFinishedUpload(fileHash string) (*uploadDb.UploadFile, error) {
	var record uploadDb.UploadFile
	err := commondb.GetDB().Where("file_hash = ? AND status = ?", fileHash, "finished").Order("id DESC").First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func persistFinalize(fileHash, filename, userID, minioURL string, req *videoupload.FinalizeUploadReq) error {
	return commondb.DB.Transaction(func(tx *gorm.DB) error {
		var reqID string
		if req.RequestId != nil {
			reqID = *req.RequestId
		}

		if txErr := tx.Where(uploadDb.UploadFile{FileHash: fileHash}).
			Assign(uploadDb.UploadFile{
				UserID:    userID,
				Filename:  filename,
				URL:       minioURL,
				Status:    "finished",
				RequestID: reqID,
			}).
			FirstOrCreate(&uploadDb.UploadFile{}).Error; txErr != nil {
			return txErr
		}

		event := commonEvents.FileUploadedEvent{
			FileHash: fileHash,
			Filename: filename,
			UserID:   userID,
			MinioURL: minioURL,
		}
		payload, _ := json.Marshal(event)
		outboxMsg := commonOutbox.Event{
			Topic:       "video.file.uploaded",
			Payload:     string(payload),
			Status:      commonOutbox.StatusPending,
			AvailableAt: time.Now(),
		}
		return tx.Create(&outboxMsg).Error
	})
}

func (s *VideoUploadServiceImpl) InitUpload(ctx context.Context, req *videoupload.InitUploadReq) (*videoupload.InitUploadResp, error) {
	resp := &videoupload.InitUploadResp{}
	fileHash := req.FileHash
	if fileHash == "" {
		resp.Code = 400
		resp.Msg = "file_hash 不能为空"
		return resp, nil
	}

	if req.GetFileSize() > 0 && req.GetFileSize() < minChunkSize {
		status := "single_shot"
		resp.Code = 200
		resp.Msg = "文件小于5MB，请使用单次上传"
		resp.Status = &status
		return resp, nil
	}

	// 秒传优先查数据库（无状态）
	record, err := findFinishedUpload(fileHash)
	if err == nil && record.URL != "" {
		status := "finished"
		resp.Code = 200
		resp.Msg = "文件已存在（秒传）"
		resp.Status = &status
		resp.Url = &record.URL
		return resp, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		resp.Code = 500
		resp.Msg = "查询文件状态失败"
		return resp, nil
	}

var uploadID string
var status = "new"
var finishedChunks []string

client := commonredis.GetClient()
if client != nil {
if existingUploadID, err := client.Get(ctx, "upload_session:"+fileHash).Result(); err == nil && existingUploadID != "" {
keys, err := client.HKeys(ctx, "upload_progress:"+existingUploadID).Result()
if err == nil {
for _, k := range keys {
if part, err := strconv.Atoi(k); err == nil {
finishedChunks = append(finishedChunks, strconv.Itoa(part-1))
}
}
uploadID = existingUploadID
status = "partial"
}
}
}

if uploadID == "" {
id, err := commonMinio.Core.NewMultipartUpload(ctx, commonMinio.BucketName, originalObject(fileHash), minio.PutObjectOptions{
ContentType: "video/mp4",
})
if err != nil {
resp.Code = 500
resp.Msg = "Init MinIO error: " + err.Error()
return resp, nil
}
uploadID = id
if client != nil {
client.Set(ctx, "upload_progress_ttl:"+uploadID, 1, 24*time.Hour)
client.Set(ctx, "upload_session:"+fileHash, uploadID, 24*time.Hour)
}
}

resp.Code = 200
resp.Msg = "可以开始上传"
if len(finishedChunks) > 0 {
status = "partial"
}
resp.Status = &status
resp.UploadId = &uploadID
resp.FinishedChunks = finishedChunks
	return resp, nil
}

func (s *VideoUploadServiceImpl) UploadChunk(ctx context.Context, req *videoupload.UploadChunkReq) (*videoupload.UploadChunkResp, error) {
	resp := &videoupload.UploadChunkResp{}
	fileHash := req.FileHash
	uploadID := req.GetUploadId()
	chunkIndex := req.ChunkIndex

	if fileHash == "" || uploadID == "" || len(req.ChunkData) == 0 {
		resp.Code = 400
		resp.Msg = "参数不完整"
		return resp, nil
	}

	if len(req.ChunkData) < minChunkSize {
		resp.Code = 400
		resp.Msg = "分片大小不能小于 5MB"
		return resp, nil
	}

	partNum := int(chunkIndex) + 1
	chunkReader := bytes.NewReader(req.ChunkData)

	// 与 Kitex RPC 超时解耦，避免大分片 I/O 被截断
	ioCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Minute)
	defer cancel()

	part, err := commonMinio.Core.PutObjectPart(ioCtx, commonMinio.BucketName, originalObject(fileHash), uploadID, partNum, chunkReader, int64(len(req.ChunkData)), minio.PutObjectPartOptions{})
	if err != nil {
		resp.Code = 500
		resp.Msg = "上传分片到MinIO失败"
		log.Printf("[UploadChunk] err: %v", err)
		return resp, nil
	}

	client := commonredis.GetClient()
	if client != nil {
		redisKey := "upload_progress:" + uploadID
		client.HSet(ctx, redisKey, strconv.Itoa(partNum), part.ETag)
	}

	resp.Code = 200
	resp.Msg = "分片上传成功"
	return resp, nil
}

func (s *VideoUploadServiceImpl) FinalizeUpload(ctx context.Context, req *videoupload.FinalizeUploadReq) (*videoupload.FinalizeUploadResp, error) {
	resp := &videoupload.FinalizeUploadResp{}
	fileHash := req.FileHash
	filename := req.Filename
	uploadID := req.GetUploadId()
	totalChunks := int(req.TotalChunks)
	userID := getUserIDFromContext(ctx, req.GetUserId())

	if fileHash == "" || filename == "" || totalChunks <= 0 || uploadID == "" {
		resp.Code = 400
		resp.Msg = "参数不完整"
		return resp, nil
	}

	// 第一层防线：数据库短路（幂等）
	record, err := findFinishedUpload(fileHash)
	if err == nil && record.URL != "" {
		resp.Code = 200
		resp.Msg = "已合并（幂等返回）"
		resp.Url = &record.URL
		return resp, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		resp.Code = 500
		resp.Msg = "查询文件状态失败"
		return resp, nil
	}

	objectName := originalObject(fileHash)
	objectExists := false

	// 第二层防线：对象探测（解决 MinIO 合并成功但 DB 失败）
	if _, statErr := commonMinio.StatObject(ctx, objectName); statErr == nil {
		objectExists = true
	}

	if !objectExists {
		client := commonredis.GetClient()
		if client == nil {
			resp.Code = 500
			resp.Msg = "Redis未就绪"
			return resp, nil
		}

		redisKey := "upload_progress:" + uploadID
		hlen, hErr := client.HLen(ctx, redisKey).Result()
		if hErr != nil || int(hlen) != totalChunks {
			resp.Code = 400
			resp.Msg = fmt.Sprintf("分片数量不一致，期望: %d, 实际: %d", totalChunks, hlen)
			return resp, nil
		}

		partsMap, pErr := client.HGetAll(ctx, redisKey).Result()
		if pErr != nil {
			resp.Code = 500
			resp.Msg = "获取分片信息失败"
			return resp, nil
		}

		var completeParts []minio.CompletePart
		for k, v := range partsMap {
			pn, _ := strconv.Atoi(k)
			completeParts = append(completeParts, minio.CompletePart{PartNumber: pn, ETag: v})
		}
		sort.Slice(completeParts, func(i, j int) bool {
			return completeParts[i].PartNumber < completeParts[j].PartNumber
		})

		ioCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Minute)
		defer cancel()
		_, cErr := commonMinio.Core.CompleteMultipartUpload(ioCtx, commonMinio.BucketName, objectName, uploadID, completeParts, minio.PutObjectOptions{})
		if cErr != nil {
			resp.Code = 500
			resp.Msg = "合成文件失败"
			log.Printf("[FinalizeUpload] err: %v", cErr)
			return resp, nil
		}
	}

	minioURL, urlErr := commonMinio.GetObjectURL(ctx, objectName, 24*time.Hour)
	if urlErr != nil {
		resp.Code = 500
		resp.Msg = "生成访问地址失败"
		return resp, nil
	}

	// 第三层防线：独立事务 + Outbox 落库（可重试）
	if txErr := persistFinalize(fileHash, filename, userID, minioURL, req); txErr != nil {
		resp.Code = 500
		resp.Msg = "保存数据库或写入Outbox失败"
		return resp, nil
	}

	if client := commonredis.GetClient(); client != nil {
		client.Del(ctx, "upload_progress:"+uploadID)
		client.Del(ctx, "upload_progress_ttl:"+uploadID)
	}

	resp.Code = 200
	resp.Msg = "合并并发布成功"
	resp.Url = &minioURL
	return resp, nil
}

func (s *VideoUploadServiceImpl) QueryProgress(ctx context.Context, req *videoupload.QueryProgressReq) (*videoupload.QueryProgressResp, error) {
	resp := &videoupload.QueryProgressResp{}
	if req.UploadId == "" {
		resp.Code = 400
		resp.Msg = "upload_id 不能为空"
		return resp, nil
	}

	client := commonredis.GetClient()
	if client != nil {
		hlen, err := client.HLen(ctx, "upload_progress:"+req.UploadId).Result()
		if err == nil {
			parts := int32(hlen)
			resp.Code = 200
			resp.Msg = "查询成功"
			resp.UploadedParts = &parts
			return resp, nil
		}
	}
	resp.Code = 500
	resp.Msg = "查询失败"
	return resp, nil
}

func (s *VideoUploadServiceImpl) AbortUpload(ctx context.Context, req *videoupload.AbortUploadReq) (*videoupload.AbortUploadResp, error) {
	resp := &videoupload.AbortUploadResp{}
	if req.UploadId == "" || req.FileHash == "" {
		resp.Code = 400
		resp.Msg = "参数不完整"
		return resp, nil
	}

	err := commonMinio.Core.AbortMultipartUpload(ctx, commonMinio.BucketName, originalObject(req.FileHash), req.UploadId)
	if err != nil {
		log.Printf("[AbortUpload] err: %v", err)
	}

	resp.Code = 200
	resp.Msg = "取消成功"
	if client := commonredis.GetClient(); client != nil {
		client.Del(ctx, "upload_progress:"+req.UploadId)
		client.Del(ctx, "upload_progress_ttl:"+req.UploadId)
	}
	return resp, nil
}

func (s *VideoUploadServiceImpl) SimpleUpload(ctx context.Context, req *videoupload.SimpleUploadReq) (*videoupload.SimpleUploadResp, error) {
	resp := &videoupload.SimpleUploadResp{}
	fileHash := req.FileHash
	minioURL, err := commonMinio.UploadStream(ctx, originalObject(fileHash), bytes.NewReader(req.FileData), int64(len(req.FileData)), "video/mp4")
	if err != nil {
		resp.Code = 500
		resp.Msg = "上传文件失败"
		return resp, nil
	}

	resp.Code = 200
	resp.Msg = "上传成功"
	resp.Url = &minioURL
	return resp, nil
}
