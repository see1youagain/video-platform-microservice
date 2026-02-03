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
