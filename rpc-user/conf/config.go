package conf

import (
	"fmt"
	"time"
	"video-platform-microservice/rpc-user/internal/db"

	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// LoadConfig 加载环境变量配置
func LoadConfig() error {
	// 加载 .env 文件
	if err := godotenv.Load(); err != nil {
		return fmt.Errorf("failed to load .env file: %w", err)
	}
	return nil
}

func InitDB(dsn string) error {
    var err error
    db.DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Info),
    })
    if err != nil {
        return fmt.Errorf("failed to connect database: %w", err)
    }
    sqlDB, err := db.DB.DB()
    if err != nil {
        return fmt.Errorf("failed to get db instance: %w", err)
    }
    sqlDB.SetMaxIdleConns(10)
    sqlDB.SetMaxOpenConns(100)
    sqlDB.SetConnMaxLifetime(time.Hour)

    // AutoMigrate：注意顺序，先 Content，再 FileMeta，再 UserContent
    if err := db.DB.AutoMigrate(&db.User{}); err != nil {
        return fmt.Errorf("failed to migrate database: %w", err)
    }
    return nil
}

func GetDB() *gorm.DB {
    return db.DB
}