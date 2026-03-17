package tests

import (
"fmt"
"strings"
"sync"
"sync/atomic"
"time"
)

// RunConcurrentTests covers race-condition and multi-chunk scenarios:
//
//F8 — 3-chunk multipart upload（验证分片 ETag 组装完整性）
//F9 — 20-way concurrent FinalizeUpload（同一 file_hash → 全部 200 幂等）
func RunConcurrentTests() (passed, failed int) {
s := NewSuite("CONCURRENT")
fmt.Println("\n══════════════════════════════════════════════════════")
fmt.Println("  PHASE 3 — 并发与多分片正确性测试")
fmt.Println("══════════════════════════════════════════════════════")

username := fmt.Sprintf("cc%d", nowMs()%10000000)
token, _ := loginUser(username, "Conc123456")
if token == "" {
s.Check("前置条件: 登录成功", false, "无法获取 token，并发测试跳过")
return s.Summary()
}
s.Check("前置条件: 登录成功", true, "")

c := NewClient()
c.Token = token

// ── F8: 三分片分片上传 ────────────────────────────────────────────────
fmt.Println("\n[F8] 三分片上传 (Init→Chunk×3→Merge，每片 ≥5MB)")

// 所有分片均 ≥5MB（服务端对所有分片强制 ≥5MB 校验）
const chunkBody = "VIDEOPEAK" // 9 bytes
chunkSize := 5*1024*1024 + 512*1024              // 5.5 MiB
chunk55 := []byte(strings.Repeat(chunkBody, chunkSize/len(chunkBody)+1))[:chunkSize]
chunkLast := chunk55 // 末片同样使用 5.5MB（服务端限制）

f8Hash := uniqueFileHash("f8_3chunk")
f8Filename := fmt.Sprintf("f8_3chunk_%d.mp4", nowMs())
f8Size := chunkSize * 3 // 3 × 5.5MB

respInit8, bodyInit8, errInit8 := c.POST("/api/video/init", map[string]interface{}{
"file_hash": f8Hash,
"filename":  f8Filename,
"file_size": f8Size,
})
f8UploadID := ""
if bodyInit8 != nil {
f8UploadID, _ = bodyInit8["upload_id"].(string)
}
s.Check("[F8] POST /api/video/init → 200 + upload_id",
errInit8 == nil && sc(respInit8) == 200 && f8UploadID != "",
fmt.Sprintf("err=%v sc=%v body=%v", errInit8, sc(respInit8), bodyInit8))

f8MergeOK := false
if f8UploadID != "" {
chunks := []struct {
data    []byte
idx     string
label   string
}{
{chunk55, "0", "5.5MB"},
{chunk55, "1", "5.5MB"},
{chunkLast, "2", "5.5MB(末片≥5MB要求)"},
}
allChunksOK := true
for _, ch := range chunks {
r, b, e := c.UploadFile("/api/video/upload_chunk", "chunk",
fmt.Sprintf("chunk%s", ch.idx), ch.data,
map[string]string{"file_hash": f8Hash, "upload_id": f8UploadID, "index": ch.idx})
ok := e == nil && sc(r) == 200
s.Check(fmt.Sprintf("[F8] chunk%s (%s) → 200", ch.idx, ch.label), ok,
fmt.Sprintf("err=%v sc=%v body=%v", e, sc(r), b))
if !ok {
allChunksOK = false
}
}

if allChunksOK {
rM8, bM8, eM8 := c.POST("/api/video/merge", map[string]interface{}{
"file_hash":    f8Hash,
"filename":     f8Filename,
"total_chunks": 3,
"upload_id":    f8UploadID,
})
f8MergeOK = eM8 == nil && sc(rM8) == 200
s.Check("[F8] POST /api/video/merge (3-chunk) → 200", f8MergeOK,
fmt.Sprintf("err=%v sc=%v body=%v", eM8, sc(rM8), bM8))
if bM8 != nil {
url8, _ := bM8["url"].(string)
s.Check("[F8] merge 响应含 url", url8 != "", fmt.Sprintf("body=%v", bM8))
}
}
}
_ = f8MergeOK

// ── F9: 并发 FinalizeUpload 幂等性验证 ────────────────────────────────
fmt.Println("\n[F9] 20-goroutine 并发 FinalizeUpload（同一 file_hash → 全 200 幂等）")

// Prepare: init + upload 1 chunk，不 merge
f9ChunkSize := 5*1024*1024 + 512*1024 // 5.5 MiB
f9Chunk := []byte(strings.Repeat("CONCTEST", f9ChunkSize/8+1))[:f9ChunkSize]
f9Hash := uniqueFileHash("f9_concurrent_merge")
f9Filename := fmt.Sprintf("f9_concurrent_%d.mp4", nowMs())

respInit9, bodyInit9, errInit9 := c.POST("/api/video/init", map[string]interface{}{
"file_hash": f9Hash,
"filename":  f9Filename,
"file_size": len(f9Chunk),
})
f9UploadID := ""
if bodyInit9 != nil {
f9UploadID, _ = bodyInit9["upload_id"].(string)
}
s.Check("[F9] init → 200 + upload_id",
errInit9 == nil && sc(respInit9) == 200 && f9UploadID != "",
fmt.Sprintf("err=%v sc=%v body=%v", errInit9, sc(respInit9), bodyInit9))

chunkOK9 := false
if f9UploadID != "" {
rC9, bC9, eC9 := c.UploadFile("/api/video/upload_chunk", "chunk", "chunk0", f9Chunk,
map[string]string{"file_hash": f9Hash, "upload_id": f9UploadID, "index": "0"})
chunkOK9 = eC9 == nil && sc(rC9) == 200
s.Check("[F9] chunk0 (5.5MB) 上传 → 200", chunkOK9,
fmt.Sprintf("err=%v sc=%v body=%v", eC9, sc(rC9), bC9))
}

if f9UploadID != "" && chunkOK9 {
mergeBody9 := map[string]interface{}{
"file_hash":    f9Hash,
"filename":     f9Filename,
"total_chunks": 1,
"upload_id":    f9UploadID,
}
const concN = 20
var successCount int64
var wg sync.WaitGroup
wg.Add(concN)
ready := make(chan struct{})
for i := 0; i < concN; i++ {
go func() {
defer wg.Done()
ci := &Client{HTTP: newHTTPClientTimeout(30 * time.Second), Token: token}
<-ready // barrier: 所有 goroutine 就位后同时释放
resp, _, _ := ci.POST("/api/video/merge", mergeBody9)
if sc(resp) == 200 {
atomic.AddInt64(&successCount, 1)
}
}()
}
// 释放屏障
close(ready)
wg.Wait()

s.Check(
fmt.Sprintf("[F9] 20 并发 merge 全部返回 200 (幂等)  got=%d/20", successCount),
int(successCount) == concN,
fmt.Sprintf("success=%d / expected=%d", successCount, concN),
)
}

return s.Summary()
}
