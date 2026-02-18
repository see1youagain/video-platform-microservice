#!/bin/bash

set -e

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_DIR"

echo "🔨 开始编译所有服务..."

# 编译 rpc-user
echo "📦 编译 rpc-user..."
cd rpc-user
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o rpc-user .
cd ..

# 编译 rpc-video
echo "📦 编译 rpc-video..."
cd rpc-video
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o rpc-video .
cd ..

# 编译 gateway
echo "📦 编译 gateway..."
cd gateway
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o gateway .
cd ..

echo "✅ 编译完成！"
ls -lh rpc-user/rpc-user rpc-video/rpc-video gateway/gateway