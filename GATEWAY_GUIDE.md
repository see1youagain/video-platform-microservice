# API Gateway 开发指南

## 📚 开发前必读

### 当前状态检查

你已经完成了：
- ✅ User 微服务开发（监听 8888 端口）
- ✅ Etcd 服务启动（监听 2379 端口）
- ✅ Gateway 项目脚手架生成

你的 Gateway 目录结构：
```
gateway/
├── main.go              # 服务启动入口（已生成）
├── router.go            # 自定义路由注册
├── router_gen.go        # hz 生成的路由
├── biz/
│   ├── handler/
│   │   └── ping.go      # 默认生成的 ping 接口
│   └── router/
│       └── register.go  # 路由注册逻辑
└── go.mod
```

---

## 🎯 开发目标

实现以下功能：
1. 初始化 RPC Client（连接 User 服务）
2. 实现 `/api/register` 接口
3. 实现 `/api/login` 接口
4. （可选）实现 JWT 鉴权中间件

---

## 📖 理论知识回顾

### 1. Gateway 的工作原理

```
┌─────────────┐
│   浏览器     │ POST /api/register
│  (Postman)   │ {"username": "alice", "password": "123"}
└──────┬──────┘
       │ HTTP 请求（JSON）
       ▼
┌─────────────────────────┐
│  Hertz Gateway (8080)   │
│  1. 解析 JSON           │
│  2. 验证参数            │
│  3. 调用 RPC Client     │
└──────┬──────────────────┘
       │ RPC 调用（Thrift Binary）
       │ UserClient.Register(req)
       ▼
┌─────────────────────────┐
│  Kitex User 服务 (8888) │
│  1. 哈希密码            │
│  2. 写入数据库          │
│  3. 返回 user_id        │
└──────┬──────────────────┘
       │ RPC 响应
       ▼
┌─────────────────────────┐
│  Hertz Gateway          │
│  1. 接收 RPC 结果       │
│  2. 组装 HTTP 响应      │
└──────┬──────────────────┘
       │ HTTP 响应（JSON）
       │ {"code": 200, "user_id": 1}
       ▼
┌─────────────┐
│   浏览器     │
└─────────────┘
```

### 2. RPC Client 的核心概念

**什么是 RPC Client？**
- 它是一个"远程函数调用器"
- 你在 Gateway 中调用 `UserClient.Register()`
- 实际执行在 User 服务的 `handler.go` 中
- 通过网络传输（TCP + Thrift 协议）

**为什么需要服务发现（Etcd）？**
```go
// ❌ 硬编码方式（不推荐）
client.WithHostPorts("127.0.0.1:8888")  
// 问题：User 服务重启、换 IP、扩容时都需要改代码

// ✅ 服务发现方式（推荐）
client.WithResolver(etcdResolver)
// Kitex 自动从 Etcd 获取最新的服务地址列表
```

### 3. Hertz Handler 的标准写法

**函数签名：**
```go
func HandlerName(ctx context.Context, c *app.RequestContext) {
    // ctx: RPC 调用的上下文（超时控制、链路追踪）
    // c: HTTP 请求上下文（类似 Gin 的 c *gin.Context）
}
```

**三个核心步骤：**
```go
func Register(ctx context.Context, c *app.RequestContext) {
    // 1. 解析请求参数
    var req api.RegisterRequest
    err := c.BindAndValidate(&req)
    if err != nil {
        c.JSON(400, utils.H{"error": "参数错误"})
        return
    }
    
    // 2. 调用 RPC 服务
    resp, err := rpc.UserClient.Register(ctx, &user.RegisterReq{
        Username: req.Username,
        Password: req.Password,
    })
    if err != nil {
        c.JSON(500, utils.H{"error": "RPC 调用失败"})
        return
    }
    
    // 3. 返回响应
    c.JSON(resp.Code, utils.H{
        "msg": resp.Msg,
        "user_id": resp.UserId,
    })
}
```

---

## 🛠️ 开发步骤（跟着做）

### 步骤 1：创建 RPC 目录结构

```bash
cd gateway
mkdir -p rpc
```

**解释：** 创建一个 `rpc/` 目录来统一管理所有 RPC Client。

---

### 步骤 2：初始化 User Client

**创建文件：`gateway/rpc/init.go`**

你需要思考的问题：
1. 如何引入 User 服务的 Kitex 生成代码？
2. Etcd 的地址是什么？（答案：`127.0.0.1:2379`）
3. 服务名是什么？（答案：`"user"`，必须与 User 服务注册时一致）

