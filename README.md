# Video Platform Microservice

这是一个基于 Go 与 CloudWeGo 的视频平台微服务项目。系统由 Gateway、用户服务、上传服务、视频管理服务、转码服务组成，使用 Etcd 做服务发现，MySQL/Redis/MinIO 做数据与对象存储，Kafka 做异步事件总线。

## 系统架构

```text
Client
  |
Gateway (Hertz :8080)
  |-- rpc-user (:8888)
  |-- rpc-videoUpload (:8083)
  |-- rpc-videoManager (:8889)
  |-- rpc-videoTranscode (:8084)

Infra:
- Etcd: service discovery
- MySQL: business data + outbox_events
- Redis: upload progress / rate limit
- MinIO: object storage
- Kafka: async event bus
```

### 关键调用关系

- `/api/video/init`、`/api/video/upload_chunk` 由 Gateway 转发到 `rpc-videoUpload`
- `/api/video/merge` 由 Gateway 转发到 `rpc-videoManager.FinalizeUpload`
  - `rpc-videoManager` 调用 `rpc-videoUpload.FinalizeUpload` 完成对象合并
  - 合并成功后 `rpc-videoManager` **同步写入** `video_files`（主链路一致性）
- `/api/video/transcode` 由 Gateway 调用 `rpc-videoManager.Transcode`
  - 在同一事务中更新业务状态 + 写 outbox 事件
  - 由 outbox dispatcher 后台投递到 Kafka

## 服务与职责

| 服务 | 端口 | 职责 |
|------|------|------|
| gateway | 8080 | HTTP 入口、JWT 鉴权、限流、Trace 透传、路由编排 |
| rpc-user | 8888 | 用户注册、登录、JWT |
| rpc-videoUpload | 8083 | 初始化上传、分片上传、合并、上传进度、秒传检查 |
| rpc-videoManager | 8889 | 视频信息管理、删除、Finalize 同步落库、转码任务管理 |
| rpc-videoTranscode | 8084 | 消费转码任务、执行转码流程、回写转码完成事件 |

## 技术栈

- HTTP: Hertz
- RPC: Kitex
- IDL: Thrift
- Registry: Etcd
- DB: MySQL + GORM
- Cache: Redis
- Object Storage: MinIO
- MQ: Kafka (`segmentio/kafka-go`)
- Auth: JWT
- Runtime: Go 1.24

## 关键技术实现

### 1. 主链路同步落盘（Merge 一致性）

当前上传主链路不是“仅依赖 Kafka 最终一致”。

- Gateway merge 入口调用 `rpc-videoManager.FinalizeUpload`
- `rpc-videoManager` 在下游合并成功后，事务写入 `video_files`（`status=finished`）
- 这样 `merge -> info -> transcode` 不再依赖 Kafka 消费时序

### 2. Transactional Outbox + 后台 Relay

项目已经落地 outbox，并且有后台发送：

- Outbox 表：`outbox_events`（见 `common/outbox/outbox.go`）
- 事务写入：
  - `rpc-videoUpload` 在 finalize 成功后写 outbox（`video.file.uploaded`）
  - `rpc-videoManager` 在创建转码任务事务中写 outbox
  - `rpc-videoTranscode` 在转码完成事务中写 outbox（`video.transcode.finished`）
- 后台发送：
  - `rpc-videoUpload/main.go` 启动 `commonOutbox.NewDispatcher(...).Run(ctx)`
  - `rpc-videoManager/main.go` 启动 `commonOutbox.NewDispatcher(...).Run(ctx)`
  - `rpc-videoTranscode/main.go` 启动 `commonOutbox.NewDispatcher(...).Run(ctx)`

### 3. 小文件/大文件路径分流

- `< 5MB`：`/api/video/init` 返回 `single_shot`，客户端走 `/api/video/upload`
- `>= 5MB`：走 `init -> upload_chunk -> merge`
- 上传入口统一使用 `file_size`

### 4. 上传幂等与防线

`rpc-videoUpload.FinalizeUpload` 具备多层保护：

- DB 短路（已完成直接返回）
- MinIO 对象探测（对象已存在则跳过重复合并）
- Redis 分片完整性检查（数量与 ETag）
- 最终事务写业务表 + outbox

### 5. 链路可观测性

- Gateway 将 `trace_id` / `request_id` 写入 metainfo 持久上下文
- 下游服务通过 metainfo 读取用户与追踪信息

## Kafka 主题

当前代码中涉及主题如下：

- `video.file.uploaded`
- `video.transcode.tasks`
- `video.transcode.finished`
- `video.transcode.requested`（在 `rpc-videoManager` 的 outbox 写入中使用）

## API 概览

需要鉴权（`Authorization: Bearer <token>`）的接口：

- `GET /api/profile`
- `POST /api/video/init`
- `POST /api/video/upload_chunk`
- `POST /api/video/merge`
- `POST /api/video/upload`
- `POST /api/video/transcode`
- `GET /api/video/transcode/status`
- `GET /api/video/download`
- `GET /api/video/info`
- `POST /api/video/delete`

无需鉴权：

- `GET /ping`
- `POST /api/register`
- `POST /api/login`

## 目录结构

```text
.
├── gateway/
├── rpc-user/
├── rpc-videoUpload/
├── rpc-videoManager/
├── rpc-videoTranscode/
├── common/
├── idl/
├── tests/
├── deploy/
└── document/
```

## 启动方式

### 1) 启动依赖

```bash
cd deploy/docker
docker-compose -f etcd-kafka-minio-compose.yml up -d
```

### 2) 启动服务

```bash
cd rpc-user && go run .
cd rpc-videoUpload && go run .
cd rpc-videoManager && go run .
cd rpc-videoTranscode && go run .
cd gateway && go run .
```

### 3) 运行测试

```bash
cd tests
go run ./cmd --host http://127.0.0.1:8080
```

## 当前测试基线

- BASIC: 12 passed, 0 failed
- FUNCTIONAL: 25 passed, 0 failed
- TOTAL: 37 passed, 0 failed

## 已知待治理项

- 转码请求相关 Kafka topic 在代码里存在 `video.transcode.tasks` / `video.transcode.requested` 两种命名，建议统一。
- Kafka 基础设施异常（如 leader/coordinator 不稳定）会影响异步路径实时性，但不应影响 merge 主链路同步落盘语义。

## License

MIT
