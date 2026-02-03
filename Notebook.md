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

网关框架：Hertz (字节跳动高性能 HTTP 框架，替代 Gin，作为 API 网关负责路由分发、参数校验与协议转换)

微服务框架：Kitex (高性能 RPC 框架，将 User 和 Video 业务拆分为独立服务)

服务注册与发现：Etcd 或 Nacos (新增组件，负责微服务的自动注册、发现与负载均衡)

接口定义 (IDL)：Thrift (定义网关与微服务之间的通信契约与数据结构)

数据库 (ORM)：MySQL + GORM (下沉至各微服务内部，负责各自领域的元数据持久化)

缓存与锁：Redis + go-redis (负责分布式锁、上传进度管理，由微服务直接调用)

认证鉴权：JWT (网关层统一拦截与解析 Token，将 UserID 通过 RPC 元数据透传至微服务)

工具库：godotenv (配置加载)、google/uuid (唯一ID生成)、hz (Hertz 代码生成工具)、kitex (RPC 代码生成工具)

### 开发工具

regard macbook vs-code as branch main, create and init project

regard windows vs-code as branch dev, continue to develop

#### User Manual for branch main:

1. 创建远程仓库
2. 初始化并搭建脚手架
   ```bash
   # 1. 找到存放代码的目录
   cd ~/Details/GoProjects  # 举例

   # 2. 克隆新仓库 (使用 SSH 或 HTTPS，推荐 SSH)
   git clone git@github.com:see1youagain/video-platform-microservice.git

   # 3. 进入目录
   cd video-platform-microservice

   # 4. 初始化 Hertz 脚手架 (假设你已经装好了 hz 工具)
   # 创建网关目录
   mkdir gateway
   cd gateway
   hz new -module video-platform-microservice/gateway
   cd ..

   # 5. 提交基础架构到 GitHub
   git add .
   git commit -m "feat: init project structure with hertz gateway"
   git push origin main
   ```

#### User Manual for branch dev:

```bash
# 1. 找到存放代码目录
cd ~/Work/Code

# 2. 克隆仓库 (只需要做一次)
git clone git@github.com:see1youagain/video-platform-microservice.git

# 3. 进入目录
cd video-platform-microservice

# 4. 验证是否看到电脑 A 提交的代码
ls -R
# 如果看到了 gateway 文件夹，说明同步成功！
```

#### User Manual for Daily Development

为了避免代码冲突，建议采用 **“分支开发法”** 。假设你想在 **电脑 A** 做网关开发，在 **电脑 B** 做用户服务开发。

#### 场景一：电脑 A 开始工作 (开发 Gateway)

1. **电脑 A - 创建分支**：

   ```Bash
   # 确保基础是新的
   git checkout main
   git pull origin main

   # 创建并切换到 feature/gateway 分支
   git checkout -b feature/gateway
   ```
2. **电脑 A - 写代码**：

   * (修改了 `gateway/main.go`...)
3. **电脑 A - 提交推送**：

   ```Bash
   git add .
   git commit -m "feat: add jwt middleware to gateway"
   # 推送到远程的 feature/gateway 分支 (远程会自动创建这个分支)
   git push origin feature/gateway
   ```

#### 场景二：电脑 B 开始工作 (开发 User Kitex)

1. **电脑 B - 同步状态**：

   ```Bash
   git checkout main
   git pull origin main
   # 此时电脑 B 知道了远程多了一个 feature/gateway 分支，但不用管它，你做你的任务
   ```
2. **电脑 B - 创建分支**：

   ```Bash
   git checkout -b feature/user-service
   ```
3. **电脑 B - 写代码**：

   * (初始化了 `services/user`...)
4. **电脑 B - 提交推送**：

   ```Bash
   git add .
   git commit -m "feat: init user kitex service"
   git push origin feature/user-service
   ```

#### 场景三：合并代码 (在 GitHub 网页操作)

现在 GitHub 上有三个分支：`main`, `feature/gateway`, `feature/user-service`。

1. 打开 GitHub 仓库页面。
2. 你会看到提示 "feature/gateway had recent pushes"，点击 **Compare & pull request**。
3. 创建一个 PR (Pull Request)，将 `feature/gateway` 合并进 `main`。
4. 自己 Review 一下，点击 **Merge pull request**。
5. 同理，处理 `feature/user-service` 的合并。

#### 场景四：第二天早上 (闭环)

第二天你回到 **电脑 A**，准备继续开发。

```Bash
# 切回主分支
git checkout main

# 拉取昨晚的所有变更 (包括电脑 A 自己的和电脑 B 写的内容)
git pull origin main

# 删除旧的开发分支 (可选，保持整洁)
git branch -d feature/gateway
```

