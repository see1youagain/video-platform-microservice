#!/bin/bash

set -e

echo "🚀 Creating common library files..."

# Config module
cat > common/config/config.go << 'EOF'
package config

import (
redis.go "fmt"
redis.go "os"

redis.go "github.com/joho/godotenv"
)

type Config struct {
redis.go DBHost        string
redis.go DBPort        string
redis.go DBUser        string
redis.go DBPassword    string
redis.go DBName        string
redis.go RedisHost     string
redis.go RedisPort     string
redis.go RedisPassword string
redis.go RedisDB       string
redis.go RPCPort       string
redis.go HTTPPort      string
redis.go EtcdEndpoints string
redis.go ServiceName   string
redis.go StoragePath   string
redis.go ChunkSize     string
redis.go JWTSecret     string
redis.go Environment   string
}

func Load() (*Config, error) {
redis.go godotenv.Load()
redis.go return &Config{
redis.go redis.go DBHost:        getEnv("DB_HOST", "127.0.0.1"),
redis.go redis.go DBPort:        getEnv("DB_PORT", "3306"),
redis.go redis.go DBUser:        getEnv("DB_USER", "root"),
redis.go redis.go DBPassword:    getEnv("DB_PASSWORD", ""),
redis.go redis.go DBName:        getEnv("DB_NAME", "video_platform"),
redis.go redis.go RedisHost:     getEnv("REDIS_HOST", "127.0.0.1"),
redis.go redis.go RedisPort:     getEnv("REDIS_PORT", "6379"),
redis.go redis.go RedisPassword: getEnv("REDIS_PASSWORD", ""),
redis.go redis.go RedisDB:       getEnv("REDIS_DB", "0"),
redis.go redis.go RPCPort:       getEnv("RPC_PORT", "8888"),
redis.go redis.go HTTPPort:      getEnv("HTTP_PORT", "8080"),
redis.go redis.go EtcdEndpoints: getEnv("ETCD_ENDPOINTS", "127.0.0.1:2379"),
redis.go redis.go ServiceName:   getEnv("SERVICE_NAME", "unknown"),
redis.go redis.go StoragePath:   getEnv("STORAGE_PATH", "/tmp/video-platform"),
redis.go redis.go ChunkSize:     getEnv("CHUNK_SIZE", "2097152"),
redis.go redis.go JWTSecret:     getEnv("JWT_SECRET", "your-secret-key"),
redis.go redis.go Environment:   getEnv("ENVIRONMENT", "dev"),
redis.go }, nil
}

func getEnv(key, defaultValue string) string {
redis.go if value := os.Getenv(key); value != "" {
redis.go redis.go return value
redis.go }
redis.go return defaultValue
}

func (c *Config) IsProd() bool {
redis.go return c.Environment == "prod"
}
EOF

# Logger module
cat > common/logger/logger.go << 'EOF'
package logger

import (
redis.go "os"

redis.go "go.uber.org/zap"
redis.go "go.uber.org/zap/zapcore"
)

var Logger *zap.Logger

func Init() error {
redis.go return InitWithLevel("info")
}

func InitWithLevel(level string) error {
redis.go var zapLevel zapcore.Level
redis.go err := zapLevel.UnmarshalText([]byte(level))
redis.go if err != nil {
redis.go redis.go zapLevel = zapcore.InfoLevel
redis.go }

redis.go encoderConfig := zapcore.EncoderConfig{
redis.go redis.go TimeKey:        "time",
redis.go redis.go LevelKey:       "level",
redis.go redis.go NameKey:        "logger",
redis.go redis.go CallerKey:      "caller",
redis.go redis.go MessageKey:     "msg",
redis.go redis.go StacktraceKey:  "stacktrace",
redis.go redis.go LineEnding:     zapcore.DefaultLineEnding,
redis.go redis.go EncodeLevel:    zapcore.CapitalLevelEncoder,
redis.go redis.go EncodeTime:     zapcore.ISO8601TimeEncoder,
redis.go redis.go EncodeDuration: zapcore.StringDurationEncoder,
redis.go redis.go EncodeCaller:   zapcore.ShortCallerEncoder,
redis.go }

redis.go core := zapcore.NewCore(
redis.go redis.go zapcore.NewJSONEncoder(encoderConfig),
redis.go redis.go zapcore.AddSync(os.Stdout),
redis.go redis.go zapLevel,
redis.go )

redis.go Logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
redis.go Logger.Info("✅ Logger initialized", zap.String("level", level))
redis.go return nil
}

