package transcode

import (
"context"
"encoding/json"
"fmt"

commonKafka "github.com/see1youagain/video-platform-microservice/common/kafka"
)

// TranscodeTaskEvent 是发布到 Kafka 的转码任务结构
type TranscodeTaskEvent struct {
TaskID      string   `json:"task_id"`
FileHash    string   `json:"file_hash"`
UserID      string   `json:"user_id"`
Resolutions []string `json:"resolutions"`
}

// PublishTaskEvent 将转码任务事件发布到 Kafka topic commonEvents.TopicTranscodeRequested
func PublishTaskEvent(ctx context.Context, taskID, fileHash, userID string, resolutions []string) error {
event := TranscodeTaskEvent{
TaskID:      taskID,
FileHash:    fileHash,
UserID:      userID,
Resolutions: resolutions,
}
payload, err := json.Marshal(event)
if err != nil {
return fmt.Errorf("序列化转码任务事件失败: %w", err)
}
key := "transcode:" + taskID
return commonKafka.Publish(ctx, "transcode-tasks", key, string(payload))
}
