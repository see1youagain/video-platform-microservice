// Video Platform CLI Client
// Usage: go run main.go [--url http://host:port]
package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ── Config ──────────────────────────────────────────────────────────────────

var (
	baseURL    = "http://127.0.0.1:8080"
	httpClient = &http.Client{Timeout: 30 * time.Second}
	token      = ""
	currentUser = ""
)

// ── HTTP helpers ─────────────────────────────────────────────────────────────

func doJSON(method, path string, body interface{}) (int, map[string]interface{}, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL+path, r)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(raw, &result)
	return resp.StatusCode, result, nil
}

func doGET(path string, params map[string]string) (int, map[string]interface{}, error) {
	url := baseURL + path
	if len(params) > 0 {
		parts := []string{}
		for k, v := range params {
			parts = append(parts, k+"="+v)
		}
		url += "?" + strings.Join(parts, "&")
	}
	req, _ := http.NewRequest("GET", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(raw, &result)
	return resp.StatusCode, result, nil
}

func doMultipart(path string, fieldName, filename string, data []byte, extras map[string]string) (int, map[string]interface{}, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range extras {
		w.WriteField(k, v)
	}
	part, _ := w.CreateFormFile(fieldName, filename)
	part.Write(data)
	w.Close()
	req, _ := http.NewRequest("POST", baseURL+path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(raw, &result)
	return resp.StatusCode, result, nil
}

// ── Pretty print ─────────────────────────────────────────────────────────────

func printResponse(code int, body map[string]interface{}) {
	b, _ := json.MarshalIndent(body, "", "  ")
	fmt.Printf("HTTP %d\n%s\n", code, string(b))
}

// ── Input helpers ─────────────────────────────────────────────────────────────

var reader = bufio.NewReader(os.Stdin)

func prompt(msg string) string {
	fmt.Print(msg)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func promptSecret(msg string) string {
	fmt.Print(msg)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// ── Commands ─────────────────────────────────────────────────────────────────

func cmdRegister() {
	fmt.Println("\n── 用户注册 ──────────────────────────────")
	username := prompt("用户名 (3-20位字母数字下划线): ")
	password := promptSecret("密码 (≥6字符): ")
	code, body, err := doJSON("POST", "/api/register", map[string]string{
		"username": username, "password": password,
	})
	if err != nil {
		fmt.Println("❌ 请求失败:", err)
		return
	}
	printResponse(code, body)
}

func cmdLogin() {
	fmt.Println("\n── 用户登录 ──────────────────────────────")
	username := prompt("用户名: ")
	password := promptSecret("密码: ")
	code, body, err := doJSON("POST", "/api/login", map[string]string{
		"username": username, "password": password,
	})
	if err != nil {
		fmt.Println("❌ 请求失败:", err)
		return
	}
	if code == 200 && body != nil {
		if t, ok := body["token"].(string); ok && t != "" {
			token = t
			currentUser = username
			fmt.Printf("✅ 登录成功，已获取 token (前20字符: %s...)\n", t[:min(20, len(t))])
		}
	}
	printResponse(code, body)
}

func cmdLogout() {
	token = ""
	currentUser = ""
	fmt.Println("✅ 已登出（本地 token 已清除）")
}

func cmdProfile() {
	fmt.Println("\n── 用户信息 ──────────────────────────────")
	code, body, err := doGET("/api/profile", nil)
	if err != nil {
		fmt.Println("❌ 请求失败:", err)
		return
	}
	printResponse(code, body)
}

func cmdSimpleUpload() {
	fmt.Println("\n── 简单上传 ──────────────────────────────")
	filePath := prompt("文件路径 (留空使用示例数据): ")
	var data []byte
	var fname string
	if filePath == "" {
		data = []byte("demo video content " + time.Now().String())
		fname = "demo.mp4"
		fmt.Println("  使用示例数据:", fname)
	} else {
		var err error
		data, err = os.ReadFile(filePath)
		if err != nil {
			fmt.Println("❌ 读取文件失败:", err)
			return
		}
		fname = filepath.Base(filePath)
	}
	code, body, err := doMultipart("/api/video/upload", "file", fname, data, nil)
	if err != nil {
		fmt.Println("❌ 请求失败:", err)
		return
	}
	printResponse(code, body)
}

func cmdChunkedUpload() {
	fmt.Println("\n── 分片上传 (Init→Chunk→Merge) ──────────")
	filePath := prompt("文件路径 (留空使用示例数据): ")
	var data []byte
	var fname string
	if filePath == "" {
		data = bytes.Repeat([]byte("video chunk data abcdefghijklmnop"), 64) // ~2KB
		fname = "chunked_demo.mp4"
		fmt.Println("  使用示例数据:", fname, "大小:", len(data), "bytes")
	} else {
		var err error
		data, err = os.ReadFile(filePath)
		if err != nil {
			fmt.Println("❌ 读取文件失败:", err)
			return
		}
		fname = filepath.Base(filePath)
	}

	hash := fmt.Sprintf("%x", md5.Sum(data))
	fmt.Println("  文件 Hash (MD5):", hash)

	// Step 1: Init
	fmt.Println("\n  [1/3] 初始化上传...")
	code, body, err := doJSON("POST", "/api/video/init", map[string]interface{}{
		"file_hash": hash,
		"filename":  fname,
		"file_size": len(data),
	})
	if err != nil {
		fmt.Println("❌ Init 失败:", err)
		return
	}
	printResponse(code, body)
	if code != 200 {
		fmt.Println("  ⚠️  Init 失败，中止")
		return
	}

	// Check if already uploaded (tombstone)
	if body != nil {
		if status, ok := body["status"].(string); ok && status == "finished" {
			fmt.Println("  ⚡ 秒传命中！文件已存在，无需重传")
			return
		}
	}

	// Step 2: Upload chunks
	chunkSizeInput := prompt("  分片大小 bytes (留空默认 1024): ")
	chunkSize := 1024
	if n, err := strconv.Atoi(chunkSizeInput); err == nil && n > 0 {
		chunkSize = n
	}

	totalChunks := (len(data) + chunkSize - 1) / chunkSize
	fmt.Printf("\n  [2/3] 上传 %d 个分片...\n", totalChunks)
	for i := 0; i < totalChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[start:end]
		code, body, err := doMultipart("/api/video/upload_chunk", "chunk",
			fmt.Sprintf("chunk_%d", i), chunk,
			map[string]string{"file_hash": hash, "index": strconv.Itoa(i)})
		if err != nil || code != 200 {
			fmt.Printf("    ❌ chunk %d 失败: err=%v code=%d body=%v\n", i, err, code, body)
			return
		}
		fmt.Printf("    ✅ chunk %d/%d\n", i+1, totalChunks)
	}

	// Step 3: Merge
	fmt.Println("\n  [3/3] 合并分片...")
	resInput := prompt("  转码分辨率 (留空跳过转码, 或输入如 720p,480p): ")
	mergeBody := map[string]interface{}{
		"file_hash":    hash,
		"filename":     fname,
		"total_chunks": int32(totalChunks),
	}
	if resInput != "" {
		mergeBody["resolutions"] = strings.Split(resInput, ",")
	}
	code, body, err = doJSON("POST", "/api/video/merge", mergeBody)
	if err != nil {
		fmt.Println("❌ Merge 失败:", err)
		return
	}
	printResponse(code, body)

	// Show task_id if present
	if body != nil {
		if taskID, ok := body["task_id"].(string); ok && taskID != "" {
			fmt.Println("\n  转码任务ID:", taskID)
			fmt.Println("  可用菜单选项 [6] 查询转码状态")
		}
	}
}

func cmdTranscode() {
	fmt.Println("\n── 创建转码任务 ──────────────────────────")
	hash := prompt("文件 Hash (MD5, 32位十六进制): ")
	resInput := prompt("分辨率 (逗号分隔, 如 720p,480p): ")
	if hash == "" || resInput == "" {
		fmt.Println("❌ hash 和 resolutions 不能为空")
		return
	}
	code, body, err := doJSON("POST", "/api/video/transcode", map[string]interface{}{
		"file_hash":   hash,
		"resolutions": strings.Split(resInput, ","),
	})
	if err != nil {
		fmt.Println("❌ 请求失败:", err)
		return
	}
	printResponse(code, body)
}

func cmdTranscodeStatus() {
	fmt.Println("\n── 查询转码状态 ──────────────────────────")
	taskID := prompt("Task ID: ")
	if taskID == "" {
		fmt.Println("❌ task_id 不能为空")
		return
	}
	code, body, err := doGET("/api/video/transcode/status", map[string]string{"task_id": taskID})
	if err != nil {
		fmt.Println("❌ 请求失败:", err)
		return
	}
	printResponse(code, body)
}

func cmdPing() {
	code, body, err := doGET("/ping", nil)
	if err != nil {
		fmt.Println("❌ Ping 失败:", err)
		return
	}
	printResponse(code, body)
}

func cmdSetURL() {
	u := prompt(fmt.Sprintf("Gateway URL [当前: %s]: ", baseURL))
	if u != "" {
		baseURL = u
	}
	fmt.Println("✅ URL 已设置为:", baseURL)
}

// ── Menu ─────────────────────────────────────────────────────────────────────

func printMenu() {
	authStatus := "未登录"
	if currentUser != "" {
		authStatus = fmt.Sprintf("已登录: %s", currentUser)
	}
	fmt.Printf(`
╔══════════════════════════════════════════════════╗
║   Video Platform CLI Client                      ║
║   Server: %-40s║
║   状态: %-43s║
╠══════════════════════════════════════════════════╣
║  用户操作                                         ║
║  [1] 注册                [2] 登录                 ║
║  [3] 查看个人信息         [4] 登出                 ║
╠══════════════════════════════════════════════════╣
║  视频操作                                         ║
║  [5] 简单上传 (单次)      [6] 分片上传 (完整流程)   ║
║  [7] 创建转码任务         [8] 查询转码状态           ║
╠══════════════════════════════════════════════════╣
║  系统                                             ║
║  [9] Ping               [0] 修改服务器地址         ║
║  [q] 退出                                        ║
╚══════════════════════════════════════════════════╝
`, baseURL, authStatus)
	fmt.Print("请选择: ")
}

func main() {
	// Parse args
	for i, arg := range os.Args[1:] {
		if arg == "--url" && i+2 < len(os.Args) {
			baseURL = os.Args[i+2]
		}
	}

	fmt.Printf("✅ Video Platform CLI 客户端启动 (服务器: %s)\n", baseURL)
	fmt.Println("   提示: 先注册 [1]，再登录 [2]，然后可使用其他功能")

	for {
		printMenu()
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		fmt.Println()
		switch choice {
		case "1":
			cmdRegister()
		case "2":
			cmdLogin()
		case "3":
			cmdProfile()
		case "4":
			cmdLogout()
		case "5":
			cmdSimpleUpload()
		case "6":
			cmdChunkedUpload()
		case "7":
			cmdTranscode()
		case "8":
			cmdTranscodeStatus()
		case "9":
			cmdPing()
		case "0":
			cmdSetURL()
		case "q", "Q", "quit", "exit":
			fmt.Println("👋 再见！")
			os.Exit(0)
		default:
			fmt.Println("❓ 无效选项，请重新输入")
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
