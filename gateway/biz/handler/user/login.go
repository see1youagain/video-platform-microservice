package user

import (
	"context"

	"video-platform-microservice/gateway/rpc"
	"video-platform-microservice/rpc-user/kitex_gen/user"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

// LoginHandler 处理用户登录请求
func LoginHandler(ctx context.Context, c *app.RequestContext) {
	// 定义请求体结构
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	// 绑定并验证请求参数
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]interface{}{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	// 调用 User 服务的 Login RPC 方法
	resp, err := rpc.UserClient.Login(ctx, &user.LoginReq{
		Username: req.Username,
		Password: req.Password,
	})

	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"code": 500,
			"msg":  "服务暂时不可用，请稍后重试",
		})
		return
	}

	// 根据业务状态码返回不同的 HTTP 状态码
	var httpStatus int
	switch resp.Code {
	case 200:
		httpStatus = consts.StatusOK
	case 401:
		httpStatus = consts.StatusUnauthorized
	case 404:
		httpStatus = consts.StatusNotFound
	default:
		httpStatus = consts.StatusInternalServerError
	}

	// 返回响应（包含 JWT token）
	c.JSON(httpStatus, map[string]interface{}{
		"code":    resp.Code,
		"msg":     resp.Msg,
		"user_id": resp.UserId,
		"token":   resp.Token,
	})
}
