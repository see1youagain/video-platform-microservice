// Package events 定义服务间 Kafka 消息的结构和 Topic 常量。
package events

// ─── Topic 常量 ────────────────────────────────────────────────────────
const (
	// TopicFileUploaded: videoUpload → video、videoTranscode
	TopicFileUploaded = "video.file.uploaded"
	// TopicTranscodeFinished: videoTranscode → video
	TopicTranscodeFinished = "video.transcode.finished"
)

// ─── 事件结构体 ─────────────────────────────────────────────────────────

// FileUploadedEvent 原片上传完成事件（videoUpload 发布）
type FileUploadedEvent struct {
	FileHash    string   `json:"file_hash"`
	Filename    string   `json:"filename"`
	UserID      string   `json:"user_id"`
	MinioURL    string   `json:"minio_url"`    // MinIO 原片地址
	FileSize    int64    `json:"file_size"`
	Width       int32    `json:"width,omitempty"`
	Height      int32    `json:"height,omitempty"`
	Resolutions []string `json:"resolutions,omitempty"` // 期望转码分辨率
	RequestID   string   `json:"request_id,omitempty"`
}

// TranscodeFinishedEvent 转码完成事件（videoTranscode 发布）
type TranscodeFinishedEvent struct {
	TaskID      string   `json:"task_id"`
	FileHash    string   `json:"file_hash"`
	UserID      string   `json:"user_id"`
	Status      string   `json:"status"`       // "completed" | "failed"
	URLs        []string `json:"urls"`         // 各分辨率转码结果 URL
	ErrorMsg    string   `json:"error_msg,omitempty"`
}
