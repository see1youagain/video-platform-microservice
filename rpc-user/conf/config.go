package conf

import (
	"fmt"
	"time"

	userdb "video-platform-microservice/rpc-user/internal/db"

	commondb "github.com/see1youagain/video-platform-microservice/common/db"

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

   // 使用common包初始化数据库连接
   db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
      Logger: logger.Default.LogMode(logger.Info),
   })
   if err != nil {
      return fmt.Errorf("failed to connect database: %w", err)
   }

   // 初始化common包的DB实例
   commondb.SetDB(db)

   sqlDB, err := db.DB()
   if err != nil {
      return fmt.Errorf("failed to get db instance: %w", err)
   }
   sqlDB.SetMaxIdleConns(10)
   sqlDB.SetMaxOpenConns(100)
   sqlDB.SetConnMaxLifetime(time.Hour)

   // AutoMigrate: 使用rpc-user中的User模型
   if err := db.AutoMigrate(&userdb.User{}); err != nil {
      return fmt.Errorf("failed to migrate database: %w", err)
   }
   return nil
}

func GetDB() *gorm.DB {
   return commondb.GetDB()
}
