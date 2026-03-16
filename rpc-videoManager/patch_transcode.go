package main

import (
"io/ioutil"
"strings"
)

func main() {
file := "/home/lzzy/project/go_project/video-platform-microservice/rpc-videoTranscode/handler.go"
content, _ := ioutil.ReadFile(file)
str := string(content)

// Add missing imports
if !strings.Contains(str, `"os/exec"`) {
str = strings.Replace(str, `"os"`, "\"os\"\n\t\"os/exec\"\n\t\"path/filepath\"\n\t\"runtime\"", 1)
}

consumerOld := `db.GetDB().Create(&transcodeDb.Job{
TaskID:    taskID,
FileHash:  event.FileHash,
UserID:    event.UserID,
Status:    "pending",
Progress:  0,
ResultURL: "[]",
}).Error; createErr != nil {
log.Printf("[videoTranscode] 创建任务失败 taskID=%s err=%v", taskID, createErr)
continue
}

go processTranscode(ctx, taskID, event.FileHash, event.UserID, event.Resolutions)
}
}`

consumerNew := `db.GetDB().Create(&transcodeDb.Job{
TaskID:    taskID,
FileHash:  event.FileHash,
UserID:    event.UserID,
Status:    "pending",
Progress:  0,
ResultURL: "[]",
}).Error; createErr != nil {
log.Printf("[videoTranscode] 创建任务失败 taskID=%s err=%v", taskID, createErr)
continue
}

sem <- struct{}{}
go func(t, fh, u string, r []string) {
defer func() { <-sem }()
processTranscode(ctx, t, fh, u, r)
}(taskID, event.FileHash, event.UserID, event.Resolutions)
}
}`

str = strings.Replace(str, `defer reader.Close()

log.Printf("[videoTranscode] 开始消费 topic=%s", commonEvents.TopicTranscodeTasks)`, `defer reader.Close()

sem := make(chan struct{}, runtime.NumCPU()*2)
log.Printf("[videoTranscode] 开始消费 topic=%s, 最大并发: %d", commonEvents.TopicTranscodeTasks, runtime.NumCPU()*2)`, 1)

str = strings.Replace(str, consumerOld, consumerNew, 1)

processOld := `resultURLs := make([]string, 0, len(resolutions))
for i, res := range resolutions {
time.Sleep(500 * time.Millisecond)

objectName := fmt.Sprintf("videos/%s/%s.mp4", fileHash, res)
dummyData := []byte(fmt.Sprintf("transcoded/%s/%s", fileHash, res))
url, err := commonMinio.UploadStream(ctx, objectName, bytes.NewReader(dummyData), int64(len(dummyData)), "video/mp4")
if err != nil {
url = fmt.Sprintf("/files/video/%s/%s.mp4", fileHash, res)
}
resultURLs = append(resultURLs, url)

progress := int32((i + 1) * 100 / len(resolutions))
urlsJSON, _ := json.Marshal(resultURLs)
_ = commondb.GetDB().Model(&transcodeDb.Job{}).Where("task_id = ?", taskID).Updates(map[string]interface{}{
"status":     "processing",
"progress":   progress,
"result_url": string(urlsJSON),
})
}`

processNew := `tmpDir, err := os.MkdirTemp("", "transcode-*" + fileHash)
if err != nil {
log.Printf("[Transcode] 创建临时目录失败: %v", err)
return
}
defer os.RemoveAll(tmpDir)

inputFile := filepath.Join(tmpDir, "input.mp4")
if err := commonMinio.DownloadFile(ctx, "videos/raw/"+fileHash+".mp4", inputFile); err != nil {
log.Printf("[Transcode] 下载源文件失败: %v", err)
_ = commondb.GetDB().Model(&transcodeDb.Job{}).Where("task_id = ?", taskID).Updates(map[string]interface{}{
"status": "failed",
})
return
}

resultURLs := make([]string, 0, len(resolutions))
for i, res := range resolutions {
scale := "-2:720"
if res == "360p" {
scale = "-2:360"
} else if res == "480p" {
scale = "-2:480"
} else if res == "720p" {
scale = "-2:720"
} else if res == "1080p" {
scale = "-2:1080"
}

outputFile := filepath.Join(tmpDir, fmt.Sprintf("out_%s.mp4", res))
cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", inputFile, "-vf", "scale="+scale, "-c:v", "libx264", "-c:a", "aac", outputFile)
if out, err := cmd.CombinedOutput(); err != nil {
log.Printf("[Transcode] ffmpeg 执行失败 res=%s err=%v out=%s", res, err, string(out))
continue
}

objectName := fmt.Sprintf("videos/%s/%s.mp4", fileHash, res)
url, err := commonMinio.UploadFile(ctx, objectName, outputFile, "video/mp4")
if err != nil {
url = fmt.Sprintf("/files/video/%s/%s.mp4", fileHash, res)
}
resultURLs = append(resultURLs, url)

progress := int32((i + 1) * 100 / len(resolutions))
urlsJSON, _ := json.Marshal(resultURLs)
_ = commondb.GetDB().Model(&transcodeDb.Job{}).Where("task_id = ?", taskID).Updates(map[string]interface{}{
"status":     "processing",
"progress":   progress,
"result_url": string(urlsJSON),
})
}`

str = strings.Replace(str, processOld, processNew, 1)

ioutil.WriteFile(file, []byte(str), 0644)
}
