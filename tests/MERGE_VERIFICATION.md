# 远程整合验证报告

日期：2026-11-01（Asia/Shanghai）。角色：独立 tester。

## 范围与结论

本次验证本地真实转码提交 `1e2ffac` 与远程新增提交 `b01bb57`、`f047bf3`、`548d8d5` 整合后的工作区。冲突由主代理解决，tester 不修改生产实现、不执行 Git 操作。

**八个模块普通检查、三个核心模块 race、四个核心模块 vet 最终全部成功。** 本次实际运行 40 个顶层 Go 测试通过，0 失败；另有 3 个依赖完整服务环境的顶层测试明确跳过。真实 FFmpeg 媒体测试实际通过，没有跳过。

本次没有启动完整基础设施、没有重跑真实 MySQL/Kafka/MinIO HTTP 全链路，也没有执行原有 37 项在线回归断言或压测。2026-10-30 的完整测试结果仍保留在 `TRANSCODE_TEST_REPORT.md`，属于历史验证，不作为本次整合后的重测结果。

## 实际结果

计数来自 `.tmp/merge-verification/*.json` 的实际 Go JSON 事件，仅统计顶层测试，不重复计算子测试，也不把无测试文件的包计为测试用例。

| 模块 | 普通检查通过 | 失败 | 跳过 | race |
|---|---:|---:|---:|---|
| common | 10 | 0 | 0 | 10 通过，0 失败，0 跳过 |
| rpc-videoTranscode | 18 | 0 | 1 | 18 通过，0 失败，1 跳过 |
| rpc-videoManager | 12 | 0 | 1 | 12 通过，0 失败，1 跳过 |
| gateway | 0（编译检查通过） | 0 | 0 | 本次未执行 |
| rpc-user | 0（编译检查通过） | 0 | 0 | 本次未执行 |
| rpc-videoUpload | 0（编译检查通过） | 0 | 0 | 本次未执行 |
| clients | 0（编译检查通过） | 0 | 0 | 本次未执行 |
| tests | 0 | 0 | 1 | 本次未执行 |

明确跳过：

- `TestMySQLDurableTaskRecoveryIntegration`
- `TestMySQLFinishedEventConvergenceIntegration`
- `TestTranscodeE2E`

三个核心模块 race 共重复验证 40 个顶层用例通过，2 个真实 MySQL 用例跳过；这些通过项不与普通检查相加充当新增测试数。

`go vet ./...` 在 `common`、`rpc-videoTranscode`、`rpc-videoManager`、`gateway` 均退出 0。实际媒体用例 `TestRealMediaIntegration` 普通检查 4.65 秒、race 2.37 秒；坏源拒绝测试也通过。

## 合并问题及修正验证

1. **Outbox 清理保护。** 清理器现使用数值 `StatusSent` 和 `updated_at` 保留期限，避免数值状态与字符串 `published` 混用误匹配待发送记录。新增 `common/outbox/cleaner_test.go` 三项测试，严格断言删除 SQL 的数值状态和期限、数据库错误回滚且不扩大删除范围、上下文取消后不执行删除；普通/race 均通过。
2. **Gateway 依赖补全。** 首轮普通检查及 vet 提示需要更新 `go.mod`。主代理执行 tidy 后补入远程 `common/crypto` 路径引入的 `golang.org/x/crypto v0.39.0` 间接依赖；随后重新运行 Gateway 普通检查和 vet，均退出 0。
3. **真实转码保留。** 服务仍从 `engine.New` 启动固定 Worker，通过 `exec.CommandContext` 执行 FFmpeg H264/AAC 编码；没有恢复 `dummyData` 占位产物。实际横屏有声、竖屏静音四档编码、ffprobe 检查及完整解码用例通过，故合并没有把真实媒体路径替换为假实现。
4. **Topic 兼容审查。** Manager 主生产者仍使用 `video.transcode.tasks`；Worker 同时订阅该主题和旧 `video.transcode.requested`，均进入同一持久化、Fetch/Commit 消费路径。`TopicTranscodeRequested` 仅是旧主题兼容别名。现有规范 topic/payload、重复入队及终态回写测试通过；本次未重新运行 Kafka broker 上双主题投递，不宣称完成该在线验证。
5. **远程上传变更保留。** 静态检查确认网关初始化上传改走 Manager、上传表 Hash/User 复合唯一索引、按 Hash/User 持久化、取消失败状态和错误传播逻辑仍在。上传相关模块编译通过；本次不将这些静态检查表述为已重跑上传并发或在线幂等测试。
6. **回归入口保留。** 远程测试 CLI 默认及 `s23`、`s4s7`、`long`、`samehash` 压测入口保留；新增 `regression` 参数限定 Basic/Functional 两组，隔离 runner 显式调用 `go run ./cmd regression`，避免未来在线回归时意外启动压测。本次只编译入口，没有实际连接 Gateway 执行该命令。

## 环境与命令

- Windows/amd64，Go 1.24.10。
- FFmpeg/ffprobe 9.0.2，路径来自工作区 `.tmp/test-tools/ffmpeg`。
- race 使用 `CGO_ENABLED=1`、`CC=D:\MinGW\bin\gcc.exe`。
- 本次未创建 WSL 发行版、容器、数据库或后台服务。

逐模块普通检查：

```text
go test ./... -count=1 -json
```

核心 race：

```text
go test -race ./... -count=1 -json
```

四个核心模块静态检查：

```text
go vet ./...
```

JSON 原始结果保存在未提交的 `.tmp/merge-verification/`。截至本次验证完成，未发现阻止本次远程整合的代码问题；结论限定于上述本机测试和静态审查范围，不替代完整集成/负载测试。
