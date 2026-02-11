#!/bin/bash

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

API_URL="http://localhost:8080"

echo "========================================="
echo "🧪 开始测试 Video Platform API"
echo "========================================="
echo ""

# 测试 1: 参数验证 - 用户名太短
echo -e "${YELLOW}测试 1: 参数验证 - 用户名太短（少于3个字符）${NC}"
response=$(curl -s -X POST $API_URL/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"ab","password":"test123"}')
echo "响应: $response"
if echo "$response" | grep -q "用户名长度不能少于"; then
    echo -e "${GREEN}✅ 通过${NC}\n"
else
    echo -e "${RED}❌ 失败${NC}\n"
fi

# 测试 2: 参数验证 - 密码太短
echo -e "${YELLOW}测试 2: 参数验证 - 密码太短（少于6个字符）${NC}"
response=$(curl -s -X POST $API_URL/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"123"}')
echo "响应: $response"
if echo "$response" | grep -q "密码长度不能少于"; then
    echo -e "${GREEN}✅ 通过${NC}\n"
else
    echo -e "${RED}❌ 失败${NC}\n"
fi

# 测试 3: 参数验证 - 用户名包含非法字符
echo -e "${YELLOW}测试 3: 参数验证 - 用户名包含非法字符${NC}"
response=$(curl -s -X POST $API_URL/api/register \
  -H "Content-Type: application/json" \
  -d '{"username":"test@user","password":"test123"}')
echo "响应: $response"
if echo "$response" | grep -q "用户名只能包含"; then
    echo -e "${GREEN}✅ 通过${NC}\n"
else
    echo -e "${RED}❌ 失败${NC}\n"
fi

# 生成随机用户名
RANDOM_USER="test_user_$(date +%s)"

# 测试 4: 成功注册新用户
echo -e "${YELLOW}测试 4: 成功注册新用户 ($RANDOM_USER)${NC}"
response=$(curl -s -X POST $API_URL/api/register \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$RANDOM_USER\",\"password\":\"password123\"}")
echo "响应: $response"
if echo "$response" | grep -q '"code":200'; then
    echo -e "${GREEN}✅ 通过 - 用户注册成功${NC}\n"
    USER_ID=$(echo "$response" | grep -o '"user_id":[0-9]*' | cut -d':' -f2)
else
    echo -e "${RED}❌ 失败${NC}\n"
fi

# 测试 5: 重复注册 - 应该失败
echo -e "${YELLOW}测试 5: 重复注册同一用户名${NC}"
response=$(curl -s -X POST $API_URL/api/register \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$RANDOM_USER\",\"password\":\"password123\"}")
echo "响应: $response"
if echo "$response" | grep -q "用户名已存在"; then
    echo -e "${GREEN}✅ 通过 - 正确拒绝重复注册${NC}\n"
else
    echo -e "${RED}❌ 失败${NC}\n"
fi

# 测试 6: 登录成功并获取 Token
echo -e "${YELLOW}测试 6: 登录并获取 JWT Token${NC}"
response=$(curl -s -X POST $API_URL/api/login \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$RANDOM_USER\",\"password\":\"password123\"}")
echo "响应: $response"
if echo "$response" | grep -q '"code":200'; then
    echo -e "${GREEN}✅ 通过 - 登录成功${NC}"
    TOKEN=$(echo "$response" | grep -o '"token":"[^"]*' | cut -d'"' -f4)
    if [ -n "$TOKEN" ]; then
        echo -e "${GREEN}✅ Token 已生成${NC}"
        echo "Token (前50字符): ${TOKEN:0:50}..."
    else
        echo -e "${RED}❌ Token 未生成${NC}"
    fi
    echo ""
else
    echo -e "${RED}❌ 失败${NC}\n"
fi

# 测试 7: 错误密码登录
echo -e "${YELLOW}测试 7: 使用错误密码登录${NC}"
response=$(curl -s -X POST $API_URL/api/login \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$RANDOM_USER\",\"password\":\"wrongpassword\"}")
echo "响应: $response"
if echo "$response" | grep -q "密码错误"; then
    echo -e "${GREEN}✅ 通过 - 正确拦截错误密码${NC}\n"
else
    echo -e "${RED}❌ 失败${NC}\n"
fi

# 测试 8: 不存在的用户登录
echo -e "${YELLOW}测试 8: 登录不存在的用户${NC}"
response=$(curl -s -X POST $API_URL/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"nonexistent_user","password":"password123"}')
echo "响应: $response"
if echo "$response" | grep -q "用户不存在"; then
    echo -e "${GREEN}✅ 通过${NC}\n"
else
    echo -e "${RED}❌ 失败${NC}\n"
fi

# 测试 9: 使用 Token 访问受保护接口
echo -e "${YELLOW}测试 9: 使用有效 Token 访问 /api/profile${NC}"
if [ -n "$TOKEN" ]; then
    response=$(curl -s -X GET $API_URL/api/profile \
      -H "Authorization: Bearer $TOKEN")
    echo "响应: $response"
    if echo "$response" | grep -q '"code":200'; then
        echo -e "${GREEN}✅ 通过 - 认证成功${NC}\n"
    else
        echo -e "${RED}❌ 失败${NC}\n"
    fi
else
    echo -e "${RED}❌ 跳过 - 没有有效的 Token${NC}\n"
fi

# 测试 10: 无 Token 访问受保护接口
echo -e "${YELLOW}测试 10: 无 Token 访问受保护接口${NC}"
response=$(curl -s -X GET $API_URL/api/profile)
echo "响应: $response"
if echo "$response" | grep -q "缺少 Authorization 头"; then
    echo -e "${GREEN}✅ 通过 - 正确拦截未认证请求${NC}\n"
else
    echo -e "${RED}❌ 失败${NC}\n"
fi

# 测试 11: 无效 Token 访问受保护接口
echo -e "${YELLOW}测试 11: 使用无效 Token 访问受保护接口${NC}"
response=$(curl -s -X GET $API_URL/api/profile \
  -H "Authorization: Bearer invalid_token_here")
echo "响应: $response"
if echo "$response" | grep -q "无效的 Token"; then
    echo -e "${GREEN}✅ 通过 - 正确拦截无效 Token${NC}\n"
else
    echo -e "${RED}❌ 失败${NC}\n"
fi

echo "========================================="
echo "✅ 测试完成！"
echo "========================================="
