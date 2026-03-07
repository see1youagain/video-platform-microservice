package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	commonEvents "github.com/see1youagain/video-platform-microservice/common/events"
	commonKafka "github.com/see1youagain/video-platform-microservice/common/kafka"
	commonMinio "github.com/see1youagain/video-platform-microservice/common/minio"

	"github.com/bytedance/gopkg/cloud/metainfo"

	"video-platform-microservice/rpc-videoUpload/kitex_gen/videoupload"
)



// getUserIDFromContext 优先从 metainfo 获取 user_id（由 gateway JWT 中间件注入），
// 回退到请求参数，最终默认 anonymous。
func getUserIDFromContext(ctx context.Context, reqUserID string) string {
	if uid, ok := metainfo.GetPersistentValue(ctx, "user_id"); ok && uid != "" {
		return uid
	}
	if reqUserID != "" {
		return reqUserID
	}
	return "anonymous"
}

// videoFileRecord mirrors the video_files table in rpc-video for DB fallback
type videoFileRecord struct {
	FileHash        string `gorm:"column:file_hash;uniqueIndex"`
	UserID          string `gorm:"column:user_id;index"`
	Filename        string `gorm:"column:filename"`
	FileSize        int64  `gorm:"column:file_size"`
	URL             string `gorm:"column:url"`
	Status          string `gorm:"column:status"`
	TranscodeStatus string `gorm:"column:transcode_status"`
	Width           int32  `gorm:"column:width"`
	Height          int32  `gorm:"column:height"`
	RequestID       string `gorm:"column:request_id;index"`
	RefCount        int32  `gorm:"column:ref_count"`
}

func (videoFileRecord) TableName() string { return "video_files" }

// writeFileRecordToDB is a fallback when Kafka is unavailable. It inserts the
// file record directly into the shared video_files table so that rpc-video can
// find the file for transcode requests.
func writeFileRecordToDB(event commonEvents.FileUploadedEvent) {
	if commondb.DB == nil {
		return
	}
	rec := &videoFileRecord{
		FileHash:        event.FileHash,
		UserID:          event.UserID,
		Filename:        event.Filename,
		FileSize:        event.FileSize,
		URL:             event.MinioURL,
		Status:          "finished",
		TranscodeStatus: "pending",
		Width:           int32(event.Width),
		Height:          int32(event.Height),
		RequestID:       event.RequestID,
		RefCount:        1,
	}
	// Use Save with conflict: if hash already exists, update the URL
	result := commondb.DB.Where(videoFileRecord{FileHash: event.FileHash, UserID: event.UserID}).
		FirstOrCreate(rec)
	if result.Error != nil {
		log.Printf("[writeFileRecordToDB] DB 写入失败 fileHash=%s: %v", event.FileHash, result.Error)
	} else {
		log.Printf("[writeFileRecordToDB] DB 回退写入成功 fileHash=%s", event.FileHash)
	}
}

// ─── 内存状态存储（生产环境应换成 Redis / DB）──────────────────────────────

var (
	// 已知秒传结果：fileHash → url
	tombstoneMu sync.RWMutex
	tombstones  = make(map[string]string)

	// 已上传的分片记录：fileHash → set<chunkIndex>
	chunksMu sync.RWMutex
	chunks   = make(map[string]map[int]bool)
)

// ─── 本地分片目录 ──────────────────────────────────────────────────────────
const chunkDir = "/tmp/video-upload-chunks"

// ─── 事务性消息 Outbox（内存版） ────────────────────────────────────────────

type outboxRecord struct {
	ID        string
	Topic     string
	Key       string
	Payload   string
	CreatedAt time.Time
	Sent      bool
}

var (
	outboxMu sync.Mutex
	outbox   []*outboxRecord
)

// addOutbox 原子写入 outbox（先落盘 outbox，再发 Kafka）
func addOutbox(topic, key, payload string) {
	outboxMu.Lock()
	defer outboxMu.Unlock()
	outbox = append(outbox, &outboxRecord{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Topic:     topic,
		Key:       key,
		Payload:   payload,
		CreatedAt: time.Now(),
	})
}

// outboxReaper 后台定时重试未发送的 outbox 消息（最终一致性保证）
func outboxReaper(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			outboxMu.Lock()
			for _, rec := range outbox {
				if rec.Sent {
					continue
				}
				err := commonKafka.Publish(ctx, rec.Topic, rec.Key, rec.Payload)
				if err != nil {
					log.Printf("[Outbox] 重试发送失败 id=%s err=%v", rec.ID, err)
					continue
				}
				rec.Sent = true
				log.Printf("[Outbox] 重试发送成功 id=%s topic=%s", rec.ID, rec.Topic)
			}
			outboxMu.Unlock()
		}
	}
}

// ─── 服务实现 ──────────────────────────────────────────────────────────────

// VideoUploadServiceImpl 实现 VideoUploadService IDL 接口
type VideoUploadServiceImpl struct{}