func GetLogger() *zap.Logger {
redis.go if Logger == nil {
redis.go redis.go Init()
redis.go }
redis.go return Logger
}

func Info(msg string, fields ...zap.Field) {
redis.go GetLogger().Info(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
redis.go GetLogger().Error(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
redis.go GetLogger().Warn(msg, fields...)
}

func Debug(msg string, fields ...zap.Field) {
redis.go GetLogger().Debug(msg, fields...)
}
EOF

# Validator module
cat > common/validator/validator.go << 'EOF'
package validator

import (
redis.go "fmt"
redis.go "regexp"
redis.go "unicode/utf8"
)

func ValidateUsername(username string) error {
redis.go if username == "" {
redis.go redis.go return fmt.Errorf("用户名不能为空")
redis.go }
redis.go length := utf8.RuneCountInString(username)
redis.go if length < 3 || length > 20 {
redis.go redis.go return fmt.Errorf("用户名长度必须在 3-20 个字符之间")
redis.go }
redis.go matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]+$`, username)
redis.go if !matched {
redis.go redis.go return fmt.Errorf("用户名只能包含字母、数字和下划线")
redis.go }
redis.go return nil
}

func ValidatePassword(password string) error {
redis.go if password == "" {
redis.go redis.go return fmt.Errorf("密码不能为空")
redis.go }
redis.go length := len(password)
redis.go if length < 6 || length > 32 {
redis.go redis.go return fmt.Errorf("密码长度必须在 6-32 个字符之间")
redis.go }
redis.go return nil
}

func ValidateFileHash(hash string) error {
redis.go if hash == "" {
redis.go redis.go return fmt.Errorf("文件哈希不能为空")
redis.go }
redis.go length := len(hash)
redis.go if length != 32 && length != 64 {
redis.go redis.go return fmt.Errorf("文件哈希长度不正确")
redis.go }
redis.go matched, _ := regexp.MatchString(`^[a-fA-F0-9]+$`, hash)
redis.go if !matched {
redis.go redis.go return fmt.Errorf("文件哈希格式不正确")
redis.go }
redis.go return nil
}
EOF

# Utils/JWT module
cat > common/utils/jwt.go << 'EOF'
package utils

import (
redis.go "fmt"
redis.go "time"

redis.go "github.com/golang-jwt/jwt/v5"
redis.go "github.com/google/uuid"
)

type Claims struct {
redis.go UserID   uint   `json:"user_id"`
redis.go Username string `json:"username"`
redis.go jwt.RegisteredClaims
}

func GenerateToken(userID uint, username string, secret string, expireHours int) (string, error) {
redis.go claims := Claims{
redis.go redis.go UserID:   userID,
redis.go redis.go Username: username,
redis.go redis.go RegisteredClaims: jwt.RegisteredClaims{
redis.go redis.go redis.go ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireHours) * time.Hour)),
redis.go redis.go redis.go IssuedAt:  jwt.NewNumericDate(time.Now()),
redis.go redis.go redis.go NotBefore: jwt.NewNumericDate(time.Now()),
redis.go redis.go redis.go Issuer:    "video-platform",
redis.go redis.go redis.go Subject:   username,
redis.go redis.go redis.go ID:        uuid.New().String(),
redis.go redis.go },
redis.go }
redis.go token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
redis.go return token.SignedString([]byte(secret))
}

func ParseToken(tokenString string, secret string) (*Claims, error) {
redis.go token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
redis.go redis.go if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
redis.go redis.go redis.go return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
redis.go redis.go }
redis.go redis.go return []byte(secret), nil
redis.go })
redis.go if err != nil {
redis.go redis.go return nil, err
redis.go }
redis.go if claims, ok := token.Claims.(*Claims); ok && token.Valid {
redis.go redis.go return claims, nil
redis.go }
redis.go return nil, fmt.Errorf("invalid token")
}

func GenerateUUID() string {
redis.go return uuid.New().String()
}
EOF

echo "✅ Common library files created successfully"
