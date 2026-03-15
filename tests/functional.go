package tests

import (
	"crypto/md5"
	"fmt"
	"strings"
	"time"
)

func loginUser(username, password string) (string, float64) {
	c := NewClient()
	c.POST("/api/register", map[string]string{"username": username, "password": password}) //nolint
	_, body, _ := c.POST("/api/login", map[string]string{"username": username, "password": password})
	if body == nil {
		return "", 0
	}
	token := ExtractToken(body)
	uid, _ := body["user_id"].(float64)
	return token, uid
}

func uniqueFileHash(seed string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("%s_%d", seed, nowMs()))))
}

func initWithRetry(c *Client, filename string, fileSize int) (string, string, map[string]interface{}) {
	for i := 0; i < 5; i++ {
		hash := uniqueFileHash(fmt.Sprintf("chunk_%d", i))
		resp, body, err := c.POST("/api/video/init", map[string]interface{}{
			"file_hash": hash,
			"filename":  filename,
			"file_size": fileSize,
		})
		if err != nil || resp == nil || resp.StatusCode != 200 || body == nil {
			continue
		}
		uploadID, _ := body["upload_id"].(string)
		status, _ := body["status"].(string)
		if uploadID != "" {
			return hash, uploadID, body
		}
		if status != "finished" {
			return hash, uploadID, body
		}
	}
	return "", "", nil
}

