package db

import (
	commonDb "github.com/see1youagain/video-platform-microservice/common/db"
)

type Job struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	TaskID    string `gorm:"size:64;not null;uniqueIndex"`
	FileHash  string `gorm:"size:64;not null;index"`
	UserID    string `gorm:"size:64;not null;index"`
	Status    string `gorm:"size:20;not null;default:'pending'"`
	Progress  int32  `gorm:"not null;default:0"`
	ResultURL string `gorm:"type:text"`
}

func (Job) TableName() string { return "transcode_jobs" }

func Init() error {
	return commonDb.GetDB().AutoMigrate(&Job{})
}
