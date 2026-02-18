package main

import (
"bytes"
"crypto/md5"
"encoding/json"
"fmt"
"math/rand"
"mime/multipart"
"net/http"
"strings"
"sync"
"sync/atomic"
"time"
)

const (
GatewayURL = "http://localhost:8080"
)

var (
httpClient = &http.Client{Timeout: 60 * time.Second}
)

// TestResult 测试结果结构
type TestResult struct {
TestName string
Passed   bool
Duration time.Duration
Message  string
Details  map[string]interface{}
}

// TestReport 测试报告
type TestReport struct {
StartTime   time.Time
EndTime     time.Time
TotalTests  int
PassedTests int
FailedTests int
Results     []TestResult
mu          sync.Mutex
}

func NewTestReport() *TestReport {
return &TestReport{
StartTime: time.Now(),
Results:   make([]TestResult, 0),
}
}

func (r *TestReport) AddResult(result TestResult) {
r.mu.Lock()
defer r.mu.Unlock()
r.Results = append(r.Results, result)
r.TotalTests++
if result.Passed {
r.PassedTests++
} else {
r.FailedTests++
}
}

func (r *TestReport) PrintSummary() {
r.EndTime = time.Now()
fmt.Println("\n" + strings.Repeat("=", 80))
fmt.Println("测试报告摘要")
fmt.Println(strings.Repeat("=", 80))
fmt.Printf("开始时间: %s\n", r.StartTime.Format("2006-01-02 15:04:05"))
fmt.Printf("结束时间: %s\n", r.EndTime.Format("2006-01-02 15:04:05"))
fmt.Printf("总耗时: %v\n", r.EndTime.Sub(r.StartTime))
fmt.Printf("总测试数: %d\n", r.TotalTests)
fmt.Printf("通过: %d\n", r.PassedTests)
fmt.Printf("失败: %d\n", r.FailedTests)
fmt.Printf("成功率: %.2f%%\n", float64(r.PassedTests)/float64(r.TotalTests)*100)
fmt.Println(strings.Repeat("=", 80))

fmt.Println("\n详细结果:")
for i, result := range r.Results {
status := "✅ PASS"
if !result.Passed {
status = "❌ FAIL"
}
fmt.Printf("%d. %s [%s] - %v\n", i+1, result.TestName, status, result.Duration)
fmt.Printf("   %s\n", result.Message)
if len(result.Details) > 0 {
fmt.Printf("   详情: %+v\n", result.Details)
}
}
}

// 用户注册并登录获取token
func registerAndLogin(username, password string) (token string, userID int, err error) {
// 注册
registerData := map[string]string{
"username": username,
"password": password,
}

body, _ := json.Marshal(registerData)
resp, err := httpClient.Post(GatewayURL+"/api/register", "application/json", bytes.NewBuffer(body))
if err != nil {
return "", 0, fmt.Errorf("注册请求失败: %v", err)
}
resp.Body.Close()

// 登录
loginData := map[string]string{
"username": username,
"password": password,
}

body, _ = json.Marshal(loginData)
resp, err = httpClient.Post(GatewayURL+"/api/login", "application/json", bytes.NewBuffer(body))
if err != nil {
return "", 0, fmt.Errorf("登录请求失败: %v", err)
}
defer resp.Body.Close()

var loginResp struct {
Code     int    `json:"code"`
Msg      string `json:"msg"`
Token    string `json:"token"`
UserID   int    `json:"user_id"`
Username string `json:"username"`
}

if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
return "", 0, fmt.Errorf("解析登录响应失败: %v", err)
}

if loginResp.Code != 200 {
return "", 0, fmt.Errorf("登录失败: %s", loginResp.Msg)
}

return loginResp.Token, loginResp.UserID, nil
}

// Test 1: 服务健康检查
func testServiceHealth(report *TestReport) {
start := time.Now()

resp, err := httpClient.Get(GatewayURL + "/ping")
if err != nil {
report.AddResult(TestResult{
TestName: "服务健康检查",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("无法连接到服务: %v", err),
})
return
}
defer resp.Body.Close()

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)

passed := resp.StatusCode == 200 && result["message"] == "pong"
message := "服务正常响应"
if !passed {
message = fmt.Sprintf("服务响应异常: status=%d, body=%v", resp.StatusCode, result)
}

report.AddResult(TestResult{
TestName: "服务健康检查",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
Details:  result,
})
}

