package main

import (
"fmt"
"log"
"net"
"strings"

"video-platform-microservice/rpc-video/internal/db"
"video-platform-microservice/rpc-video/internal/storage"
"video-platform-microservice/rpc-video/internal/transcode"
video "video-platform-microservice/rpc-video/kitex_gen/video/videoservice"

"github.com/see1youagain/video-platform-microservice/common/config"
commonDb "github.com/see1youagain/video-platform-microservice/common/db"
"github.com/see1youagain/video-platform-microservice/common/logger"
"github.com/see1youagain/video-platform-microservice/common/redis"

"github.com/cloudwego/kitex/pkg/rpcinfo"
"github.com/cloudwego/kitex/server"
etcd "github.com/kitex-contrib/registry-etcd"
)

func main() {
// 加载配置
cfg, err := config.Load()
if err != nil {
log.Fatalf("❌ 配置加载失败: %v", err)
}

// 初始化日志
if err := logger.Init(); err != nil {
log.Fatalf("❌ 日志初始化失败: %v", err)
}

// 初始化数据库
if err := commonDb.InitDB(); err != nil {
log.Fatalf("❌ 数据库初始化失败: %v", err)
}

// 初始化数据库表
if err := db.Init(); err != nil {
log.Fatalf("❌ 数据库表初始化失败: %v", err)
}

// 初始化 Redis
if err := redis.InitRedis(); err != nil {
log.Fatalf("❌ Redis 初始化失败: %v", err)
}
defer redis.Close()

// 初始化存储
if err := storage.InitStorage(); err != nil {
log.Fatalf("❌ 存储初始化失败: %v", err)
}

// 初始化转码管理器（2个工作协程）
transcode.InitTranscodeManager(2)

// 获取服务端口
port := cfg.RPCPort
if port == "" {
port = "8889"
}

// 配置 Etcd 服务注册
r, err := etcd.NewEtcdRegistry(strings.Split(cfg.EtcdEndpoints, ","))
if err != nil {
log.Fatalf("❌ Etcd 注册中心初始化失败: %v", err)
}

// 创建服务
svr := video.NewServer(
new(VideoServiceImpl),
server.WithServiceAddr(&net.TCPAddr{Port: mustParsePort(port)}),
server.WithRegistry(r),
server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
ServiceName: "video",
}),
)

fmt.Printf("🚀 Video RPC 服务启动在端口 %s\n", port)
fmt.Println("✅ Etcd 注册成功")
fmt.Println("✅ 转码服务已启动")

err = svr.Run()
if err != nil {
log.Fatalf("❌ 服务启动失败: %v", err)
}
}

func mustParsePort(port string) int {
var p int
_, err := fmt.Sscanf(port, "%d", &p)
if err != nil {
log.Fatalf("❌ 端口解析失败: %v", err)
}
return p
}
