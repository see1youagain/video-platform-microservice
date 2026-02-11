package main

import (
	handler "video-platform-microservice/gateway/biz/handler"
	userHandler "video-platform-microservice/gateway/biz/handler/user"
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
			protected.GET("/profile", userHandler.GetProfileHandler)
			// 后续添加更多需要认证的接口
			// protected.POST("/upload", videoHandler.UploadHandler)
		}
	}
}
