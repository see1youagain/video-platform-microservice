# Video Platform Microservice

基于 CloudWeGo 技术栈的视频平台微服务系统，采用 Hertz + Kitex 实现网关与 RPC 服务分离，支持大文件分片上传、断点续传、秒传、视频转码等功能。

## 项目架构

```
客户端 --> Gateway (Hertz HTTP) --> RPC Services (Kitex)
                |                       |
                |                  +---------+---------+
                |                  |                   |
             JWT Auth         rpc-user            rpc-video
                              (用户服务)          (视频服务)
                                  |                   |
                              MySQL              MySQL + Redis
                                                 + 本地存储
```

整体分为三个独立服务，通过 Etcd 做服务注册与发现：

- **gateway** - HTTP API 网关，基于 Hertz，负责路由分发、JWT 认证、请求追踪
- **rpc-user** - 用户服务，基于 Kitex，处理注册/登录，生成 JWT Token
- **rpc-video** - 视频服务，基于 Kitex，处理视频上传（分片/秒传/断点续传）、下载、转码

公共模块 `common` 提供统一的数据库、Redis、日志、配置等基础能力。

## 目录结构

```
.
├── gateway/          # HTTP 网关服务
│   ├── biz/          # 业务逻辑（handler、middleware）
│   ├── internal/     # 内部工具（JWT、日志、参数校验）
│   ├── kitex_gen/    # Kitex 生成的 RPC 客户端代码
│   ├── rpc/          # RPC 客户端初始化
│   ├── router.go     # 路由注册
│   └── main.go
├── rpc-user/         # 用户 RPC 服务
│   ├── handler.go    # 服务实现
│   ├── internal/     # 数据库操作、认证工具
│   ├── kitex_gen/    # Kitex 生成的服务端代码
│   └── main.go
├── rpc-video/        # 视频 RPC 服务
│   ├── handler.go    # 服务实现
│   ├── internal/     # 数据库、Redis、存储、转码
│   ├── kitex_gen/    # Kitex 生成的服务端代码
│   └── main.go
├── common/           # 公共模块
│   ├── config/       # 配置加载
│   ├── db/           # MySQL 连接（GORM）
│   ├── redis/        # Redis 连接
│   ├── logger/       # Zap 日志
│   ├── utils/        # JWT 工具
│   └── validator/    # 参数校验
├── idl/              # Thrift IDL 接口定义
│   ├── user.thrift
│   └── video.thrift
├── test/             # 集成测试
└── deploy/           # 部署脚本
```

## 技术栈

| 组件 | 技术选型 |
|------|---------|
| HTTP 框架 | Hertz (CloudWeGo) |
| RPC 框架 | Kitex (CloudWeGo) |
| IDL | Apache Thrift |
| 服务注册/发现 | Etcd |
| 数据库 | MySQL + GORM |
| 缓存 | Redis |
| 认证 | JWT (golang-jwt/v5) |
| 日志 | Zap |
| 语言 | Go 1.24 |

## 核心功能

### 用户服务
- 用户注册（密码 bcrypt 加密存储）
- 用户登录（返回 JWT Token）
- 用户信息查询

### 视频上传
- **分片上传** - 大文件切分为多个分片，通过 RPC 传输并落盘
- **断点续传** - 上传中断后重新初始化，服务端返回已完成分片列表，客户端跳过已上传部分
- **秒传** - 基于 file_hash + user_id 判断，命中 Redis 墓碑缓存或数据库记录后直接返回 URL
- **幂等性** - 支持 request_id 去重，防止重复提交
- **分片去重** - 同一分片重复上传会被检测并跳过

### 视频下载
- 支持 HTTP Range 请求，按字节范围读取文件

### 视频转码
- 异步转码任务队列
- 支持多分辨率转码（720p、480p、360p 等）
- 转码进度查询

## API 接口

所有接口前缀 `/api`，除注册和登录外均需在 Header 中携带 `Authorization: Bearer <token>`。

### 公开接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /ping | 健康检查 |
| POST | /api/register | 用户注册 |
| POST | /api/login | 用户登录 |

