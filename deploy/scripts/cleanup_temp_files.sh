#!/bin/bash

# 清理项目中的临时文件和备份文件
# 作者: GitHub Copilot
# 日期: 2026-02-18

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$PROJECT_ROOT"

echo "=========================================="
echo "清理项目临时文件"
echo "=========================================="
echo "项目根目录: $PROJECT_ROOT"
echo ""

# 定义要清理的文件模式
PATTERNS=(
    "*.bak"
    "*.bak_*"
    "*.tmp"
    "*.swp"
    "*~"
)

# 定义要排除的目录
EXCLUDE_DIRS=(
    "./vendor"
    "./.git"
    "./node_modules"
    "./.vscode"
    "./.idea"
)

# 构建 find 命令的排除参数
EXCLUDE_ARGS=""
for dir in "${EXCLUDE_DIRS[@]}"; do
    EXCLUDE_ARGS="$EXCLUDE_ARGS -path $dir -prune -o"
done

# 计数器
TOTAL_DELETED=0
TOTAL_SIZE=0

echo "🔍 搜索需要清理的文件..."
echo ""

# 遍历每个文件模式
for pattern in "${PATTERNS[@]}"; do
    echo "查找模式: $pattern"
    
    # 查找匹配的文件（排除指定目录）
    FILES=$(eval "find . $EXCLUDE_ARGS -type f -name '$pattern' -print" 2>/dev/null || true)
    
    if [ -n "$FILES" ]; then
        while IFS= read -r file; do
            if [ -f "$file" ]; then
                # 获取文件大小
                SIZE=$(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null || echo "0")
                
                echo "  🗑️  删除: $file ($(numfmt --to=iec-i --suffix=B $SIZE 2>/dev/null || echo "${SIZE} bytes"))"
                
                rm -f "$file"
                
                TOTAL_DELETED=$((TOTAL_DELETED + 1))
                TOTAL_SIZE=$((TOTAL_SIZE + SIZE))
            fi
        done <<< "$FILES"
    fi
done

echo ""

# 查找并删除空的 .txt 文件（仅在项目根目录、test、rpc-*、gateway 等目录）
echo "查找空的 .txt 文件..."
EMPTY_TXT=$(find . -maxdepth 3 \( -name "test" -o -name "rpc-*" -o -name "gateway" -o -path "./deploy" \) -type d -exec find {} -maxdepth 2 -name "*.txt" -type f -empty \; 2>/dev/null || true)

if [ -n "$EMPTY_TXT" ]; then
    while IFS= read -r file; do
        if [ -f "$file" ]; then
            echo "  🗑️  删除空文件: $file"
            rm -f "$file"
            TOTAL_DELETED=$((TOTAL_DELETED + 1))
        fi
    done <<< "$EMPTY_TXT"
fi

# 查找并列出 info.txt 等测试文件
echo ""
echo "查找测试信息文件..."
TEST_FILES=$(find ./test -maxdepth 1 -name "info.txt" -o -name "debug.txt" -o -name "test.txt" 2>/dev/null || true)

if [ -n "$TEST_FILES" ]; then
    while IFS= read -r file; do
        if [ -f "$file" ]; then
            SIZE=$(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null || echo "0")
            echo "  🗑️  删除: $file ($(numfmt --to=iec-i --suffix=B $SIZE 2>/dev/null || echo "${SIZE} bytes"))"
            rm -f "$file"
            TOTAL_DELETED=$((TOTAL_DELETED + 1))
            TOTAL_SIZE=$((TOTAL_SIZE + SIZE))
        fi
    done <<< "$TEST_FILES"
fi

echo ""
echo "=========================================="
echo "✅ 清理完成"
echo "=========================================="
echo "删除文件数: $TOTAL_DELETED"
echo "释放空间: $(numfmt --to=iec-i --suffix=B $TOTAL_SIZE 2>/dev/null || echo "${TOTAL_SIZE} bytes")"
echo ""

if [ $TOTAL_DELETED -eq 0 ]; then
    echo "没有找到需要清理的文件 ✨"
else
    echo "项目已清理干净 🎉"
fi
