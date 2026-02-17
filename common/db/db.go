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

type Config struct {
Host     string
Port     string
User     string
Password string
DBName   string
LogLevel logger.LogLevel
}

func InitDB() error {
config := Config{
Host:     getEnv("DB_HOST", "127.0.0.1"),
Port:     getEnv("DB_PORT", "3306"),
User:     getEnv("DB_USER", "root"),
Password: getEnv("DB_PASSWORD", ""),
DBName:   getEnv("DB_NAME", "video_platform"),
LogLevel: logger.Info,
}
return InitDBWithConfig(config)
}

func InitDBWithConfig(config Config) error {
dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
config.User, config.Password, config.Host, config.Port, config.DBName)

var err error
DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
Logger: logger.Default.LogMode(config.LogLevel),
})
if err != nil {
return fmt.Errorf("failed to connect to database: %w", err)
}

sqlDB, err := DB.DB()
if err != nil {
return fmt.Errorf("failed to get database instance: %w", err)
}

sqlDB.SetMaxIdleConns(10)
sqlDB.SetMaxOpenConns(100)
sqlDB.SetConnMaxLifetime(time.Hour)
sqlDB.SetConnMaxIdleTime(10 * time.Minute)

fmt.Println("✅ Database connected successfully")
return nil
}

func GetDB() *gorm.DB {
return DB
}

func Close() error {
if DB != nil {
sqlDB, err := DB.DB()
if err != nil {
return err
}
return sqlDB.Close()
}
return nil
}

func getEnv(key, defaultValue string) string {
if value := os.Getenv(key); value != "" {
return value
}
return defaultValue
}
