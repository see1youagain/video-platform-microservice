package main

import (
	handler "video-platform-microservice/gateway/biz/handler"
	userHandler "video-platform-microservice/gateway/biz/handler/user"

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
	}
}
