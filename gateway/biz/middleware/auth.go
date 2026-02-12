package middleware

import (
	"context"
	"strings"
	"video-platform-microservice/gateway/internal/utils"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func JWTAuthMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		// 在这里实现 JWT 验证逻辑
		// 例如，提取并验证 JWT token，如果无效则返回 401 错误
		// 步骤1 提取Token
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) == 0 {
			c.JSON(consts.StatusUnauthorized, map[string]interface{}{
				"code": 401,
				"msg":  "未授权: 缺少 Authorization 头",
			})
			c.Abort() // 中断请求链，停止后续处理
			return
		}
		tokenString := strings.TrimPrefix(string(authHeader), "Bearer ")
		if tokenString == string(authHeader) {
			c.JSON(consts.StatusUnauthorized, map[string]interface{}{
				"code": 401,
				"msg":  "认证格式错误",
			})
			c.Abort() // 中断请求链，停止后续处理
			return
		}
		// 步骤2 验证Token
		claims, err := utils.ParseToken(tokenString)
		if err != nil {
			c.JSON(consts.StatusUnauthorized, map[string]interface{}{
				"code": 401,
				"msg":  "无效的 Token",
			})
			c.Abort() // 中断请求链，停止后续处理
			return
		}
		// 步骤3 提取用户信息
		userID, ok := claims["user_id"].(float64)
		if !ok {
			c.JSON(consts.StatusUnauthorized, map[string]interface{}{
				"code": 401,
				"msg":  "Token 中缺少用户信息",
			})
			c.Abort() // 中断请求链，停止后续处理
			return
		}

		username, ok := claims["username"].(string)
		if !ok {
			c.JSON(consts.StatusUnauthorized, map[string]interface{}{
				"code": 401,
				"msg":  "Token 中缺少用户名信息",
			})
			c.Abort() // 中断请求链，停止后续处理
			return
		}
		// 步骤4 注入上下文
		c.Set("user_id", int64(userID))
		c.Set("username", username)

		// 步骤5 继续执行
		c.Next(ctx)
	}
}