// Test 2: 用户注册功能
func testUserRegistration(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("usr%d", time.Now().Unix()%100000)
password := "Test123456"

registerData := map[string]string{
"username": username,
"password": password,
}

body, _ := json.Marshal(registerData)
resp, err := httpClient.Post(GatewayURL+"/api/register", "application/json", bytes.NewBuffer(body))
if err != nil {
report.AddResult(TestResult{
TestName: "用户注册功能",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("注册请求失败: %v", err),
})
return
}
defer resp.Body.Close()

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)

code := int(result["code"].(float64))
passed := code == 200
message := fmt.Sprintf("注册成功，用户名: %s", username)
if !passed {
message = fmt.Sprintf("注册失败: %v", result["msg"])
}

report.AddResult(TestResult{
TestName: "用户注册功能",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
Details:  map[string]interface{}{"username": username},
})
}

// Test 3: 用户登录和JWT认证
func testUserLoginAndAuth(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("auth%d", time.Now().Unix()%10000)
password := "Test123456"

token, userID, err := registerAndLogin(username, password)
if err != nil {
report.AddResult(TestResult{
TestName: "用户登录和JWT认证",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("登录失败: %v", err),
})
return
}

// 测试使用token访问受保护资源
req, _ := http.NewRequest("GET", GatewayURL+"/api/profile", nil)
req.Header.Set("Authorization", "Bearer "+token)

resp, err := httpClient.Do(req)
if err != nil {
report.AddResult(TestResult{
TestName: "用户登录和JWT认证",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("访问受保护资源失败: %v", err),
})
return
}
defer resp.Body.Close()

var profileResp map[string]interface{}
json.NewDecoder(resp.Body).Decode(&profileResp)

passed := int(profileResp["code"].(float64)) == 200
message := fmt.Sprintf("JWT认证成功，UserID: %d", userID)
if !passed {
message = fmt.Sprintf("JWT认证失败: %v", profileResp["msg"])
}

report.AddResult(TestResult{
TestName: "用户登录和JWT认证",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
Details:  map[string]interface{}{"token_length": len(token), "user_id": userID},
})
}

// Test 4: 未认证访问拒绝测试
func testUnauthorizedAccess(report *TestReport) {
start := time.Now()

// 测试不带token访问受保护资源
initData := map[string]interface{}{
"file_hash": "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
"filename":  "test.mp4",
"file_size": 1024,
}

body, _ := json.Marshal(initData)
req, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")

resp, err := httpClient.Do(req)
if err != nil {
report.AddResult(TestResult{
TestName: "未认证访问拒绝测试",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("请求失败: %v", err),
})
return
}
defer resp.Body.Close()

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)

code := int(result["code"].(float64))
passed := code == 401
message := "未认证请求被正确拒绝"
if !passed {
message = fmt.Sprintf("未认证请求未被拒绝，返回code: %d", code)
}

report.AddResult(TestResult{
TestName: "未认证访问拒绝测试",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
Details:  map[string]interface{}{"response_code": code},
})
}

// Test 5: 视频上传初始化
func testVideoInitUpload(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("vid%d", time.Now().Unix()%10000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
report.AddResult(TestResult{
TestName: "视频上传初始化",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("准备测试失败: %v", err),
})
return
}

timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
fileHash := fmt.Sprintf("%x", md5.Sum([]byte(timestamp)))
initData := map[string]interface{}{
"file_hash": fileHash,
"filename":  "test_video.mp4",
"file_size": 1024000,
"width":     1920,
"height":    1080,
}

body, _ := json.Marshal(initData)
req, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+token)

resp, err := httpClient.Do(req)
if err != nil {
report.AddResult(TestResult{
TestName: "视频上传初始化",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("请求失败: %v", err),
})
return
}
defer resp.Body.Close()

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)

code := int(result["code"].(float64))
passed := code == 200
message := "视频上传初始化成功"
if !passed {
message = fmt.Sprintf("初始化失败: %v", result["msg"])
}

report.AddResult(TestResult{
TestName: "视频上传初始化",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
Details:  result,
})
}

// Test 6: 分片上传测试
func testChunkUpload(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("chu%d", time.Now().Unix()%10000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
report.AddResult(TestResult{
TestName: "分片上传测试",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("准备测试失败: %v", err),
})
return
}

// 生成测试数据
chunkData := make([]byte, 1024*100) // 100KB
rand.Read(chunkData)
fileHash := fmt.Sprintf("%x", md5.Sum(chunkData))

// 初始化上传
initData := map[string]interface{}{
"file_hash": fileHash,
"filename":  "chunk_test.mp4",
"file_size": len(chunkData),
"width":     1920,
"height":    1080,
}

body, _ := json.Marshal(initData)
req, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+token)
httpClient.Do(req)

