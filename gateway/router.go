package main

import (
	handler "video-platform-microservice/gateway/biz/handler"
	userHandler "video-platform-microservice/gateway/biz/handler/user"
	videoHandler "video-platform-microservice/gateway/biz/handler/video"
	"video-platform-microservice/gateway/biz/middleware"

	"github.com/cloudwego/hertz/pkg/app/server"
)

// customizeRegister registers customize routers.
func customizedRegister(r *server.Hertz) {
	// 全局中间件：请求追踪 ID
	r.Use(middleware.TraceIDMiddleware())

	r.GET("/ping", handler.Ping)

	// API 路由组
	api := r.Group("/api")
	{
		// 用户相关路由（无需认证）
		api.POST("/register", userHandler.RegisterHandler)
		api.POST("/login", userHandler.LoginHandler)

		// 需要认证的路由组
		protected := api.Group("/", middleware.JWTAuthMiddleware())
		{
			// 用户相关
			protected.GET("/profile", userHandler.GetProfileHandler)

			// 视频上传相关
			protected.POST("/video/init", videoHandler.InitUploadHandler)       // 初始化上传
			protected.POST("/video/chunk", videoHandler.UploadChunkHandler)     // 上传分片
			protected.POST("/video/merge", videoHandler.MergeFileHandler)       // 合并文件
			protected.POST("/video/upload", videoHandler.SimpleUploadHandler)   // 简单上传
			protected.POST("/video/hash", videoHandler.CalculateFileHashHandler) // 计算 Hash
		}
	}
}