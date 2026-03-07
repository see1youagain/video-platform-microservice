package rpc

import (
"log"
"os"
"strings"

"video-platform-microservice/gateway/kitex_gen/user/userservice"
"video-platform-microservice/gateway/kitex_gen/video/videoservice"
"video-platform-microservice/gateway/kitex_gen/videoupload/videouploadservice"

"github.com/cloudwego/kitex/client"
"github.com/cloudwego/kitex/pkg/circuitbreak"
etcd "github.com/kitex-contrib/registry-etcd"
)

var UserClient userservice.Client
var VideoClient videoservice.Client
var VideoUploadClient videouploadservice.Client

func InitRPC() {
etcdEndpoints := os.Getenv("ETCD_ENDPOINTS")
if etcdEndpoints == "" {
if single := os.Getenv("ETCD_ADDRESS"); single != "" {
etcdEndpoints = single
} else {
etcdEndpoints = "127.0.0.1:2379"
}
}
endpoints := strings.Split(etcdEndpoints, ",")

r, err := etcd.NewEtcdResolver(endpoints)
if err != nil {
log.Fatalf("创建 Etcd 解析器失败: %v", err)
}

cb := circuitbreak.NewCBSuite(circuitbreak.RPCInfo2Key)

UserClient, err = userservice.NewClient(
"user",
client.WithResolver(r),
client.WithCircuitBreaker(cb),
)
if err != nil {
log.Fatalf("初始化 User 客户端失败: %v", err)
}

VideoClient, err = videoservice.NewClient(
"video",
client.WithResolver(r),
client.WithCircuitBreaker(cb),
)
if err != nil {
log.Fatalf("初始化 Video 客户端失败: %v", err)
}

VideoUploadClient, err = videouploadservice.NewClient(
"videoupload",
client.WithResolver(r),
client.WithCircuitBreaker(cb),
)
if err != nil {
log.Fatalf("初始化 VideoUpload 客户端失败: %v", err)
}

log.Printf("✅ RPC 客户端初始化成功 (User + Video + VideoUpload), etcd=%v", endpoints)
}
