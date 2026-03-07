package middleware

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bytedance/gopkg/cloud/metainfo"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	commonRedis "github.com/see1youagain/video-platform-microservice/common/redis"
	"github.com/see1youagain/video-platform-microservice/common/utils"
)

func JWTAuthMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) == 0 {
			c.JSON(consts.StatusUnauthorized, map[string]interface{}{
				"code": 401,
				"msg":  "未授权: 缺少 Authorization 头",
			})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(string(authHeader), "Bearer ")
		if tokenString == string(authHeader) {
			c.JSON(consts.StatusUnauthorized, map[string]interface{}{
				"code": 401,
				"msg":  "认证格式错误",
			})
			c.Abort()
			return
		}

		secret := utils.GetCurrentSecretKey()
		if secret == "" {
			secret = os.Getenv("JWT_SECRET")
		}

		claims, err := utils.ParseToken(tokenString, secret)
		if err != nil {
			if oldSecret, redisErr := commonRedis.GetClient().Get(ctx, "jwt:old_secret_key").Result(); redisErr == nil && oldSecret != "" {
				claims, err = utils.ParseToken(tokenString, oldSecret)
			}
		}
		if err != nil {
			c.JSON(consts.StatusUnauthorized, map[string]interface{}{
				"code": 401,
				"msg":  "无效的 Token",
			})
			c.Abort()
			return
		}

		userID := claims.UserID
		username := claims.Username
		if userID == 0 || username == "" {
			c.JSON(consts.StatusUnauthorized, map[string]interface{}{
				"code": 401,
				"msg":  "Token 中缺少用户信息",
			})
			c.Abort()
			return
		}

		if cachedToken, redisErr := utils.GetCachedToken(ctx, commonRedis.GetClient(), strconv.FormatUint(uint64(userID), 10)); redisErr == nil {
			if cachedToken != tokenString {
				c.JSON(consts.StatusUnauthorized, map[string]interface{}{
					"code": 401,
					"msg":  "Token 已失效",
				})
				c.Abort()
				return
			}
		}

		c.Set("user_id", int64(userID))
		c.Set("username", username)

		ctx = metainfo.WithPersistentValue(ctx, "user_id", fmt.Sprintf("%d", userID))
		ctx = metainfo.WithPersistentValue(ctx, "username", username)

		c.Next(ctx)
	}
}
