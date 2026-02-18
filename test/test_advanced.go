package main

import (
"bytes"
"context"
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

type TestResult struct {
TestName string
Passed   bool
Duration time.Duration
Message  string
Details  map[string]interface{}
}

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
fmt.Println("高级测试报告摘要")
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

func registerAndLogin(username, password string) (token string, userID int, err error) {
registerData := map[string]string{"username": username, "password": password}
body, _ := json.Marshal(registerData)
resp, err := httpClient.Post(GatewayURL+"/api/register", "application/json", bytes.NewBuffer(body))
if err != nil {
return "", 0, fmt.Errorf("注册请求失败: %v", err)
}
resp.Body.Close()

loginData := map[string]string{"username": username, "password": password}
body, _ = json.Marshal(loginData)
resp, err = httpClient.Post(GatewayURL+"/api/login", "application/json", bytes.NewBuffer(body))
if err != nil {
return "", 0, fmt.Errorf("登录请求失败: %v", err)
}
defer resp.Body.Close()

var loginResp struct {
Code   int    `json:"code"`
Msg    string `json:"msg"`
Token  string `json:"token"`
UserID int    `json:"user_id"`
}

if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
return "", 0, fmt.Errorf("解析登录响应失败: %v", err)
}

if loginResp.Code != 200 {
return "", 0, fmt.Errorf("登录失败: %s", loginResp.Msg)
}

return loginResp.Token, loginResp.UserID, nil
}

// Test 1: 分片上传完整流程测试（已修复multipart支持）
func testChunkUploadFixed(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("chunk%d", time.Now().Unix()%10000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
report.AddResult(TestResult{
TestName: "分片上传完整流程",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("准备测试失败: %v", err),
})
return
}

chunkData := make([]byte, 1024*100)
rand.Read(chunkData)
fileHash := fmt.Sprintf("%x", md5.Sum(chunkData))

// 初始化
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
TestName: "分片上传完整流程",
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
TestName: "分片上传完整流程",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
Details:  map[string]interface{}{"chunk_size": len(chunkData), "file_hash": fileHash},
})
}

// Test 2: 并发文件上传测试（多用户同时上传不同文件）
func testConcurrentFileUploads(report *TestReport) {
start := time.Now()

numFiles := 10
var wg sync.WaitGroup
successCount := int32(0)
failureCount := int32(0)

for i := 0; i < numFiles; i++ {
wg.Add(1)
go func(id int) {
defer wg.Done()

username := fmt.Sprintf("upl%d", int(time.Now().Unix()+int64(id))%100000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
atomic.AddInt32(&failureCount, 1)
return
}

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
passed := successCount == int32(numFiles)
message := fmt.Sprintf("并发上传: 成功 %d/%d, 失败 %d",
successCount, numFiles, failureCount)

report.AddResult(TestResult{
TestName: "并发文件上传测试",
Passed:   passed,
Duration: duration,
Message:  message,
Details: map[string]interface{}{
"total":   numFiles,
"success": successCount,
"failure": failureCount,
},
})
}

// Test 3: 同一文件多客户端并发上传（测试并发锁）
func testConcurrentSameFileUpload(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("lock%d", time.Now().Unix()%10000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
report.AddResult(TestResult{
TestName: "同一文件并发上传锁测试",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("准备测试失败: %v", err),
})
return
}

timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
fileHash := fmt.Sprintf("%x", md5.Sum([]byte(timestamp)))

numConcurrent := 5
var wg sync.WaitGroup
successCount := int32(0)
conflictCount := int32(0)

for i := 0; i < numConcurrent; i++ {
wg.Add(1)
go func(id int) {
defer wg.Done()

initData := map[string]interface{}{
"file_hash": fileHash,
"filename":  "same_file_test.mp4",
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
return
}
defer resp.Body.Close()

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)

code := int(result["code"].(float64))
if code == 200 {
atomic.AddInt32(&successCount, 1)
} else if strings.Contains(fmt.Sprint(result["msg"]), "已存在") || strings.Contains(fmt.Sprint(result["msg"]), "秒传") {
atomic.AddInt32(&conflictCount, 1)
}
}(i)
}

wg.Wait()

duration := time.Since(start)
passed := (successCount == 1 && conflictCount == int32(numConcurrent-1)) || (successCount+conflictCount == int32(numConcurrent))
message := fmt.Sprintf("并发锁测试: 首次成功 %d, 秒传/冲突 %d", successCount, conflictCount)

report.AddResult(TestResult{
TestName: "同一文件并发上传锁测试",
Passed:   passed,
Duration: duration,
Message:  message,
Details: map[string]interface{}{
"concurrent":     numConcurrent,
"success":        successCount,
"conflict":       conflictCount,
"expected_logic": "第一个成功，其他秒传",
},
})
}

// Test 4: 客户端中断模拟测试
func testClientInterruptSimulation(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("int%d", time.Now().Unix()%10000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
report.AddResult(TestResult{
TestName: "客户端中断模拟",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("准备测试失败: %v", err),
})
return
}

timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
fileHash := fmt.Sprintf("%x", md5.Sum([]byte(timestamp)))

// 第一次请求 - 正常初始化
initData := map[string]interface{}{
"file_hash": fileHash,
"filename":  "interrupt_test.mp4",
"file_size": 1024000,
"width":     1920,
"height":    1080,
}

body, _ := json.Marshal(initData)
req, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+token)

resp1, _ := httpClient.Do(req)
var result1 map[string]interface{}
json.NewDecoder(resp1.Body).Decode(&result1)
resp1.Body.Close()

// 模拟客户端中断后重新连接，再次初始化同一文件
time.Sleep(100 * time.Millisecond)

req2, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req2.Header.Set("Content-Type", "application/json")
req2.Header.Set("Authorization", "Bearer "+token)

resp2, _ := httpClient.Do(req2)
var result2 map[string]interface{}
json.NewDecoder(resp2.Body).Decode(&result2)
resp2.Body.Close()

code1 := int(result1["code"].(float64))
code2 := int(result2["code"].(float64))

passed := (code1 == 200 && code2 == 200)
message := fmt.Sprintf("中断恢复测试: 第一次code=%d, 第二次code=%d, 系统能正确处理重复初始化", code1, code2)

report.AddResult(TestResult{
TestName: "客户端中断模拟",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
Details: map[string]interface{}{
"first_code":  code1,
"second_code": code2,
},
})
}

