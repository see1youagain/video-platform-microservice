package main

import (
	"context"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"video-platform-microservice/rpc-videoUpload/internal/db"
	"video-platform-microservice/rpc-videoUpload/kitex_gen/videoupload/videouploadservice"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	etcd "github.com/kitex-contrib/registry-etcd"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	commonKafka "github.com/see1youagain/video-platform-microservice/common/kafka"
	commonMinio "github.com/see1youagain/video-platform-microservice/common/minio"
	commonOutbox "github.com/see1youagain/video-platform-microservice/common/outbox"
	commonredis "github.com/see1youagain/video-platform-microservice/common/redis"
)

func main() {
	if err := commonMinio.InitMinIO(); err != nil {
		log.Printf("⚠️ MinIO 初始化失败: %v", err)
	}

	if err := commondb.InitDB(); err != nil {
		log.Fatalf("❌ DB 初始化失败: %v", err)
	}
	if err := db.Init(); err != nil {
		log.Fatalf("❌ upload_files 表初始化失败: %v", err)
	}
	if err := commonredis.InitRedis(); err != nil {
		log.Fatalf("❌ Redis 初始化失败: %v", err)
	}
	defer commonredis.Close()

	outboxRepo := commonOutbox.NewRepository(commondb.GetDB())
	if err := outboxRepo.InitSchema(); err != nil {
		log.Fatalf("❌ outbox_events 表初始化失败: %v", err)
	}

	if err := commonKafka.InitKafkaProducer("video.file.uploaded"); err != nil {
		log.Printf("⚠️ Kafka producer 初始化失败，依赖 outbox 异步重试: %v", err)
	}

	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go commonOutbox.NewDispatcher(
		outboxRepo,
		"videoupload-outbox-worker",
		100,
		2*time.Second,
		func(ctx context.Context, topic, key, payload string) error {
			return commonKafka.Publish(ctx, topic, key, payload)
		},
	).Run(workerCtx)

	etcdEndpoints := os.Getenv("ETCD_ENDPOINTS")
	if etcdEndpoints == "" {
		if single := os.Getenv("ETCD_ADDRESS"); single != "" {
			etcdEndpoints = single
		} else {
			etcdEndpoints = "127.0.0.1:2379"
		}
	}
	r, err := etcd.NewEtcdRegistry(strings.Split(etcdEndpoints, ","))
	if err != nil {
		log.Fatalf("❌ etcd 初始化失败: %v", err)
	}

	addr := os.Getenv("VIDEOUPLOAD_ADDR")
	if addr == "" {
		addr = ":8083"
	}
	listenAddr, _ := net.ResolveTCPAddr("tcp", addr)

	svr := videouploadservice.NewServer(
		&VideoUploadServiceImpl{},
		server.WithServiceAddr(listenAddr),
		server.WithRegistry(r),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: "videoupload"}),
	)

	log.Printf("✅ videoUpload 服务启动，监听 %s", addr)
	if err := svr.Run(); err != nil {
		log.Fatalf("❌ 服务运行失败: %v", err)
	}
}
