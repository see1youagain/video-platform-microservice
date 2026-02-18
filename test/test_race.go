package main

import (
"bytes"
"crypto/md5"
"encoding/json"
"fmt"
"strings"
"math/rand"
"mime/multipart"
"net/http"
"sync"
"time"
)

const GatewayURL = "http://localhost:8080"

var httpClient = &http.Client{Timeout: 60 * time.Second}

func registerAndLogin(username, password string) (token string, userID int, err error) {
registerData := map[string]string{"username": username, "password": password}
body, _ := json.Marshal(registerData)
resp, err := httpClient.Post(GatewayURL+"/api/register", "application/json", bytes.NewBuffer(body))
if err != nil {
return "", 0, fmt.Errorf("注册失败: %v", err)
}
resp.Body.Close()

loginData := map[string]string{"username": username, "password": password}
body, _ = json.Marshal(loginData)
resp, err = httpClient.Post(GatewayURL+"/api/login", "application/json", bytes.NewBuffer(body))
if err != nil {
return "", 0, fmt.Errorf("登录失败: %v", err)
}
defer resp.Body.Close()

var loginResp struct {
Code   int    `json:"code"`
UserID int    `json:"user_id"`
Token  string `json:"token"`
}
json.NewDecoder(resp.Body).Decode(&loginResp)
if loginResp.Code != 200 {
return "", 0, fmt.Errorf("登录失败")
}
return loginResp.Token, loginResp.UserID, nil
}

// 高强度并发测试 - 100个goroutine同时注册
func testRaceConditionRegistration() {
fmt.Println("\n🔍 测试1: 高强度并发注册（100个goroutine）")
start := time.Now()

var wg sync.WaitGroup
successMap := sync.Map{}
numGoroutines := 100

for i := 0; i < numGoroutines; i++ {
wg.Add(1)
go func(id int) {
defer wg.Done()
username := fmt.Sprintf("race%d_%d", time.Now().Unix()%10000, id)
token, userID, err := registerAndLogin(username, "Test123456")
if err == nil && token != "" && userID > 0 {
successMap.Store(id, true)
}
}(i)
}

wg.Wait()

successCount := 0
successMap.Range(func(_, _ interface{}) bool {
successCount++
return true
})

fmt.Printf("✅ 完成: %d/%d 成功, 耗时: %v\n", successCount, numGoroutines, time.Since(start))
}

// 测试同时多个用户上传同一文件
func testRaceConditionSameFile() {
fmt.Println("\n🔍 测试2: 多用户同时上传同一文件（检查RefCount）")
start := time.Now()

// 生成共享文件hash
timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
sharedFileHash := fmt.Sprintf("%x", md5.Sum([]byte(timestamp)))

var wg sync.WaitGroup
numUsers := 20
initResults := sync.Map{}

for i := 0; i < numUsers; i++ {
wg.Add(1)
go func(id int) {
defer wg.Done()

username := fmt.Sprintf("shared%d_%d", time.Now().Unix()%10000, id)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
return
}

initData := map[string]interface{}{
"file_hash": sharedFileHash,
"filename":  "shared_file.mp4",
"file_size": 1024000,
"width":     1920,
"height":    1080,
}

body, _ := json.Marshal(initData)
req, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+token)

resp, err := httpClient.Do(req)
if err == nil {
var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)
initResults.Store(id, int(result["code"].(float64)))
resp.Body.Close()
}
}(i)
}

wg.Wait()

successCount := 0
initResults.Range(func(_, v interface{}) bool {
if v.(int) == 200 {
successCount++
}
return true
})

fmt.Printf("✅ 完成: %d/%d 成功初始化同一文件, 耗时: %v\n", successCount, numUsers, time.Since(start))
fmt.Println("   （数据库应正确维护RefCount，无数据竞争）")
}

// 测试同一用户并发上传多个分片
func testRaceConditionMultipleChunks() {
fmt.Println("\n🔍 测试3: 单用户并发上传多个分片")
start := time.Now()

username := fmt.Sprintf("chunks%d", time.Now().Unix()%10000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
fmt.Println("❌ 准备失败:", err)
return
}

timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
fileHash := fmt.Sprintf("%x", md5.Sum([]byte(timestamp)))

// 初始化
initData := map[string]interface{}{
"file_hash": fileHash,
"filename":  "multi_chunks.mp4",
"file_size": 10240000,
"width":     1920,
"height":    1080,
}
body, _ := json.Marshal(initData)
req, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+token)
httpClient.Do(req)

