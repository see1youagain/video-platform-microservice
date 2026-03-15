// Package events 定义服务间 Kafka 消息的结构和 Topic 常量。
package events

const (
// videoUpload -> video / videoTranscode
TopicFileUploaded = "video.file.uploaded"
// video -> videoTranscode
TopicTranscodeTasks = "video.transcode.tasks"
// videoTranscode -> video
TopicTranscodeFinished = "video.transcode.finished"
)

// FileUploadedEvent 原片上传完成事件（videoUpload 发布）
type FileUploadedEvent struct {
FileHash    string   `json:"file_hash"`
Filename    string   `json:"filename"`
UserID      string   `json:"user_id"`
MinioURL    string   `json:"minio_url"`
FileSize    int64    `json:"file_size"`
Width       int32    `json:"width,omitempty"`
Height      int32    `json:"height,omitempty"`
Resolutions []string `json:"resolutions,omitempty"`
RequestID   string   `json:"request_id,omitempty"`
}

// TranscodeTaskEvent 转码任务事件（video 发布，videoTranscode 消费）
type TranscodeTaskEvent struct {
TaskID      string   `json:"task_id"`
FileHash    string   `json:"file_hash"`
UserID      string   `json:"user_id"`
Resolutions []string `json:"resolutions"`
}

// TranscodeFinishedEvent 转码完成事件（videoTranscode 发布，video 消费）
type TranscodeFinishedEvent struct {
TaskID   string   `json:"task_id"`
FileHash string   `json:"file_hash"`
UserID   string   `json:"user_id"`
Status   string   `json:"status"`
URLs     []string `json:"urls"`
ErrorMsg string   `json:"error_msg,omitempty"`
}
