# Video Platform Microservice

基于 CloudWeGo 技术栈构建的视频平台微服务项目，用于学习 Hertz、Kitex、Etcd、Kafka 等组件在 Go 微服务中的实际应用。

## 架构

```
客户端
  |
Gateway (Hertz, :8080)
  |  JWT 认证 + 速率限制 + Trace ID
  |
  +---> rpc-user       (:8888)  注册/登录
  +---> rpc-videoUpload(:8083)  分片上传 / 合并 / 秒传
  +---> rpc-video      (:8889)  文件信息 / 转码任务管理
  +---> rpc-videoTranscode(:8084) 转码任务消费与执行

消息队列 (Kafka)
  video.file.uploaded      <- rpc-videoUpload 发布, rpc-video 消费（建立文件记录）
  transcode-tasks          <- rpc-video 发布, rpc-videoTranscode 消费
  video.transcode.finished <- rpc-videoTranscode 发布, rpc-video 消费（更新转码状态）

服务注册/发现: Etcd
公共模块: common/ (DB、Redis、Kafka、MinIO、日志)
```

## 服务说明

| 服务 | 端口 | 职责 |
|------|------|------|
| gateway | 8080 | HTTP 入口，JWT 验证，路由分发 |
| rpc-user | 8888 | 用户注册、登录，生成 JWT |
| rpc-videoUpload | 8083 | 分片上传、合并、秒传，合并后发 Kafka 事件 |
| rpc-video | 8889 | 消费上传事件建立文件记录，管理转码任务 |
| rpc-videoTranscode | 8084 | 消费转码任务，调 ffmpeg 执行，回写状态 |

## 技术栈

- HTTP 框架: Hertz (CloudWeGo)
- RPC 框架: Kitex (CloudWeGo)
- IDL: Apache Thrift
- 服务注册: Etcd
- 消息队列: Kafka (segmentio/kafka-go)
- 数据库: MySQL + GORM
- 缓存: Redis
- 对象存储: MinIO
- 认证: JWT (golang-jwt/v5)
- 语言: Go 1.24

## 目录结构

```
.
├── gateway/            # HTTP 网关
├── rpc-user/           # 用户服务
├── rpc-video/          # 视频信息 + 转码任务管理
├── rpc-videoUpload/    # 视频上传服务
├── rpc-videoTranscode/ # 转码执行服务
├── common/             # 公共模块（DB/Redis/Kafka/MinIO）
├── idl/                # Thrift 接口定义
├── tests/              # 集成测试 + 压力测试
└── deploy/             # Docker Compose（Etcd/Kafka/MinIO）
```

## API 接口

所有 `/api` 前缀的接口（除注册/登录外）需要 `Authorization: Bearer <token>` header。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /ping | 健康检查 |
| POST | /api/register | 用户注册 |
| POST | /api/login | 用户登录 |
| GET | /api/profile | 用户信息 |
| POST | /api/video/init | 初始化上传（秒传检查） |
| POST | /api/video/upload_chunk | 上传分片 |
| POST | /api/video/merge | 合并分片 |
| POST | /api/video/upload | 简单上传（小文件） |
| POST | /api/video/transcode | 创建转码任务 |
| GET | /api/video/transcode/status | 查询转码状态 |
| GET | /api/video/download | 下载视频 |
| GET | /api/video/info | 视频信息 |

## 上传流程

```
1. 客户端计算文件 MD5 作为 file_hash
2. POST /api/video/init
   - 返回 status=finished  -> 秒传命中，直接拿 URL
   - 返回 status=partial   -> 断点续传，跳过已上传分片
   - 返回 status=new       -> 正常上传
3. POST /api/video/upload_chunk  (每个分片，支持并发)
4. POST /api/video/merge
   - 服务端合并，上传 MinIO，发布 video.file.uploaded 事件
   - rpc-video 消费事件，在数据库建立文件记录
5. 之后可调 /api/video/transcode 创建转码任务
```

## 快速启动

### 依赖服务

```bash
cd deploy/docker
docker-compose -f etcd-kafka-minio-compose.yml up -d
```

### 环境变量

每个服务目录下创建 `.env`（已在 .gitignore 中排除）：

```env
# 示例：rpc-video/.env
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=video_user
DB_PASSWORD=your_password
DB_NAME=video_platform
REDIS_HOST=127.0.0.1
REDIS_PORT=6379
ETCD_ENDPOINTS=127.0.0.1:2379
KAFKA_BROKERS=127.0.0.1:9092
RPC_PORT=8889
STORAGE_PATH=/tmp/video-platform
```

注意：gateway 和 rpc-user 的 `JWT_SECRET` 必须相同。

### 启动服务

```bash
cd rpc-user           && go run .
cd rpc-video          && go run .
cd rpc-videoUpload    && go run .
cd rpc-videoTranscode && go run .
cd gateway            && go run .
```

### 运行测试

```bash
cd tests/cmd && go run main.go
```

## 身份透传

Gateway 的 JWT 中间件验证通过后，通过 Kitex metainfo 将 `user_id` 注入上下文：

```go
// 下游服务获取用户 ID，不依赖业务字段（防止伪造）
uid, _ := metainfo.GetPersistentValue(ctx, "user_id")
```

## License

MIT
