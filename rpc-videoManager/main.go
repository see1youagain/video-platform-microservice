package main

import (
"context"
"fmt"
"log"
"net"
"os"
"strings"
"time"

"github.com/cloudwego/kitex/pkg/rpcinfo"
"github.com/cloudwego/kitex/server"
"github.com/joho/godotenv"
etcd "github.com/kitex-contrib/registry-etcd"
commondb "github.com/see1youagain/video-platform-microservice/common/db"
commonKafka "github.com/see1youagain/video-platform-microservice/common/kafka"
commonOutbox "github.com/see1youagain/video-platform-microservice/common/outbox"
commonredis "github.com/see1youagain/video-platform-microservice/common/redis"

"video-platform-microservice/rpc-videoManager/internal/consumer"
"video-platform-microservice/rpc-videoManager/internal/db"
videomanagerSvc "video-platform-microservice/rpc-videoManager/kitex_gen/videomanager/videomanagerservice"
)

func main() {
if err := godotenv.Load(); err != nil {
log.Println("⚠️  未找到 .env 文件，使用默认配置")
}

// ─── 基础设施 ─────────────────────────────────────────────────────
if err := commondb.InitDB(); err != nil {
log.Fatalf("❌ 数据库初始化失败: %v", err)
}
if err := db.Init(); err != nil {
log.Fatalf("❌ 业务表初始化失败: %v", err)
}
if err := commonredis.InitRedis(); err != nil {
log.Fatalf("❌ Redis 初始化失败: %v", err)
}
defer commonredis.Close()

// ─── Outbox 表 + Dispatcher (Relay) ─────────────────────────────
outboxRepo := commonOutbox.NewRepository(commondb.GetDB())
if err := outboxRepo.InitSchema(); err != nil {
log.Fatalf("❌ Outbox 表初始化失败: %v", err)
}

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

go commonOutbox.NewDispatcher(
outboxRepo,
"videomanager-outbox-worker",
100,
2*time.Second,
func(ctx context.Context, topic, key, payload string) error {
return commonKafka.Publish(ctx, topic, key, payload)
},
).Run(ctx)

// ─── Kafka 消费者（监听上游事件，如 video.file.uploaded）──────────
consumer.StartConsumers(ctx)

// ─── etcd + videoUpload Kitex 客户端 ─────────────────────────────
etcdEndpoints := os.Getenv("ETCD_ENDPOINTS")
if etcdEndpoints == "" {
etcdEndpoints = "127.0.0.1:2379"
}
InitVideoUploadClient(strings.Split(etcdEndpoints, ","))

// ─── Kitex 服务端端口 + 注册 ──────────────────────────────────────
port := os.Getenv("RPC_PORT")
if port == "" {
port = "8889"
}

r, err := etcd.NewEtcdRegistry(strings.Split(etcdEndpoints, ","))
if err != nil {
log.Fatalf("❌ Etcd 注册中心初始化失败: %v", err)
}

svr := videomanagerSvc.NewServer(
new(VideoManagerServiceImpl),
server.WithServiceAddr(&net.TCPAddr{Port: mustParsePort(port)}),
server.WithRegistry(r),
server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
ServiceName: "videomanager",
}),
)

fmt.Printf("🚀 videoManager RPC 服务启动，端口 %s\n", port)
if err := svr.Run(); err != nil {
log.Fatalf("❌ 服务启动失败: %v", err)
}
}

func mustParsePort(port string) int {
var p int
if _, err := fmt.Sscanf(port, "%d", &p); err != nil {
log.Fatalf("❌ 端口解析失败: %v", err)
}
return p
}
