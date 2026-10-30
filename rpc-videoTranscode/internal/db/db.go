package db

import (
	commonDb "github.com/see1youagain/video-platform-microservice/common/db"
	"time"
)

type Job struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement"`
	TaskID      string     `gorm:"size:64;not null;uniqueIndex"`
	FileHash    string     `gorm:"size:64;not null;index"`
	UserID      string     `gorm:"size:64;not null;index"`
	Status      string     `gorm:"size:20;not null;default:'pending';index:idx_job_queue,priority:1"`
	Progress    int32      `gorm:"not null;default:0"`
	ResultURL   string     `gorm:"type:text"`
	Resolutions string     `gorm:"type:text"`
	ErrorMsg    string     `gorm:"type:text"`
	LeaseOwner  string     `gorm:"size:64"`
	LeaseUntil  *time.Time `gorm:"index:idx_job_queue,priority:2"`
	Attempts    int        `gorm:"not null;default:0"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Job) TableName() string { return "transcode_jobs" }

func Init() error {
	return commonDb.GetDB().AutoMigrate(&Job{})
}
