package transcode

import (
"encoding/json"
"fmt"
"log"
"os"
"os/exec"
"path/filepath"
"strings"
"sync"

"github.com/google/uuid"
"video-platform-microservice/rpc-video/internal/db"
"video-platform-microservice/rpc-video/internal/storage"
)

// ResolutionConfig 分辨率配置
type ResolutionConfig struct {
Name   string
Width  int
Height int
Bitrate string
}

var resolutions = map[string]ResolutionConfig{
"1080p": {Name: "1080p", Width: 1920, Height: 1080, Bitrate: "5000k"},
"720p":  {Name: "720p", Width: 1280, Height: 720, Bitrate: "2500k"},
"480p":  {Name: "480p", Width: 854, Height: 480, Bitrate: "1000k"},
"360p":  {Name: "360p", Width: 640, Height: 360, Bitrate: "500k"},
}

// TaskStatus 任务状态结构
type TaskStatus struct {
TaskID        string   `json:"task_id"`
Status        string   `json:"status"`
Progress      int32    `json:"progress"`
CompletedURLs []string `json:"completed_urls"`
Error         string   `json:"error,omitempty"`
}

// Manager 转码管理器
type Manager struct {
mu     sync.RWMutex
tasks  map[string]*TaskStatus
queue  chan string // 任务队列
workers int
}

var manager *Manager

// InitTranscodeManager 初始化转码管理器
func InitTranscodeManager(workers int) {
if workers <= 0 {
workers = 2 // 默认2个工作协程
}

manager = &Manager{
tasks:   make(map[string]*TaskStatus),
queue:   make(chan string, 100),
workers: workers,
}

// 启动工作协程
for i := 0; i < workers; i++ {
go manager.worker()
}

log.Printf("✅ 转码管理器已启动 (workers: %d)", workers)
}

// CreateTask 创建转码任务
func CreateTask(fileHash, userID string, resolutionList []string) (string, error) {
taskID := uuid.New().String()

// 验证分辨率
for _, res := range resolutionList {
if _, ok := resolutions[res]; !ok {
return "", fmt.Errorf("不支持的分辨率: %s", res)
}
}

// 保存到数据库
resolutionsJSON, _ := json.Marshal(resolutionList)
if err := db.CreateTranscodeTask(taskID, fileHash, userID, string(resolutionsJSON), ""); err != nil {
return "", err
}

// 添加到内存状态
manager.mu.Lock()
manager.tasks[taskID] = &TaskStatus{
TaskID:        taskID,
Status:        "pending",
Progress:      0,
CompletedURLs: []string{},
}
manager.mu.Unlock()

// 加入队列
manager.queue <- taskID

log.Printf("转码任务已创建: %s (file: %s, resolutions: %v)", taskID, fileHash, resolutionList)
return taskID, nil
}

// GetTaskStatus 获取任务状态
func GetTaskStatus(taskID string) (*TaskStatus, error) {
manager.mu.RLock()
status, ok := manager.tasks[taskID]
manager.mu.RUnlock()

if ok {
return status, nil
}

// 从数据库加载
task, err := db.GetTranscodeTask(taskID)
if err != nil {
return nil, err
}
if task == nil {
return nil, fmt.Errorf("任务不存在: %s", taskID)
}

var urls []string
if task.ResultURLs != "" {
json.Unmarshal([]byte(task.ResultURLs), &urls)
}

status = &TaskStatus{
TaskID:        task.TaskID,
Status:        task.Status,
Progress:      task.Progress,
CompletedURLs: urls,
}

return status, nil
}

// worker 工作协程
func (m *Manager) worker() {
for taskID := range m.queue {
log.Printf("开始处理转码任务: %s", taskID)

// 更新状态为 processing
m.updateStatus(taskID, "processing", 0, nil)

// 执行转码
if err := m.processTask(taskID); err != nil {
log.Printf("❌ 转码失败: %s, error: %v", taskID, err)
m.updateStatus(taskID, "failed", 0, nil)

// 更新数据库
db.UpdateTranscodeTaskProgress(taskID, "failed", 0, "")
} else {
log.Printf("✅ 转码完成: %s", taskID)
}
}
}

