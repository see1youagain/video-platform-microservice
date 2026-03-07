package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	commonEvents "github.com/see1youagain/video-platform-microservice/common/events"
	commonKafka "github.com/see1youagain/video-platform-microservice/common/kafka"
	commonMinio "github.com/see1youagain/video-platform-microservice/common/minio"

	"video-platform-microservice/rpc-videoTranscode/kitex_gen/videotranscode"
)

// ─── 任务状态（内存版，生产应换 DB）──────────────────────────────────────────

type taskRecord struct {
	TaskID       string
	FileHash     string
	UserID       string
	Status       string   // pending | processing | completed | failed
	Progress     int32
	ResultURLs   []string
	ErrorMsg     string
	Resolutions  []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

var (
	tasksMu      sync.RWMutex
	tasksByID    = make(map[string]*taskRecord) // taskID → record
	tasksByHash  = make(map[string]*taskRecord) // fileHash → record（幂等 key）
)

// ─── 服务实现 ──────────────────────────────────────────────────────────────

type VideoTranscodeServiceImpl struct{}

// CreateTask 手动触发转码任务（幂等：相同 fileHash 返回已有任务）
func (s *VideoTranscodeServiceImpl) CreateTask(ctx context.Context, req *videotranscode.TranscodeTaskReq) (*videotranscode.TranscodeTaskResp, error) {
	resp := &videotranscode.TranscodeTaskResp{}

	fileHash := req.FileHash
	if fileHash == "" {
		resp.Code = 400
		resp.Msg = "file_hash 不能为空"
		return resp, nil
	}

	// 幂等检查：相同 fileHash 直接返回已有 taskID
	tasksMu.RLock()
	existing, ok := tasksByHash[fileHash]
	tasksMu.RUnlock()
	if ok {
		log.Printf("[CreateTask] 幂等命中 fileHash=%s taskID=%s", fileHash, existing.TaskID)
		taskID := existing.TaskID
		resp.Code = 200
		resp.Msg = "任务已存在（幂等）"
		resp.TaskId = &taskID
		return resp, nil
	}

	// 创建新任务
	taskID := fmt.Sprintf("tc-%s-%d", fileHash[:8], time.Now().UnixMilli())
	record := &taskRecord{
		TaskID:      taskID,
		FileHash:    fileHash,
		Status:      "pending",
		Resolutions: req.ResolutionList,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	tasksMu.Lock()
	tasksByID[taskID] = record
	tasksByHash[fileHash] = record
	tasksMu.Unlock()

	// 异步执行转码（使用 MinioURL 作为原片来源）
	go processTranscode(context.Background(), record, req.MinioUrl)

	log.Printf("[CreateTask] 新任务创建 fileHash=%s taskID=%s", fileHash, taskID)
	resp.Code = 200
	resp.Msg = "转码任务已创建"
	resp.TaskId = &taskID
	return resp, nil
}

// GetStatus 查询转码任务状态
func (s *VideoTranscodeServiceImpl) GetStatus(ctx context.Context, req *videotranscode.TranscodeStatusReq) (*videotranscode.TranscodeStatusResp, error) {
	resp := &videotranscode.TranscodeStatusResp{}

	tasksMu.RLock()
	rec, ok := tasksByID[req.TaskId]
	tasksMu.RUnlock()

	if !ok {
		resp.Code = 404
		resp.Msg = "任务不存在"
		return resp, nil
	}

	tasksMu.RLock()
	status := rec.Status
	progress := rec.Progress
	urls := append([]string{}, rec.ResultURLs...)
	errMsg := rec.ErrorMsg
	tasksMu.RUnlock()

	resp.Code = 200
	resp.Msg = "ok"
	resp.Status = &status
	resp.Progress = &progress
	resp.Urls = urls
	if errMsg != "" {
		resp.ErrorMsg = &errMsg
	}
	return resp, nil
}

// ─── Kafka 消费者：订阅 FileUploaded ─────────────────────────────────────────

// StartFileUploadedConsumer 订阅 video.file.uploaded 事件，触发转码（幂等）
func StartFileUploadedConsumer(ctx context.Context) {
	reader := commonKafka.NewReader(commonEvents.TopicFileUploaded, "transcode-service")
	defer reader.Close()

	log.Printf("[videoTranscode] 开始消费 topic=%s", commonEvents.TopicFileUploaded)
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[videoTranscode] ReadMessage 错误: %v", err)
			time.Sleep(time.Second)
			continue
		}

		var event commonEvents.FileUploadedEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			log.Printf("[videoTranscode] 解析消息失败: %v", err)
			continue
		}

		log.Printf("[videoTranscode] 收到 FileUploaded fileHash=%s user=%s", event.FileHash, event.UserID)

		// 幂等检查
		tasksMu.RLock()
		_, alreadyExists := tasksByHash[event.FileHash]
		tasksMu.RUnlock()
		if alreadyExists {
			log.Printf("[videoTranscode] 幂等：fileHash=%s 已有任务，跳过", event.FileHash)
			continue
		}

		// 创建并执行转码
		taskID := fmt.Sprintf("tc-%s-%d", event.FileHash[:min(8, len(event.FileHash))], time.Now().UnixMilli())
		resolutions := event.Resolutions
		if len(resolutions) == 0 {
			resolutions = []string{"360p", "720p", "1080p"}
		}
		record := &taskRecord{
			TaskID:      taskID,
			FileHash:    event.FileHash,
			UserID:      event.UserID,
			Status:      "pending",
			Resolutions: resolutions,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		tasksMu.Lock()
		tasksByID[taskID] = record
		tasksByHash[event.FileHash] = record
		tasksMu.Unlock()

		go processTranscode(ctx, record, event.MinioURL)
	}
}

