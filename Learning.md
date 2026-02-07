## CloudWeGo（Hertz+Kitex）经典架构和核心逻辑

### 经典的Hertz网关架构（api-gateway）

#### **Hertz 是 Gin 的“同类竞品”，它是完全的替代者。**

* **属性**：Hertz 和 Gin 一样，都是 **HTTP Web 框架**。
* **功能**：它们都负责监听端口（如 8080）、解析 HTTP 请求（GET/POST）、处理路由、解析 JSON 参数、返回结果给前端。
* **关系**：在新架构中，Hertz **完全替代** Gin。不再需要在项目中引入 Gin。

#### **为什么叫“网关架构”？**

在单体应用（单机版的代码）中，Gin 是**全能店长**，它既负责接待客人（解析 HTTP），又负责炒菜（处理业务逻辑 `internal/logic`），还负责记账（读写数据库 `internal/db`）。

而在微服务架构中，Hertz 退化成了 **“前台接待（网关 Gateway）”**：

1. **只负责接待**：Hertz 依然监听 8080 端口，接收前端发来的 HTTP 请求。
2. **不负责炒菜**：Hertz 接到请求后，不再自己计算逻辑，也不连数据库。
3. **转交任务**：Hertz 把请求参数打包，通过电话（RPC）打给后厨（Kitex 微服务）。

**Hertz vs Gin 的区别（技术层面）：**

* **底层核心**：Gin 基于 Go 原生的 `net/http` 库。Hertz 基于字节跳动自研的 `Netpoll` 网络库（类似于 Linux 的 epoll），在超高并发下性能更强，延迟更低。
* **API 风格**：非常相似。
  * Gin: `c.JSON(200, obj)`
  * Hertz: `c.JSON(200, obj)` (几乎无缝迁移)

```plaintext
api-gateway/
├── biz/                <-- 【核心】业务逻辑层
│   ├── handler/        <-- 控制器 (Controller)，处理 HTTP 请求
│   │   ├── user/       <-- 对应 user 路由组的逻辑
│   │   └── upload/   
│   ├── router/         <-- 路由注册 (生成的，通常不需要手动改)
│   ├── model/          <-- 根据 IDL 生成的 Go 结构体 (Request/Response)
│   └── service/        <-- (可选) 如果网关有复杂逻辑，可以加这一层
├── idl/                <-- 存放 .thrift 文件
├── script/             <-- 启动脚本
├── go.mod
├── main.go             <-- 程序入口，注册中间件，初始化 RPC Client
└── router.go           <-- 自定义路由扩展点
```

**经典的逻辑流：**

1. **Request** 到达 `main.go`。
2. 进入 **Middleware** (如 JWT 鉴权)。
3. 进入 `biz/router` 匹配路由。
4. 进入 `biz/handler`：
   * 使用 `c.BindAndValidate` 解析请求参数。
   * **关键动作**：调用全局初始化的 **RPC Client** (如 `UserClient`) 发送请求。
   * 处理 RPC 返回的错误或结果。
   * 使用 `c.JSON` 返回 HTTP 响应。

### 经典的 Kitex 微服务架构 (`rpc-user` / `rpc-video`)

#### **Kitex 是 Gin 的“下游合作伙伴”，它替代的是你原来的 `internal/logic` 层。**

* **属性**：Kitex 是一个 **RPC（远程过程调用）框架**。
* **它对于 Gin 是什么？**
  * 它**不是** Gin 的替代品（Kitex 听不懂 HTTP 协议，浏览器不能直接访问 Kitex）。
  * 它是 Gin（或 Hertz）的\*\*“后方支援”\*\*。

#### **核心差异：HTTP vs RPC**

* **Gin (HTTP)**：像\*\*“写信”\*\*。格式是文本（JSON），臃肿，每次都要握手，适合给浏览器看。
* **Kitex (RPC/Thrift)**：像\*\*“电报”\*\*。格式是二进制（Binary），极小，传输极快，适合服务器之间内部沟通。

#### **架构视角的转变**

**以前（Gin 单体）：**

