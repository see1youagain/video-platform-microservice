package user

import (
	"context"

	"video-platform-microservice/gateway/internal/logger"
	"video-platform-microservice/gateway/internal/validator"
	"video-platform-microservice/gateway/rpc"
	"video-platform-microservice/rpc-user/kitex_gen/user"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"go.uber.org/zap"
)

// RegisterHandler 处理用户注册请求
func RegisterHandler(ctx context.Context, c *app.RequestContext) {
	// 定义请求体结构
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	// 获取 trace_id
	traceID, _ := c.Get("trace_id")

	// 绑定并验证请求参数
	if err := c.BindAndValidate(&req); err != nil {
		logger.Logger.Warn("注册参数绑定失败",
			zap.Any("trace_id", traceID),
			zap.Error(err),
		)
		c.JSON(consts.StatusBadRequest, map[string]interface{}{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	// 验证用户名格式
	if err := validator.ValidateUsername(req.Username); err != nil {
		logger.Logger.Warn("用户名格式验证失败",
			zap.Any("trace_id", traceID),
			zap.String("username", req.Username),
			zap.Error(err),
		)
		c.JSON(consts.StatusBadRequest, map[string]interface{}{
			"code": 400,
			"msg":  err.Error(),
		})
		return
	}

	// 验证密码强度
	if err := validator.ValidatePassword(req.Password); err != nil {
		logger.Logger.Warn("密码格式验证失败",
			zap.Any("trace_id", traceID),
			zap.Error(err),
		)
		c.JSON(consts.StatusBadRequest, map[string]interface{}{
			"code": 400,
			"msg":  err.Error(),
		})
		return
	}

	// 调用 User 服务的 Register RPC 方法
	logger.Logger.Info("调用 RPC Register",
		zap.Any("trace_id", traceID),
		zap.String("username", req.Username),
	)

	resp, err := rpc.UserClient.Register(ctx, &user.RegisterReq{
		Username: req.Username,
		Password: req.Password,
	})

	if err != nil {
		logger.Logger.Error("RPC 调用失败",
			zap.Any("trace_id", traceID),
			zap.Error(err),
		)
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
	case 400:
		httpStatus = consts.StatusBadRequest
	default:
		httpStatus = consts.StatusInternalServerError
	}

	// 记录结果
	logger.Logger.Info("注册请求处理完成",
		zap.Any("trace_id", traceID),
		zap.Int32("code", resp.Code),
		zap.Int64("user_id", resp.UserId),
	)

	// 返回响应
	c.JSON(httpStatus, map[string]interface{}{
		"code":    resp.Code,
		"msg":     resp.Msg,
		"user_id": resp.UserId,
	})
}
