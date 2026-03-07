package db

import (
"fmt"
"log"
"time"

commonDb "github.com/see1youagain/video-platform-microservice/common/db"
"gorm.io/gorm"
)

func GetDB() *gorm.DB { return commonDb.GetDB() }

// ─── 模型定义 ──────────────────────────────────────────────────────────────

// File 视频文件记录
type File struct {
ID              uint      `gorm:"primaryKey"`
FileHash        string    `gorm:"index;size:64;not null"`
UserID          string    `gorm:"index;size:64;not null;default:'anonymous'"`
Filename        string    `gorm:"size:255;not null"`
FileSize        int64     `gorm:"not null;default:0"`
URL             string    `gorm:"size:512;not null;default:''"`
Status          string    `gorm:"size:20;default:'uploading'"`
TranscodeStatus string    `gorm:"size:20;default:'pending'"`
TranscodeURLs   string    `gorm:"type:text"`
Width           int32     `gorm:"default:0"`
Height          int32     `gorm:"default:0"`
RequestID       string    `gorm:"index;size:64;default:''"`
RefCount        int32     `gorm:"default:1"`
CreatedAt       time.Time `gorm:"autoCreateTime"`
UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}

func (File) TableName() string { return "video_files" }

// TranscodeTask 转码任务
type TranscodeTask struct {
ID          uint      `gorm:"primaryKey"`
TaskID      string    `gorm:"uniqueIndex;size:64;not null"`
FileHash    string    `gorm:"size:64;not null;index"`
UserID      string    `gorm:"size:64;not null;index"`
Resolutions string    `gorm:"type:text"`
Status      string    `gorm:"size:20;default:'pending'"`
Progress    int32     `gorm:"default:0"`
ResultURLs  string    `gorm:"type:text"`
RequestID   string    `gorm:"index;size:64"`
CreatedAt   time.Time `gorm:"autoCreateTime"`
UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

func (TranscodeTask) TableName() string { return "transcode_tasks" }

// ─── 初始化 ────────────────────────────────────────────────────────────────

func Init() error {
if err := GetDB().AutoMigrate(&File{}, &TranscodeTask{}); err != nil {
return fmt.Errorf("failed to auto migrate: %w", err)
}
log.Println("✅ Database tables initialized")
return nil
}

// ─── File 操作 ─────────────────────────────────────────────────────────────

func FileExistsByHash(fileHash string) (bool, string, error) {
var f File
err := GetDB().Where("file_hash = ? AND status = ?", fileHash, "finished").First(&f).Error
if err == gorm.ErrRecordNotFound {
return false, "", nil
}
return err == nil, f.URL, err
}

func FileExistsByHashAndUser(fileHash, userID string) (bool, string, error) {
var f File
err := GetDB().Where("file_hash = ? AND user_id = ? AND status = ?", fileHash, userID, "finished").First(&f).Error
if err == gorm.ErrRecordNotFound {
return false, "", nil
}
return err == nil, f.URL, err
}

func CheckFileByRequestID(requestID string) (*File, error) {
var f File
err := GetDB().Where("request_id = ?", requestID).First(&f).Error
if err == gorm.ErrRecordNotFound {
return nil, nil
}
return &f, err
}

// CreateFile 创建新的文件记录（接受字段参数，向后兼容）
func CreateFile(fileHash, filename string, fileSize int64, url string) error {
return GetDB().Create(&File{
FileHash: fileHash,
Filename: filename,
FileSize: fileSize,
URL:      url,
Status:   "uploading",
}).Error
}

// CreateFileRecord 创建文件记录（接受完整 File 结构体）
func CreateFileRecord(file *File) error {
return GetDB().Create(file).Error
}

func UpdateFileStatus(fileHash, status, url string) error {
return GetDB().Model(&File{}).
Where("file_hash = ?", fileHash).
Updates(map[string]interface{}{"status": status, "url": url}).Error
}

func GetFileByHash(fileHash string) (*File, error) {
var f File
err := GetDB().Where("file_hash = ?", fileHash).First(&f).Error
if err == gorm.ErrRecordNotFound {
return nil, nil
}
return &f, err
}

func GetFileByHashAndUser(fileHash, userID string) (*File, error) {
var f File
err := GetDB().Where("file_hash = ? AND user_id = ?", fileHash, userID).First(&f).Error
if err == gorm.ErrRecordNotFound {
return nil, nil
}
return &f, err
}

func DeleteFile(fileHash string) error {
return GetDB().Where("file_hash = ?", fileHash).Delete(&File{}).Error
}

func GetUploadingFiles() ([]File, error) {
var files []File
err := GetDB().Where("status = ?", "uploading").Find(&files).Error
return files, err
}

// UpdateTranscodeStatus 更新转码状态和结果 URL
func UpdateTranscodeStatus(fileHash, userID, transcodeStatus, transcodeURLsJSON string) error {
return GetDB().Model(&File{}).
Where("file_hash = ? AND user_id = ?", fileHash, userID).
Updates(map[string]interface{}{
"transcode_status": transcodeStatus,
"transcode_urls":   transcodeURLsJSON,
}).Error
}

func UpdateFileTranscodeStatus(fileHash, status, resultURLsJSON string) error {
return GetDB().Model(&File{}).Where("file_hash = ?", fileHash).
Updates(map[string]interface{}{
"transcode_status": status,
"transcode_urls":   resultURLsJSON,
}).Error
}

// FindStaleTranscoding 查找超时的"转码中"记录（用于最终一致性补偿）
func FindStaleTranscoding(threshold time.Time) ([]File, error) {
var files []File
err := GetDB().Where("transcode_status = ? AND updated_at < ?", "transcoding", threshold).Find(&files).Error
return files, err
}

// ─── TranscodeTask 操作 ────────────────────────────────────────────────────

func CreateTranscodeTask(task *TranscodeTask) error {
return GetDB().Create(task).Error
}

func GetTranscodeTask(taskID string) (*TranscodeTask, error) {
var t TranscodeTask
err := GetDB().Where("task_id = ?", taskID).First(&t).Error
if err == gorm.ErrRecordNotFound {
return nil, nil
}
return &t, err
}

func GetTranscodeTaskByHash(fileHash string) (*TranscodeTask, error) {
var t TranscodeTask
err := GetDB().Where("file_hash = ?", fileHash).First(&t).Error
if err == gorm.ErrRecordNotFound {
return nil, nil
}
return &t, err
}

func UpdateTranscodeTaskProgress(taskID string, status string, progress int32, resultURLsJSON string) error {
return GetDB().Model(&TranscodeTask{}).Where("task_id = ?", taskID).
Updates(map[string]interface{}{
"status":      status,
"progress":    progress,
"result_urls": resultURLsJSON,
}).Error
}

// CreateFileWithMetadata 便捷函数：按字段参数创建完整文件记录
func CreateFileWithMetadata(fileHash, userID, filename string, fileSize int64, url string, width, height int32, requestID string) error {
return CreateFileRecord(&File{
FileHash:  fileHash,
UserID:    userID,
Filename:  filename,
FileSize:  fileSize,
URL:       url,
Status:    "finished",
Width:     width,
Height:    height,
RequestID: requestID,
})
}

// CheckTranscodeTaskByRequestID 按 request_id 查找转码任务（幂等性检查）
func CheckTranscodeTaskByRequestID(requestID string) (*TranscodeTask, error) {
var t TranscodeTask
err := GetDB().Where("request_id = ?", requestID).First(&t).Error
if err == gorm.ErrRecordNotFound {
return nil, nil
}
return &t, err
}