> **用户** --> (HTTP) --> **Gin** --> (函数调用) --> **Service/Logic 代码** --> **DB**
>
> * *Gin 和业务逻辑在一个进程里，直接调函数。*

**以后（Hertz + Kitex 微服务）：**

> **用户** --> (HTTP) --> **Hertz (网关)** --> **[网络/RPC协议]** --> **Kitex (微服务)** --> **DB**
>
> * *Hertz 和 Kitex 在不同的进程（甚至不同的电脑）上。*
> * *Hertz 不能直接调函数，必须通过网络发 RPC 请求给 Kitex。*


```plaintext
rpc-user/
├── kitex_gen/          <-- 【核心】生成的 RPC 底层代码 (勿动)
│   └── user/           <-- 包含 struct 定义、Client 接口、Server 接口
├── conf/               <-- 配置文件
├── handler.go          <-- 【重点】你需要写代码的地方！实现 IDL 定义的接口
├── main.go             <-- 程序入口，初始化 Server，注册到 ETCD
├── script/             <-- 启动脚本
├── build.sh            <-- 构建脚本
└── go.mod

// 对于开发rpc-user的时候
rpc-user/
├── main.go              # 【1】服务启动入口（像单机版的 cmd/server/main.go）
├── handler.go           # 【2】业务逻辑处理（像单机版的 internal/handler/user.go）
├── conf/
│   └── config.go        # 【3】配置文件（读取数据库连接等）
├── internal/
│   ├── db/              # 【4】数据库相关代码
│   │   └── user.go      # 用户表操作（增删改查）
│   └── utils/           # 【5】工具函数
│       └── auth.go      # 密码加密、JWT 生成
└── kitex_gen/           # 【自动生成，别动】Thrift 生成的代码
    └── user/
        └── user.go      # 你看到的那个文件（定义了 RegisterReq 等结构体）
```

**经典的逻辑流：**

1. **Server 启动**：`main.go` 启动，连接 ETCD，监听端口。
2. **RPC 请求到达**：Kitex 框架反序列化数据。
3. **进入 Handler**：`handler.go` 中的方法（如 `Register`）被调用。
4. **业务逻辑**：在 `handler.go` 中调用 Database (GORM) 或 Redis。
5. **返回结果**：Return 结构体，Kitex 自动序列化并返回给网关。

### 必要重点

#### 1. 上下文透传 (Context Propagation)

这是微服务最容易出错的地方。

* **问题**：网关解析了 JWT 拿到 `user_id`，怎么传给 `rpc-video` 服务？
* **Gin 的做法**：`c.Set("uid", 123)`，但这只在当前进程有效。
* **CloudWeGo 的做法**：使用 **Metainfo**。
  * **网关侧 (Hertz)**：

    ```Go
    // 将数据写入 RPC 的元信息中
    ctx = metainfo.WithPersistentValue(ctx, "user_id", "1001")
    VideoClient.UploadChunk(ctx, req)
    ```
  * **服务侧 (Kitex)**：

    ```Go
    // 从 context 中读出元信息
    uid, ok := metainfo.GetPersistentValue(ctx, "user_id")
    ```

#### 2. 错误处理 (Error Handling)

* **RPC 错误**：如果是网络断了，RPC Client 会返回 error。
* **业务错误**：如果是“密码错误”，通常定义在 `Response.Code` 中，而不是返回 error。
* **重点**：不要把 GORM 的 error 直接通过 RPC 返回，要转换成定义好的 `code` 和 `msg`。

#### 3. IDL 变更管理

* **原则**：`idl/*.thrift` 是**唯一的真理来源**。
* **操作**：如果你修改了 thrift 文件，**必须**重新运行 `hz update` 或 `kitex -module ...` 命令来重新生成代码。不要试图手动修改 `kitex_gen` 或 `biz/model` 里的代码，下次生成会被覆盖。

---

## Etcd 与服务发现机制

### 什么是 Etcd？

**Etcd** 是一个分布式键值存储系统（类似一个"云端字典"），专门用于**配置管理**和**服务发现**。

