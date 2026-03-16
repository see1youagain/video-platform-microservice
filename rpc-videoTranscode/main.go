package main

import (
	"context"
	"log"
	"net"
	"os"
	"strings"
	"time"

	transcodeDb "video-platform-microservice/rpc-videoTranscode/internal/db"
	"video-platform-microservice/rpc-videoTranscode/kitex_gen/videotranscode/videotranscodeservice"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"

	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	commonEvents "github.com/see1youagain/video-platform-microservice/common/events"
	commonKafka "github.com/see1youagain/video-platform-microservice/common/kafka"
	commonMinio "github.com/see1youagain/video-platform-microservice/common/minio"
	commonOutbox "github.com/see1youagain/video-platform-microservice/common/outbox"
)

func main() {
	// ─── MinIO 初始化 ───────────────────────────────────────────────────
	if err := commonMinio.InitMinIO(); err != nil {
		log.Printf("⚠️  MinIO 初始化失败: %v", err)
	}

	if err := commondb.InitDB(); err != nil {
		log.Fatalf("❌ DB 初始化失败: %v", err)
	}
	if err := transcodeDb.Init(); err != nil {
		log.Fatalf("❌ transcode_jobs 表初始化失败: %v", err)
	}
	outboxRepo := commonOutbox.NewRepository(commondb.GetDB())
	if err := outboxRepo.InitSchema(); err != nil {
		log.Fatalf("❌ outbox_events 表初始化失败: %v", err)
	}

	// ─── Kafka 消费者注册（file.uploaded 订阅）───────────────────────────
	if err := commonKafka.InitKafkaConsumer(commonEvents.TopicTranscodeRequested, "transcode-service"); err != nil {
		log.Printf("⚠️  Kafka Consumer 初始化失败: %v", err)
	}

	// ─── 启动 Kafka 消费 goroutine ──────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go StartFileUploadedConsumer(ctx)

	go commonOutbox.NewDispatcher(
		outboxRepo,
		"videotranscode-outbox-worker",
		100,
		2*time.Second,
		func(ctx context.Context, topic, key, payload string) error {
			return commonKafka.Publish(ctx, topic, key, payload)
		},
	).Run(ctx)

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
	addr := os.Getenv("VIDEOTRANSCODE_ADDR")
	if addr == "" {
		addr = ":8084"
	}
	listenAddr, _ := net.ResolveTCPAddr("tcp", addr)

	svr := videotranscodeservice.NewServer(
		&VideoTranscodeServiceImpl{},
		server.WithServiceAddr(listenAddr),
		server.WithRegistry(r),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: "videotranscode",
		}),
	)

	log.Printf("✅ videoTranscode 服务启动，监听 %s", addr)
	if err := svr.Run(); err != nil {
		log.Fatalf("服务运行失败: %v", err)
	}
}
