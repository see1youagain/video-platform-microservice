#!/usr/bin/env bash
set -e

BASE="$(cd "$(dirname "$0")/.." && pwd)"
LOGS_DIR="$BASE/logs"
PID_DIR="$BASE/.pids"

mkdir -p "$LOGS_DIR" "$PID_DIR"

stop_services() {
    echo "🛑 停止所有服务..."
    for f in "$PID_DIR"/*.pid; do
        [ -f "$f" ] || continue
        pid=$(cat "$f")
        name=$(basename "$f" .pid)
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" && echo "  已停止 $name (pid=$pid)"
        fi
        rm -f "$f"
    done
}

build_service() {
    local svc=$1
    echo "🔨 编译 $svc ..."
    cd "$BASE/$svc"
    go build -o "$BASE/$svc/${svc##rpc-}" . 2>&1 | head -20
    cd "$BASE"
}

start_service() {
    local svc=$1
    local bin_name="${svc##rpc-}"
    local log="$LOGS_DIR/$svc.log"
    local pid_file="$PID_DIR/$svc.pid"
    local env_file="$BASE/$svc/.env"

    echo "🚀 启动 $svc ..."
    cd "$BASE/$svc"

    if [ -f "$env_file" ]; then
        export $(grep -v '^#' "$env_file" | grep -v '^$' | xargs) 2>/dev/null || true
    fi

    ./"$bin_name" > "$log" 2>&1 &
    local pid=$!
    echo $pid > "$pid_file"
    echo "   PID=$pid  日志: $log"
    cd "$BASE"
}

case "${1:-start}" in
start)
    echo "╔══════════════════════════════════════════════╗"
    echo "║  Video Platform — 启动所有微服务              ║"
    echo "╚══════════════════════════════════════════════╝"

    # Check deps
    echo "🔍 检查基础设施..."
    etcdctl endpoint health --endpoints=127.0.0.1:2379 > /dev/null 2>&1 && echo "  ✅ etcd" || { echo "  ❌ etcd 未运行！"; exit 1; }
    redis-cli ping > /dev/null 2>&1 && echo "  ✅ redis" || { echo "  ❌ redis 未运行！"; exit 1; }
    mysql -h 127.0.0.1 -u video_user -plzzy136994 video_platform -e "SELECT 1;" > /dev/null 2>&1 && echo "  ✅ mysql" || { echo "  ❌ mysql 未运行！"; exit 1; }

    # Build all
    for svc in rpc-user rpc-video rpc-videoUpload rpc-videoTranscode gateway; do
        build_service "$svc"
    done

    # Start backend services
    start_service "rpc-user"
    start_service "rpc-video"
    start_service "rpc-videoUpload"
    start_service "rpc-videoTranscode"

    # Wait for backend to register with etcd
    echo "⏳ 等待 RPC 服务注册到 etcd (5s)..."
    sleep 5

    # Start gateway
    start_service "gateway"

    # Wait for gateway
    echo "⏳ 等待 gateway 就绪..."
    for i in $(seq 1 20); do
        if curl -sf http://127.0.0.1:8080/ping > /dev/null 2>&1; then
            echo "✅ Gateway 已就绪！"
            break
        fi
        sleep 1
        if [ $i -eq 20 ]; then
            echo "❌ Gateway 启动超时，查看日志: $LOGS_DIR/gateway.log"
            tail -20 "$LOGS_DIR/gateway.log"
            exit 1
        fi
    done

    echo ""
    echo "✅ 所有服务已启动！"
    echo "   Gateway:          http://127.0.0.1:8080"
    echo "   rpc-user:         :8888 (kitex, etcd)"
    echo "   rpc-video:        :8889 (kitex, etcd)"
    echo "   rpc-videoUpload:  :8083 (kitex, etcd)"
    echo "   rpc-videoTranscode: :8084 (kitex, etcd)"
    echo ""
    echo "   运行测试:  cd tests/cmd && go run main.go"
    echo "   运行客户端: cd clients && go run main.go"
    echo "   停止服务:  $0 stop"
    ;;

stop)
    stop_services
    ;;

restart)
    stop_services
    sleep 2
    "$0" start
    ;;

status)
    echo "服务状态:"
    for f in "$PID_DIR"/*.pid; do
        [ -f "$f" ] || continue
        pid=$(cat "$f")
        name=$(basename "$f" .pid)
        if kill -0 "$pid" 2>/dev/null; then
            echo "  ✅ $name (pid=$pid)"
        else
            echo "  ❌ $name (pid=$pid, 已停止)"
        fi
    done
    ;;

logs)
    svc="${2:-gateway}"
    tail -f "$LOGS_DIR/$svc.log"
    ;;

*)
    echo "用法: $0 {start|stop|restart|status|logs [service]}"
    exit 1
    ;;
esac