#### 类比：电话簿系统

想象你在一个大公司工作：

* **以前（单体应用）**：所有部门在一栋楼里，你想找财务部，直接走到3楼302房间。
* **现在（微服务）**：各部门分散在全国各地，办公地址可能随时变动（服务器IP会变、端口会变、服务可能重启到新机器）。

**问题**：Hertz（网关）怎么知道去哪里找 `rpc-user` 服务？

**传统方案（硬编码）**：
```go
// ❌ 不好的做法
client, _ := user.NewClient("user", client.WithHostPorts("127.0.0.1:8888"))
```
**问题**：
- 如果 `rpc-user` 重启到 `192.168.1.100:9999`，代码要改
- 如果有3台 `rpc-user` 做负载均衡，怎么办？
- 如果某台机器宕机了，怎么自动切换？

**Etcd 方案（动态发现）**：
```go
// ✅ 好的做法
resolver, _ := etcd.NewEtcdResolver([]string{"127.0.0.1:2379"})
client, _ := user.NewClient("user", client.WithResolver(resolver))
```

### Etcd 的核心功能

#### 1. 服务注册（Service Registration）

当 `rpc-user` 服务启动时：

```go
// main.go
r, err := etcd.NewEtcdRegistry([]string{"127.0.0.1:2379"})
svr := user.NewServer(
    new(UserServiceImpl),
    server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
        ServiceName: "user",  // ← 关键：注册的服务名
    }),
    server.WithRegistry(r),   // ← 把自己注册到 Etcd
)
```

**Etcd 中会记录**：
```
服务名: user
地址: 192.168.1.10:8888
状态: 健康
时间戳: 2024-01-15 10:30:00
```

#### 2. 服务发现（Service Discovery）

当 Hertz 网关需要调用用户服务时：

```go
// api-gateway 中
resolver, _ := etcd.NewEtcdResolver([]string{"127.0.0.1:2379"})
userClient, _ := user.NewClient("user", client.WithResolver(resolver))

// 发起调用时，Kitex 自动：
// 1. 查询 Etcd："给我所有名为 'user' 的服务地址"
// 2. Etcd 返回：["192.168.1.10:8888", "192.168.1.11:8888"]
// 3. 选择一个地址（负载均衡）发送请求
```

#### 3. 健康检查（Health Check）

- 服务每隔几秒钟向 Etcd 发送**心跳**："我还活着"
- 如果某个服务崩溃了，Etcd 会自动把它从列表中删除
- 客户端下次查询时，就不会拿到已挂掉的服务地址

### 为什么微服务必须要 Etcd？

| 场景 | 没有 Etcd | 有 Etcd |
|------|-----------|---------|
| **服务地址变更** | 需要修改代码重新部署 | 自动更新，无需改代码 |
| **服务宕机** | 请求会失败，需要手动摘除 | 自动检测并移除 |
| **负载均衡** | 需要手动配置 Nginx | Kitex 自动轮询多个实例 |
| **动态扩容** | 需要修改配置文件 | 新服务启动自动加入 |

### Etcd 的替代品

- **Consul**：HashiCorp 出品，功能更丰富
- **Nacos**：阿里出品，支持配置中心+服务发现
- **ZooKeeper**：Apache 出品，老牌方案

它们的核心功能都一样：**让服务能够找到彼此**。

---

## Kitex 的 `With*` 函数模式详解

### 什么是 `With*` 模式？

这是 Go 语言中常见的**函数式选项模式（Functional Options Pattern）**。

#### 问题背景

创建一个 Kitex 服务器，可能有很多配置项：

```go
// ❌ 传统做法：参数列表会非常长
NewServer(handler, serviceName, address, port, registry, timeout, maxConnNum, ...)
```

**问题**：
- 参数太多，记不住顺序
- 很多参数是可选的，每次都要传空值
- 以后新增配置项，会破坏 API 兼容性

#### 解决方案：使用 `With*` 函数

