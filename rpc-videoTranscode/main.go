package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	transcodeDb "video-platform-microservice/rpc-videoTranscode/internal/db"
	"video-platform-microservice/rpc-videoTranscode/internal/engine"
	"video-platform-microservice/rpc-videoTranscode/kitex_gen/videotranscode/videotranscodeservice"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	"github.com/joho/godotenv"
	etcd "github.com/kitex-contrib/registry-etcd"

	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	commonKafka "github.com/see1youagain/video-platform-microservice/common/kafka"
	commonMinio "github.com/see1youagain/video-platform-microservice/common/minio"
	commonOutbox "github.com/see1youagain/video-platform-microservice/common/outbox"
)

func main() {
	_ = godotenv.Load()
	workers, err := positiveInt(os.Getenv("TRANSCODE_WORKERS"), 2)
	if err != nil {
		log.Fatal(err)
	}
	threads, err := positiveInt(os.Getenv("TRANSCODE_THREADS"), 2)
	if err != nil {
		log.Fatal(err)
	}
	timeout := 30 * time.Minute
	if raw := os.Getenv("TRANSCODE_TIMEOUT"); raw != "" {
		timeout, err = time.ParseDuration(raw)
		if err != nil || timeout <= 0 {
			log.Fatal("invalid TRANSCODE_TIMEOUT")
		}
	}
	encoder, err := engine.New(engine.Config{
		FFmpegPath: os.Getenv("FFMPEG_PATH"), FFprobePath: os.Getenv("FFPROBE_PATH"),
		TempDir: os.Getenv("TRANSCODE_TEMP_DIR"), Timeout: timeout, Threads: threads,
	}, minioStore{})
	if err != nil {
		log.Fatalf("transcode engine: %v", err)
	}
	// ─── MinIO 初始化 ───────────────────────────────────────────────────
	if err := commonMinio.InitMinIO(); err != nil {
		log.Fatalf("❌ MinIO 初始化失败: %v", err)
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

	// ─── 启动 Kafka 消费 goroutine ──────────────────────────────────────
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	wg := StartWorkers(ctx, encoder, workers)
	defer func() {
		cancel()
		wg.Wait()
	}()
	go StartTaskConsumer(ctx)

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
	listenAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		log.Fatalf("invalid VIDEOTRANSCODE_ADDR: %v", err)
	}

	svr := videotranscodeservice.NewServer(
		&VideoTranscodeServiceImpl{},
		server.WithServiceAddr(listenAddr),
		server.WithRegistry(r),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: "videotranscode",
		}),
	)

	log.Printf("✅ videoTranscode 服务启动，监听 %s", addr)
	go func() {
		<-ctx.Done()
		_ = svr.Stop()
	}()
	if err := svr.Run(); err != nil {
		log.Printf("服务运行失败: %v", err)
	}
	cancel()
}
