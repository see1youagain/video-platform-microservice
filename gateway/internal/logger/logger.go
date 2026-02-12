package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger *zap.Logger

// InitLogger 初始化日志系统
func InitLogger() error {
	config := zap.NewProductionConfig()

	// 设置日志级别
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)

	// 设置日志格式
	config.Encoding = "json"

	// 设置时间格式
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// 设置日志输出
	config.OutputPaths = []string{"stdout"}
	config.ErrorOutputPaths = []string{"stderr"}

	var err error
	Logger, err = config.Build()
	if err != nil {
		return err
	}

	Logger.Info("日志系统初始化成功")
	return nil
}

// Sync 刷新日志缓冲区
func Sync() {
	if Logger != nil {
		_ = Logger.Sync()
	}
}
