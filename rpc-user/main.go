package main

import (
	"log"
	"os"
	"video-platform-microservice/rpc-user/conf"
	"video-platform-microservice/rpc-user/internal/utils"
	user "video-platform-microservice/rpc-user/kitex_gen/user/userservice"

	"github.com/cloudwego/kitex/pkg/rpcinfo"
	"github.com/cloudwego/kitex/server"
	"github.com/joho/godotenv"
	etcd "github.com/kitex-contrib/registry-etcd"
)

func main() {
	godotenv.Load()
	if err := conf.LoadConfig(); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatalf("DB_DSN 环境变量未设置")
	}
	if err := conf.InitDB(dsn); err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	log.Println("数据库连接成功")

	// 🆕 初始化 JWT
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
	log.Println("用户服务启动中...")

	err = svr.Run()

	if err != nil {
		log.Println(err.Error())
	}
}
