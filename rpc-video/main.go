package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"video-platform-microservice/rpc-video/internal/consumer"
	"video-platform-microservice/rpc-video/internal/db"
	"video-platform-microservice/rpc-video/internal/storage"
	"video-platform-microservice/rpc-video/internal/transcode"
	video "video-platform-microservice/rpc-video/kitex_gen/video/videoservice"

	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	commonredis "github.com/see1youagain/video-platform-microservice/common/redis"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	"github.com/joho/godotenv"
	etcd "github.com/kitex-contrib/registry-etcd"
)

func main() {
	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  未找到 .env 文件，使用默认配置")
	}

	// 初始化数据库
	if err := commondb.InitDB(); err != nil {
		log.Fatalf("❌ 数据库初始化失败: %v", err)
	}

	// 初始化数据库表
	if err := db.Init(); err != nil {
		log.Fatalf("❌ 数据库表初始化失败: %v", err)
	}

	// 初始化 Redis
	if err := commonredis.InitRedis(); err != nil {
		log.Fatalf("❌ Redis 初始化失败: %v", err)
	}
	defer commonredis.Close()

	// 初始化存储
	if err := storage.InitStorage(); err != nil {
		log.Fatalf("❌ 存储初始化失败: %v", err)
	}

	// 初始化转码管理器
	transcode.InitTranscodeManager(2)

	// ─── 启动 Kafka 事件消费者 + 补偿任务 ─────────────────────────────────
	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()
	consumer.StartConsumers(consumerCtx)

	// 获取服务端口
	port := os.Getenv("RPC_PORT")
	if port == "" {
		port = "8889"
	}

	// 配置 Etcd 服务注册
	etcdEndpoints := os.Getenv("ETCD_ENDPOINTS")
	if etcdEndpoints == "" {
		etcdEndpoints = "127.0.0.1:2379"
	}

	r, err := etcd.NewEtcdRegistry(strings.Split(etcdEndpoints, ","))
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