**代码模板（你需要填空）：**
```go
package rpc

import (
    "log"
    
    "github.com/cloudwego/kitex/client"
    etcd "github.com/kitex-contrib/registry-etcd"
    
    // TODO 1: 引入 User 服务的 kitex_gen 包
    // 提示：路径是 "你的module名/rpc-user/kitex_gen/user/userservice"
    "???"
)

var UserClient ??? // TODO 2: 填写 Client 类型

func InitRPC() {
    // TODO 3: 创建 Etcd Resolver
    r, err := etcd.NewEtcdResolver([]string{???})
    if err != nil {
        log.Fatalf("创建 Etcd Resolver 失败: %v", err)
    }
    
    // TODO 4: 创建 User Client
    UserClient, err = userservice.NewClient(
        ???,  // 服务名
        client.WithResolver(r),
    )
    if err != nil {
        log.Fatalf("创建 User Client 失败: %v", err)
    }
    
    log.Println("RPC Client 初始化成功")
}
```

**提示：**
- 查看 `rpc-user/kitex_gen/user/userservice/` 目录
- 里面有 `client.go`，说明包名是 `userservice`
- Client 类型应该是 `userservice.Client`

---

### 步骤 3：在 main.go 中调用初始化

**修改文件：`gateway/main.go`**

**当前代码：**
```go
func main() {
    h := server.Default()
    register(h)
    h.Spin()
}
```

**你需要做什么？**
- 在 `h := server.Default()` 之后调用 `rpc.InitRPC()`
- 引入 `"你的项目/gateway/rpc"` 包

**修改后的代码（你来写）：**
```go
package main

import (
    "github.com/cloudwego/hertz/pkg/app/server"
    // TODO: 引入 rpc 包
)

func main() {
    h := server.Default()
    
    // TODO: 初始化 RPC Client
    
    register(h)
    h.Spin()
}
```

---

### 步骤 4：创建 User Handler

**创建文件：`gateway/biz/handler/user/register.go`**

你需要创建目录：
```bash
mkdir -p gateway/biz/handler/user
```

**代码框架：**
```go
package user

import (
    "context"
    
    "github.com/cloudwego/hertz/pkg/app"
    "github.com/cloudwego/hertz/pkg/protocol/consts"
    
    // TODO: 引入 rpc 包和 User 服务的结构体
)

// RegisterRequest 定义 HTTP 请求参数
type RegisterRequest struct {
    Username string `json:"username" vd:"len($)>0"`  // vd 是 Hertz 的验证标签
    Password string `json:"password" vd:"len($)>=6"` // 密码至少 6 位
}

func Register(ctx context.Context, c *app.RequestContext) {
    // TODO 1: 解析请求参数
    var req RegisterRequest
    err := c.BindAndValidate(&req)
    if err != nil {
        c.JSON(consts.StatusBadRequest, map[string]interface{}{
            "error": "参数错误: " + err.Error(),
        })
        return
    }
    
    // TODO 2: 调用 RPC 服务
    // 提示：使用 rpc.UserClient.Register(ctx, &user.RegisterReq{...})
    
    // TODO 3: 处理 RPC 返回结果
    // 如果 err != nil，说明 RPC 调用失败
    // 如果 resp.Code != 200，说明业务逻辑失败
    
    // TODO 4: 返回 HTTP 响应
    c.JSON(???, map[string]interface{}{
        "code": resp.Code,
        "msg": resp.Msg,
        "user_id": resp.UserId,
    })
}
```

---

### 步骤 5：注册路由

**修改文件：`gateway/router.go`**

**当前代码（可能为空或只有注释）：**
```go
package main

// 自定义路由注册
func customizedRegister(h *server.Hertz) {
    // 你的路由
}
```

**你需要添加：**
```go
package main

import (
    "github.com/cloudwego/hertz/pkg/app/server"
    
    // TODO: 引入 user handler
    "你的项目/gateway/biz/handler/user"
)

func customizedRegister(h *server.Hertz) {
    // 用户相关路由
    apiGroup := h.Group("/api")
    {
        apiGroup.POST("/register", user.Register)
        // apiGroup.POST("/login", user.Login)  // 稍后实现
    }
}
```

**然后在 `main.go` 中调用：**
```go
func main() {
    h := server.Default()
    rpc.InitRPC()
    
    register(h)
    customizedRegister(h)  // ← 加上这一行
    
    h.Spin()
}
```

---

### 步骤 6：安装依赖

```bash
cd gateway

# 安装 Etcd 依赖
go get github.com/kitex-contrib/registry-etcd

# 引入 User 服务的 kitex_gen（重要！）
# 方法1：如果在同一个 workspace（推荐）
go mod edit -replace video-platform-microservice/rpc-user=../rpc-user

# 方法2：直接引用
go get video-platform-microservice/rpc-user

# 整理依赖
go mod tidy
```

---

### 步骤 7：测试运行

**启动 User 服务（如果还没启动）：**
```bash
cd rpc-user
go run .
```

**启动 Gateway：**
```bash
cd gateway
go run .

# 预期输出：
# 2026/02/05 XX:XX:XX RPC Client 初始化成功
# 2026/02/05 XX:XX:XX [Info] HERTZ: HTTP server listening on address=[::]:8080
```

