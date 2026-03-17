package main

import (
	"log"
	"os"
	"strings"

	"video-platform-microservice/rpc-user/internal/utils"
	user "video-platform-microservice/rpc-user/kitex_gen/user/userservice"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	"github.com/joho/godotenv"
	etcd "github.com/kitex-contrib/registry-etcd"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	userdb "video-platform-microservice/rpc-user/internal/db"
	commonlogger "github.com/see1youagain/video-platform-microservice/common/logger"
	commonredis "github.com/see1youagain/video-platform-microservice/common/redis"
)

func main() {
	godotenv.Load()

	commonlogger.Init()

	if err := commondb.InitDB(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer commondb.Close()
	log.Println("✅ 数据库连接成功")

	// AutoMigrate: 确保 users 表存在
	if err := commondb.GetDB().AutoMigrate(&userdb.User{}); err != nil {
		log.Fatalf("AutoMigrate users 失败: %v", err)
	}
	log.Println("✅ users 表迁移完成")

	if err := commonredis.InitRedis(); err != nil {
		log.Fatalf("Redis 初始化失败: %v", err)
	}
	defer commonredis.Close()
	log.Println("✅ Redis 连接成功")

	if err := utils.InitJWT(); err != nil {
		log.Fatalf("JWT 初始化失败: %v", err)
	}

	etcdEndpoints := os.Getenv("ETCD_ENDPOINTS")
	if etcdEndpoints == "" {
		etcdEndpoints = os.Getenv("ETCD_ADDRESS")
	}
	if etcdEndpoints == "" {
		etcdEndpoints = "127.0.0.1:2379"
	}
	r, err := etcd.NewEtcdRegistry(strings.Split(etcdEndpoints, ","))
	if err != nil {
		log.Fatalf("创建 Etcd 注册中心失败: %v", err)
	}

	svr := user.NewServer(
		new(UserServiceImpl),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: "user"}),
		server.WithRegistry(r),
	)
	log.Println("📡 用户服务启动中...")

	if err := svr.Run(); err != nil {
		log.Println(err.Error())
	}
}
