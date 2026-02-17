package main

import (
	"log"
	"video-platform-microservice/gateway/internal/utils"
	"video-platform-microservice/gateway/rpc"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/joho/godotenv"
	commonlogger "github.com/see1youagain/video-platform-microservice/common/logger"
	"go.uber.org/zap"
)

func main() {
	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Println("警告: 未找到 .env 文件")
	}

	// 使用 common 库初始化日志
	commonlogger.Init()

	// 初始化 JWT（使用本地配置）
	if err := utils.InitJWT(); err != nil {
		commonlogger.Logger.Fatal("JWT 初始化失败", zap.Error(err))
	}

	// 初始化 RPC 客户端
	rpc.InitRPC()

	h := server.Default(server.WithHostPorts(":8080"))

	register(h)

	commonlogger.Logger.Info("✅ Gateway 服务启动成功", zap.String("port", "8080"))
	h.Spin()
}