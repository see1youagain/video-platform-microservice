package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func GetProfileHandler(ctx context.Context, c *app.RequestContext) {
	userID,existed := c.Get("user_id")
	if !existed {
		c.JSON(401, map[string]interface{}{
			"code": 401,
			"msg":  "未授权: 用户未登陆",
		})
		return
	}
	username, ok := c.Get("username")
	if !ok {
		c.JSON(401, map[string]interface{}{
			"code": 401,
			"msg":  "未授权: 用户未登陆",
		})
		return
	}
	c.JSON(consts.StatusOK, map[string]interface{}{
		"code":     200,
		"msg":      "获取用户信息成功",
		"data": map[string]interface{}{
			"user_id":  userID,
			"username": username,
		},
	})
}