```go
// ✅ 优雅的做法
svr := user.NewServer(
    new(UserServiceImpl),                   // 必填参数
    server.WithServerBasicInfo(...),        // 可选配置1
    server.WithRegistry(r),                 // 可选配置2
    server.WithServiceAddr(addr),           // 可选配置3
)
```

### 原理解析

#### 1. 核心定义

```go
// 定义一个配置函数类型
type Option func(*Options)

// 创建服务器时接收可变参数
func NewServer(handler, opts ...Option) Server {
    options := &Options{
        // 默认配置
        Port: 8888,
        Timeout: 30 * time.Second,
    }
    
    // 依次应用所有配置函数
    for _, opt := range opts {
        opt(options)
    }
    
    // 使用最终的配置创建服务器
    return &server{options: options}
}
```

#### 2. 每个 `With*` 函数

```go
// WithServiceAddr 设置服务监听地址
func WithServiceAddr(addr net.Addr) Option {
    return func(o *Options) {
        o.Address = addr
    }
}

// WithRegistry 设置服务注册中心
func WithRegistry(r registry.Registry) Option {
    return func(o *Options) {
        o.Registry = r
    }
}

// WithServerBasicInfo 设置服务基本信息
func WithServerBasicInfo(info *rpcinfo.EndpointBasicInfo) Option {
    return func(o *Options) {
        o.ServiceName = info.ServiceName
    }
}
```

### 常用的 `With*` 配置项

#### Server 端（服务提供者）

```go
svr := user.NewServer(
    new(UserServiceImpl),
    
    // 【必需】设置服务名（用于服务发现）
    server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
        ServiceName: "user",
    }),
    
    // 【推荐】注册到 Etcd
    server.WithRegistry(etcdRegistry),
    
    // 【可选】指定监听地址（默认随机端口）
    server.WithServiceAddr(addr),
    
    // 【可选】设置超时时间
    server.WithReadWriteTimeout(30 * time.Second),
    
    // 【可选】限制最大连接数
    server.WithLimit(&limit.Option{
        MaxConnections: 10000,
        MaxQPS:         5000,
    }),
    
    // 【可选】添加中间件
    server.WithMiddleware(func(next endpoint.Endpoint) endpoint.Endpoint {
        return func(ctx context.Context, req, resp interface{}) (err error) {
            log.Println("请求开始")
            err = next(ctx, req, resp)
            log.Println("请求结束")
            return err
        }
    }),
)
```

#### Client 端（服务调用者）

```go
client, err := user.NewClient(
    "user",  // 服务名
    
    // 【推荐】使用服务发现
    client.WithResolver(etcdResolver),
    
    // 【备选】直接指定地址（不推荐生产环境使用）
    // client.WithHostPorts("127.0.0.1:8888"),
    
    // 【可选】设置超时时间
    client.WithRPCTimeout(5 * time.Second),
    
    // 【可选】设置重试策略
    client.WithFailureRetry(retry.NewFailurePolicy()),
    
    // 【可选】负载均衡策略
    client.WithLoadBalancer(loadbalance.NewWeightedRandomBalancer()),
    
    // 【可选】熔断器
    client.WithCircuitBreaker(circuitbreak.NewCBSuite(...)),
)
```

### 为什么这么设计？

#### 优点

1. **可读性强**：每个配置项的名字一目了然
   ```go
   server.WithRegistry(r)          // 一看就知道是设置注册中心
   server.WithServiceAddr(addr)    // 一看就知道是设置监听地址
   ```

2. **灵活性高**：想加什么配置就加什么，不需要的就不写
   ```go
   // 最简配置
   svr := user.NewServer(handler)
   
   // 完整配置
   svr := user.NewServer(handler, opt1, opt2, opt3, ...)
   ```

3. **向后兼容**：新增配置项不会破坏老代码
   ```go
   // 旧代码（只有2个配置）
   svr := user.NewServer(handler, server.WithRegistry(r))
   
   // 新版本增加了 WithTracer，但旧代码依然能正常工作
   ```

4. **链式调用**：代码结构清晰
   ```go
   client, err := user.NewClient("user",
       client.WithResolver(resolver),
       client.WithRPCTimeout(5*time.Second),
       client.WithRetry(...),
   )
   ```