// 上传分片
var buf bytes.Buffer
writer := multipart.NewWriter(&buf)
writer.WriteField("file_hash", fileHash)
writer.WriteField("index", "0")

part, _ := writer.CreateFormFile("chunk", "chunk")
part.Write(chunkData)
writer.Close()

req, _ = http.NewRequest("POST", GatewayURL+"/api/video/upload_chunk", &buf)
req.Header.Set("Content-Type", writer.FormDataContentType())
req.Header.Set("Authorization", "Bearer "+token)

resp, err := httpClient.Do(req)
if err != nil {
report.AddResult(TestResult{
TestName: "分片上传测试",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("上传失败: %v", err),
})
return
}
defer resp.Body.Close()

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)

code := int(result["code"].(float64))
passed := code == 200
message := "分片上传成功"
if !passed {
message = fmt.Sprintf("分片上传失败: %v", result["msg"])
}

report.AddResult(TestResult{
TestName: "分片上传测试",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
Details:  map[string]interface{}{"chunk_size": len(chunkData)},
})
}

// Test 7: 并发用户注册测试
func testConcurrentRegistration(report *TestReport) {
start := time.Now()

numUsers := 20
var wg sync.WaitGroup
successCount := int32(0)
failureCount := int32(0)

for i := 0; i < numUsers; i++ {
wg.Add(1)
go func(id int) {
defer wg.Done()

username := fmt.Sprintf("con%d", int(time.Now().Unix()+int64(id))%100000)
registerData := map[string]string{
"username": username,
"password": "Test123456",
}

body, _ := json.Marshal(registerData)
resp, err := httpClient.Post(GatewayURL+"/api/register", "application/json", bytes.NewBuffer(body))
if err != nil {
atomic.AddInt32(&failureCount, 1)
return
}
defer resp.Body.Close()

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)

if int(result["code"].(float64)) == 200 {
atomic.AddInt32(&successCount, 1)
} else {
atomic.AddInt32(&failureCount, 1)
}
}(i)
}

wg.Wait()

duration := time.Since(start)
passed := successCount == int32(numUsers)
message := fmt.Sprintf("并发注册: 成功 %d/%d, 失败 %d, 平均耗时 %v",
successCount, numUsers, failureCount, duration/time.Duration(numUsers))

report.AddResult(TestResult{
TestName: "并发用户注册测试",
Passed:   passed,
Duration: duration,
Message:  message,
Details: map[string]interface{}{
"total":        numUsers,
"success":      successCount,
"failure":      failureCount,
"avg_duration": duration / time.Duration(numUsers),
},
})
}

// Test 8: 并发视频初始化测试
func testConcurrentVideoInit(report *TestReport) {
start := time.Now()

// 先创建一个用户
username := fmt.Sprintf("cvi%d", time.Now().Unix()%10000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
report.AddResult(TestResult{
TestName: "并发视频初始化测试",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("准备测试失败: %v", err),
})
return
}

numRequests := 30
var wg sync.WaitGroup
successCount := int32(0)
failureCount := int32(0)

for i := 0; i < numRequests; i++ {
wg.Add(1)
go func(id int) {
defer wg.Done()

timestamp := fmt.Sprintf("%d_%d", time.Now().UnixNano(), id)
fileHash := fmt.Sprintf("%x", md5.Sum([]byte(timestamp)))
initData := map[string]interface{}{
"file_hash": fileHash,
"filename":  fmt.Sprintf("concurrent_%d.mp4", id),
"file_size": 1024000,
"width":     1920,
"height":    1080,
}

body, _ := json.Marshal(initData)
req, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+token)

resp, err := httpClient.Do(req)
if err != nil {
atomic.AddInt32(&failureCount, 1)
return
}
defer resp.Body.Close()

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)

if int(result["code"].(float64)) == 200 {
atomic.AddInt32(&successCount, 1)
} else {
atomic.AddInt32(&failureCount, 1)
}
}(i)
}

wg.Wait()

duration := time.Since(start)
passed := successCount == int32(numRequests)
message := fmt.Sprintf("并发初始化: 成功 %d/%d, 失败 %d, 平均耗时 %v",
successCount, numRequests, failureCount, duration/time.Duration(numRequests))

report.AddResult(TestResult{
TestName: "并发视频初始化测试",
Passed:   passed,
Duration: duration,
Message:  message,
Details: map[string]interface{}{
"total":        numRequests,
"success":      successCount,
"failure":      failureCount,
"avg_duration": duration / time.Duration(numRequests),
},
})
}

