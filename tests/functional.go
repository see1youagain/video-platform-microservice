package tests

import (
	"crypto/md5"
	"fmt"
	"strings"
	"time"
)

// loginUser is a shared helper: register (ignore error) then login, returns (token, userID)
func loginUser(username, password string) (string, float64) {
	c := NewClient()
	c.POST("/api/register", map[string]string{"username": username, "password": password}) //nolint
	_, body, _ := c.POST("/api/login", map[string]string{"username": username, "password": password})
	if body == nil {
		return "", 0
	}
	token, _ := body["token"].(string)
	uid, _ := body["user_id"].(float64)
	return token, uid
}

// RunFunctionalTests covers video upload (simple + chunked), transcode flow
func RunFunctionalTests() (passed, failed int) {
	s := NewSuite("FUNCTIONAL")
	fmt.Println("\n══════════════════════════════════════════════")
	fmt.Println("  PHASE 2 — 功能测试")
	fmt.Println("══════════════════════════════════════════════")

	username := fmt.Sprintf("fn%d", nowMs()%10000000)
	token, _ := loginUser(username, "Func123456")
	if token == "" {
		s.Check("前置条件: 登录成功", false, "无法获取 token，后续功能测试跳过")
		return s.Summary()
	}
	s.Check("前置条件: 登录成功", true, "")

	c := NewClient()
	c.Token = token

	// ── F1: 简单上传 ──────────────────────────────────────
	fmt.Println("\n[F1] 简单上传 (POST /api/video/upload)")
	fileContent := []byte("fake video content for simple upload test")
	resp, body, err := c.UploadFile("/api/video/upload", "file", "test.mp4", fileContent, nil)
	s.Check("POST /api/video/upload → 200", err == nil && sc(resp) == 200,
		fmt.Sprintf("err=%v status=%v body=%v", err, sc(resp), body))

	simpleURL := ""
	if body != nil {
		simpleURL, _ = body["url"].(string)
	}
	s.Check("简单上传响应含 url", simpleURL != "", fmt.Sprintf("body=%v", body))

	// no file field
	resp2, _, _ := c.UploadFile("/api/video/upload", "wrong_field", "test.mp4", fileContent, nil)
	s.Check("POST /api/video/upload 缺少 file 字段 → 400", sc(resp2) == 400, "")

	// need auth
	c2 := NewClient()
	resp3, _, _ := c2.UploadFile("/api/video/upload", "file", "test.mp4", fileContent, nil)
	s.Check("POST /api/video/upload 无 token → 401", sc(resp3) == 401, "")

	// ── F2: 分片上传完整流程 ────────────────────────────────
	fmt.Println("\n[F2] 分片上传流程 (Init→Chunk→Merge)")

	// generate a fake file hash
	fileData := []byte(strings.Repeat("abcdefghijklmnop", 256)) // 4KB
	hash := fmt.Sprintf("%x", md5.Sum(fileData))
	filename := "chunk_test.mp4"
	totalChunks := int32(2)

	// 2a. Init upload
	initBody := map[string]interface{}{
		"file_hash":  hash,
		"filename":   filename,
		"file_size":  len(fileData),
		"total_chunks": totalChunks,
	}
	resp4, body4, err4 := c.POST("/api/video/init", initBody)
	s.Check("POST /api/video/init → 200", err4 == nil && sc(resp4) == 200,
		fmt.Sprintf("err=%v status=%v body=%v", err4, sc(resp4), body4))

	// init without file_hash
	resp5, _, _ := c.POST("/api/video/init", map[string]interface{}{"filename": "x.mp4", "file_size": 100})
	s.Check("POST /api/video/init 缺 file_hash → 400", sc(resp5) == 400, "")

	// 2b. Upload chunks
	chunkSize := len(fileData) / 2
	chunk0 := fileData[:chunkSize]
	chunk1 := fileData[chunkSize:]

	respC0, bodyC0, errC0 := c.UploadFile("/api/video/upload_chunk", "chunk", "chunk_0", chunk0,
		map[string]string{"file_hash": hash, "index": "0"})
	s.Check("POST /api/video/upload_chunk chunk0 → 200", errC0 == nil && sc(respC0) == 200,
		fmt.Sprintf("err=%v status=%v body=%v", errC0, sc(respC0), bodyC0))

	// idempotent: upload chunk0 again
	respC0b, _, _ := c.UploadFile("/api/video/upload_chunk", "chunk", "chunk_0", chunk0,
		map[string]string{"file_hash": hash, "index": "0"})
	s.Check("POST /api/video/upload_chunk 幂等重传 → 200", sc(respC0b) == 200, "")

	respC1, bodyC1, errC1 := c.UploadFile("/api/video/upload_chunk", "chunk", "chunk_1", chunk1,
		map[string]string{"file_hash": hash, "index": "1"})
	s.Check("POST /api/video/upload_chunk chunk1 → 200", errC1 == nil && sc(respC1) == 200,
		fmt.Sprintf("err=%v status=%v body=%v", errC1, sc(respC1), bodyC1))

	// chunk without file_hash
	respCX, _, _ := c.UploadFile("/api/video/upload_chunk", "chunk", "chunk_0", chunk0, nil)
	s.Check("POST /api/video/upload_chunk 缺 file_hash → 400", sc(respCX) == 400, "")

	// 2c. Merge
	mergeBody := map[string]interface{}{
		"file_hash":    hash,
		"filename":     filename,
		"total_chunks": totalChunks,
		"resolutions":  []string{"720p"},
	}
	respM, bodyM, errM := c.POST("/api/video/merge", mergeBody)
	s.Check("POST /api/video/merge → 200", errM == nil && sc(respM) == 200,
		fmt.Sprintf("err=%v status=%v body=%v", errM, sc(respM), bodyM))

	mergeURL := ""
	if bodyM != nil {
		mergeURL, _ = bodyM["url"].(string)
	}
	s.Check("merge 响应含 url", mergeURL != "", fmt.Sprintf("body=%v", bodyM))

	// 等待 Kafka 消费者处理 FileUploaded 事件，创建 DB 记录
	fmt.Print("  ⏳ 等待 merge 事件落库 (2s)...")
	time.Sleep(2 * time.Second)
	fmt.Println("OK")

	// ── F3: 转码 ──────────────────────────────────────────
	fmt.Println("\n[F3] 转码 (POST /api/video/transcode)")

	transcodeBody := map[string]interface{}{
		"file_hash":   hash,
		"resolutions": []string{"720p", "480p"},
	}
	respT, bodyT, errT := c.POST("/api/video/transcode", transcodeBody)
	s.Check("POST /api/video/transcode → 200", errT == nil && sc(respT) == 200,
		fmt.Sprintf("err=%v status=%v body=%v", errT, sc(respT), bodyT))

	taskID := ""
	if bodyT != nil {
		taskID, _ = bodyT["task_id"].(string)
	}
	s.Check("transcode 响应含 task_id", taskID != "", fmt.Sprintf("body=%v", bodyT))

	// transcode without resolutions
	respT2, _, _ := c.POST("/api/video/transcode", map[string]interface{}{"file_hash": hash})
	s.Check("POST /api/video/transcode 缺 resolutions → 400", sc(respT2) == 400, "")

	// ── F4: 查询转码状态 ────────────────────────────────────
	fmt.Println("\n[F4] 查询转码状态 (GET /api/video/transcode/status)")
	if taskID != "" {
		respS, bodyS, errS := c.GET("/api/video/transcode/status", map[string]string{"task_id": taskID})
		s.Check("GET /api/video/transcode/status → 200", errS == nil && sc(respS) == 200,
			fmt.Sprintf("err=%v status=%v body=%v", errS, sc(respS), bodyS))
	}

	// missing task_id
	respS2, _, _ := c.GET("/api/video/transcode/status", nil)
	s.Check("GET /api/video/transcode/status 缺 task_id → 400", sc(respS2) == 400, "")

	// ── F5: 秒传（已上传文件重复提交 init） ────────────────────
	fmt.Println("\n[F5] 秒传检测")
	resp6, body6, _ := c.POST("/api/video/init", initBody)
	s.Check("已存在文件 /api/video/init → 返回 finished", sc(resp6) == 200 && body6 != nil, "")
	if body6 != nil {
		status, _ := body6["status"].(string)
		s.Check("秒传 status = finished", status == "finished", fmt.Sprintf("status=%q body=%v", status, body6))
	}

	return s.Summary()
}
