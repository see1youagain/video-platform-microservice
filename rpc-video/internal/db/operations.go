package db

import (
	"fmt"
	"log"

	commonDb "github.com/see1youagain/video-platform-microservice/common/db"
	"gorm.io/gorm"
)

// GetDB returns the database instance
func GetDB() *gorm.DB {
	return commonDb.GetDB()
}

// File represents a file record in database
type File struct {
	ID       uint   `gorm:"primaryKey"`
	FileHash string `gorm:"uniqueIndex;size:64;not null"`
	Filename string `gorm:"size:255;not null"`
	FileSize int64  `gorm:"not null"`
	URL      string `gorm:"size:512;not null"`
	Status   string `gorm:"size:20;default:'uploading'"`
}

// TableName specifies the table name for File model
func (File) TableName() string {
	return "video_files"
}

// Init initializes database tables
func Init() error {
	if err := GetDB().AutoMigrate(&File{}); err != nil {
		return fmt.Errorf("failed to auto migrate File: %w", err)
	}
	log.Println("✅ Database tables initialized")
	return nil
}

// FileExistsByHash checks if a file exists and returns its URL
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

// CreateFile creates a new file record
func CreateFile(fileHash, filename string, fileSize int64, url string) error {
	file := &File{
		FileHash: fileHash,
		Filename: filename,
		FileSize: fileSize,
		URL:      url,
		Status:   "uploading",
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