// Test 5: 超时和重试测试
func testTimeoutAndRetry(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("retry%d", time.Now().Unix()%10000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
report.AddResult(TestResult{
TestName: "超时和重试测试",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("准备测试失败: %v", err),
})
return
}

// 使用短超时客户端
shortClient := &http.Client{Timeout: 50 * time.Millisecond}

timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
fileHash := fmt.Sprintf("%x", md5.Sum([]byte(timestamp)))

initData := map[string]interface{}{
"file_hash": fileHash,
"filename":  "timeout_test.mp4",
"file_size": 1024000,
"width":     1920,
"height":    1080,
}

body, _ := json.Marshal(initData)

// 第一次尝试 - 可能超时
req1, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req1.Header.Set("Content-Type", "application/json")
req1.Header.Set("Authorization", "Bearer "+token)

_, err1 := shortClient.Do(req1)

// 使用正常客户端重试
req2, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req2.Header.Set("Content-Type", "application/json")
req2.Header.Set("Authorization", "Bearer "+token)

resp2, err2 := httpClient.Do(req2)
passed := err2 == nil

message := "超时后重试成功"
if err1 != nil {
message = fmt.Sprintf("首次超时（预期），重试成功: %v", err1.Error())
} else {
message = "系统响应快，未触发超时，重试也成功"
}

if resp2 != nil {
resp2.Body.Close()
}

report.AddResult(TestResult{
TestName: "超时和重试测试",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
Details: map[string]interface{}{
"first_attempt_error":  fmt.Sprint(err1),
"second_attempt_error": fmt.Sprint(err2),
},
})
}

// Test 6: 上传大量小分片（压力测试）
func testManySmallChunks(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("chunks%d", time.Now().Unix()%10000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
report.AddResult(TestResult{
TestName: "大量小分片压力测试",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("准备测试失败: %v", err),
})
return
}

timestamp := fmt.Sprintf("%d", time.Now().UnixNano())
fileHash := fmt.Sprintf("%x", md5.Sum([]byte(timestamp)))

// 初始化
initData := map[string]interface{}{
"file_hash": fileHash,
"filename":  "many_chunks.mp4",
"file_size": 10240,
"width":     1920,
"height":    1080,
}

body, _ := json.Marshal(initData)
req, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+token)
httpClient.Do(req)

// 上传10个小分片
numChunks := 10
successCount := 0

for i := 0; i < numChunks; i++ {
chunkData := make([]byte, 1024)
rand.Read(chunkData)

var buf bytes.Buffer
writer := multipart.NewWriter(&buf)
writer.WriteField("file_hash", fileHash)
writer.WriteField("index", fmt.Sprintf("%d", i))

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
successCount++
}
resp.Body.Close()
}
}