#### 常见问题与避坑指南

1. **电脑 B 推送失败 (Permission denied)**：
   * 原因：电脑 B 的 SSH Key 没加到 GitHub 账户里。
   * 解决：在电脑 B 生成 `ssh-keygen`，把 `id_rsa.pub` 内容复制到 GitHub Settings -> SSH and GPG keys。
2. **忘记 `git pull` 就写代码了**：
   * 这是最常见的情况。当你 `git push` 时会报错，提示远程比本地新。
   * 解决：

     ```Bash
     git pull origin main --rebase  # 把远程代码拉下来，并把你的提交“接”在后面
     # 如果有冲突，手动解决文件冲突
     git add .
     git rebase --continue
     git push origin main
     ```
3. **两个分支修改了同一个文件**：
   * 合并时会报 Conflict。这在微服务初期很少见（因为 gateway 和 user 目录是分开的），如果遇到了，Git 会在文件里标记 `<<<<<<<`，你需要手动保留需要的代码，然后再次提交。

## Plan For Rebuild

### 项目迭代路线图

我们将把项目拆分为三个独立的代码库（或者在一个 Monorepo 中的三个目录）：

1. **`rpc-user`**: 用户服务（Kitex），负责注册、登录、用户信息。
2. **`rpc-video`**: 视频服务（Kitex），负责上传逻辑、分片管理、文件合并。
3. **`api-gateway`**: 网关服务（Hertz），负责路由、JWT 校验、RPC 调用、文件流转发。

### 步骤一：环境与工具准备

在开始代码之前，你需要安装 CloudWeGo 的代码生成工具和基础设施。

1. **安装工具**：

   ```Bash
   go install github.com/cloudwego/kitex/tool/cmd/kitex@latest
   go install github.com/cloudwego/hertz/cmd/hz@latest
   go install github.com/cloudwego/thriftgo@latest
   ```
2. **运行 Etcd** (服务注册中心)：
   使用 Docker 快速启动：

   在 CloudWeGo (Kitex + Hertz) 的微服务架构中，**Etcd** 扮演着 **“服务注册中心” (Service Registry)** 的关键角色，类似于一个“动态电话簿”。

   * **它的作用**：
     * 当你的 `rpc-user`（用户服务）启动时，它会告诉 Etcd：“我是用户服务，我的 IP 是 192.168.1.5，端口是 8888”。
     * 当你的 `api-gateway`（网关）需要调用用户服务时，它会问 Etcd：“用户服务在哪里？”，Etcd 会告诉它 IP 和端口。
     * 如果没有 Etcd，你的网关就需要把服务的 IP 写死在代码或配置文件里，一旦服务换了机器或扩容，就必须改代码重启，这在分布式系统中是不可接受的。
   * **为什么用 Docker 启动？**
     * **零侵入安装**：Etcd 是一个独立的二进制程序，直接安装到 Mac/Windows/Linux 往往需要配置系统服务、数据目录等。使用 Docker，只需要一行命令 `docker run` 就能拉起一个标准化的 Etcd 实例。
     * **开发便利性**：你提供的命令中 `--env ALLOW_NONE_AUTHENTICATION=yes` 表示允许无密码访问。这在**开发阶段**非常方便，避免了复杂的证书和权限配置，让你能立刻开始写 Go 代码。
     * **依赖解耦**：你当前的项目 `video-platform` 已经依赖了 `MySQL` 和 `Redis`。现在的架构升级引入了 `Etcd`。使用 Docker 可以避免你的电脑上装满各种数据库软件，用完即删。

   ```Bash
   docker run -d --name etcd-server \
     --publish 2379:2379 \
     --env ALLOW_NONE_AUTHENTICATION=yes \
     bitnami/etcd:latest
   ```

### 步骤二：定义 IDL (接口定义语言)

这是微服务的灵魂。我们需要用 Thrift 定义服务间的通信契约。

创建一个 `idl` 目录，并编写以下文件：

#### 1. `idl/user.thrift` (用户服务)

定义注册和登录接口。

```Thrift
namespace go user

struct RegisterReq {
    1: string username
    2: string password
}

struct RegisterResp {
    1: i32 code
    2: string msg
    3: i64 user_id
}

struct LoginReq {
    1: string username
    2: string password
}

struct LoginResp {
    1: i32 code
    2: string msg
    3: string token // JWT 在网关生成，但这里可以是 UserID 让网关生成
    4: i64 user_id
}

service UserService {
    RegisterResp Register(1: RegisterReq req)
    LoginResp Login(1: LoginReq req)
}
```

