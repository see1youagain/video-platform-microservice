package main

import (
	"log"
	"video-platform-microservice/gateway/internal/utils"
	"video-platform-microservice/gateway/rpc"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/joho/godotenv"
)

func main() {
	    // 加载环境变量（新增）
    if err := godotenv.Load(); err != nil {
        log.Println("警告: 未找到 .env 文件")
    }
    
    // 初始化 JWT（新增）
    if err := utils.InitJWT(); err != nil {
        log.Fatalf("JWT 初始化失败: %v", err)
    }
	// 初始化 RPC 客户端
	rpc.InitRPC()

	h := server.Default(server.WithHostPorts(":8080"))

	register(h)
	h.Spin()
}
