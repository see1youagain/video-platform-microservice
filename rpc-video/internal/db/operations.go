package db

import (
"fmt"
"log"
"time"

commonDb "github.com/see1youagain/video-platform-microservice/common/db"
"gorm.io/gorm"
)

// GetDB returns the database instance
func GetDB() *gorm.DB {
return commonDb.GetDB()
}

// File represents a file record in database
type File struct {
ID              uint      `gorm:"primaryKey"`
FileHash        string    `gorm:"uniqueIndex:idx_user_file;size:64;not null"`
UserID          string    `gorm:"uniqueIndex:idx_user_file;size:64;not null;index"`
Filename        string    `gorm:"size:255;not null"`
FileSize        int64     `gorm:"not null"`
URL             string    `gorm:"size:512;not null"`
Status          string    `gorm:"size:20;default:'uploading'"`
RefCount        int32     `gorm:"default:1"`           // 引用计数，用于处理删除
RequestID       string    `gorm:"index;size:64"`       // 请求ID，用于幂等性
Width           int32     `gorm:"default:0"`           // 视频宽度
Height          int32     `gorm:"default:0"`           // 视频高度
TranscodeStatus string    `gorm:"size:20;default:'pending'"` // 转码状态
TranscodeURLs   string    `gorm:"type:text"`          // JSON格式存储转码URL列表
CreatedAt       time.Time `gorm:"autoCreateTime"`
UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}