#### 2. `idl/video.thrift` (视频服务)

定义分片上传的核心逻辑。注意 `UploadChunkReq` 中包含二进制数据。

```Thrift
namespace go video

struct InitUploadReq {
    1: string file_hash
    2: string filename // 可选
}

struct InitUploadResp {
    1: i32 code
    2: string msg
    3: string status // "uploading" or "finished"
    4: list<string> finished_chunks
    5: string url
}

struct UploadChunkReq {
    1: string file_hash
    2: string index
    3: binary data // 核心：通过 RPC 传输分片数据
}

struct UploadChunkResp {
    1: i32 code
    2: string msg
}

struct MergeFileReq {
    1: string file_hash
    2: string filename
    3: i32 total_chunks
}

struct MergeFileResp {
    1: i32 code
    2: string msg
    3: string url
}

service VideoService {
    InitUploadResp InitUpload(1: InitUploadReq req)
    UploadChunkResp UploadChunk(1: UploadChunkReq req)
    MergeFileResp MergeFile(1: MergeFileReq req)
}
```

### 步骤三：开发 User 微服务 (`rpc-user`)

这个服务将接管原 `internal/handler/user.go` 和 `internal/db` 中关于 User 的逻辑。

**操作步骤：**

1. **初始化项目**：
   **Bash**

   ```
   mkdir rpc-user && cd rpc-user
   kitex -module rpc-user -service user ../idl/user.thrift
   go mod tidy
   ```
2. **迁移代码**：

   * 将原项目的 `internal/db/models.go` 中的 `User` 结构体迁移过来。
   * 将原项目的 `internal/utils/auth.go` (HashPassword, CheckPasswordHash) 迁移过来。
   * 初始化 GORM 连接（参考原 `repo.go`）。
3. **实现 Handler (`handler.go`)**：

   * `Register`: 接收 `req.Username`, `req.Password` -> Hash处理 -> 存入 MySQL -> 返回 UserID。
   * `Login`: 查 MySQL -> 校验密码 -> 返回 UserID (Token 生成逻辑建议上移至网关，微服务只负责验证身份返回 ID)。
4. **注册到 Etcd (`main.go`)**：
   **Go**

   ```
   import (
       "github.com/cloudwego/kitex/pkg/rpcinfo"
       "github.com/cloudwego/kitex/server"
       etcd "github.com/kitex-contrib/registry-etcd"
   )

   func main() {
       r, _ := etcd.NewEtcdRegistry([]string{"127.0.0.1:2379"})
       svr := user.NewServer(new(UserServiceImpl),
           server.WithServerBasicInfo(&rpcinfo.EndpointBasicInfo{ServiceName: "user"}),
           server.WithRegistry(r), // 注册到 Etcd
       )
       svr.Run()
   }
   ```

### 步骤四：开发 Video 微服务 (`rpc-video`)

这是最复杂的部分，它承载了上传、Redis 状态管理和文件存储。

**操作步骤：**

1. **初始化项目**：
   **Bash**

   ```
   mkdir rpc-video && cd rpc-video
   kitex -module rpc-video -service video ../idl/video.thrift
   go mod tidy
   ```
2. **迁移代码**：

   * **DB**: 迁移 `FileMeta`, `UserFile` 模型及 GORM 初始化。
   * **Redis**: 迁移 `internal/redis/*.go` 所有逻辑（Chunk 记录、墓碑机制）。
   * **Store**: 迁移 `internal/store/store.go`（本地文件写入）。
   * **Logic**: 迁移 `internal/logic/upload.go` 的核心业务逻辑。
3. **实现 Handler (`handler.go`)**：

   * `InitUpload`: 调用 Redis 检查分片 -> 查 MySQL 秒传 -> 返回状态。
   * `UploadChunk`: **接收 `req.Data` (二进制)** -> 调用 `store.WriteChunk` 写入磁盘 -> 更新 Redis Set。
   * `MergeFile`: 调用 `store.MergeChunks` -> 更新 MySQL -> 清理 Redis -> 返回 URL。
4. **注册到 Etcd**：与 User 服务类似，注册名为 `video`。

> **架构思考**：
>
> 在微服务中，`UploadChunk` 接收文件流并写入磁盘。这意味着 **Video Service 是有状态的**（文件存在该服务所在机器的磁盘上）。
>
> * 如果部署多个 Video Service 实例，会导致分片分散在不同机器，无法合并。
> * **解决方案**：在这个阶段，要么只部署一个 Video Service 实例，要么配置所有 Video Service 挂载同一个网络共享存储（如 NFS 或 NAS），或者代码中 `store` 层改为上传到 MinIO/OSS。
> * *基于你现有代码是 LocalStore，建议开发环境只启动一个 Video Service 实例，或者确保所有实例共享 `uploads/` 目录。*

