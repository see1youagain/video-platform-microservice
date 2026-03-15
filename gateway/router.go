package main

import (
	"os"
	"strconv"

	handler "video-platform-microservice/gateway/biz/handler"
	userHandler "video-platform-microservice/gateway/biz/handler/user"
	videoHandler "video-platform-microservice/gateway/biz/handler/video"
	"video-platform-microservice/gateway/biz/middleware"

	"github.com/cloudwego/hertz/pkg/app/server"
	commonRedis "github.com/see1youagain/video-platform-microservice/common/redis"
)

func getenvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// customizeRegister registers customize routers.
func customizedRegister(r *server.Hertz) {
	// 全局中间件：请求追踪 ID
	r.Use(middleware.TraceIDMiddleware())

	// 全局限流中间件
	r.Use(middleware.TokenBucketMiddleware(middleware.RateLimitConfig{
		Rate:        getenvInt("RL_GLOBAL_RATE", 500),
		Capacity:    getenvInt("RL_GLOBAL_CAPACITY", 1000),
		RedisClient: commonRedis.GetClient(),
	}))

	r.GET("/ping", handler.Ping)

	// API 路由组
	api := r.Group("/api")
	{
		// 用户相关路由（无需认证）
		userGroup := api.Group("/")
		userGroup.Use(middleware.TokenBucketMiddleware(middleware.RateLimitConfig{
			Rate:        getenvInt("RL_USER_RATE", 200),
			Capacity:    getenvInt("RL_USER_CAPACITY", 400),
			RedisClient: commonRedis.GetClient(),
		}))
		{
			userGroup.POST("/register", userHandler.RegisterHandler)
			userGroup.POST("/login", userHandler.LoginHandler)
		}

		// 需要认证的路由组
		protected := api.Group("/", middleware.JWTAuthMiddleware())
		{
			protected.GET("/profile", userHandler.GetProfileHandler)

			// 视频下载和信息查看
			protected.GET("/video/download", videoHandler.DownloadHandler)
			protected.GET("/video/info", videoHandler.GetVideoInfoHandler)
			protected.POST("/video/delete", videoHandler.DeleteVideoHandler)

			// 视频上传相关
			uploadGroup := protected.Group("/video")
			uploadGroup.Use(middleware.TokenBucketMiddleware(middleware.RateLimitConfig{
				Rate:        getenvInt("RL_UPLOAD_RATE", 50),
				Capacity:    getenvInt("RL_UPLOAD_CAPACITY", 100),
				RedisClient: commonRedis.GetClient(),
			}))
			{
				uploadGroup.POST("/init", videoHandler.InitUploadHandler)
				uploadGroup.POST("/upload_chunk", videoHandler.UploadChunkHandler)
				uploadGroup.POST("/merge", videoHandler.MergeFileHandler)
				uploadGroup.POST("/upload", videoHandler.SimpleUploadHandler)
				uploadGroup.POST("/hash", videoHandler.CalculateFileHashHandler)
			}

			// 转码相关
			protected.POST("/video/transcode", videoHandler.TranscodeHandler)
			protected.GET("/video/transcode/status", videoHandler.GetTranscodeStatusHandler)
		}
	}
}
