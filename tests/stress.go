package tests

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const issueLogPath = "/tmp/svc-logs/concurrency-issues.log"

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

func recordScenarioResult(service string, r StressResult) {
	failRate := float64(r.Fail) / float64(r.Total)
	if failRate >= 0.2 {
		AppendRunLog(issueLogPath, fmt.Sprintf("FAIL service=%s scenario=%s failRate=%.2f qps=%.1f p99=%v", service, r.Name, failRate, r.RPS, r.LatencyP99))
	}
}

// RunStressTests runs all stress test scenarios
func RunStressTests() {
	fmt.Println("\n══════════════════════════════════════════════════")
	fmt.Println("  PHASE 3 — 压力测试")
	fmt.Println("══════════════════════════════════════════════════")
	AppendRunLog(issueLogPath, "==== 新一轮压测开始 ====")

	// Pre-create a token for authenticated endpoints
	token := ""
	username := fmt.Sprintf("stress%d", nowMs()%10000000)
	c := NewClient()
	c.POST("/api/register", map[string]string{"username": username, "password": "StressPass1"})
	resp, body, err := c.POST("/api/login", map[string]string{"username": username, "password": "StressPass1"})
	if err == nil && resp != nil && resp.StatusCode == 200 {
		token = ExtractToken(body)
	}

	if token == "" {
		AppendRunLog(issueLogPath, fmt.Sprintf("token 获取失败, login body=%v", body))
	}

	results := map[string]StressResult{}
	serviceMap := map[string]string{}

	// S1: Gateway ping
	r1 := runLoad("GET /ping", 1000, 100, func() bool {
		c2 := NewClient()
		resp, _, err := c2.GET("/ping", nil)
		return err == nil && resp != nil && resp.StatusCode == 200
	})
	r1.Print()
	results[r1.Name] = r1
	serviceMap[r1.Name] = "gateway"
	recordScenarioResult("gateway", r1)

	// S2: register -> rpc-user
	var regCounter int64
	r2 := runLoad("POST /api/register", 300, 60, func() bool {
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
	results[r2.Name] = r2
	serviceMap[r2.Name] = "rpc-user"
	recordScenarioResult("rpc-user", r2)

	// S3: login -> rpc-user
	r3 := runLoad("POST /api/login", 500, 100, func() bool {
		c2 := NewClient()
		resp, _, err := c2.POST("/api/login", map[string]string{
			"username": username,
			"password": "StressPass1",
		})
		return err == nil && resp != nil && resp.StatusCode == 200
	})
	r3.Print()
	results[r3.Name] = r3
	serviceMap[r3.Name] = "rpc-user"
	recordScenarioResult("rpc-user", r3)

	if token != "" {
		// S4: profile -> rpc-user
		r4 := runLoad("GET /api/profile", 500, 100, func() bool {
			c2 := NewClient()
			c2.Token = token
			resp, _, err := c2.GET("/api/profile", nil)
			return err == nil && resp != nil && resp.StatusCode == 200
		})
		r4.Print()
		results[r4.Name] = r4
		serviceMap[r4.Name] = "rpc-user"
		recordScenarioResult("rpc-user", r4)

		// S5: init upload -> rpc-videoupload
		var initCounter int64
		r5 := runLoad("POST /api/video/init", 300, 60, func() bool {
			idx := atomic.AddInt64(&initCounter, 1)
			c2 := NewClient()
			c2.Token = token
			hash := fmt.Sprintf("%032x", nowMs()+idx)
			resp, body, err := c2.POST("/api/video/init", map[string]interface{}{
				"file_hash": hash,
				"filename":  fmt.Sprintf("stress_%d.mp4", idx),
				"file_size": 1024,
			})
			return err == nil && resp != nil && resp.StatusCode == 200 && body != nil
		})
		r5.Print()
		results[r5.Name] = r5
		serviceMap[r5.Name] = "rpc-videoupload"
		recordScenarioResult("rpc-videoupload", r5)

		// S6: transcode -> rpc-videomanager
		seedHash := fmt.Sprintf("%032x", nowMs())
		c.POST("/api/video/init", map[string]interface{}{"file_hash": seedHash, "filename": "seed.mp4", "file_size": 512})
		r6 := runLoad("POST /api/video/transcode", 300, 60, func() bool {
			c2 := NewClient()
			c2.Token = token
			resp, _, err := c2.POST("/api/video/transcode", map[string]interface{}{
				"file_hash":   seedHash,
				"resolutions": []string{"720p"},
			})
			return err == nil && resp != nil && (resp.StatusCode == 200 || resp.StatusCode == 400)
		})
		r6.Print()
		results[r6.Name] = r6
		serviceMap[r6.Name] = "rpc-videomanager"
		recordScenarioResult("rpc-videomanager", r6)
	} else {
		fmt.Println("⚠️  鉴权 token 缺失，跳过鉴权接口压测")
		AppendRunLog(issueLogPath, "鉴权 token 缺失，跳过鉴权接口压测")
	}

	fmt.Println("\n[QPS 汇总]（按场景映射到微服务）")
	for name, r := range results {
		fmt.Printf("- service=%s, scenario=%s, qps=%.1f, fail=%d/%d\n", serviceMap[name], name, r.RPS, r.Fail, r.Total)
	}

	firstCrash := "none"
	for name, r := range results {
		if float64(r.Fail)/float64(r.Total) >= 0.2 {
			firstCrash = fmt.Sprintf("service=%s scenario=%s failRate=%.2f", serviceMap[name], name, float64(r.Fail)/float64(r.Total))
			break
		}
	}
	fmt.Printf("\n[首个明显异常服务] %s\n", firstCrash)
	AppendRunLog(issueLogPath, "首个明显异常服务: "+firstCrash)

	fmt.Println("\n✅ 压力测试完成")
}
