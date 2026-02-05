package main

/*

状态码	含义	使用场景
200	成功	操作成功
400	请求错误	参数错误、用户名已存在
401	未授权	密码错误
404	资源不存在	用户不存在
500	服务器错误	数据库连接失败、加密失败

*/

import (
	"context"
	"video-platform-microservice/rpc-user/internal/db"
	"video-platform-microservice/rpc-user/internal/utils"
	user "video-platform-microservice/rpc-user/kitex_gen/user"
)

// UserServiceImpl implements the last service interface defined in the IDL.
type UserServiceImpl struct{}

// Register implements the UserServiceImpl interface.
func (s *UserServiceImpl) Register(ctx context.Context, req *user.RegisterReq) (resp *user.RegisterResp, err error) {
    hashedPassword, err := utils.HashPassword(req.Password)
    if err != nil {
        return &user.RegisterResp{
            Code:   500,
            Msg:    "密码加密失败",
            UserId: 0,
        }, err
    }

    userID, err := db.CreateUser(req.Username, hashedPassword)
    if err != nil {
        return &user.RegisterResp{
            Code:   400,
            Msg:    "用户名可能已存在",
            UserId: 0,
        }, err
    }

    return &user.RegisterResp{
        Code:   200,
        Msg:    "注册成功",
        UserId: int64(userID),
    }, nil
}

// Login implements the UserServiceImpl interface.
func (s *UserServiceImpl) Login(ctx context.Context, req *user.LoginReq) (resp *user.LoginResp, err error) {
    existingUser, err := db.GetUserByUsername(req.Username)
    if err != nil {
        return &user.LoginResp{
            Code:   404,
            Msg:    "用户不存在",
            Token:  "",
            UserId: 0,
        }, err
    }

    if !utils.CheckPasswordHash(req.Password, existingUser.Password) {
        return &user.LoginResp{
            Code:   401,
            Msg:    "密码错误",
            Token:  "",
            UserId: 0,
        }, nil  // 注意：这里是 nil，不是 err
    }

    return &user.LoginResp{
        Code:   200,
        Msg:    "登录成功",
        Token:  "",  // 微服务不生成 Token，由网关生成
        UserId: int64(existingUser.ID),
    }, nil
}