// InitUpload 初始化上传：支持秒传（墓碑 + MinIO 检查）
func (s *VideoUploadServiceImpl) InitUpload(ctx context.Context, req *videoupload.InitUploadReq) (*videoupload.InitUploadResp, error) {
	resp := &videoupload.InitUploadResp{}
	fileHash := req.FileHash
	if fileHash == "" {
		resp.Code = 400
		resp.Msg = "file_hash 不能为空"
		return resp, nil
	}

	// 1. 内存墓碑：最快路径
	tombstoneMu.RLock()
	url, hit := tombstones[fileHash]
	tombstoneMu.RUnlock()
	if hit {
		status := "finished"
		resp.Code = 200
		resp.Msg = "秒传成功"
		resp.Status = &status
		resp.Url = &url
		return resp, nil
	}

	// 2. MinIO 检查
	minioURL, err := commonMinio.GetObjectURL(ctx, "videos/"+fileHash+"/original", 24*time.Hour)
	if err == nil && minioURL != "" {
		tombstoneMu.Lock()
		tombstones[fileHash] = minioURL
		tombstoneMu.Unlock()
		status := "finished"
		resp.Code = 200
		resp.Msg = "文件已存在（秒传）"
		resp.Status = &status
		resp.Url = &minioURL
		return resp, nil
	}

	// 3. 检查已上传分片
	chunksMu.RLock()
	chunkSet, ok := chunks[fileHash]
	chunksMu.RUnlock()

	var finishedList []string
	if ok {
		for idx := range chunkSet {
			finishedList = append(finishedList, strconv.Itoa(idx))
		}
		sort.Strings(finishedList)
	}

	status := "new"
	if len(finishedList) > 0 {
		status = "partial"
	}
	resp.Code = 200
	resp.Msg = "准备上传"
	resp.Status = &status
	resp.FinishedChunks = finishedList
	return resp, nil
}

// UploadChunk 上传单个分片，保存到本地临时目录
func (s *VideoUploadServiceImpl) UploadChunk(ctx context.Context, req *videoupload.UploadChunkReq) (*videoupload.UploadChunkResp, error) {
	resp := &videoupload.UploadChunkResp{}
	fileHash := req.FileHash

	// 幂等：如果分片已存在则直接返回
	chunksMu.RLock()
	_, exists := chunks[fileHash][int(req.ChunkIndex)]
	chunksMu.RUnlock()
	if exists {
		resp.Code = 200
		resp.Msg = "分片已存在"
		alreadyUp := true
		resp.AlreadyUploaded = &alreadyUp
		return resp, nil
	}

	// 保存分片到临时目录
	dir := filepath.Join(chunkDir, fileHash)
	if err := os.MkdirAll(dir, 0755); err != nil {
		resp.Code = 500
		resp.Msg = "创建目录失败"
		return resp, nil
	}
	chunkPath := filepath.Join(dir, fmt.Sprintf("chunk_%d", req.ChunkIndex))
	if err := os.WriteFile(chunkPath, req.ChunkData, 0644); err != nil {
		resp.Code = 500
		resp.Msg = "写入分片失败"
		return resp, nil
	}

	// 记录分片
	chunksMu.Lock()
	if chunks[fileHash] == nil {
		chunks[fileHash] = make(map[int]bool)
	}
	chunks[fileHash][int(req.ChunkIndex)] = true
	chunksMu.Unlock()

	resp.Code = 200
	resp.Msg = "分片上传成功"
	return resp, nil
}

