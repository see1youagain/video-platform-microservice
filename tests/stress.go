package tests

import (
"fmt"
"sort"
"sync"
"sync/atomic"
"time"
)

// runLoad runs `total` requests with `concurrency` goroutines, returns StressResult
func runLoad(name string, total, concurrency int, fn func() bool) StressResult {
start := time.Now()
var success, fail int64
latencies := make([]time.Duration, 0, total)
var mu sync.Mutex

sem := make(chan struct{}, concurrency)
var wg sync.WaitGroup
wg.Add(total)
for i := 0; i < total; i++ {
sem <- struct{}{}
go func() {
defer wg.Done()
defer func() { <-sem }()
t0 := time.Now()
ok := fn()
lat := time.Since(t0)
mu.Lock()
latencies = append(latencies, lat)
mu.Unlock()
if ok {
atomic.AddInt64(&success, 1)
} else {
atomic.AddInt64(&fail, 1)
}
}()
}
wg.Wait()
elapsed := time.Since(start)

sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
rps := float64(total) / elapsed.Seconds()

return StressResult{
Name:       name,
Total:      total,
Success:    int(success),
Fail:       int(fail),
Elapsed:    elapsed,
LatencyP50: Percentile(latencies, 50),
LatencyP99: Percentile(latencies, 99),
RPS:        rps,
}
}

// RunStressTests runs all stress test scenarios
func RunStressTests() {
fmt.Println("\n══════════════════════════════════════════════════")
fmt.Println("  PHASE 3 — 压力测试")
fmt.Println("══════════════════════════════════════════════════")

// Pre-create a token for authenticated endpoints
token := ""
username := fmt.Sprintf("stress%d", nowMs()%10000000)
c := NewClient()
c.POST("/api/register", map[string]string{"username": username, "password": "StressPass1"})
resp, body, err := c.POST("/api/login", map[string]string{"username": username, "password": "StressPass1"})
if err == nil && resp.StatusCode == 200 {
if data, ok := body["data"].(map[string]interface{}); ok {
if t, ok := data["token"].(string); ok {
token = t
}
}
}

// S1: GET /ping — 200 req / 20 concurrency
r1 := runLoad("GET /ping", 200, 20, func() bool {
c2 := NewClient()
resp, _, err := c2.GET("/ping", nil)
return err == nil && resp != nil && resp.StatusCode == 200
})
r1.Print()

// S2: POST /api/register — 100 req / 10 concurrency (unique users)
var regCounter int64
r2 := runLoad("POST /api/register (unique users)", 100, 10, func() bool {
idx := atomic.AddInt64(&regCounter, 1)
uname := fmt.Sprintf("sreg%d_%d", nowMs()%1000000, idx)
c2 := NewClient()
resp, _, err := c2.POST("/api/register", map[string]string{
"username": uname,
"password": "Pass123456",
})
return err == nil && resp != nil && resp.StatusCode == 200
})
r2.Print()

// S3: POST /api/login — 200 req / 20 concurrency
r3 := runLoad("POST /api/login", 200, 20, func() bool {
c2 := NewClient()
resp, _, err := c2.POST("/api/login", map[string]string{
"username": username,
"password": "StressPass1",
})
return err == nil && resp != nil && resp.StatusCode == 200
})
r3.Print()

// S4: GET /api/profile — 300 req / 30 concurrency (requires token)
if token != "" {
r4 := runLoad("GET /api/profile (authed)", 300, 30, func() bool {
c2 := NewClient()
c2.Token = token
resp, _, err := c2.GET("/api/profile", nil)
return err == nil && resp != nil && resp.StatusCode == 200
})
r4.Print()
} else {
fmt.Println("⚠️  跳过 /api/profile 压测 (token 获取失败)")
}

// S5: POST /api/video/upload (simple) — 100 req / 10 concurrency
if token != "" {
var uploadCounter int64
r5 := runLoad("POST /api/video/upload (simple)", 100, 10, func() bool {
idx := atomic.AddInt64(&uploadCounter, 1)
c2 := NewClient()
c2.Token = token
content := []byte(fmt.Sprintf("stress-upload-%d-%d", nowMs(), idx))
resp, _, err := c2.UploadFile("/api/video/upload", "video", "sv.mp4", content, map[string]string{
"title":       fmt.Sprintf("StressVid%d", idx),
"description": "stress test",
})
return err == nil && resp != nil && (resp.StatusCode == 200 || resp.StatusCode == 201)
})
r5.Print()
} else {
fmt.Println("⚠️  跳过 /api/video/upload 压测 (token 获取失败)")
}

// S6: POST /api/video/init — 100 req / 10 concurrency
if token != "" {
var initCounter int64
r6 := runLoad("POST /api/video/init (chunked)", 100, 10, func() bool {
idx := atomic.AddInt64(&initCounter, 1)
c2 := NewClient()
c2.Token = token
resp, _, err := c2.POST("/api/video/init", map[string]interface{}{
"filename":    fmt.Sprintf("stress_%d.mp4", idx),
"total_size":  1024 * 100,
"chunk_count": 1,
"hash":        fmt.Sprintf("stresshash%d%d", nowMs(), idx),
})
return err == nil && resp != nil && (resp.StatusCode == 200 || resp.StatusCode == 201)
})
r6.Print()
} else {
fmt.Println("⚠️  跳过 /api/video/init 压测 (token 获取失败)")
}

fmt.Println("\n✅ 压力测试完成")
}
