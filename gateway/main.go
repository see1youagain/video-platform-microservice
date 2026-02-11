package main

import (
	"log"
	"video-platform-microservice/gateway/internal/logger"
	"video-platform-microservice/gateway/internal/utils"
	"video-platform-microservice/gateway/rpc"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

func main() {
	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Println("警告: 未找到 .env 文件")
	}

	// 初始化日志系统
	if err := logger.InitLogger(); err != nil {
		log.Fatalf("日志系统初始化失败: %v", err)
	}
	defer logger.Sync()

	// 初始化 JWT
	if err := utils.InitJWT(); err != nil {
		logger.Logger.Fatal("JWT 初始化失败", zap.Error(err))
	}

	// 初始化 RPC 客户端
	rpc.InitRPC()

	h := server.Default(server.WithHostPorts(":8080"))

	register(h)

	logger.Logger.Info("Gateway 服务启动成功", zap.String("port", "8080"))
	h.Spin()
}