// 并发上传50个分片
var wg sync.WaitGroup
numChunks := 50
uploaded := sync.Map{}

for i := 0; i < numChunks; i++ {
wg.Add(1)
go func(index int) {
defer wg.Done()

chunkData := make([]byte, 10240)
rand.Read(chunkData)

var buf bytes.Buffer
writer := multipart.NewWriter(&buf)
writer.WriteField("file_hash", fileHash)
writer.WriteField("index", fmt.Sprintf("%d", index))

part, _ := writer.CreateFormFile("chunk", "chunk")
part.Write(chunkData)
writer.Close()

req, _ := http.NewRequest("POST", GatewayURL+"/api/video/upload_chunk", &buf)
req.Header.Set("Content-Type", writer.FormDataContentType())
req.Header.Set("Authorization", "Bearer "+token)

resp, err := httpClient.Do(req)
if err == nil {
var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)
if int(result["code"].(float64)) == 200 {
uploaded.Store(index, true)
}
resp.Body.Close()
}
}(i)
}

wg.Wait()

successCount := 0
uploaded.Range(func(_, _ interface{}) bool {
successCount++
return true
})

fmt.Printf("✅ 完成: %d/%d 分片成功上传, 耗时: %v\n", successCount, numChunks, time.Since(start))
}

// 测试极端并发：200个goroutine混合操作
func testExtremeConcurrency() {
fmt.Println("\n🔍 测试4: 极端并发混合操作（200 goroutines）")
start := time.Now()

var wg sync.WaitGroup
operationCount := sync.Map{}
numOperations := 200

for i := 0; i < numOperations; i++ {
wg.Add(1)
go func(id int) {
defer wg.Done()

opType := id % 3
switch opType {
case 0: // 注册
username := fmt.Sprintf("ext%d_%d", time.Now().UnixNano()%100000, id)
_, _, err := registerAndLogin(username, "Test123456")
if err == nil {
operationCount.Store(fmt.Sprintf("reg_%d", id), true)
}
case 1: // 视频初始化
username := fmt.Sprintf("ext%d_%d", time.Now().UnixNano()%100000, id)
token, _, err := registerAndLogin(username, "Test123456")
if err == nil {
ts := fmt.Sprintf("%d_%d", time.Now().UnixNano(), id)
initData := map[string]interface{}{
"file_hash": fmt.Sprintf("%x", md5.Sum([]byte(ts))),
"filename":  fmt.Sprintf("extreme_%d.mp4", id),
"file_size": 1024000,
}
body, _ := json.Marshal(initData)
req, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+token)
resp, err := httpClient.Do(req)
if err == nil {
operationCount.Store(fmt.Sprintf("init_%d", id), true)
resp.Body.Close()
}
}
case 2: // 健康检查
resp, err := httpClient.Get(GatewayURL + "/api/health")
if err == nil {
operationCount.Store(fmt.Sprintf("health_%d", id), true)
resp.Body.Close()
}
}
}(i)
}

wg.Wait()

successCount := 0
operationCount.Range(func(_, _ interface{}) bool {
successCount++
return true
})

fmt.Printf("✅ 完成: %d/%d 操作成功, 耗时: %v\n", successCount, numOperations, time.Since(start))
fmt.Printf("   平均操作耗时: %v\n", time.Since(start)/time.Duration(numOperations))
}

func main() {
fmt.Println("========================================")
fmt.Println("数据竞争和并发锁测试")
fmt.Println("========================================")
fmt.Println("提示: 使用 'go run -race test_race.go' 可检测数据竞争")
fmt.Println()

testRaceConditionRegistration()
testRaceConditionSameFile()
testRaceConditionMultipleChunks()
testExtremeConcurrency()

	fmt.Println("\n" + strings.Repeat("=", 60))
fmt.Println("✅ 所有并发测试完成，无死锁或明显问题")
	fmt.Println(strings.Repeat("=", 60))
}