// processTask 处理单个转码任务
func (m *Manager) processTask(taskID string) error {
// 从数据库获取任务信息
task, err := db.GetTranscodeTask(taskID)
if err != nil {
return err
}

// 获取原文件
file, err := db.GetFileByHashAndUser(task.FileHash, task.UserID)
if err != nil {
return err
}
if file == nil {
return fmt.Errorf("文件不存在: %s", task.FileHash)
}

// 解析分辨率列表
var resolutionList []string
if err := json.Unmarshal([]byte(task.Resolutions), &resolutionList); err != nil {
return err
}

// 获取源文件路径
sourcePath := storage.GetFilePath(task.FileHash, file.Filename)
if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
return fmt.Errorf("源文件不存在: %s", sourcePath)
}

// 逐个转码
completedURLs := []string{}
total := len(resolutionList)

for i, resName := range resolutionList {
log.Printf("转码 %s (%d/%d): %s", taskID, i+1, total, resName)

outputURL, err := transcodeVideo(sourcePath, task.FileHash, resName)
if err != nil {
log.Printf("❌ 转码失败 (%s): %v", resName, err)
continue
}

completedURLs = append(completedURLs, outputURL)

// 更新进度
progress := int32((i + 1) * 100 / total)
m.updateStatus(taskID, "processing", progress, completedURLs)

// 更新数据库
urlsJSON, _ := json.Marshal(completedURLs)
db.UpdateTranscodeTaskProgress(taskID, "processing", progress, string(urlsJSON))
}

// 标记完成
status := "completed"
if len(completedURLs) == 0 {
status = "failed"
}

m.updateStatus(taskID, status, 100, completedURLs)

// 更新数据库
urlsJSON, _ := json.Marshal(completedURLs)
db.UpdateTranscodeTaskProgress(taskID, status, 100, string(urlsJSON))

// 更新文件的转码状态
db.UpdateFileTranscodeStatus(task.FileHash, task.UserID, status, string(urlsJSON))

return nil
}

// transcodeVideo 执行单个视频转码
func transcodeVideo(sourcePath, fileHash, resolutionName string) (string, error) {
config, ok := resolutions[resolutionName]
if !ok {
return "", fmt.Errorf("不支持的分辨率: %s", resolutionName)
}

// 输出文件路径
ext := filepath.Ext(sourcePath)
outputFilename := fmt.Sprintf("%s_%s%s", fileHash, resolutionName, ext)
outputPath := filepath.Join(storage.StoragePath, "files", outputFilename)

// 检查ffmpeg是否存在
if _, err := exec.LookPath("ffmpeg"); err != nil {
return "", fmt.Errorf("ffmpeg 未安装或不在 PATH 中")
}

// 构建ffmpeg命令
args := []string{
"-i", sourcePath,
"-vf", fmt.Sprintf("scale=%d:%d", config.Width, config.Height),
"-b:v", config.Bitrate,
"-c:v", "libx264",
"-c:a", "aac",
"-y", // 覆盖输出文件
outputPath,
}

// 执行转码
cmd := exec.Command("ffmpeg", args...)
output, err := cmd.CombinedOutput()
if err != nil {
return "", fmt.Errorf("ffmpeg 执行失败: %v, output: %s", err, string(output))
}

// 生成URL
url := fmt.Sprintf("/files/%s", outputFilename)
return url, nil
}

// updateStatus 更新任务状态（内存）
func (m *Manager) updateStatus(taskID, status string, progress int32, urls []string) {
m.mu.Lock()
defer m.mu.Unlock()

if task, ok := m.tasks[taskID]; ok {
task.Status = status
task.Progress = progress
if urls != nil {
task.CompletedURLs = urls
}
}
}

// ExtractResolution 提取视频分辨率
func ExtractResolution(filePath string) (int32, int32, error) {
// 使用ffprobe获取视频信息
if _, err := exec.LookPath("ffprobe"); err != nil {
log.Println("⚠️ ffprobe 未安装，无法提取分辨率")
return 0, 0, nil
}

cmd := exec.Command("ffprobe",
"-v", "error",
"-select_streams", "v:0",
"-show_entries", "stream=width,height",
"-of", "csv=p=0",
filePath,
)

output, err := cmd.Output()
if err != nil {
return 0, 0, fmt.Errorf("ffprobe 执行失败: %v", err)
}

// 解析输出 (格式: width,height)
parts := strings.Split(strings.TrimSpace(string(output)), ",")
if len(parts) != 2 {
return 0, 0, fmt.Errorf("无法解析分辨率: %s", output)
}

var width, height int32
fmt.Sscanf(parts[0], "%d", &width)
fmt.Sscanf(parts[1], "%d", &height)

return width, height, nil
}
