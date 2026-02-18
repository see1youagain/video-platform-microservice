package main

import (
"bytes"
"crypto/sha256"
"encoding/hex"
"encoding/json"
"fmt"
"io"
"mime/multipart"
"net/http"
"os"
"time"
)

const (
gatewayURL = "http://localhost:8080"
testUserID = "test_user_001"
)

// 测试响应结构
type Response struct {
Code            int32    `json:"code"`
Msg             string   `json:"msg"`
Status          string   `json:"status,omitempty"`
URL             string   `json:"url,omitempty"`
FinishedChunks  []string `json:"finished_chunks,omitempty"`
TranscodeStatus string   `json:"transcode_status,omitempty"`
TaskID          string   `json:"task_id,omitempty"`
Progress        int32    `json:"progress,omitempty"`
}

func main() {
fmt.Println("==========================================")
fmt.Println("视频平台完整功能测试")
fmt.Println("==========================================\n")

// 测试1: 创建测试视频文件
fmt.Println("【测试1】创建测试视频文件...")
testFile := createTestFile()
defer os.Remove(testFile)
fmt.Printf("✅ 测试文件创建成功: %s\n\n", testFile)

// 测试2: 计算文件哈希
fmt.Println("【测试2】计算文件哈希...")
fileHash := calculateFileHash(testFile)
fmt.Printf("✅ 文件哈希: %s\n\n", fileHash)

// 测试3: 初始化上传（秒传检查）
fmt.Println("【测试3】初始化上传...")
initResp := testInitUpload(fileHash, "test_video.mp4", testUserID)
if initResp.Status == "finished" {
fmt.Println("⚡ 秒传成功！文件已存在")
return
}
fmt.Printf("✅ 初始化成功，状态: %s\n\n", initResp.Status)

// 测试4: 分片上传
fmt.Println("【测试4】分片上传...")
chunkCount := testChunkUpload(fileHash, testFile, testUserID)
fmt.Printf("✅ 上传完成，共 %d 个分片\n\n", chunkCount)

// 测试5: 合并文件
fmt.Println("【测试5】合并文件...")
mergeResp := testMergeFile(fileHash, "test_video.mp4", chunkCount, testUserID)
fmt.Printf("✅ 合并成功，URL: %s\n\n", mergeResp.URL)

// 测试6: 获取视频信息
fmt.Println("【测试6】获取视频信息...")
testGetVideoInfo(fileHash, testUserID)
fmt.Println()

// 测试7: 分片下载
fmt.Println("【测试7】测试分片下载...")
testDownload(fileHash, testUserID)
fmt.Println()

// 测试8: 秒传测试（再次上传同一文件）
fmt.Println("【测试8】测试秒传功能...")
initResp2 := testInitUpload(fileHash, "test_video.mp4", testUserID)
if initResp2.Status == "finished" {
fmt.Println("✅ 秒传成功！")
}
fmt.Println()

// 测试9: 转码测试（如果有ffmpeg）
if checkFFmpeg() {
fmt.Println("【测试9】测试转码功能...")
testTranscode(fileHash, testUserID)
fmt.Println()
} else {
fmt.Println("⚠️  跳过转码测试（需要安装ffmpeg）\n")
}

fmt.Println("==========================================")
fmt.Println("✅ 所有测试完成！")
fmt.Println("==========================================")
}

func createTestFile() string {
filename := "/tmp/test_video.dat"
// 创建1MB测试文件
data := bytes.Repeat([]byte("A"), 1024*1024)
os.WriteFile(filename, data, 0644)
return filename
}

func calculateFileHash(filename string) string {
data, _ := os.ReadFile(filename)
hash := sha256.Sum256(data)
return hex.EncodeToString(hash[:])
}

func testInitUpload(fileHash, filename, userID string) *Response {
reqBody := map[string]interface{}{
"file_hash": fileHash,
"filename":  filename,
"user_id":   userID,
"file_size": 1024 * 1024,
"width":     1920,
"height":    1080,
}
data, _ := json.Marshal(reqBody)

resp, err := http.Post(gatewayURL+"/api/video/init", "application/json", bytes.NewReader(data))
if err != nil {
fmt.Printf("❌ 初始化上传失败: %v\n", err)
return nil
}
defer resp.Body.Close()

var result Response
json.NewDecoder(resp.Body).Decode(&result)
return &result
}

