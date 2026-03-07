package main

import (
handler "video-platform-microservice/gateway/biz/handler"
userHandler "video-platform-microservice/gateway/biz/handler/user"
videoHandler "video-platform-microservice/gateway/biz/handler/video"
"video-platform-microservice/gateway/biz/middleware"

"github.com/cloudwego/hertz/pkg/app/server"
commonRedis "github.com/see1youagain/video-platform-microservice/common/redis"
)

// customizeRegister registers customize routers.
func customizedRegister(r *server.Hertz) {
// 全局中间件：请求追踪 ID
r.Use(middleware.TraceIDMiddleware())

// 全局限流中间件（500 req/s，容量 1000）
r.Use(middleware.TokenBucketMiddleware(middleware.RateLimitConfig{
Rate:        500,
Capacity:    1000,
RedisClient: commonRedis.GetClient(),
}))

r.GET("/ping", handler.Ping)

// API 路由组
api := r.Group("/api")
{
// 用户相关路由（无需认证，但需要更严格的限流）
userGroup := api.Group("/")
userGroup.Use(middleware.TokenBucketMiddleware(middleware.RateLimitConfig{
Rate:        200, // 200 req/s
Capacity:    400,  // 容量 20
RedisClient: commonRedis.GetClient(),
}))
{
userGroup.POST("/register", userHandler.RegisterHandler)
userGroup.POST("/login", userHandler.LoginHandler)
}

// 需要认证的路由组 - 所有视频操作强制要求认证
protected := api.Group("/", middleware.JWTAuthMiddleware())
{
// 用户相关
protected.GET("/profile", userHandler.GetProfileHandler)

// 视频下载和信息查看（需要认证）
protected.GET("/video/download", videoHandler.DownloadHandler)
protected.GET("/video/info", videoHandler.GetVideoInfoHandler)

// 视频上传相关（更宽松的限流）
uploadGroup := protected.Group("/video")
uploadGroup.Use(middleware.TokenBucketMiddleware(middleware.RateLimitConfig{
Rate:        50,  // 50 req/s
Capacity:    100, // 容量 100
RedisClient: commonRedis.GetClient(),
}))
{
uploadGroup.POST("/init", videoHandler.InitUploadHandler)         // 初始化上传
uploadGroup.POST("/upload_chunk", videoHandler.UploadChunkHandler) // 上传分片
uploadGroup.POST("/merge", videoHandler.MergeFileHandler)         // 合并文件
uploadGroup.POST("/upload", videoHandler.SimpleUploadHandler)     // 简单上传
uploadGroup.POST("/hash", videoHandler.CalculateFileHashHandler)  // 计算 Hash
}

// 转码相关
protected.POST("/video/transcode", videoHandler.TranscodeHandler) // 创建转码任务
protected.GET("/video/transcode/status", videoHandler.GetTranscodeStatusHandler) // 查询转码状态
}
}
}
