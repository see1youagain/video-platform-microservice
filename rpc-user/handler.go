package main

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"video-platform-microservice/rpc-user/internal/db"

	user "video-platform-microservice/rpc-user/kitex_gen/user"

	commonredis "github.com/see1youagain/video-platform-microservice/common/redis"
	commonutils "github.com/see1youagain/video-platform-microservice/common/utils"
)

// UserServiceImpl implements the last service interface defined in the IDL.
type UserServiceImpl struct{}

// Register implements the UserServiceImpl interface.
func (s *UserServiceImpl) Register(ctx context.Context, req *user.RegisterReq) (resp *user.RegisterResp, err error) {
	hashedPassword, err := commonutils.HashPassword(req.Password)
	if err != nil {
		return &user.RegisterResp{Code: 500, Msg: "密码加密失败", UserId: 0}, nil
	}

	userID, err := db.CreateUser(req.Username, hashedPassword)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") || strings.Contains(err.Error(), "idx_users_username") {
			return &user.RegisterResp{Code: 400, Msg: "用户名已存在", UserId: 0}, nil
		}
		return &user.RegisterResp{Code: 500, Msg: "注册失败，请稍后重试", UserId: 0}, nil
	}

	return &user.RegisterResp{Code: 200, Msg: "注册成功", UserId: int64(userID)}, nil
}

// Login implements the UserServiceImpl interface.
func (s *UserServiceImpl) Login(ctx context.Context, req *user.LoginReq) (resp *user.LoginResp, err error) {
	existingUser, err := db.GetUserByUsername(req.Username)
	if err != nil {
		return &user.LoginResp{Code: 404, Msg: "用户不存在", Token: "", UserId: 0}, nil
	}

	if !commonutils.CheckPasswordHash(req.Password, existingUser.Password) {
		return &user.LoginResp{Code: 401, Msg: "密码错误", Token: "", UserId: 0}, nil
	}

	secret := os.Getenv("JWT_SECRET")
	if redisSecret, redisErr := commonredis.GetClient().Get(ctx, "jwt:secret_key").Result(); redisErr == nil && redisSecret != "" {
		secret = redisSecret
	}

	token, err := commonutils.GenerateToken(existingUser.ID, existingUser.Username, secret, 24)
	if err != nil {
		return &user.LoginResp{Code: 500, Msg: "生成 Token 失败", Token: "", UserId: 0}, nil
	}

	_ = commonutils.CacheToken(ctx, commonredis.GetClient(), strconv.FormatUint(uint64(existingUser.ID), 10), token, 24*time.Hour)

	return &user.LoginResp{Code: 200, Msg: "登录成功", Token: token, UserId: int64(existingUser.ID)}, nil
}