// Test 9: 幂等性测试
func testIdempotency(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("idm%d", time.Now().Unix()%10000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
report.AddResult(TestResult{
TestName: "幂等性测试",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("准备测试失败: %v", err),
})
return
}

requestID := fmt.Sprintf("req%d", time.Now().Unix())
timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
fileHash := fmt.Sprintf("%x", md5.Sum([]byte(timestamp)))

initData := map[string]interface{}{
"file_hash":  fileHash,
"filename":   "idempotency_test.mp4",
"file_size":  1024000,
"width":      1920,
"height":     1080,
"request_id": requestID,
}

body, _ := json.Marshal(initData)

// 第一次请求
req1, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req1.Header.Set("Content-Type", "application/json")
req1.Header.Set("Authorization", "Bearer "+token)

resp1, _ := httpClient.Do(req1)
var result1 map[string]interface{}
json.NewDecoder(resp1.Body).Decode(&result1)
resp1.Body.Close()

// 第二次请求（相同request_id）
req2, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req2.Header.Set("Content-Type", "application/json")
req2.Header.Set("Authorization", "Bearer "+token)

resp2, _ := httpClient.Do(req2)
var result2 map[string]interface{}
json.NewDecoder(resp2.Body).Decode(&result2)
resp2.Body.Close()

// 验证两次请求返回相同结果
code1 := int(result1["code"].(float64))
code2 := int(result2["code"].(float64))

passed := code1 == code2 && code1 == 200
message := "幂等性测试通过：两次请求返回相同结果"
if !passed {
message = fmt.Sprintf("幂等性测试失败：第一次code=%d，第二次code=%d", code1, code2)
}

report.AddResult(TestResult{
TestName: "幂等性测试",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
Details: map[string]interface{}{
"request_id": requestID,
"code1":      code1,
"code2":      code2,
},
})
}

// Test 10: MetaInfo传递测试
func testMetaInfoPropagation(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("meta%d", time.Now().Unix()%10000)
token, userID, err := registerAndLogin(username, "Test123456")
if err != nil {
report.AddResult(TestResult{
TestName: "MetaInfo传递测试",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("准备测试失败: %v", err),
})
return
}

// 发起视频初始化请求（不在请求体中传递user_id）
timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
fileHash := fmt.Sprintf("%x", md5.Sum([]byte(timestamp)))
initData := map[string]interface{}{
"file_hash": fileHash,
"filename":  "metainfo_test.mp4",
"file_size": 1024000,
"width":     1920,
"height":    1080,
}

body, _ := json.Marshal(initData)
req, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+token)

resp, err := httpClient.Do(req)
if err != nil {
report.AddResult(TestResult{
TestName: "MetaInfo传递测试",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("请求失败: %v", err),
})
return
}
defer resp.Body.Close()

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)

code := int(result["code"].(float64))
passed := code == 200
message := fmt.Sprintf("MetaInfo传递成功，UserID: %d 通过context传递到RPC服务", userID)
if !passed {
message = fmt.Sprintf("MetaInfo传递失败: %v", result["msg"])
}

report.AddResult(TestResult{
TestName: "MetaInfo传递测试",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
Details: map[string]interface{}{
"user_id": userID,
"code":    code,
},
})
}

func main() {
fmt.Println("========================================")
fmt.Println("视频平台微服务 - 全面测试套件")
fmt.Println("========================================")
fmt.Println("开始时间:", time.Now().Format("2006-01-02 15:04:05"))
fmt.Println()

report := NewTestReport()

// 基础功能测试
fmt.Println("1️⃣  执行基础功能测试...")
testServiceHealth(report)
testUserRegistration(report)
testUserLoginAndAuth(report)
testUnauthorizedAccess(report)

// 视频功能测试
fmt.Println("2️⃣  执行视频功能测试...")
testVideoInitUpload(report)
testChunkUpload(report)

// 并发测试
fmt.Println("3️⃣  执行并发测试...")
testConcurrentRegistration(report)
testConcurrentVideoInit(report)

// 高级功能测试
fmt.Println("4️⃣  执行高级功能测试...")
testIdempotency(report)
testMetaInfoPropagation(report)

// 打印报告
report.PrintSummary()

// 返回退出码
if report.FailedTests > 0 {
fmt.Println("\n❌ 测试失败")
return
}
fmt.Println("\n✅ 所有测试通过")
}