### 需要认证的接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/profile | 获取用户信息 |
| POST | /api/video/init | 初始化上传（秒传检查） |
| POST | /api/video/upload_chunk | 上传分片 |
| POST | /api/video/merge | 合并分片 |
| POST | /api/video/upload | 简单上传（小文件） |
| POST | /api/video/hash | 计算文件哈希 |
| GET | /api/video/download | 下载视频（支持 Range） |
| GET | /api/video/info | 获取视频信息 |
| POST | /api/video/transcode | 创建转码任务 |
| GET | /api/video/transcode/status | 查询转码状态 |

## 环境依赖

- Go >= 1.24
- MySQL 8.0
- Redis 6+
- Etcd 3.5+

## 快速启动

### 1. 准备基础服务

确保 MySQL、Redis、Etcd 已启动。

```bash
# MySQL 建库建用户
mysql -u root -p -e "
CREATE DATABASE IF NOT EXISTS video_platform;
CREATE USER IF NOT EXISTS 'video_user'@'localhost' IDENTIFIED BY 'your_password';
GRANT ALL PRIVILEGES ON video_platform.* TO 'video_user'@'localhost';
FLUSH PRIVILEGES;
"
```

### 2. 配置环境变量

每个服务目录下创建 `.env` 文件：

**gateway/.env**
```
JWT_SECRET=your_jwt_secret_key_at_least_32_chars
ETCD_ADDRESS=127.0.0.1:2379
```

**rpc-user/.env**
```
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=video_user
DB_PASSWORD=your_password
DB_NAME=video_platform
JWT_SECRET=your_jwt_secret_key_at_least_32_chars
ETCD_ADDRESS=127.0.0.1:2379
```

**rpc-video/.env**
```
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=video_user
DB_PASSWORD=your_password
DB_NAME=video_platform
REDIS_HOST=127.0.0.1
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0
RPC_PORT=8889
ETCD_ENDPOINTS=127.0.0.1:2379
STORAGE_PATH=/tmp/video-platform
CHUNK_SIZE=2097152
```

注意：三个服务的 `JWT_SECRET` 必须一致。

### 3. 启动服务

分别在三个终端中启动：

```bash
# 终端 1 - 启动用户服务
cd rpc-user && go run .

# 终端 2 - 启动视频服务
cd rpc-video && go run .

# 终端 3 - 启动网关
cd gateway && go run .
```

网关默认监听 `8080` 端口。

### 4. 验证

```bash
# 健康检查
curl http://localhost:8080/ping

# 注册
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"Test123456"}'

# 登录
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"Test123456"}'
```

## 上传流程

```
1. 客户端计算文件 MD5/SHA256 作为 file_hash
2. POST /api/video/init  -->  检查秒传 / 断点续传
   - 返回 status="finished" + url  -->  秒传成功，流程结束
   - 返回 status="uploading" + finished_chunks  -->  断点续传，跳过已完成分片
3. 将文件切分为固定大小的分片（默认 2MB）
4. POST /api/video/upload_chunk  -->  逐个上传分片（支持并发）
5. POST /api/video/merge  -->  服务端合并所有分片，写入数据库，设置墓碑缓存
```

## 认证机制

系统使用 JWT 进行身份认证，认证流程：

1. 用户通过 `/api/login` 获取 Token
2. 后续请求在 Header 中携带 `Authorization: Bearer <token>`
3. Gateway 的 JWT 中间件验证 Token，提取 user_id 和 username
4. 通过 Kitex 的 metainfo 机制将用户信息透传给下游 RPC 服务

所有视频相关操作（上传、下载、转码等）均强制要求 Token，不允许匿名访问。

## 测试

项目在 `test/` 目录下提供了集成测试，覆盖以下场景：

- 用户注册/登录
- JWT 认证与未认证拒绝
- 视频初始化上传、分片上传、合并
- 并发上传
- 断点续传
- 幂等性验证
- MetaInfo 传递验证

```bash
cd test && go run test_comprehensive.go
```

## License

MIT