// FinalizeUpload 合并分片 → 写入 MinIO → 原子发布 FileUploaded 事件
func (s *VideoUploadServiceImpl) FinalizeUpload(ctx context.Context, req *videoupload.FinalizeUploadReq) (*videoupload.FinalizeUploadResp, error) {
	resp := &videoupload.FinalizeUploadResp{}
	fileHash := req.FileHash
	filename := req.Filename
	totalChunks := int(req.TotalChunks)
	userID := getUserIDFromContext(ctx, req.GetUserId())

	// 检查所有分片是否就绪
	chunksMu.RLock()
	chunkSet := chunks[fileHash]
	chunksMu.RUnlock()
	for i := 0; i < totalChunks; i++ {
		if !chunkSet[i] {
			resp.Code = 400
			resp.Msg = fmt.Sprintf("分片 %d 尚未上传", i)
			return resp, nil
		}
	}

	// 合并分片到临时文件
	tmpFile := filepath.Join(chunkDir, fileHash, "merged")
	f, err := os.Create(tmpFile)
	if err != nil {
		resp.Code = 500
		resp.Msg = "创建合并文件失败"
		return resp, nil
	}
	var totalSize int64
	for i := 0; i < totalChunks; i++ {
		chunkPath := filepath.Join(chunkDir, fileHash, fmt.Sprintf("chunk_%d", i))
		data, readErr := os.ReadFile(chunkPath)
		if readErr != nil {
			f.Close()
			resp.Code = 500
			resp.Msg = fmt.Sprintf("读取分片 %d 失败", i)
			return resp, nil
		}
		f.Write(data)
		totalSize += int64(len(data))
	}
	f.Close()

	// 上传合并文件到 MinIO
	mergedData, err := os.ReadFile(tmpFile)
	if err != nil {
		resp.Code = 500
		resp.Msg = "读取合并文件失败"
		return resp, nil
	}
	objectName := fileHash + "/original"
	minioURL, err := commonMinio.UploadStream(ctx, "videos/"+objectName, bytes.NewReader(mergedData), int64(len(mergedData)), "video/mp4")
	if err != nil {
		// MinIO 失败时降级：使用本地 URL
		minioURL = fmt.Sprintf("/files/video/%s/original", fileHash)
		log.Printf("[FinalizeUpload] MinIO 上传失败，使用本地路径: %v", err)
	}

	// ─── 事务性消息：先写 outbox，再发 Kafka ────────────────────────────
	event := commonEvents.FileUploadedEvent{
		FileHash:    fileHash,
		Filename:    filename,
		UserID:      userID,
		MinioURL:    minioURL,
		FileSize:    totalSize,
		Width:       req.GetWidth(),
		Height:      req.GetHeight(),
		Resolutions: req.Resolutions,
		RequestID:   req.GetRequestId(),
	}
	payload, _ := json.Marshal(event)

	// 1. 先写 outbox（保证不丢消息）
	addOutbox(commonEvents.TopicFileUploaded, "file:"+fileHash, string(payload))

	// 2. 立即尝试发 Kafka
	if pubErr := commonKafka.Publish(ctx, commonEvents.TopicFileUploaded, "file:"+fileHash, string(payload)); pubErr != nil {
		log.Printf("[FinalizeUpload] Kafka 发布失败，outbox 将重试: %v", pubErr)
		writeFileRecordToDB(event) // DB 直写回退
	} else {
		// 标记 outbox 为已发送
		outboxMu.Lock()
		for _, rec := range outbox {
			if rec.Key == "file:"+fileHash && !rec.Sent {
				rec.Sent = true
				break
			}
		}
		outboxMu.Unlock()
		log.Printf("[FinalizeUpload] FileUploaded 事件发布成功 fileHash=%s", fileHash)
	}

	// 清理分片
	go func() {
		time.Sleep(5 * time.Second)
		_ = os.RemoveAll(filepath.Join(chunkDir, fileHash))
		chunksMu.Lock()
		delete(chunks, fileHash)
		chunksMu.Unlock()
	}()

	// 写入墓碑
	tombstoneMu.Lock()
	tombstones[fileHash] = minioURL
	tombstoneMu.Unlock()

	resp.Code = 200
	resp.Msg = "上传完成，转码任务已提交"
	resp.Url = &minioURL
	return resp, nil
}

// SimpleUpload 简单直传（小文件 / 降级路径），也发布 FileUploaded 事件
func (s *VideoUploadServiceImpl) SimpleUpload(ctx context.Context, req *videoupload.SimpleUploadReq) (*videoupload.SimpleUploadResp, error) {
	resp := &videoupload.SimpleUploadResp{}
	fileHash := req.FileHash
	filename := req.Filename
	userID := getUserIDFromContext(ctx, req.GetUserId())

	objectName := fileHash + "/original"
	minioURL, err := commonMinio.UploadStream(ctx, "videos/"+objectName, bytes.NewReader(req.FileData), int64(len(req.FileData)), "video/mp4")
	if err != nil {
		minioURL = fmt.Sprintf("/files/video/%s/original", fileHash)
		log.Printf("[SimpleUpload] MinIO 上传失败，使用本地路径: %v", err)
	}

	event := commonEvents.FileUploadedEvent{
		FileHash: fileHash,
		Filename: filename,
		UserID:   userID,
		MinioURL: minioURL,
		FileSize: int64(len(req.FileData)),
	}
	payload, _ := json.Marshal(event)

	addOutbox(commonEvents.TopicFileUploaded, "file:"+fileHash, string(payload))
	if strings.Contains(minioURL, "/files/") {
		// MinIO 降级，outbox 重试 + DB 回退
		writeFileRecordToDB(event)
	} else {
		if pubErr := commonKafka.Publish(ctx, commonEvents.TopicFileUploaded, "file:"+fileHash, string(payload)); pubErr != nil {
			log.Printf("[SimpleUpload] Kafka 发布失败，outbox 将重试: %v", pubErr)
			writeFileRecordToDB(event) // DB 直写回退
		} else {
			outboxMu.Lock()
			for _, rec := range outbox {
				if rec.Key == "file:"+fileHash && !rec.Sent {
					rec.Sent = true
					break
				}
			}
			outboxMu.Unlock()
		}
	}

	tombstoneMu.Lock()
	tombstones[fileHash] = minioURL
	tombstoneMu.Unlock()

	resp.Code = 200
	resp.Msg = "上传成功"
	resp.Url = &minioURL
	return resp, nil
}
