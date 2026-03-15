package db

import (
	commonDb "github.com/see1youagain/video-platform-microservice/common/db"
)

type UploadFile struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	FileHash  string `gorm:"size:64;not null;index:idx_hash_user,priority:1"`
	UserID    string `gorm:"size:64;not null;index:idx_hash_user,priority:2"`
	Filename  string `gorm:"size:255;not null"`
	FileSize  int64  `gorm:"not null;default:0"`
	URL       string `gorm:"size:512;not null"`
	Status    string `gorm:"size:20;not null;default:'finished'"`
	RequestID string `gorm:"size:64;index"`
}

func (UploadFile) TableName() string { return "upload_files" }

func Init() error {
	return commonDb.GetDB().AutoMigrate(&UploadFile{})
}