### 实战示例对比

#### 场景：从开发环境迁移到生产环境

**开发环境**（直连，不用 Etcd）：
```go
svr := user.NewServer(
    new(UserServiceImpl),
    server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
        ServiceName: "user",
    }),
    server.WithServiceAddr(&net.TCPAddr{Port: 8888}),
)
```

**生产环境**（需要服务发现、限流、监控）：
```go
svr := user.NewServer(
    new(UserServiceImpl),
    server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{
        ServiceName: "user",
    }),
    server.WithRegistry(etcdRegistry),              // ← 加上注册中心
    server.WithLimit(&limit.Option{                 // ← 加上限流
        MaxQPS: 10000,
    }),
    server.WithTracer(prometheus.NewServerTracer()), // ← 加上监控
)
```

**只需要增加配置项，核心代码不变！**

---

## 完整调用链路图

```
┌─────────────┐
│   浏览器     │
└──────┬──────┘
       │ HTTP: POST /api/login
       ▼
┌─────────────────────────┐
│  Hertz 网关 (8080端口)   │
│  - 解析 JSON            │
│  - JWT 鉴权 (可选)       │
└──────┬──────────────────┘
       │ RPC (Thrift Binary)
       │ 1. 查询 Etcd："user 服务在哪？"
       │ 2. Etcd 返回："192.168.1.10:8888"
       ▼
┌─────────────────────────┐
│  Kitex rpc-user 服务    │
│  - 执行 Login 逻辑       │
│  - 查询 MySQL           │
│  - 返回 user_id         │
└──────┬──────────────────┘
       │ RPC Response
       ▼
┌─────────────────────────┐
│  Hertz 网关             │
│  - 接收 RPC 结果         │
│  - 可能生成 JWT Token    │
│  - 组装 HTTP Response    │
└──────┬──────────────────┘
       │ HTTP: 200 OK + JSON
       ▼
┌─────────────┐
│   浏览器     │
└─────────────┘
```

---

## 常见问题 FAQ

### Q1: 为什么不能直接 HTTP 调用 Kitex？

**答**：Kitex 使用 Thrift 二进制协议，浏览器只认识 HTTP+JSON。就像你拿中文报纸给只懂英文的人看一样。

### Q2: Etcd 挂了怎么办？

**答**：
1. Kitex 有本地缓存，短时间内可以使用旧的服务列表
2. 生产环境应该部署 Etcd 集群（至少3个节点）保证高可用
3. 可以降级到硬编码地址（但会失去动态发现能力）

### Q3: 一个服务启动多个实例，怎么负载均衡？

**答**：
- 所有实例注册到 Etcd 时用**同一个服务名**（如 `user`）
- Kitex Client 会自动拿到所有实例的地址列表
- 使用负载均衡策略选择一个实例发送请求（默认是加权随机）

```go
// 启动3个 rpc-user 实例
// 实例1: 监听 127.0.0.1:8881，注册为 "user"
// 实例2: 监听 127.0.0.1:8882，注册为 "user"
// 实例3: 监听 127.0.0.1:8883，注册为 "user"

// Hertz 调用时
userClient.Login(ctx, req) // Kitex 自动选择一个实例
```

### Q4: `With*` 函数的执行顺序重要吗？

**答**：一般不重要，但有些特殊情况需要注意：

```go
// ❌ 错误示例：后面的配置会覆盖前面的
server.WithServiceAddr(addr1),  // 这个会被覆盖
server.WithServiceAddr(addr2),  // 最终使用这个
```

**建议**：每种配置只写一次，避免重复。

---

## 学习资源

- [CloudWeGo 官方文档](https://www.cloudwego.io/)
- [Kitex 教程](https://www.cloudwego.io/zh/docs/kitex/)
- [Hertz 教程](https://www.cloudwego.io/zh/docs/hertz/)
- [Etcd 官方文档](https://etcd.io/docs/)
- [Thrift IDL 语法](https://thrift.apache.org/docs/idl)