// TranscodeTask represents a transcoding task
type TranscodeTask struct {
ID          uint      `gorm:"primaryKey"`
TaskID      string    `gorm:"uniqueIndex;size:64;not null"`
FileHash    string    `gorm:"size:64;not null;index"`
UserID      string    `gorm:"size:64;not null;index"`
Resolutions string    `gorm:"type:text"` // JSON格式
Status      string    `gorm:"size:20;default:'pending'"`
Progress    int32     `gorm:"default:0"`
ResultURLs  string    `gorm:"type:text"` // JSON格式
RequestID   string    `gorm:"index;size:64"` // 用于幂等性
CreatedAt   time.Time `gorm:"autoCreateTime"`
UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

// TableName specifies the table name for File model
func (File) TableName() string {
return "video_files"
}

// TableName specifies the table name for TranscodeTask model
func (TranscodeTask) TableName() string {
return "transcode_tasks"
}

// Init initializes database tables
func Init() error {
if err := GetDB().AutoMigrate(&File{}, &TranscodeTask{}); err != nil {
return fmt.Errorf("failed to auto migrate: %w", err)
}
log.Println("✅ Database tables initialized")
return nil
}

// FileExistsByHashAndUser checks if a file exists for a specific user
func FileExistsByHashAndUser(fileHash string, userID string) (bool, string, error) {
var file File
err := GetDB().Where("file_hash = ? AND user_id = ? AND status = ?", fileHash, userID, "finished").First(&file).Error
if err == gorm.ErrRecordNotFound {
return false, "", nil
}
if err != nil {
return false, "", err
}
return true, file.URL, nil
}

// FileExistsByHash checks if a file exists (any user)
func FileExistsByHash(fileHash string) (bool, string, error) {
var file File
err := GetDB().Where("file_hash = ? AND status = ?", fileHash, "finished").First(&file).Error
if err == gorm.ErrRecordNotFound {
return false, "", nil
}
if err != nil {
return false, "", err
}
return true, file.URL, nil
}

// CheckFileByRequestID checks if a file with the same request ID already exists (for idempotency)
func CheckFileByRequestID(requestID string) (*File, error) {
if requestID == "" {
return nil, nil
}

var file File
err := GetDB().Where("request_id = ?", requestID).First(&file).Error
if err == gorm.ErrRecordNotFound {
return nil, nil
}
if err != nil {
return nil, err
}
return &file, nil
}

// CreateFile creates a new file record
func CreateFile(fileHash, userID, filename string, fileSize int64, url string) error {
file := &File{
FileHash: fileHash,
UserID:   userID,
Filename: filename,
FileSize: fileSize,
URL:      url,
Status:   "uploading",
RefCount: 1,
}
return GetDB().Create(file).Error
}

// CreateFileWithMetadata creates a file record with video metadata and request ID for idempotency
func CreateFileWithMetadata(fileHash, userID, filename string, fileSize int64, url string, width, height int32, requestID string) error {
file := &File{
FileHash:  fileHash,
UserID:    userID,
Filename:  filename,
FileSize:  fileSize,
URL:       url,
Status:    "finished",
Width:     width,
Height:    height,
RefCount:  1,
RequestID: requestID,
}
return GetDB().Create(file).Error
}

// UpdateFileStatus updates file status and URL
func UpdateFileStatus(fileHash string, status string, url string) error {
return GetDB().Model(&File{}).
Where("file_hash = ?", fileHash).
Updates(map[string]interface{}{
"status": status,
"url":    url,
}).Error
}

// UpdateFileMetadata updates file metadata (width, height, etc.)
func UpdateFileMetadata(fileHash, userID string, width, height int32) error {
return GetDB().Model(&File{}).
Where("file_hash = ? AND user_id = ?", fileHash, userID).
Updates(map[string]interface{}{
"width":  width,
"height": height,
}).Error
}

// IncrementRefCount increments the reference count of a file
func IncrementRefCount(fileHash, userID string) error {
return GetDB().Model(&File{}).
Where("file_hash = ? AND user_id = ?", fileHash, userID).
UpdateColumn("ref_count", gorm.Expr("ref_count + ?", 1)).Error
}

// DecrementRefCount decrements the reference count and deletes if reaches 0
func DecrementRefCount(fileHash, userID string) error {
tx := GetDB().Begin()
defer func() {
if r := recover(); r != nil {
tx.Rollback()
}
}()

// Decrement ref count
result := tx.Model(&File{}).
Where("file_hash = ? AND user_id = ?", fileHash, userID).
UpdateColumn("ref_count", gorm.Expr("ref_count - ?", 1))

if result.Error != nil {
tx.Rollback()
return result.Error
}

// Check if ref count is 0
var file File
if err := tx.Where("file_hash = ? AND user_id = ? AND ref_count <= 0", fileHash, userID).First(&file).Error; err == nil {
// Delete the file record
if err := tx.Delete(&file).Error; err != nil {
tx.Rollback()
return err
}
// TODO: Also delete physical file from storage
log.Printf("File deleted due to zero ref count: %s (user: %s)", fileHash, userID)
}

return tx.Commit().Error
}

// GetFileByHashAndUser retrieves a file record by hash and user
func GetFileByHashAndUser(fileHash, userID string) (*File, error) {
var file File
err := GetDB().Where("file_hash = ? AND user_id = ?", fileHash, userID).First(&file).Error
if err == gorm.ErrRecordNotFound {
return nil, nil
}
if err != nil {
return nil, err
}
return &file, nil
}

// GetFileByHash retrieves a file record by hash
func GetFileByHash(fileHash string) (*File, error) {
var file File
err := GetDB().Where("file_hash = ?", fileHash).First(&file).Error
if err == gorm.ErrRecordNotFound {
return nil, nil
}
if err != nil {
return nil, err
}
return &file, nil
}

// CreateTranscodeTask creates a new transcode task with request ID for idempotency
func CreateTranscodeTask(taskID, fileHash, userID, resolutions, requestID string) error {
task := &TranscodeTask{
TaskID:      taskID,
FileHash:    fileHash,
UserID:      userID,
Resolutions: resolutions,
Status:      "pending",
Progress:    0,
RequestID:   requestID,
}
return GetDB().Create(task).Error
}

// CheckTranscodeTaskByRequestID checks if a task with the same request ID already exists
func CheckTranscodeTaskByRequestID(requestID string) (*TranscodeTask, error) {
if requestID == "" {
return nil, nil
}

var task TranscodeTask
err := GetDB().Where("request_id = ?", requestID).First(&task).Error
if err == gorm.ErrRecordNotFound {
return nil, nil
}
if err != nil {
return nil, err
}
return &task, nil
}

// UpdateTranscodeTaskProgress updates task progress and status
func UpdateTranscodeTaskProgress(taskID string, status string, progress int32, resultURLs string) error {
updates := map[string]interface{}{
"status":   status,
"progress": progress,
}
if resultURLs != "" {
updates["result_urls"] = resultURLs
}
return GetDB().Model(&TranscodeTask{}).
Where("task_id = ?", taskID).
Updates(updates).Error
}

// GetTranscodeTask retrieves a transcode task
func GetTranscodeTask(taskID string) (*TranscodeTask, error) {
var task TranscodeTask
err := GetDB().Where("task_id = ?", taskID).First(&task).Error
if err == gorm.ErrRecordNotFound {
return nil, nil
}
if err != nil {
return nil, err
}
return &task, nil
}

// GetPendingTranscodeTasks gets all pending transcode tasks
func GetPendingTranscodeTasks(limit int) ([]TranscodeTask, error) {
var tasks []TranscodeTask
err := GetDB().Where("status = ?", "pending").Limit(limit).Find(&tasks).Error
return tasks, err
}

// DeleteFile soft deletes a file record
func DeleteFile(fileHash string) error {
return GetDB().Where("file_hash = ?", fileHash).Delete(&File{}).Error
}

// GetUploadingFiles gets all files with uploading status
func GetUploadingFiles() ([]File, error) {
var files []File
err := GetDB().Where("status = ?", "uploading").Find(&files).Error
return files, err
}

// UpdateFileTranscodeStatus updates file transcode status
func UpdateFileTranscodeStatus(fileHash, userID, status, transcodeURLs string) error {
updates := map[string]interface{}{
"transcode_status": status,
}
if transcodeURLs != "" {
updates["transcode_urls"] = transcodeURLs
}
return GetDB().Model(&File{}).
Where("file_hash = ? AND user_id = ?", fileHash, userID).
Updates(updates).Error
}