func testChunkUpload(fileHash, filename, userID string) int {
data, _ := os.ReadFile(filename)
chunkSize := 256 * 1024 // 256KB per chunk
totalChunks := (len(data) + chunkSize - 1) / chunkSize

for i := 0; i < totalChunks; i++ {
start := i * chunkSize
end := start + chunkSize
if end > len(data) {
end = len(data)
}

chunk := data[start:end]

// 创建multipart请求
body := &bytes.Buffer{}
writer := multipart.NewWriter(body)
writer.WriteField("file_hash", fileHash)
writer.WriteField("chunk_index", fmt.Sprintf("%d", i))
writer.WriteField("user_id", userID)

part, _ := writer.CreateFormFile("chunk", fmt.Sprintf("chunk_%d", i))
part.Write(chunk)
writer.Close()

req, _ := http.NewRequest("POST", gatewayURL+"/api/video/upload_chunk", body)
req.Header.Set("Content-Type", writer.FormDataContentType())

client := &http.Client{Timeout: 30 * time.Second}
resp, err := client.Do(req)
if err != nil {
fmt.Printf("❌ 上传分片 %d 失败: %v\n", i, err)
continue
}
resp.Body.Close()

fmt.Printf("   分片 %d/%d 上传完成\n", i+1, totalChunks)
}

return totalChunks
}

func testMergeFile(fileHash, filename string, totalChunks int, userID string) *Response {
reqBody := map[string]interface{}{
"file_hash":    fileHash,
"filename":     filename,
"total_chunks": totalChunks,
"user_id":      userID,
"width":        1920,
"height":       1080,
}
data, _ := json.Marshal(reqBody)

resp, err := http.Post(gatewayURL+"/api/video/merge", "application/json", bytes.NewReader(data))
if err != nil {
fmt.Printf("❌ 合并文件失败: %v\n", err)
return nil
}
defer resp.Body.Close()

var result Response
json.NewDecoder(resp.Body).Decode(&result)
return &result
}

func testGetVideoInfo(fileHash, userID string) {
url := fmt.Sprintf("%s/api/video/info?file_hash=%s&user_id=%s", gatewayURL, fileHash, userID)
resp, err := http.Get(url)
if err != nil {
fmt.Printf("❌ 获取视频信息失败: %v\n", err)
return
}
defer resp.Body.Close()

var result map[string]interface{}
json.NewDecoder(resp.Body).Decode(&result)
fmt.Printf("   视频信息: %v\n", result)
}

func testDownload(fileHash, userID string) {
url := fmt.Sprintf("%s/api/video/download?file_hash=%s&start=0&end=1024", gatewayURL, fileHash)
resp, err := http.Get(url)
if err != nil {
fmt.Printf("❌ 下载失败: %v\n", err)
return
}
defer resp.Body.Close()

data, _ := io.ReadAll(resp.Body)
fmt.Printf("✅ 下载成功，读取了 %d 字节\n", len(data))
}

func testTranscode(fileHash, userID string) {
reqBody := map[string]interface{}{
"file_hash":   fileHash,
"user_id":     userID,
"resolutions": []string{"720p", "480p"},
}
data, _ := json.Marshal(reqBody)

resp, err := http.Post(gatewayURL+"/api/video/transcode", "application/json", bytes.NewReader(data))
if err != nil {
fmt.Printf("❌ 转码请求失败: %v\n", err)
return
}
defer resp.Body.Close()

var result Response
json.NewDecoder(resp.Body).Decode(&result)
fmt.Printf("✅ 转码任务已创建: %s\n", result.TaskID)

// 等待一会儿再查询状态
time.Sleep(2 * time.Second)

// 查询转码状态
statusURL := fmt.Sprintf("%s/api/video/transcode/status?task_id=%s", gatewayURL, result.TaskID)
statusResp, _ := http.Get(statusURL)
if statusResp != nil {
defer statusResp.Body.Close()
var statusResult Response
json.NewDecoder(statusResp.Body).Decode(&statusResult)
fmt.Printf("   转码进度: %d%%, 状态: %s\n", statusResult.Progress, statusResult.Msg)
}
}

func checkFFmpeg() bool {
_, err := os.Stat("/usr/bin/ffmpeg")
return err == nil
}
