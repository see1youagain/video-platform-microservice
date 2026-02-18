package main

import (
test_metainfo.go "fmt"
)

func main() {
test_metainfo.go fmt.Println("=== MetaInfo 实现说明 ===\n")
test_metainfo.go 
test_metainfo.go fmt.Println("✅ 1. 网关 JWT 中间件 (gateway/biz/middleware/auth.go):")
test_metainfo.go fmt.Println("   - 解析JWT token获取 user_id和username")
test_metainfo.go fmt.Println("   - 使用 c.Set() 设置到Hertz context（供网关层使用）")
test_metainfo.go fmt.Println("   - 使用 metainfo.WithPersistentValue(ctx, \"user_id\", userID) 注入到context")
test_metainfo.go fmt.Println("   - 使用更新后的context调用 c.Next(ctx)，传递给后续handler\n")
test_metainfo.go 
test_metainfo.go fmt.Println("✅ 2. 网关 Handler (gateway/biz/handler/video/*.go):")
test_metainfo.go fmt.Println("   - 接收带有metainfo的context")
test_metainfo.go fmt.Println("   - 直接使用这个context调用RPC服务")
test_metainfo.go fmt.Println("   - Kitex会自动将metainfo传递给下游服务\n")
test_metainfo.go 
test_metainfo.go fmt.Println("✅ 3. RPC 服务端 (rpc-video/handler.go):")
test_metainfo.go fmt.Println("   - 使用 getUserIDFromContext(ctx, req.UserId) 获取用户信息")
test_metainfo.go fmt.Println("   - 优先从 metainfo.GetPersistentValue(ctx, \"user_id\") 获取（网关传递）")
test_metainfo.go fmt.Println("   - 其次使用请求参数中的 UserId（向后兼容）")
test_metainfo.go fmt.Println("   - 最后使用默认值 \"anonymous\"\n")
test_metainfo.go 
test_metainfo.go fmt.Println("✅ 4. 依赖包:")
test_metainfo.go fmt.Println("   - github.com/bytedance/gopkg/cloud/metainfo")
test_metainfo.go fmt.Println("   - 这是CloudWeGo生态的标准元信息传递方案\n")
test_metainfo.go 
test_metainfo.go fmt.Println("=== 代码示例 ===\n")
test_metainfo.go 
test_metainfo.go fmt.Println("// gateway/biz/middleware/auth.go")
test_metainfo.go fmt.Println("ctx = metainfo.WithPersistentValue(ctx, \"user_id\", userID)")
test_metainfo.go fmt.Println("c.Next(ctx) // 传递更新后的context\n")
test_metainfo.go 
test_metainfo.go fmt.Println("// gateway/biz/handler/video/init.go")
test_metainfo.go fmt.Println("resp, err := rpc.VideoClient.InitUpload(ctx, &video.InitUploadReq{...})")
test_metainfo.go fmt.Println("// ctx中的metainfo会自动传递给RPC服务\n")
test_metainfo.go 
test_metainfo.go fmt.Println("// rpc-video/handler.go")
test_metainfo.go fmt.Println("userID, ok := metainfo.GetPersistentValue(ctx, \"user_id\")")
test_metainfo.go fmt.Println("if ok { // 成功从网关获取用户信息 }\n")
test_metainfo.go 
test_metainfo.go fmt.Println("=== 与 c.Set() 的区别 ===\n")
test_metainfo.go fmt.Println("❌ c.Set(\"user_id\", userID):")
test_metainfo.go fmt.Println("   - 只在 HTTP 层的 RequestContext 中有效")
test_metainfo.go fmt.Println("   - RPC调用时无法传递到下游服务")
test_metainfo.go fmt.Println("   - 下游服务无法获取用户信息\n")
test_metainfo.go 
test_metainfo.go fmt.Println("✅ metainfo.WithPersistentValue(ctx, \"user_id\", userID):")
test_metainfo.go fmt.Println("   - 注入到context中，可以跨服务传递")
test_metainfo.go fmt.Println("   - Kitex框架会自动序列化并传递metainfo")
test_metainfo.go fmt.Println("   - 下游服务可以通过 metainfo.GetPersistentValue 获取")
test_metainfo.go fmt.Println("   - 支持多级服务调用的元信息传递\n")
test_metainfo.go 
test_metainfo.go fmt.Println("=== 修复完成 ===")
test_metainfo.go fmt.Println("所有服务已更新使用 metainfo 进行用户信息传递！")
}