// ─── 转码逻辑（模拟）──────────────────────────────────────────────────────────

func processTranscode(ctx context.Context, rec *taskRecord, minioURL string) {
	fileHash := rec.FileHash
	taskID := rec.TaskID

	// 更新为 processing
	tasksMu.Lock()
	rec.Status = "processing"
	rec.Progress = 0
	rec.UpdatedAt = time.Now()
	tasksMu.Unlock()

	log.Printf("[Transcode] 开始转码 taskID=%s fileHash=%s", taskID, fileHash)

	// 模拟转码进度
	var resultURLs []string
	for i, res := range rec.Resolutions {
		time.Sleep(500 * time.Millisecond) // 模拟转码耗时

		objectName := fmt.Sprintf("%s/%s.mp4", fileHash, res)
		// 尝试写入 MinIO（生产中应是真实转码后上传）
		dummyData := []byte(fmt.Sprintf("transcoded/%s/%s", fileHash, res))
		url, err := commonMinio.UploadStream(ctx, "videos/"+objectName, bytes.NewReader(dummyData), int64(len(dummyData)), "video/mp4")
		if err != nil {
			url = fmt.Sprintf("/files/video/%s/%s.mp4", fileHash, res)
		}
		resultURLs = append(resultURLs, url)

		progress := int32((i + 1) * 100 / len(rec.Resolutions))
		tasksMu.Lock()
		rec.Progress = progress
		rec.UpdatedAt = time.Now()
		tasksMu.Unlock()
	}

	// 转码完成
	tasksMu.Lock()
	rec.Status = "completed"
	rec.Progress = 100
	rec.ResultURLs = resultURLs
	rec.UpdatedAt = time.Now()
	tasksMu.Unlock()

	log.Printf("[Transcode] 转码完成 taskID=%s urls=%v", taskID, resultURLs)

	// 发布 TranscodeFinished 事件 → video 服务订阅
	finishedEvt := commonEvents.TranscodeFinishedEvent{
		TaskID:   taskID,
		FileHash: fileHash,
		UserID:   rec.UserID,
		Status:   "completed",
		URLs:     resultURLs,
	}
	payload, _ := json.Marshal(finishedEvt)
	if err := commonKafka.Publish(ctx, commonEvents.TopicTranscodeFinished, "tc:"+taskID, string(payload)); err != nil {
		log.Printf("[Transcode] 发布 TranscodeFinished 失败: %v", err)
	} else {
		log.Printf("[Transcode] TranscodeFinished 事件已发布 taskID=%s", taskID)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
