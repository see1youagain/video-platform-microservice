package db

import (
	"fmt"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// File 文件元数据模型
type File struct {
	ID        uint      `gorm:"primaryKey"`
	FileHash  string    `gorm:"uniqueIndex;size:64;not null" json:"file_hash"`
	Filename  string    `gorm:"size:255;not null" json:"filename"`
	FileSize  int64     `gorm:"not null" json:"file_size"`
	URL       string    `gorm:"size:512;not null" json:"url"`
	Status    string    `gorm:"size:20;default:'uploading'" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// InitDB 初始化数据库连接
func InitDB() error {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		user, password, host, port, dbName)

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// 自动迁移
	if err := DB.AutoMigrate(&File{}); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	fmt.Println("✅ 数据库连接成功")
	return nil
}

// CreateFile 创建文件记录
func CreateFile(fileHash, filename string, fileSize int64, url string) error {
	file := &File{
		FileHash: fileHash,
		Filename: filename,
		FileSize: fileSize,
		URL:      url,
		Status:   "uploading",
	}
	return DB.Create(file).Error
}

// GetFileByHash 根据 Hash 获取文件记录
func GetFileByHash(fileHash string) (*File, error) {
	var file File
	err := DB.Where("file_hash = ?", fileHash).First(&file).Error
	if err != nil {
		return nil, err
	}
	return &file, nil
}

// UpdateFileStatus 更新文件状态
func UpdateFileStatus(fileHash string, status string, url string) error {
	return DB.Model(&File{}).
		Where("file_hash = ?", fileHash).
		Updates(map[string]interface{}{
			"status": status,
			"url":    url,
		}).Error
}

// FileExistsByHash 检查文件是否已存在
func FileExistsByHash(fileHash string) (bool, string, error) {
	var file File
	err := DB.Where("file_hash = ? AND status = ?", fileHash, "finished").First(&file).Error
	if err == gorm.ErrRecordNotFound {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, file.URL, nil
}