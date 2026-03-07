package main

import (
"context"
"log"
"net"
"os"
"strings"

"video-platform-microservice/rpc-videoUpload/kitex_gen/videoupload/videouploadservice"

"github.com/cloudwego/kitex/pkg/circuitbreak"
"github.com/cloudwego/kitex/server"
	"github.com/cloudwego/kitex/pkg/rpcinfo"
etcd "github.com/kitex-contrib/registry-etcd"

commonKafka "github.com/see1youagain/video-platform-microservice/common/kafka"
commonMinio "github.com/see1youagain/video-platform-microservice/common/minio"
)

func main() {
// ─── MinIO 初始化 ───────────────────────────────────────────────────
if err := commonMinio.InitMinIO(); err != nil {
log.Printf("⚠️  MinIO 初始化失败（降级到本地存储）: %v", err)
}

// ─── Kafka 生产者初始化 ─────────────────────────────────────────────
if err := commonKafka.InitKafkaProducer("video.file.uploaded"); err != nil {
log.Printf("⚠️  Kafka 生产者初始化失败（outbox 将重试）: %v", err)
}

// ─── 启动 outbox 后台重试 goroutine ─────────────────────────────────
go outboxReaper(context.Background())

// ─── etcd 注册 ──────────────────────────────────────────────────────
etcdEndpoints := os.Getenv("ETCD_ENDPOINTS")
if etcdEndpoints == "" {
if single := os.Getenv("ETCD_ADDRESS"); single != "" {
etcdEndpoints = single
} else {
etcdEndpoints = "127.0.0.1:2379"
}
}
endpoints := strings.Split(etcdEndpoints, ",")

r, err := etcd.NewEtcdRegistry(endpoints)
if err != nil {
log.Fatalf("创建 etcd 注册器失败: %v", err)
}

// ─── 端口配置 ────────────────────────────────────────────────────────
addr := os.Getenv("VIDEOUPLOAD_ADDR")
if addr == "" {
addr = ":8083"
}
listenAddr, _ := net.ResolveTCPAddr("tcp", addr)

// ─── Kitex 服务器 ────────────────────────────────────────────────────
cb := circuitbreak.NewCBSuite(circuitbreak.RPCInfo2Key)
_ = cb

svr := videouploadservice.NewServer(
&VideoUploadServiceImpl{},
server.WithServiceAddr(listenAddr),
server.WithRegistry(r),
	server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
		ServiceName: "videoupload",
	}),
)

log.Printf("✅ videoUpload 服务启动，监听 %s", addr)
if err := svr.Run(); err != nil {
log.Fatalf("服务运行失败: %v", err)
}
}
