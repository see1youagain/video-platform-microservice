package main

import (
	"log"
	"os"
	"video-platform-microservice/rpc-user/internal/utils"
	user "video-platform-microservice/rpc-user/kitex_gen/user/userservice"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	"github.com/joho/godotenv"
	etcd "github.com/kitex-contrib/registry-etcd"
	commondb "github.com/see1youagain/video-platform-microservice/common/db"
	commonlogger "github.com/see1youagain/video-platform-microservice/common/logger"
)

func main() {
	godotenv.Load()
	
	// 初始化 Logger
	commonlogger.Init()

	// 初始化数据库（使用 common 库）
	if err := commondb.InitDB(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	defer commondb.Close()
	log.Println("✅ 数据库连接成功")

	// 初始化 JWT
	if err := utils.InitJWT(); err != nil {
		log.Fatalf("JWT 初始化失败: %v", err)
	}

	r, err := etcd.NewEtcdRegistry([]string{os.Getenv("ETCD_ADDRESS")})
	if err != nil {
		log.Fatalf("创建 Etcd 注册中心失败: %v", err)
	}

	svr := user.NewServer(
		new(UserServiceImpl),
		server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
			ServiceName: "user",
		}),
		server.WithRegistry(r),
	)
	log.Println("📡 用户服务启动中...")

	err = svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}