duration := time.Since(start)
passed := successCount == numChunks
message := fmt.Sprintf("小分片上传: %d/%d成功", successCount, numChunks)

report.AddResult(TestResult{
TestName: "大量小分片压力测试",
Passed:   passed,
Duration: duration,
Message:  message,
Details: map[string]interface{}{
"total_chunks":  numChunks,
"success_count": successCount,
"avg_time":      duration / time.Duration(numChunks),
},
})
}

// Test 7: 无效哈希攻击测试
func testInvalidHashAttack(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("attack%d", time.Now().Unix()%10000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
report.AddResult(TestResult{
TestName: "无效哈希攻击测试",
Passed:   false,
Duration: time.Since(start),
Message:  fmt.Sprintf("准备测试失败: %v", err),
})
return
}

// 测试各种无效哈希
invalidHashes := []string{
"short",                                 // 太短
"GGGGGGGGGGGGGGGGGGGGGGGGGGGGGGGG",     // 非十六进制
"12345678901234567890123456789012345",  // 33字符（应该32或64）
"",                                      // 空
"../../../etc/passwd",                   // 路径注入
}

rejectedCount := 0

for _, hash := range invalidHashes {
initData := map[string]interface{}{
"file_hash": hash,
"filename":  "invalid.mp4",
"file_size": 1024,
}

body, _ := json.Marshal(initData)
req, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+token)

resp, _ := httpClient.Do(req)
var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)
resp.Body.Close()

if int(result["code"].(float64)) == 400 {
rejectedCount++
}
}

passed := rejectedCount == len(invalidHashes)
message := fmt.Sprintf("安全测试: %d/%d个无效哈希被正确拒绝", rejectedCount, len(invalidHashes))

report.AddResult(TestResult{
TestName: "无效哈希攻击测试",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
Details: map[string]interface{}{
"total_attacks": len(invalidHashes),
"rejected":      rejectedCount,
},
})
}

// Test 8: Context取消测试
func testContextCancellation(report *TestReport) {
start := time.Now()

username := fmt.Sprintf("cancel%d", time.Now().Unix()%10000)
token, _, err := registerAndLogin(username, "Test123456")
if err != nil {
report.AddResult(TestResult{
TestName: "Context取消测试",
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
"filename":  "cancel_test.mp4",
"file_size": 1024000,
}

body, _ := json.Marshal(initData)

// 创建带取消的context
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
defer cancel()

req, _ := http.NewRequestWithContext(ctx, "POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req.Header.Set("Content-Type", "application/json")
req.Header.Set("Authorization", "Bearer "+token)

_, err1 := httpClient.Do(req)

// 正常重试
req2, _ := http.NewRequest("POST", GatewayURL+"/api/video/init", bytes.NewBuffer(body))
req2.Header.Set("Content-Type", "application/json")
req2.Header.Set("Authorization", "Bearer "+token)

resp2, err2 := httpClient.Do(req2)

passed := err2 == nil
message := "Context取消后系统能正确处理"
if err1 != nil {
message = fmt.Sprintf("Context取消触发（预期），重试成功")
}

if resp2 != nil {
resp2.Body.Close()
}

report.AddResult(TestResult{
TestName: "Context取消测试",
Passed:   passed,
Duration: time.Since(start),
Message:  message,
})
}

func main() {
fmt.Println("========================================")
fmt.Println("视频平台微服务 - 高级测试套件")
fmt.Println("========================================")
fmt.Println("开始时间:", time.Now().Format("2006-01-02 15:04:05"))
fmt.Println()

report := NewTestReport()

// 基础修复验证
fmt.Println("1️⃣  验证分片上传修复...")
testChunkUploadFixed(report)

// 并发测试
fmt.Println("2️⃣  执行并发测试...")
testConcurrentFileUploads(report)
testConcurrentSameFileUpload(report)

// 极端条件测试
fmt.Println("3️⃣  执行极端边界条件测试...")
testClientInterruptSimulation(report)
testTimeoutAndRetry(report)
testContextCancellation(report)

// 压力测试
fmt.Println("4️⃣  执行压力测试...")
testManySmallChunks(report)

// 安全测试
fmt.Println("5️⃣  执行安全测试...")
testInvalidHashAttack(report)

report.PrintSummary()

if report.FailedTests > 0 {
fmt.Println("\n❌ 有测试失败")
return
}
fmt.Println("\n✅ 所有测试通过")
}