**测试 Register 接口：**
```bash
# 使用 curl 测试
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username": "alice", "password": "123456"}'

# 预期返回：
# {"code":200,"msg":"注册成功","user_id":1}
```

---

## 🐛 常见错误与解决

### 错误 1: `cannot find package`

**错误信息：**
```
build command-line-arguments: cannot find package "video-platform-microservice/rpc-user/kitex_gen/user/userservice"
```

**原因：** Gateway 的 go.mod 找不到 User 服务的包。

**解决方案：**
```bash
cd gateway

# 查看当前 module 名
grep "module" go.mod

# 如果两个项目在同一个目录下，使用 replace
go mod edit -replace video-platform-microservice/rpc-user=../rpc-user

go mod tidy
```

---

### 错误 2: `connection refused`

**错误信息：**
```
RPC 调用失败: dial tcp 127.0.0.1:8888: connect: connection refused
```

**原因：** User 服务没有启动。

**解决方案：**
```bash
# 打开新终端
cd rpc-user
go run .
```

---

### 错误 3: `service not found`

**错误信息：**
```
创建 User Client 失败: no instance remains for discovery
```

**原因：** User 服务注册的服务名与 Gateway 查询的不一致。

**检查方法：**
```bash
# 检查 User 服务的 main.go
grep "ServiceName" rpc-user/main.go
# 应该看到：ServiceName: "user"

# 检查 Gateway 的 rpc/init.go
grep "NewClient" gateway/rpc/init.go
# 第一个参数应该是 "user"
```

---

## ✅ 验收标准

完成以下所有测试，说明你成功了：

### 测试 1: 注册新用户
```bash
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username": "bob", "password": "111111"}'
```

**预期结果：**
```json
{
  "code": 200,
  "msg": "注册成功",
  "user_id": 2
}
```

### 测试 2: 重复注册（应该失败）
```bash
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username": "bob", "password": "222222"}'
```

**预期结果：**
```json
{
  "code": 400,
  "msg": "用户名可能已存在"
}
```

### 测试 3: 参数验证
```bash
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username": "charlie", "password": "123"}'
```

**预期结果：**
```json
{
  "error": "参数错误: Key: 'RegisterRequest.Password' Error:Field validation..."
}
```

---

## 📝 下一步任务

当你完成 Register 接口后，可以继续实现：

### Task 1: 实现 Login 接口

**文件：`gateway/biz/handler/user/login.go`**

**核心逻辑：**
1. 调用 `rpc.UserClient.Login()`
2. 如果成功，在 Gateway 生成 JWT Token
3. 返回 Token 和 user_id

**需要学习的新知识：**
- 如何在 Gateway 生成 JWT？
- JWT 密钥应该放在哪里？
- Token 过期时间怎么设置？

### Task 2: 实现 JWT 中间件

**目标：** 保护需要登录才能访问的接口（如上传视频）。

**核心逻辑：**
1. 从 HTTP Header 中提取 `Authorization: Bearer <token>`
2. 验证 Token 是否有效
3. 解析出 `user_id` 并写入 Context
4. 后续 Handler 可以从 Context 读取 `user_id`

---

## 💡 学习建议

### 1. 先理解，再动手
- 不要复制粘贴代码
- 每一行都要知道它在做什么
- 遇到不懂的函数，用 `godoc` 查看文档

### 2. 调试技巧
```go
// 在关键位置打印日志
log.Printf("收到注册请求: username=%s", req.Username)
log.Printf("RPC 返回: code=%d, msg=%s", resp.Code, resp.Msg)
```

### 3. 阅读源码
- 打开 `rpc-user/kitex_gen/user/userservice/client.go`
- 看看 `NewClient` 函数做了什么
- 理解 Kitex 的工作原理

---

## 🎯 自我检查清单

在向我提问之前，先检查这些：

- [ ] User 服务是否在运行？（`lsof -i :8888`）
- [ ] Etcd 是否在运行？（`lsof -i :2379`）
- [ ] Gateway 是否启动成功？（看到 "listening on address" 日志）
- [ ] 是否执行了 `go mod tidy`？
- [ ] `rpc/init.go` 中的服务名是否正确？
- [ ] 路由是否正确注册到 `/api/register`？

---

## ❓ 现在轮到你了

请告诉我：

1. **你理解了哪些概念？**
   - 比如："RPC Client 初始化"、"服务发现原理"、"Handler 写法"

2. **你想先完成哪个步骤？**
   - 步骤 2: 初始化 User Client
   - 步骤 4: 创建 Register Handler
   - 步骤 5: 注册路由
   - 其他

3. **你遇到了什么问题？**
   - 具体的错误信息
   - 哪一步卡住了

**记住：我不会直接给你完整代码，而是引导你思考、填空、调试，这样你才能真正掌握！** 🚀
