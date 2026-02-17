package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger *zap.Logger

func Init() error {
   return InitWithLevel("info")
}

func InitWithLevel(level string) error {
   var zapLevel zapcore.Level
   err := zapLevel.UnmarshalText([]byte(level))
   if err != nil {
      zapLevel = zapcore.InfoLevel
   }

   encoderConfig := zapcore.EncoderConfig{
      TimeKey:        "time",
      LevelKey:       "level",
      NameKey:        "logger",
      CallerKey:      "caller",
      MessageKey:     "msg",
      StacktraceKey:  "stacktrace",
      LineEnding:     zapcore.DefaultLineEnding,
      EncodeLevel:    zapcore.CapitalLevelEncoder,
      EncodeTime:     zapcore.ISO8601TimeEncoder,
      EncodeDuration: zapcore.StringDurationEncoder,
      EncodeCaller:   zapcore.ShortCallerEncoder,
   }

   core := zapcore.NewCore(
      zapcore.NewJSONEncoder(encoderConfig),
      zapcore.AddSync(os.Stdout),
      zapLevel,
   )

   Logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
   Logger.Info("✅ Logger initialized", zap.String("level", level))
   return nil
}

func GetLogger() *zap.Logger {
   if Logger == nil {
      Init()
   }
   return Logger
}

func Info(msg string, fields ...zap.Field) {
   GetLogger().Info(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
   GetLogger().Error(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
   GetLogger().Warn(msg, fields...)
}

func Debug(msg string, fields ...zap.Field) {
   GetLogger().Debug(msg, fields...)
}
