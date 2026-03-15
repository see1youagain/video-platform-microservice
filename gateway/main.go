package main

import (
	"log"
	"time"

	"video-platform-microservice/gateway/internal/logger"
	"video-platform-microservice/gateway/rpc"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/joho/godotenv"
	commonlogger "github.com/see1youagain/video-platform-microservice/common/logger"
	commonRedis "github.com/see1youagain/video-platform-microservice/common/redis"
	commonUtils "github.com/see1youagain/video-platform-microservice/common/utils"
	"go.uber.org/zap"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("警告: 未找到 .env 文件")
	}

	commonlogger.Init()

	if err := logger.InitLogger(); err != nil {
		commonlogger.Logger.Fatal("Gateway Logger 初始化失败", zap.Error(err))
	}

	if err := commonRedis.InitRedis(); err != nil {
		commonlogger.Logger.Fatal("Redis 初始化失败", zap.Error(err))
	}

	if err := commonUtils.InitJWTWithRotation(commonRedis.GetClient(), 12*time.Hour); err != nil {
		commonlogger.Logger.Fatal("JWT 密钥轮换初始化失败", zap.Error(err))
	}

	rpc.InitRPC()

	maxBody := getenvInt("MAX_REQUEST_BODY_SIZE", 50*1024*1024) // 默认 50MB
	h := server.Default(
		server.WithHostPorts(":8080"),
		server.WithMaxRequestBodySize(maxBody),
	)
	register(h)

	commonlogger.Logger.Info("✅ Gateway 服务启动成功",
		zap.String("port", "8080"),
		zap.Int("max_request_body_size", maxBody),
	)
	logger.Logger.Info("✅ Gateway 内部日志系统已初始化")
	h.Spin()
}
