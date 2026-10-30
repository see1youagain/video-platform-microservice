# 转码开发独立测试报告

验证日期：2026-10-30（Asia/Shanghai）。执行角色：独立 tester；生产实现由主代理完成。本文记录实际运行结果，不将未启动的测试、仅编译通过的包或被跳过的测试计为通过。

## 验证结论

当前实现通过单元测试、真实媒体/数据库集成测试、HTTP 全链路测试及原有回归测试。隔离 Linux runner 最终退出码为 **0**；未发现阻止本次转码功能交付的问题。

| 类别 | 顶层测试/断言数 | 通过 | 失败 | 最终完整环境跳过 |
|---|---:|---:|---:|---:|
| 新增 Go 单元测试（含 SQL 合约测试） | 35 | 35 | 0 | 0 |
| 真实 FFmpeg 集成测试 | 2 | 2 | 0 | 0 |
| 真实 MySQL 8 集成测试 | 2 | 2 | 0 | 0 |
| HTTP 全链路转码测试 | 1 | 1 | 0 | 0 |
| 原有 BASIC 回归断言 | 12 | 12 | 0 | 0 |
| 原有 FUNCTIONAL 回归断言 | 25 | 25 | 0 | 0 |

新增 Go 用例共 **40 个顶层 `Test...` 函数**，不重复计算表驱动子测试。旧测试套件的 37 项为其自定义断言，和 Go 顶层测试分别统计；无测试文件的包不计入用例。`common`、`rpc-videoTranscode`、`rpc-videoManager` 在隔离 Linux 环境另经 `go test -race ./... -count=1` 验证通过，未发现竞争。Windows 下主代理也完成了这三个模块的 race 检查。

普通模块检查覆盖 `common`、`rpc-videoTranscode`、`rpc-videoManager`、`gateway`、`rpc-user`、`rpc-videoUpload`、`clients`；测试模块的 HTTP 集成和原有套件另行实际执行。主代理另完成核心模块 `go vet ./...`。

## 实际环境

- Windows 原生：Go 1.24.10；FFmpeg/ffprobe 9.0.2，用于独立引擎单元和真实媒体测试。
- 完整隔离环境：本任务单独创建的 Alpine 3.22 WSL 发行版，专属 Docker 28.0.4 daemon、Unix socket 和数据目录；未复用用户 Docker Desktop 镜像、容器或数据库。
- Linux runner：Go 1.24.10 / Debian bookworm，FFmpeg/ffprobe **5.1.9-0+deb12u1**。
- MySQL **8.0.46**、Redis **7.4.12**、Etcd **3.5.17**、Apache Kafka **3.9.0**（Strimzi 0.45.0 发行镜像，真实 KRaft broker）。
- MinIO：从官方 `RELEASE.2025-04-22T22-12-26Z` 源码构建，标签对应 Git 提交 **`0d7408fc9969caf07de6a8c3a84f9fbb10a6739e`**；运行版本标识为 `DEVELOPMENT.GOGET (go1.24.10 linux/amd64)`。交付的 `Dockerfile.minio` 固定该提交。
- 测试数据库 `video_platform_test`、专属 Redis、专属 MinIO bucket 和 Compose 网络。MySQL/MinIO 数据使用临时文件系统；Go 缓存使用测试项目专属 volume。

本机 Docker Desktop 无法启动，原因包括历史失效 Unix socket，以及其 WSL 发行版磁盘只读问题。独立测试发行版成功绕过这些问题。Docker Hub 在测试网络中超时，因此本次运行使用 Amazon 公开 Docker Official Images 缓存中的 MySQL/Redis/Go/Alpine 镜像、Quay 的 Etcd/Strimzi Kafka；网络替代仅位于未提交的 `.tmp/compose.registry-override.yml`。默认 Compose 仍使用官方镜像名称。MinIO 的官方二进制归档返回 410，故使用真实源码构建，未用模拟存储替代。

## 用例覆盖

### 单元及 SQL 合约

- MD5/SHA256 Hash 校验、任务 ID 路径安全、支持的分辨率及去重顺序。
- 引擎缺依赖、下载失败、坏输入、ffprobe 非法 JSON、无视频流、编码失败、输出编码校验失败、上传失败和空 URL。
- 真实子进程取消/超时终止、进度持久化失败传播、临时目录及部分对象清理、子进程诊断内存上限。
- Manager 用户归属、上下文身份优先、同 request_id 完成前后重放、参数冲突、活动任务复用、业务状态和规范 topic 事件同事务提交及 Outbox 失败回滚。
- Worker 持久入队重复/冲突、领取锁与租约、旧 owner 提交 fencing、完成/失败的精确状态、进度、URL 和完成事件 payload、Outbox 失败回滚。
- 消费者重复终态事件、不匹配身份、旧任务结果不覆盖新任务、完成/失败双表更新以及数据库失败回滚。
- Outbox 过期/空租约 processing 记录回收、领取失败回滚、退避上限、错误截断、带 owner 的发送续租及失败重试。

独立测试发现 `positiveInt` 原先使用 `fmt.Sscan` 会错误接受 `2oops`；实现改为严格整数解析，回归测试通过。