func waitVideoInfoReady(c *Client, fileHash string, maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		resp, body, err := c.GET("/api/video/info", map[string]string{"file_hash": fileHash})
		if err == nil && resp != nil && resp.StatusCode == 200 && body != nil {
			if code, ok := body["code"].(float64); ok && int(code) == 200 {
				return true
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
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

	resp2, _, _ := c.UploadFile("/api/video/upload", "wrong_field", "test.mp4", fileContent, nil)
	s.Check("POST /api/video/upload 缺少 file 字段 → 400", sc(resp2) == 400, "")

	c2 := NewClient()
	resp3, _, _ := c2.UploadFile("/api/video/upload", "file", "test.mp4", fileContent, nil)
	s.Check("POST /api/video/upload 无 token → 401", sc(resp3) == 401, "")

	fmt.Println("\n[F1.1] 小文件分流验证 (Init 返回 single_shot)")
	smallHash := uniqueFileHash("small_file_init")
	respSmallInit, bodySmallInit, errSmallInit := c.POST("/api/video/init", map[string]interface{}{
		"file_hash": smallHash,
		"filename":  "small_only_init.mp4",
		"file_size": 1024 * 1024,
	})
	smallSingleShot := false
	if bodySmallInit != nil {
		if status, ok := bodySmallInit["status"].(string); ok && status == "single_shot" {
			smallSingleShot = true
		}
	}
	s.Check("POST /api/video/init 小文件 → single_shot", errSmallInit == nil && sc(respSmallInit) == 200 && smallSingleShot,
		fmt.Sprintf("err=%v status=%v body=%v", errSmallInit, sc(respSmallInit), bodySmallInit))

	fmt.Println("\n[F2] 分片上传流程 (Init→Chunk→Merge)")
	fileData := []byte(strings.Repeat("abcdefghijklmnop", 393216)) // 6MiB
	filename := fmt.Sprintf("chunk_test_%d.mp4", nowMs())

	hash, uploadID, initBody := initWithRetry(c, filename, len(fileData))
	s.Check("POST /api/video/init 大文件 → 200 且返回 upload_id", hash != "" && uploadID != "",
		fmt.Sprintf("hash=%s upload_id=%s body=%v", hash, uploadID, initBody))
	if hash == "" || uploadID == "" {
		return s.Summary()
	}
	if initBody != nil {
		status, _ := initBody["status"].(string)
		s.Check("POST /api/video/init 大文件不走 single_shot", status != "single_shot", fmt.Sprintf("status=%q body=%v", status, initBody))
	}

	chunk0 := fileData

	respC0, bodyC0, errC0 := c.UploadFile("/api/video/upload_chunk", "chunk", "chunk_0", chunk0,
		map[string]string{"file_hash": hash, "upload_id": uploadID, "index": "0"})
	s.Check("POST /api/video/upload_chunk chunk0 → 200", errC0 == nil && sc(respC0) == 200,
		fmt.Sprintf("err=%v status=%v body=%v", errC0, sc(respC0), bodyC0))

	respC0b, _, _ := c.UploadFile("/api/video/upload_chunk", "chunk", "chunk_0", chunk0,
		map[string]string{"file_hash": hash, "upload_id": uploadID, "index": "0"})
	s.Check("POST /api/video/upload_chunk 幂等重传 → 200", sc(respC0b) == 200, "")

	respCX, _, _ := c.UploadFile("/api/video/upload_chunk", "chunk", "chunk_0", chunk0, nil)
	s.Check("POST /api/video/upload_chunk 缺 file_hash → 400", sc(respCX) == 400, "")
	tinyChunk := []byte(strings.Repeat("x", 1024))
	respTiny, bodyTiny, errTiny := c.UploadFile("/api/video/upload_chunk", "chunk", "chunk_tiny", tinyChunk,
		map[string]string{"file_hash": hash, "upload_id": uploadID, "index": "9"})
	s.Check("POST /api/video/upload_chunk 小于5MB → 400", errTiny == nil && sc(respTiny) == 400,
		fmt.Sprintf("err=%v status=%v body=%v", errTiny, sc(respTiny), bodyTiny))

	mergeBody := map[string]interface{}{
		"file_hash":    hash,
		"filename":     filename,
		"total_chunks": int32(1),
		"upload_id":    uploadID,
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

	fmt.Print("  ⏳ 等待视频信息同步 (最多 12s)...")
	ready := waitVideoInfoReady(c, hash, 12*time.Second)
	if ready {
		fmt.Println("OK")
	} else {
		fmt.Println("TIMEOUT")
	}
	s.Check("视频信息可查询", ready, "videoManager 可能尚未消费 file.uploaded 事件")

	fmt.Println("\n[F3] 查看视频信息 (GET /api/video/info)")
	respI, bodyI, errI := c.GET("/api/video/info", map[string]string{"file_hash": hash})
	bizOK := false
	if bodyI != nil {
		if code, ok := bodyI["code"].(float64); ok && int(code) == 200 {
			bizOK = true
		}
	}
	s.Check("GET /api/video/info → 200", errI == nil && sc(respI) == 200 && bizOK,
		fmt.Sprintf("err=%v status=%v body=%v", errI, sc(respI), bodyI))

	respD, _, errD := c.GETNoRedirect("/api/video/download", map[string]string{"file_hash": hash})
	s.Check("GET /api/video/download → 3xx", errD == nil && sc(respD) >= 300 && sc(respD) < 400,
		fmt.Sprintf("err=%v status=%v", errD, sc(respD)))

	fmt.Println("\n[F4] 转码 (POST /api/video/transcode)")
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

	respT2, _, _ := c.POST("/api/video/transcode", map[string]interface{}{"file_hash": hash})
	s.Check("POST /api/video/transcode 缺 resolutions → 400", sc(respT2) == 400, "")

	fmt.Println("\n[F5] 查询转码状态 (GET /api/video/transcode/status)")
	if taskID != "" {
		respS, bodyS, errS := c.GET("/api/video/transcode/status", map[string]string{"task_id": taskID})
		s.Check("GET /api/video/transcode/status → 200", errS == nil && sc(respS) == 200,
			fmt.Sprintf("err=%v status=%v body=%v", errS, sc(respS), bodyS))
	}

	respS2, _, _ := c.GET("/api/video/transcode/status", nil)
	s.Check("GET /api/video/transcode/status 缺 task_id → 400", sc(respS2) == 400, "")

	fmt.Println("\n[F6] 秒传检测")
	resp6, body6, _ := c.POST("/api/video/init", map[string]interface{}{
		"file_hash": hash,
		"filename":  filename,
		"file_size": len(fileData),
	})
	s.Check("已存在文件 /api/video/init → 返回 finished", sc(resp6) == 200 && body6 != nil, "")
	if body6 != nil {
		status, _ := body6["status"].(string)
		s.Check("秒传 status = finished", status == "finished", fmt.Sprintf("status=%q body=%v", status, body6))
	}

	fmt.Println("\n[F7] 删除视频 (POST /api/video/delete)")
	respDel, bodyDel, errDel := c.POST("/api/video/delete", map[string]interface{}{"file_hash": hash})
	s.Check("POST /api/video/delete → 200", errDel == nil && sc(respDel) == 200,
		fmt.Sprintf("err=%v status=%v body=%v", errDel, sc(respDel), bodyDel))

	return s.Summary()
}
