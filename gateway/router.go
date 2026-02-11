package main

import (
	handler "video-platform-microservice/gateway/biz/handler"
	userHandler "video-platform-microservice/gateway/biz/handler/user"
	"video-platform-microservice/gateway/biz/middleware"

	"github.com/cloudwego/hertz/pkg/app/server"
)

// customizeRegister registers customize routers.
func customizedRegister(r *server.Hertz) {
	r.GET("/ping", handler.Ping)

	// API 路由组
	api := r.Group("/api")
	{
		// 用户相关路由
		api.POST("/register", userHandler.RegisterHandler)
		api.POST("/login", userHandler.LoginHandler)
		protected := api.Group("/", middleware.JWTAuthMiddleware())
		{
			// TODO: 后续添加需要登录的接口
            protected.GET("/profile", userHandler.GetProfileHandler)
            // protected.POST("/upload", videoHandler.UploadHandler)
		}
	}
}
