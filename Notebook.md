## Details

本项目基于[video-platform](https://github.com/see1youagain/video-platform)进行版本递进，从gin+redis+jwt的单机框架，改为基于CloudWeGo(Hertz+Kitex+Redis)的框架，本地详细递进细节如下：

### Old Version

开发语言：Go 1.24.1

Web 框架：Gin (高性能 HTTP 框架)

数据库 (ORM)：MySQL + GORM (负责元数据持久化，如用户信息、文件索引)

缓存与锁：Redis + go-redis (负责分布式锁、上传状态记录、墓碑管理)

认证鉴权：JWT (JSON Web Tokens) 实现用户注册与登录认证

工具库：godotenv (配置加载)、google/uuid (唯一ID生成)

### This Project

开发语言：Go 1.24.1

网关框架：Hertz (字节跳动高性能 HTTP 框架，替代 Gin，作为 这套架构完成后,你的项目将具备水平扩展能力（User 服务和 Video 服务可以独立扩容），并掌握了字节跳动核心技术栈的落地实践。

---

## 实战开发日志

### 阶段一：User 微服务开发（已完成 ✅）

#### 1.1 环境准备

**安装 Etcd 服务注册中心：**

```bash
# macOS 使用 Homebrew 安装
brew install etcd

# 启动 Etcd 服务
brew services start etcd

# 验证 Etcd 是否启动（应看到监听 2379 端口）
lsof -i :2379
```

**为什么需要 Etcd？**

- 微服务架构中，服务地址是动态的（IP、端口可能变化）
- Etcd 作为"服务电话簿"，自动管理服务注册、发现和健康检查
- 避免在代码中硬编码服务地址，实现真正的动态扩缩容

#### 1.2 生成 Kitex 脚手架

```bash
# 在项目根目录下
mkdir rpc-user && cd rpc-user

# 使用 kitex 工具生成代码（基于 user.thrift）
kitex -module video-platform-microservice/rpc-user \
      -service user \
      ../idl/user.thrift

# 初始化 Go 模块依赖
go mod tidy
```

**命令解析：**

- `-module`: 指定 Go module 路径（对应 go.mod 中的 module 名）
- `-service user`: 创建名为 `user` 的 Kitex 服务
- `../idl/user.thrift`: IDL 文件路径

**生成的文件结构：**

```
rpc-user/
├── handler.go           # 【需要实现】业务逻辑入口
├── main.go              # 【需要修改】服务启动文件
├── kitex_gen/           # 【自动生成】RPC 框架代码
│   └── user/
│       ├── user.go      # Thrift 结构体定义
│       └── userservice/ # RPC Client/Server 接口
└── go.mod
```

#### 1.3 数据库配置

**创建配置文件：**

```bash
# 在 rpc-user/ 目录下创建配置目录
mkdir -p conf internal/db internal/utils

# 创建环境变量文件
touch .env .env.example
```

**`.env` 文件内容：**

```properties
# 数据库连接配置
DB_DSN=video_user:lzzy136994@tcp(127.0.0.1:3306)/video_platform?charset=utf8mb4&parseTime=True&loc=Local

# JWT 密钥
JWT_SECRET=mysecretkey

# Etcd 服务地址
ETCD_ADDRESS=127.0.0.1:2379
```

**`.env.example` 文件内容（不包含敏感信息）：**

```properties
DB_DSN=your_username:your_password@tcp(127.0.0.1:3306)/video_platform?charset=utf8mb4&parseTime=True&loc=Local
JWT_SECRET=your_secret_key_here
ETCD_ADDRESS=127.0.0.1:2379
```

**添加到 .gitignore：**

```bash
# 确保敏感信息不会被提交
echo ".env" >> .gitignore
```

#### 1.4 实现数据层

**文件：`conf/config.go`**

- 功能：初始化数据库连接，自动迁移表结构
- 核心逻辑：
  - 使用 GORM 连接 MySQL
  - 自动创建 `users` 表
  - 配置连接池（10 空闲连接，100 最大连接）

**文件：`internal/db/user.go`**

- 功能：用户数据模型和数据库操作
- 包含方法：
  - `CreateUser(username, password)`: 创建用户
  - `GetUserByUsername(username)`: 根据用户名查询用户

**文件：`internal/utils/auth.go`**

- 功能：密码加密和 JWT 处理
- 核心函数：
  - `HashPassword(password)`: 使用 bcrypt 加密密码
  - `CheckPasswordHash(password, hash)`: 验证密码
  - `GenerateToken(userID, username)`: 生成 JWT Token
  - `ParseToken(tokenString)`: 解析 JWT Token

**关键技术点：**

- 使用 `bcrypt` 进行密码哈希（成本因子 14）
- JWT Token 有效期 24 小时
- 密钥从环境变量读取，避免硬编码

#### 1.5 实现业务逻辑

**文件：`handler.go`**

实现两个 RPC 接口：

**Register 接口（注册）：**

```go
func (s *UserServiceImpl) Register(ctx context.Context, req *user.RegisterReq) (resp *user.RegisterResp, err error) {
    // 1. 密码加密
    hashedPassword, err := utils.HashPassword(req.Password)
  
    // 2. 创建用户
    userID, err := db.CreateUser(req.Username, hashedPassword)
  
    // 3. 返回结果（CloudWeGo 标准返回格式）
    return &user.RegisterResp{
        Code:   200,
        Msg:    "注册成功",
        UserId: int64(userID),
    }, nil
}
```

**Login 接口（登录）：**

```go
func (s *UserServiceImpl) Login(ctx context.Context, req *user.LoginReq) (resp *user.LoginResp, err error) {
    // 1. 查询用户
    existingUser, err := db.GetUserByUsername(req.Username)
  
    // 2. 验证密码
    if !utils.CheckPasswordHash(req.Password, existingUser.Password) {
        return &user.LoginResp{
            Code:   401,
            Msg:    "密码错误",
            UserId: 0,
        }, nil  // 注意：业务错误返回 nil，不是 err
    }
  
    // 3. 返回 UserID（Token 由网关生成）
    return &user.LoginResp{
        Code:   200,
        Msg:    "登录成功",
        UserId: int64(existingUser.ID),
    }, nil
}
```

**CloudWeGo 返回值规范：**

- **成功**：返回 `(response, nil)`，response.Code = 200
- **业务错误**：返回 `(response, nil)`，response.Code = 400/401/404
- **系统错误**：返回 `(response, err)`，response.Code = 500

#### 1.6 配置服务注册

**文件：`main.go`**

```go
func main() {
    // 1. 加载环境变量
    godotenv.Load()
  
    // 2. 初始化数据库
    if err := conf.InitDB(os.Getenv("DB_DSN")); err != nil {
        log.Fatalf("数据库连接失败: %v", err)
    }
  
    // 3. 创建 Etcd 注册中心
    r, err := etcd.NewEtcdRegistry([]string{os.Getenv("ETCD_ADDRESS")})
  
    // 4. 创建 Kitex Server（核心配置）
    svr := user.NewServer(
        new(UserServiceImpl),
        server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
            ServiceName: "user",  // ← 服务名，网关通过这个名字调用
        }),
        server.WithRegistry(r),   // ← 注册到 Etcd
    )
  
    // 5. 启动服务
    svr.Run()
}
```

**With* 函数模式解析：**

- `WithServerBasicInfo`: 设置服务元信息（服务名用于服务发现）
- `WithRegistry`: 指定注册中心（Etcd）
- `WithServiceAddr`: 指定监听地址（可选，默认随机端口）

#### 1.7 安装依赖

```bash
# 在 rpc-user/ 目录下执行
go get github.com/joho/godotenv                # 环境变量加载
go get github.com/kitex-contrib/registry-etcd  # Etcd 服务注册
go get golang.org/x/crypto/bcrypt              # 密码加密
go get github.com/golang-jwt/jwt/v5            # JWT 处理
go get gorm.io/gorm                            # ORM 框架
go get gorm.io/driver/mysql                    # MySQL 驱动

# 整理依赖
go mod tidy
```

#### 1.8 启动测试

```bash
# 启动服务
go run .

# 预期输出：
# 2026/02/05 19:23:21 数据库连接成功
# 2026/02/05 19:23:21 用户服务启动中...
# 2026/02/05 19:23:21 [Info] KITEX: server listen at addr=[::]:8888
# 2026/02/05 19:23:23 [Info] start keepalive lease xxx for etcd registry
```

**验证服务是否启动：**

```bash
# 检查端口
netstat -an | grep 8888

# 应看到：
# tcp46  0  0  *.8888  *.*  LISTEN
```

#### 1.9 提交代码

```bash
# 查看修改
git status

# 添加到暂存区
git add .

# 提交（使用规范的 commit message）
git commit -m "feat: 完成 User 微服务开发

- 实现 Register 和 Login RPC 接口
- 集成 Etcd 服务注册与发现
- 添加数据库连接和用户模型
- 实现密码哈希和 JWT 工具函数
- 配置环境变量支持 (.env)
- 完善 Learning.md 文档（Etcd、With* 模式详解）
- 服务已启动并成功注册到 Etcd"
```

#### 1.10 知识点总结

**核心技术栈：**

- **Kitex**: 字节跳动高性能 RPC 框架
- **Thrift**: 接口定义语言（IDL）
- **Etcd**: 分布式键值存储，用于服务注册发现
- **GORM**: Go 语言 ORM 框架
- **bcrypt**: 密码哈希算法
- **JWT**: JSON Web Token 认证

**微服务设计原则：**

1. **单一职责**：User 服务只负责用户相关逻辑
2. **无状态**：服务不保存用户会话，通过 JWT 传递身份
3. **服务发现**：通过 Etcd 动态发现服务地址
4. **配置分离**：敏感配置通过环境变量管理
5. **错误分层**：区分业务错误（400）和系统错误（500）

**下一步计划：**

- 开发 API Gateway（Hertz）
- 实现 HTTP 到 RPC 的协议转换
- 添加 JWT 鉴权中间件
- 测试端到端调用链路

---

### 阶段二：Gateway 服务开发（已完成 ✅）

#### 2.1 初始化 Gateway 项目

**生成 Hertz 脚手架：**

```bash
# 在项目根目录下
mkdir gateway && cd gateway

# 使用 hz 工具生成 HTTP 服务脚手架
hz new -module video-platform-microservice/gateway

# 初始化依赖
go mod tidy
```

**目录结构：**

```
gateway/
├── main.go              # 服务启动入口
├── router.go            # 路由配置
├── biz/
│   ├── handler/        # HTTP 处理器
│   │   ├── ping.go
│   │   └── user/       # 用户相关处理器
│   │       ├── register.go
│   │       └── login.go
│   └── router/
├── rpc/                # RPC 客户端
│   └── init.go         # RPC 客户端初始化
└── go.mod
```

#### 2.2 修复模块导入问题

**问题：** Gateway 需要导入 `rpc-user` 生成的 Kitex 代码，但遇到模块路径不匹配。

**解决方案：统一模块命名**

1. **修改 `rpc-user/go.mod` 模块名：**
```go
module video-platform-microservice/rpc-user  // 统一命名规范
```

2. **更新 `rpc-user` 中的所有导入路径：**
```bash
# 更新以下文件中的导入路径
# - main.go
# - handler.go
# - conf/config.go
# 从 "rpc-user/..." 改为 "video-platform-microservice/rpc-user/..."
```

3. **重新生成 Kitex 代码（关键步骤）：**
```bash
cd rpc-user
kitex -module video-platform-microservice/rpc-user \
      -service user \
      ../idl/user.thrift
```

4. **在 `gateway/go.mod` 中添加依赖：**
```go
require (
    video-platform-microservice/rpc-user v0.0.0
)

replace video-platform-microservice/rpc-user => ../rpc-user
```

5. **清理依赖：**
```bash
cd gateway && go mod tidy
```

#### 2.3 实现 RPC 客户端初始化

**创建 `gateway/rpc/init.go`：**

```go
package rpc

import (
	"log"

	"github.com/cloudwego/kitex/client"
	etcd "github.com/kitex-contrib/registry-etcd"
	"video-platform-microservice/rpc-user/kitex_gen/user/userservice"
)

var UserClient userservice.Client

// InitRPC 初始化所有 RPC 客户端
func InitRPC() {
	// 创建 Etcd 服务发现解析器
	r, err := etcd.NewEtcdResolver([]string{"127.0.0.1:2379"})
	if err != nil {
		log.Fatalf("创建 Etcd 解析器失败: %v", err)
	}

	// 初始化 User 服务客户端
	UserClient, err = userservice.NewClient("user", client.WithResolver(r))
	if err != nil {
		log.Fatalf("初始化 User 客户端失败: %v", err)
	}

	log.Println("RPC 客户端初始化成功")
}
```

**关键点：**
- `etcd.NewEtcdResolver`: 从 Etcd 中发现服务
- `userservice.NewClient("user", ...)`: 服务名 "user" 需与 rpc-user 注册时一致
- `client.WithResolver(r)`: 启用服务发现

**安装依赖：**
```bash
cd gateway
go get github.com/kitex-contrib/registry-etcd
go mod tidy
```

#### 2.4 修改 Gateway 启动入口

**编辑 `gateway/main.go`：**

```go
package main

import (
	"video-platform-microservice/gateway/rpc"

	"github.com/cloudwego/hertz/pkg/app/server"
)

func main() {
	// 初始化 RPC 客户端（连接到 User 服务）
	rpc.InitRPC()

	// 创建 Hertz 服务器（监听 8080 端口）
	h := server.Default(server.WithHostPorts(":8080"))

	// 注册路由
	register(h)
	
	// 启动服务
	h.Spin()
}
```

**端口分配：**
- Gateway (Hertz)：8080
- User Service (Kitex)：8888

#### 2.5 实现 HTTP 处理器

**创建 `gateway/biz/handler/user/register.go`：**

```go
package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"video-platform-microservice/gateway/rpc"
	"video-platform-microservice/rpc-user/kitex_gen/user"
)

// RegisterHandler 处理用户注册请求
func RegisterHandler(ctx context.Context, c *app.RequestContext) {
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

	// 调用 User 服务的 Register RPC 方法
	resp, err := rpc.UserClient.Register(ctx, &user.RegisterReq{
		Username: req.Username,
		Password: req.Password,
	})

	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]interface{}{
			"code": 500,
			"msg":  "RPC 调用失败: " + err.Error(),
		})
		return
	}

	// 返回响应
	c.JSON(consts.StatusOK, map[string]interface{}{
		"code":    resp.Code,
		"msg":     resp.Msg,
		"user_id": resp.UserId,
	})
}
```

**创建 `gateway/biz/handler/user/login.go`：**

```go
package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"video-platform-microservice/gateway/rpc"
	"video-platform-microservice/rpc-user/kitex_gen/user"
)

// LoginHandler 处理用户登录请求
func LoginHandler(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

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
			"msg":  "RPC 调用失败: " + err.Error(),
		})
		return
	}

	// 返回响应（包含 JWT token）
	c.JSON(consts.StatusOK, map[string]interface{}{
		"code":    resp.Code,
		"msg":     resp.Msg,
		"user_id": resp.UserId,
		"token":   resp.Token,
	})
}
```

#### 2.6 配置路由

**编辑 `gateway/router.go`：**

```go
package main

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	handler "video-platform-microservice/gateway/biz/handler"
	userHandler "video-platform-microservice/gateway/biz/handler/user"
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
```

#### 2.7 编译和测试

**编译 Gateway：**

```bash
cd gateway
go build .
```

**启动服务（确保 rpc-user 已运行）：**

```bash
# 终端 1: 启动 User 服务
cd rpc-user && ./rpc-user

# 终端 2: 启动 Gateway
cd gateway && ./gateway
```

**测试 API：**

```bash
# 测试注册接口
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser123","password":"password123"}'

# 预期响应：
# {"code":200,"msg":"注册成功","user_id":1}

# 测试登录接口
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser123","password":"password123"}'

# 预期响应：
# {"code":200,"msg":"登录成功","token":"","user_id":1}

# 测试错误密码
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser123","password":"wrongpassword"}'

# 预期响应：
# {"code":401,"msg":"密码错误","token":"","user_id":0}
```

#### 2.8 架构总结

**服务调用链路：**

```
客户端 (curl/浏览器)
    ↓ HTTP (8080)
Gateway (Hertz)
    ↓ RPC (Kitex)
User Service (Kitex) ← 注册到 Etcd (2379)
    ↓
MySQL (3306)
```

**关键技术点：**

1. **服务发现：** Gateway 通过 Etcd 动态发现 User 服务地址
2. **协议转换：** Gateway 将 HTTP 请求转换为 Kitex RPC 调用
3. **错误处理：** 统一的状态码和错误消息格式
4. **模块管理：** 使用 `replace` 指令实现本地依赖

**当前架构优势：**

- ✅ 服务解耦：Gateway 和 User 服务可独立部署和扩展
- ✅ 高性能：Hertz + Kitex 都是字节跳动优化的高性能框架
- ✅ 可扩展：轻松添加新的微服务（Video、Comment 等）
- ✅ 容错性：Etcd 提供服务健康检查和自动故障转移

#### 2.9 常见问题解决

**问题 1：导入路径错误**
```
error: package rpc-user/kitex_gen/user is not in std
```

**解决：** 确保执行以下步骤：
1. 修改 `rpc-user/go.mod` 模块名
2. 更新所有导入路径
3. **重新运行 `kitex` 命令生成代码**（关键）
4. 在 gateway 中执行 `go mod tidy`

**问题 2：端口冲突**
```
panic: listen tcp :8888: bind: address already in use
```

**解决：** Gateway 和 User 服务使用不同端口：
- User Service: 8888
- Gateway: 8080

**问题 3：RPC 调用失败**
```
RPC 调用失败: no instance remains for service...
```

**解决：** 检查以下项：
1. Etcd 是否正常运行（`lsof -i :2379`）
2. User 服务是否成功注册到 Etcd
3. Gateway 的 Etcd 地址配置是否正确

---
