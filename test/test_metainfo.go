package main

import (
"bytes"
"encoding/json"
"fmt"
"io"
"net/http"
)

const (
GatewayURL = "http://localhost:8080"
)

func main() {
fmt.Println("=== 测试 MetaInfo 传递 ===")

// 1. 注册测试用户
username := fmt.Sprintf("metainfo_test_%d", 12345)
password := "Test123456"

registerData := map[string]string{
"username": username,
"password": password,
}

body, _ := json.Marshal(registerData)
resp, err := http.Post(GatewayURL+"/api/register", "application/json", bytes.NewBuffer(body))
if err != nil {
fmt.Printf("❌ 注册失败: %v\n", err)
return
}
resp.Body.Close()

// 2. 登录获取token
loginData := map[string]string{
"username": username,
"password": password,
}

body, _ = json.Marshal(loginData)
resp, err = http.Post(GatewayURL+"/api/login", "application/json", bytes.NewBuffer(body))
if err != nil {
fmt.Printf("❌ 登录失败: %v\n", err)
return
}

var loginResp struct {
Code     int    `json:"code"`
Msg      string `json:"msg"`
Token    string `json:"token"`
UserID   string `json:"user_id"`
Username string `json:"username"`
}

json.NewDecoder(resp.Body).Decode(&loginResp)
resp.Body.Close()

if loginResp.Code != 200 {
fmt.Printf("❌ 登录失败: %s\n", loginResp.Msg)
return
}

fmt.Printf("✅ 用户登录成功: %s (UserID: %s, Token: %s...)\n", 
loginResp.Username, loginResp.UserID, loginResp.Token[:20])

// 3. 使用JWT token初始化上传（不传user_id参数）
initData := map[string]interface{}{
"file_hash": "test_metainfo_hash_12345",
"filename":  "metainfo_test.mp4",
"file_size": 1024,
"width":     1920,
"height":    1080,
}

body, _ = json.Marshal(initData)
req, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+loginResp.Token)

client := &http.Client{}
resp, err = client.Do(req)
if err != nil {
fmt.Printf("❌ 初始化上传失败: %v\n", err)
return
}

var initResp map[string]interface{}
bodyBytes, _ := io.ReadAll(resp.Body)
json.Unmarshal(bodyBytes, &initResp)
resp.Body.Close()

fmt.Printf("\n初始化上传响应:\n")
fmt.Printf("  Code: %v\n", initResp["code"])
fmt.Printf("  Msg: %v\n", initResp["msg"])
fmt.Printf("  Status: %v\n", initResp["status"])

if code, ok := initResp["code"].(float64); ok && code == 200 {
fmt.Println("\n✅ MetaInfo 传递测试成功！")
fmt.Println("说明：网关通过 metainfo.WithPersistentValue 将 user_id 传递给了 RPC 服务")
fmt.Println("RPC服务从 metainfo.GetPersistentValue 中成功获取到了用户信息")
} else {
fmt.Printf("\n❌ MetaInfo 传递测试失败: %v\n", initResp["msg"])
}

// 4. 测试下载接口（验证JWT认证和metainfo传递）
fmt.Println("\n--- 测试下载接口的认证 ---")
req, _ = http.NewRequest("GET", GatewayURL+"/api/video/download?file_hash=test_hash", nil)
req.Header.Set("Authorization", "Bearer "+loginResp.Token)

resp, err = client.Do(req)
if err != nil {
fmt.Printf("❌ 下载请求失败: %v\n", err)
return
}

var downloadResp map[string]interface{}
json.NewDecoder(resp.Body).Decode(&downloadResp)
resp.Body.Close()

fmt.Printf("下载响应: code=%v, msg=%v\n", downloadResp["code"], downloadResp["msg"])

// 5. 测试未认证的请求应该被拒绝
fmt.Println("\n--- 测试未认证请求 ---")
req, _ = http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
// 不设置 Authorization header

resp, err = client.Do(req)
if err != nil {
fmt.Printf("❌ 请求失败: %v\n", err)
return
}

var unauthResp map[string]interface{}
json.NewDecoder(resp.Body).Decode(&unauthResp)
resp.Body.Close()

if code, ok := unauthResp["code"].(float64); ok && code == 401 {
fmt.Println("✅ 未认证请求被正确拒绝")
} else {
fmt.Printf("❌ 未认证请求未被拒绝: code=%v\n", unauthResp["code"])
}

fmt.Println("\n=== 测试完成 ===")
}
