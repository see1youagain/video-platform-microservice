package rpc

import (
"log"

"video-platform-microservice/gateway/kitex_gen/user/userservice"
"video-platform-microservice/gateway/kitex_gen/video/videoservice"

"github.com/cloudwego/kitex/client"
etcd "github.com/kitex-contrib/registry-etcd"
)

var UserClient userservice.Client
var VideoClient videoservice.Client

// InitRPC 初始化所有 RPC 客户端
func InitRPC() {
// 创建 Etcd 服务发现解析器
r, err := etcd.NewEtcdResolver([]string{"127.0.0.1:2379"})
if err != nil {
log.Fatalf("创建 Etcd 解析器失败: %v", err)
}

// 初始化 User 服务客户端
UserClient, err = userservice.NewClient("user", client.WithResolver(r))
if err != nil {
log.Fatalf("初始化 User 客户端失败: %v", err)
}

// 初始化 Video 服务客户端
VideoClient, err = videoservice.NewClient("video", client.WithResolver(r))
if err != nil {
log.Fatalf("初始化 Video 客户端失败: %v", err)
}

log.Println("✅ RPC 客户端初始化成功 (User + Video)")
}