### 步骤五：开发 API 网关 (`api-gateway`)

Hertz 将替代 Gin，作为流量入口。

**操作步骤：**

1. **初始化项目**：

   ```Bash
   mkdir api-gateway && cd api-gateway
   hz new -module api-gateway
   go mod tidy
   ```
2. **初始化 RPC 客户端**：
   在网关中建立 `rpc/user.go` 和 `rpc/video.go`，使用 `kitex/client` 初始化客户端，并配置 Etcd Resolver。
   **Go**

   ```
   // rpc/init.go 示例
   var UserClient user.Client
   var VideoClient video.Client

   func Init() {
       r, _ := etcd.NewEtcdResolver([]string{"127.0.0.1:2379"})
       UserClient, _ = user.NewClient("user", client.WithResolver(r))
       VideoClient, _ = video.NewClient("video", client.WithResolver(r))
   }
   ```
3. **编写 JWT 中间件**：

   * 重写原 `internal/middleware/auth.go`，适配 Hertz 的 `app.RequestContext`。
   * 校验通过后，将 `user_id` 写入 context。
4. **实现路由与聚合逻辑**：

   * `POST /register`: 解析 JSON -> 调用 `UserClient.Register` -> 返回结果。
   * `POST /login`: 解析 JSON -> 调用 `UserClient.Login` -> **在网关层生成 JWT Token** -> 返回 Token。
   * `POST /upload/init`: JWT 校验 -> 调用 `VideoClient.InitUpload`。
   * `POST /upload/chunk`:
     * **Hertz 获取文件流**：`fileHeader, _ := c.FormFile("data")`
     * **读取文件内容**：`fileContent, _ := fileHeader.Open(); buf := make([]byte, size); ...`
     * **RPC 调用**：`VideoClient.UploadChunk(..., Data: buf)`。
   * `POST /upload/merge`: 调用 `VideoClient.MergeFile`。

---

### 📊 技术细节对比与改变


| **功能模块**   | **Old Version (Gin 单体)**          | **New Project (Hertz + Kitex 微服务)**      | **改变带来的影响**                                                                     |
| -------------- | ----------------------------------- | ------------------------------------------- | -------------------------------------------------------------------------------------- |
| **HTTP 框架**  | Gin (`*gin.Context`)                | Hertz (`*app.RequestContext`)               | 更高性能，API 写法略有不同，利用 Netpoll 网络库。                                      |
| **服务通信**   | 进程内函数调用 (`logic.InitUpload`) | RPC 调用 (`VideoClient.InitUpload`)         | 引入了网络延迟，需要处理 RPC 错误和超时。                                              |
| **文件流转**   | HTTP -> 磁盘                        | HTTP -> 网关内存 -> RPC -> 服务内存 -> 磁盘 | **性能瓶颈风险**。大文件分片通过 RPC 传输会有序列化开销，建议分片大小控制在 5MB 以内。 |
| **用户上下文** | Gin Context Set/Get                 | RPC Metadata (Metainfo)                     | 网关解析 JWT 后，需通过`metainfo.WithPersistentValue`将 UserID 传递给下游微服务。      |
| **配置管理**   | `godotenv`读取本地 .env             | `godotenv`+ (可选 Nacos 配置中心)           | 每个微服务需要维护自己的`.env`(如 DB 连接串)。                                         |
| **依赖注入**   | 全局变量 (`db.DB`,`rdb.Client`)     | 依赖注入或服务内单例                        | 数据库连接下沉，网关不再连接 MySQL，只连 Etcd。                                        |

### ✅ 下一步行动建议

1. **按顺序开发**：先跑通 `rpc-user` 和 `api-gateway` 的登录注册流程，验证 Etcd 服务发现是否正常。
2. **重点攻克上传**：实现 `rpc-video` 时，务必注意 `UploadChunk` 的 Thrift 定义中 `binary` 类型的处理，确保 Hertz 网关正确读取 `multipart/form-data` 并转换为 `[]byte` 传给 Kitex。
3. **数据隔离**：确保 `rpc-user` 和 `rpc-video` 使用独立的数据库配置（即使物理上连同一个库，逻辑上也要解耦）。

这套架构完成后，你的项目将具备水平扩展能力（User 服务和 Video 服务可以独立扩容），并掌握了字节跳动核心技术栈的落地实践。
