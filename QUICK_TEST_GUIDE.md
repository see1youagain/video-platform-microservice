# 🚀 快速测试指南

## ⚡ 一键启动测试

### 步骤 1: 安装 Etcd (如果未安装)

```bash
# Ubuntu/Debian
sudo apt-get update && sudo apt-get install -y etcd

# macOS
brew install etcd

# 验证安装
etcd --version
```

### 步骤 2: 启动所有服务

在项目根目录执行：

```bash
# 启动 Etcd
etcd > /tmp/etcd.log 2>&1 &

# 等待 2 秒让 Etcd 完全启动
sleep 2

# 启动 RPC User 服务
cd rpc-user && ./rpc-user-test > /tmp/rpc-user.log 2>&1 &
cd ..

# 再等待 2 秒
sleep 2

# 启动 Gateway
cd gateway && ./gateway_test > /tmp/gateway.log 2>&1 &
cd ..

# 等待服务完全启动
sleep 3

echo "✅ 所有服务已启动！"
```

### 步骤 3: 运行测试

```bash
./test_api.sh
```

### 步骤 4: 查看日志

```bash
# 查看 Gateway 日志（漂亮的 JSON 格式）
tail -f /tmp/gateway.log

# 查看 RPC User 日志
tail -f /tmp/rpc-user.log

# 查看 Etcd 日志
tail -f /tmp/etcd.log
```

---

## 🎯 测试重点功能

### 1. 参数验证测试

```bash
# 测试用户名太短
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"ab","password":"test123"}'
# 预期：{"code":400,"msg":"用户名长度不能少于 3 个字符"}

# 测试密码太短
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"12"}'
# 预期：{"code":400,"msg":"密码长度不能少于 6 个字符"}

# 测试非法字符
curl -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"test@user","password":"test123"}'
# 预期：{"code":400,"msg":"用户名只能包含字母、数字和下划线"}
```

### 2. JWT Token 测试

```bash
# 登录获取 Token
curl -X POST http://localhost:8080/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"alice123"}'
# 保存返回的 token

# 使用 Token 访问受保护接口
curl -X GET http://localhost:8080/api/profile \
  -H "Authorization: Bearer YOUR_TOKEN_HERE"
# 预期：返回用户信息

#测试无 Token 访问
curl -X GET http://localhost:8080/api/profile
# 预期：{"code":401,"msg":"未授权: 缺少 Authorization 头"}
```

### 3. 请求追踪 ID 测试

```bash
# 发送请求并查看 Trace ID
curl -v -X POST http://localhost:8080/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"trace_test","password":"test123"}'

# 响应头中会包含 X-Trace-ID
# 然后在日志中搜索这个 Trace ID，可以追踪整个请求链路
```

---

## 🛑 停止服务

```bash
# 停止所有服务
pkill -f gateway_test
pkill -f rpc-user-test
pkill etcd

echo "✅ 所有服务已停止"
```

---

## 📊 检查服务状态

```bash
# 检查服务是否运行
pgrep -f etcd && echo "✅ Etcd 运行中" || echo "❌ Etcd 未运行"
pgrep -f rpc-user-test && echo "✅ RPC User 运行中" || echo "❌ RPC User 未运行"
pgrep -f gateway_test && echo "✅ Gateway 运行中" || echo "❌ Gateway 未运行"

# 检查端口占用
lsof -i :2379 && echo "✅ Etcd (2379)" || echo "❌ Etcd 未监听"
lsof -i :8888 && echo "✅ RPC User (8888)" || echo "❌ RPC User 未监听"
lsof -i :8080 && echo "✅ Gateway (8080)" || echo "❌ Gateway 未监听"
```

---

## 🔧 常见问题

### Q1: Etcd 启动失败
**A:** 检查端口 2379 是否被占用：
```bash
lsof -i :2379
# 如果被占用，停止占用的进程
kill -9 <PID>
```

### Q2: 数据库连接失败
**A:** 检查 MySQL 是否运行，并确认 `rpc-user/.env` 中的配置：
```bash
mysql -u video_user -p -e "SHOW DATABASES;"
```

### Q3: JWT Token 验证失败
**A:** 确保 `gateway/.env` 和 `rpc-user/.env` 中的 `JWT_SECRET` 相同

### Q4: 编译错误
**A:** 重新整理依赖：
```bash
cd gateway && go mod tidy && go build .
cd ../rpc-user && go mod tidy && go build .
```

---

## 📝 查看结构化日志

Gateway 的日志是 JSON 格式，可以使用 `jq` 美化输出：

```bash
# 安装 jq (如果未安装)
sudo apt-get install jq  # Ubuntu/Debian
brew install jq          # macOS

# 美化日志输出
tail -f /tmp/gateway.log | jq '.'
```

示例输出：
```json
{
  "level": "info",
  "timestamp": "2026-02-11T04:50:00.123Z",
  "msg": "请求开始",
  "trace_id": "550e8400-e29b-41d4-a716-446655440000",
  "method": "POST",
  "path": "/api/register",
  "client_ip": "127.0.0.1"
}
```

---

**提示**: 详细的功能说明请查看 [FEATURE_ENHANCEMENT.md](FEATURE_ENHANCEMENT.md)
