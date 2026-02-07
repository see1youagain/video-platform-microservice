package main

import (
	"video-platform-microservice/gateway/rpc"

	"github.com/cloudwego/hertz/pkg/app/server"
)

func main() {
	// 初始化 RPC 客户端
	rpc.InitRPC()

	h := server.Default(server.WithHostPorts(":8080"))

	register(h)
	h.Spin()
}
