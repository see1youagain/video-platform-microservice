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

### 6. 长阻塞调用与高并发熔断退避机制

针对大文件在网关透传和 `Merge` 时触发 MinIO 物理磁盘 `Compose` 的长耗时特性，网关层对 `rpc-videoUpload` 进行了特定的配置切分：
- 显式声明 `client.WithRPCTimeout(10 * time.Minute)`，以适配秒级以上的网络和磁盘 I/O 阻塞。
- 配套微服务原生的 Circuit Breaker 熔断器，当出现极端网络故障或者 I/O 队列超载时，网关主动向前端阻断抛出 `HTTP 503 Service Unavailable`，防止全局协程池泄漏与崩溃。
- 需要注意：当前这部分结论主要由上传/合并链路压测支撑，不应外推到登录等认证链路；`/api/login` 在高并发下仍有待单独治理。

### 7. 断点续传能力（当前实现）

当前实现已在 `InitUpload` 中返回 `finished_chunks`，语义如下：
- 由 `rpc-videoUpload` 从 Redis 集合键 `upload:chunks:<file_hash>` 读取已完成分片索引。
- 当集合非空时，`InitUploadResp.status` 返回 `partial`，并同步返回 `finished_chunks`。
- 当 Redis 不可用或读取失败时，接口会降级为不返回分片列表（`finished_chunks` 为空），不会触发 `503` 语义。
- 该能力依赖 Redis 中间状态，当前不是 MySQL 持久化恢复模型。

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
# 全量：基础 + 功能 + 并发正确性 + 全量压测
go run ./cmd

# 分步压测
# 第一步：S2/S3（注册/登录吞吐）
go run ./cmd s23
# 第二步：S4-S7（鉴权后接口与峰值大文件场景）
go run ./cmd s4s7
# 第三步：500MB / 100 分片长会话
 go run ./cmd long
# 第四步：同一 file_hash 并发 merge（默认 10 轮）
SAMEHASH_ROUNDS=20 go run ./cmd samehash
```

## 当前测试与压测基线

功能正确性：
- BASIC: 12 passed, 0 failed
- FUNCTIONAL: 25 passed, 0 failed
- CONCURRENT: 10 passed, 0 failed
- TOTAL: 47 passed, 0 failed

专项压测结论（2026-03-17 二轮）：
- `S4-S7`：在修正压测脚本 `file_hash` 长度后全部通过，其中 `S7`（10 并发三分片大文件上传）为 10/10 成功。
- `500MB / 100 分片` 长会话：上传与 merge 成功，样本中 goroutine 与内存曲线未出现失控增长。
- `同一 file_hash` 并发 merge：`50 并发 × 20 轮 = 1000/1000` 返回 200，验证了 finalize 幂等路径在高压下成立。
- `S3 /api/login`：在将 bcrypt cost 从 14 调整到 12 后仅出现阶段性改善，仍然存在高并发下不稳定问题；当前不能声称“认证链路压测已稳定通过”。

## 已知待治理项

- 转码请求相关 Kafka topic 在代码里存在 `video.transcode.tasks` / `video.transcode.requested` 两种命名，建议统一。
- Kafka 基础设施异常（如 leader/coordinator 不稳定）会影响异步路径实时性，但不应影响 merge 主链路同步落盘语义。

## License

MIT