### 真实媒体集成

生成横屏有声、竖屏静音视频，各实际输出 360p/480p/720p/1080p。逐个使用 ffprobe 验证 H264/AAC、偶数尺寸、档位边界、方向及有效时长，并使用 FFmpeg 完整解码，确认不是占位文本。损坏源文件被拒绝且没有输出对象。

### 真实 MySQL 集成

`TestMySQLDurableTaskRecoveryIntegration` 实际验证重复消息只创建一个工作项、租约过期重新领取、旧 owner 无法提交、业务终态与唯一 Outbox 完成事件，以及四个并发领取者分别获得不同记录（真实 `FOR UPDATE SKIP LOCKED`）。

`TestMySQLFinishedEventConvergenceIntegration` 实际验证新旧任务结果隔离、任务/视频双表完成收敛、重复终态不可退化、身份冲突和失败结果收敛。测试要求显式启用且 `DB_NAME` 必须以 `_test` 结尾，清理仅限自己 UUID 标识的记录。

### HTTP 全链路

`TestTranscodeE2E` 实际运行 **13.07 秒**，通过真实 Gateway → Manager → Kafka → Worker → FFmpeg → MinIO → 完成事件 → Manager 查询闭环：

1. 注册/登录两个用户，上传可解码源文件，分片合并并同步取得视频记录。
2. 请求转码并轮询到 `completed` / progress=1，验证分辨率去重、下载每个产物、检查 H264/目标高度并完整解码。
3. 验证同 request_id 在完成前后返回原 task_id、改参数返回 HTTP/business 409、非法 Hash/档位返回 400、未鉴权返回 401、跨用户提交/查询返回 404。
4. 验证视频状态收敛到 `published`。
5. 上传真实损坏源，等待 `failed`，无成功产物，视频状态收敛到 `transcode_failed`。

原有 `go run ./cmd` 实际结果：**BASIC 12 passed / 0 failed；FUNCTIONAL 25 passed / 0 failed；TOTAL 37 passed / 0 failed**。

## 可复现入口

在项目根目录执行（需要可用 Docker/Compose 和镜像/Go 模块网络）：

```bash
docker compose -f tests/integration/compose.yml build
docker compose -f tests/integration/compose.yml run --rm runner
docker compose -f tests/integration/compose.yml down -v
```

Runner 等待独立服务健康及 Kafka topics 初始化完成，然后执行普通测试、三个核心模块 race、构建并启动五个服务、HTTP 全链路和旧 37 项回归。交付配置将当前项目只读挂载到 `/workspace`，二进制/日志/媒体全部写 `/tmp`，避免后续使用旧镜像中的源码快照；本次实际运行使用刚重建的镜像源码，并通过下述 SHA256 核对确认与工作区一致。真实媒体测试依赖 FFmpeg；真实数据库/HTTP 测试在独立运行时需要显式 `RUN_TRANSCODE_DB_INTEGRATION=1` / `RUN_TRANSCODE_E2E=1`，否则会明确 skip。默认 runner 已全部开启，最终本次完整环境没有跳过这些测试。

本次网络受限的实际命令采用相同入口，额外指定 `-f .tmp/compose.registry-override.yml`，先启动基础设施再运行 runner。全部实际输出保存到未提交的 `.tmp/transcode-runner.log`；基础设施启动记录保存到 `.tmp/transcode-stack-start.log`。

最终验证额外比对了运行镜像与当前工作区中 Worker、Manager、Outbox、Manager 测试、两个 MySQL 集成测试及 HTTP E2E 文件的 SHA256，全部一致。提交前源码变更不会被过期构建快照掩盖。

## 验证边界及剩余事项

- 本次未做长视频/大规模负载、GPU 编码、Kafka 多 broker 故障或实际进程 `kill -9` 后等待租约恢复的压力/故障注入。租约崩溃恢复通过真实 MySQL 中模拟过期及重新领取验证；消息重复通过持久化与完成消费测试验证，没有宣称 broker 层的 exactly-once。
- 原有 37 项回归中的小文件上传仅验证旧接口响应；它仍属于先前已有的网关本地文件路径问题，不代表本次已将小文件链路改造为统一对象存储。
- 预签名 URL 生命周期、上传用户隔离等此前识别的非转码问题，不在本次测试通过的范围内。
- 没有发现本次转码功能的未修复阻塞缺陷。结论限定于上述版本、输入及覆盖范围。

## 环境清理

测试结束后已执行独立 Compose `down -v`，删除本任务容器、网络及 Go 缓存 volume；核对注册路径后已注销唯一的 `codex-video-transcode-test` WSL 发行版，其专属临时 VHD 目录已不存在。

Docker Desktop 的原 `run` 目录已从 `run.codex-backup-20261030-181053` 恢复，未修改 Docker settings 或用户原有发行版。为避免删除重解析点，本次失败启动产生的目录保留为 `C:\Users\Li_Zhao\AppData\Local\Docker\run.codex-attempt-20261030-184611`（仅包含本次生成的运行时 socket）。原有 Docker Desktop 故障未被宣称修复；完整验证使用的是已经清理的独立测试环境。
