package tests

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
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
		AppendRunLog(issueLogPath, fmt.Sprintf("FAIL service=%s scenario=%s failRate=%.2f qps=%.1f p99=%v",
			service, r.Name, failRate, r.RPS, r.LatencyP99))
	}
}

// Step 1: S2/S3
func RunStressS2S3() (StressResult, StressResult, bool) {
	fmt.Println("\n══════════════════════════════════════════════════════")
	fmt.Println("  STEP 1 — 重跑 S2/S3（注册/登录吞吐）")
	fmt.Println("══════════════════════════════════════════════════════")

	username := fmt.Sprintf("stress%d", nowMs()%10000000)
	password := "StressPass1"

	var regCounter int64
	r2 := runLoad("POST /api/register", 300, 60, func() bool {
		idx := atomic.AddInt64(&regCounter, 1)
		uname := fmt.Sprintf("s2reg%d_%d", nowMs()%1000000, idx)
		c2 := NewClient()
		resp, _, err := c2.POST("/api/register", map[string]string{
			"username": uname,
			"password": password,
		})
		return err == nil && resp != nil && resp.StatusCode == 200
	})
	r2.Print()

	seed := NewClient()
	seed.POST("/api/register", map[string]string{"username": username, "password": password})

	r3 := runLoad("POST /api/login", 500, 100, func() bool {
		c2 := NewClient()
		resp, _, err := c2.POST("/api/login", map[string]string{
			"username": username,
			"password": password,
		})
		return err == nil && resp != nil && resp.StatusCode == 200
	})
	r3.Print()

	loginHealthy := r3.Fail == 0
	fmt.Printf("\n[S2S3] loginHealthy=%v  s2_fail=%d/%d  s3_fail=%d/%d\n",
		loginHealthy, r2.Fail, r2.Total, r3.Fail, r3.Total)
	return r2, r3, loginHealthy
}

// Step 2: S4-S7
func RunStressS4S7() map[string]StressResult {
	fmt.Println("\n══════════════════════════════════════════════════════")
	fmt.Println("  STEP 2 — 重跑 S4-S7（确认是否独立问题）")
	fmt.Println("══════════════════════════════════════════════════════")

	results := map[string]StressResult{}

	username := fmt.Sprintf("s4u%d", nowMs()%10000000)
	token, _ := loginUser(username, "StressPass1")
	if token == "" {
		fmt.Println("❌ token 获取失败，S4-S7 跳过")
		return results
	}

	r4 := runLoad("GET /api/profile", 500, 100, func() bool {
		c2 := NewClient()
		c2.Token = token
		resp, _, err := c2.GET("/api/profile", nil)
		return err == nil && resp != nil && resp.StatusCode == 200
	})
	r4.Print()
	results["S4"] = r4
	recordScenarioResult("rpc-user", r4)

	var initCounter int64
	r5 := runLoad("POST /api/video/init", 300, 60, func() bool {
		idx := atomic.AddInt64(&initCounter, 1)
		c2 := NewClient()
		c2.Token = token
		hash := fmt.Sprintf("%032x", nowMs()+idx)
		resp, body, err := c2.POST("/api/video/init", map[string]interface{}{
			"file_hash": hash,
			"filename":  fmt.Sprintf("s5_%d.mp4", idx),
			"file_size": 1024,
		})
		return err == nil && resp != nil && resp.StatusCode == 200 && body != nil
	})
	r5.Print()
	results["S5"] = r5
	recordScenarioResult("rpc-videoupload", r5)

	seedHash := fmt.Sprintf("%032x", nowMs())
	seed := NewClient()
	seed.Token = token
	seed.POST("/api/video/init", map[string]interface{}{"file_hash": seedHash, "filename": "seed.mp4", "file_size": 512})
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
	results["S6"] = r6
	recordScenarioResult("rpc-videomanager", r6)

	fmt.Println("\n[S7] 峰值大文件 3-chunk 完整上传（10 并发用户）")
	peakChunkSize := 5*1024*1024 + 512*1024
	chunk := []byte(strings.Repeat("PEAKLOAD", peakChunkSize/8+1))[:peakChunkSize]

	var s7counter int64
	r7 := runLoad("S7 峰值大文件3分片完整上传", 10, 10, func() bool {
		idx := atomic.AddInt64(&s7counter, 1)
		ci := &Client{HTTP: newHTTPClientTimeout(120 * time.Second), Token: token}
		hash := uniqueFileHash(fmt.Sprintf("s7_%d", idx))
		filename := fmt.Sprintf("s7_%d.mp4", idx)

		_, b, e := ci.POST("/api/video/init", map[string]interface{}{
			"file_hash": hash,
			"filename":  filename,
			"file_size": peakChunkSize * 3,
		})
		if e != nil || b == nil {
			return false
		}
		uploadID, _ := b["upload_id"].(string)
		if uploadID == "" {
			return false
		}

		for chIdx := 0; chIdx < 3; chIdx++ {
			r, _, e2 := ci.UploadFile("/api/video/upload_chunk", "chunk", fmt.Sprintf("c%d", chIdx), chunk,
				map[string]string{"file_hash": hash, "upload_id": uploadID, "index": fmt.Sprintf("%d", chIdx)})
			if e2 != nil || sc(r) != 200 {
				return false
			}
		}

		mResp, _, _ := ci.POST("/api/video/merge", map[string]interface{}{
			"file_hash":    hash,
			"filename":     filename,
			"total_chunks": 3,
			"upload_id":    uploadID,
		})
		return sc(mResp) == 200
	})
	r7.Print()
	results["S7"] = r7
	recordScenarioResult("rpc-videoupload", r7)

	return results
}

type runtimePoint struct {
	Sec       int64
	ClientGo  int
	ClientMem uint64
	Gateway   int64
	User      int64
	Upload    int64
	Manager   int64
	Transcode int64
}

func getVmRSSMB(pid string) int64 {
	b, err := os.ReadFile("/proc/" + pid + "/status")
	if err != nil {
		return 0
	}
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(ln, "VmRSS:") {
			fs := strings.Fields(ln)
			if len(fs) >= 2 {
				v, _ := strconv.ParseInt(fs[1], 10, 64)
				return v / 1024
			}
		}
	}
	return 0
}

func findServiceRSS() map[string]int64 {
	res := map[string]int64{"gateway": 0, "rpc-user": 0, "rpc-videoUpload": 0, "rpc-videoManager": 0, "rpc-videoTranscode": 0}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return res
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		cmd, err := os.ReadFile("/proc/" + pid + "/cmdline")
		if err != nil || len(cmd) == 0 {
			continue
		}
		c := strings.ReplaceAll(string(cmd), "\x00", " ")
		for svc := range res {
			if strings.Contains(c, "/exe/"+svc) || strings.Contains(c, " "+svc+" ") {
				res[svc] += getVmRSSMB(pid)
			}
		}
	}
	return res
}

// Step 3: 100 chunks / 500MB long session + runtime curve
func RunLongSession100Chunks() {
	fmt.Println("\n══════════════════════════════════════════════════════")
	fmt.Println("  STEP 3 — 100分片长会话（500MB）曲线观测")
	fmt.Println("══════════════════════════════════════════════════════")

	username := fmt.Sprintf("long%d", nowMs()%10000000)
	token, _ := loginUser(username, "LongSess123")
	if token == "" {
		fmt.Println("❌ token 获取失败，长会话测试终止")
		return
	}
	c := &Client{HTTP: newHTTPClientTimeout(300 * time.Second), Token: token}

	chunkSize := 5 * 1024 * 1024
	totalChunks := 100
	fileSize := chunkSize * totalChunks
	fileHash := uniqueFileHash("long500mb")
	filename := fmt.Sprintf("long_500mb_%d.mp4", nowMs())
	chunk := bytes.Repeat([]byte{'A'}, chunkSize)

	ri, bi, ei := c.POST("/api/video/init", map[string]interface{}{
		"file_hash": fileHash,
		"filename":  filename,
		"file_size": fileSize,
	})
	if ei != nil || sc(ri) != 200 || bi == nil {
		fmt.Printf("❌ init failed err=%v status=%d body=%v\n", ei, sc(ri), bi)
		return
	}
	uploadID, _ := bi["upload_id"].(string)
	if uploadID == "" {
		fmt.Printf("❌ init missing upload_id body=%v\n", bi)
		return
	}

	start := time.Now()
	points := make([]runtimePoint, 0, 512)
	stop := make(chan struct{})
	var mWG sync.WaitGroup
	mWG.Add(1)
	go func() {
		defer mWG.Done()
		tk := time.NewTicker(1 * time.Second)
		defer tk.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				rss := findServiceRSS()
				points = append(points, runtimePoint{
					Sec:       int64(time.Since(start).Seconds()),
					ClientGo:  runtime.NumGoroutine(),
					ClientMem: m.Alloc / 1024 / 1024,
					Gateway:   rss["gateway"],
					User:      rss["rpc-user"],
					Upload:    rss["rpc-videoUpload"],
					Manager:   rss["rpc-videoManager"],
					Transcode: rss["rpc-videoTranscode"],
				})
			}
		}
	}()

	uploadOK := true
	for i := 0; i < totalChunks; i++ {
		r, b, e := c.UploadFile("/api/video/upload_chunk", "chunk", fmt.Sprintf("chunk_%d", i), chunk,
			map[string]string{"file_hash": fileHash, "upload_id": uploadID, "index": fmt.Sprintf("%d", i)})
		if e != nil || sc(r) != 200 {
			uploadOK = false
			fmt.Printf("❌ chunk[%d] fail err=%v status=%d body=%v\n", i, e, sc(r), b)
			break
		}
		if (i+1)%10 == 0 {
			fmt.Printf("  progress %d/%d\n", i+1, totalChunks)
		}
	}

	mergeOK := false
	if uploadOK {
		rm, bm, em := c.POST("/api/video/merge", map[string]interface{}{
			"file_hash":    fileHash,
			"filename":     filename,
			"total_chunks": totalChunks,
			"upload_id":    uploadID,
		})
		mergeOK = em == nil && sc(rm) == 200
		fmt.Printf("merge status=%d err=%v body=%v\n", sc(rm), em, bm)
	}

	close(stop)
	mWG.Wait()
	elapsed := time.Since(start)

	maxGo := 0
	var maxMem uint64
	maxGateway, maxUser, maxUpload, maxManager, maxTranscode := int64(0), int64(0), int64(0), int64(0), int64(0)
	for _, p := range points {
		if p.ClientGo > maxGo {
			maxGo = p.ClientGo
		}
		if p.ClientMem > maxMem {
			maxMem = p.ClientMem
		}
		if p.Gateway > maxGateway {
			maxGateway = p.Gateway
		}
		if p.User > maxUser {
			maxUser = p.User
		}
		if p.Upload > maxUpload {
			maxUpload = p.Upload
		}
		if p.Manager > maxManager {
			maxManager = p.Manager
		}
		if p.Transcode > maxTranscode {
			maxTranscode = p.Transcode
		}
	}

	fmt.Printf("\n[LONG-SESSION] uploadOK=%v mergeOK=%v elapsed=%v\n", uploadOK, mergeOK, elapsed.Round(time.Second))
	fmt.Printf("[PEAK] client_go=%d client_mem=%dMB gateway=%dMB user=%dMB upload=%dMB manager=%dMB transcode=%dMB\n",
		maxGo, maxMem, maxGateway, maxUser, maxUpload, maxManager, maxTranscode)
	fmt.Println("[SAMPLES] t(s) go memMB gateway user upload manager transcode")
	for i, p := range points {
		if i%5 != 0 && i != len(points)-1 {
			continue
		}
		fmt.Printf("%5d %3d %5d %7d %4d %6d %7d %9d\n", p.Sec, p.ClientGo, p.ClientMem, p.Gateway, p.User, p.Upload, p.Manager, p.Transcode)
	}
}

// Step 4: same file_hash merge with 50 concurrency under pressure
func RunSameHashMerge50(rounds int) {
	if rounds <= 0 {
		rounds = 10
	}
	fmt.Println("\n══════════════════════════════════════════════════════")
	fmt.Printf("  STEP 4 — 同一 file_hash 并发 merge（50并发×%d轮）\n", rounds)
	fmt.Println("══════════════════════════════════════════════════════")

	username := fmt.Sprintf("samehash%d", nowMs()%10000000)
	token, _ := loginUser(username, "SameHash123")
	if token == "" {
		fmt.Println("❌ token 获取失败，same-hash 压测终止")
		return
	}

	chunkSize := 5*1024*1024 + 512*1024
	chunk := []byte(strings.Repeat("IDEM", chunkSize/4+1))[:chunkSize]
	totalReq, total200, totalNon200 := 0, 0, 0

	start := time.Now()
	for r := 1; r <= rounds; r++ {
		c0 := &Client{HTTP: newHTTPClientTimeout(120 * time.Second), Token: token}
		hash := uniqueFileHash(fmt.Sprintf("same_%d", r))
		filename := fmt.Sprintf("same_%d.mp4", r)
		_, b, e := c0.POST("/api/video/init", map[string]interface{}{
			"file_hash": hash,
			"filename":  filename,
			"file_size": len(chunk),
		})
		if e != nil || b == nil {
			fmt.Printf("round=%d init fail err=%v body=%v\n", r, e, b)
			continue
		}
		uploadID, _ := b["upload_id"].(string)
		if uploadID == "" {
			fmt.Printf("round=%d no upload_id body=%v\n", r, b)
			continue
		}
		rc, bc, ec := c0.UploadFile("/api/video/upload_chunk", "chunk", "chunk0", chunk,
			map[string]string{"file_hash": hash, "upload_id": uploadID, "index": "0"})
		if ec != nil || sc(rc) != 200 {
			fmt.Printf("round=%d chunk fail err=%v sc=%d body=%v\n", r, ec, sc(rc), bc)
			continue
		}

		mergeBody := map[string]interface{}{
			"file_hash":    hash,
			"filename":     filename,
			"total_chunks": 1,
			"upload_id":    uploadID,
		}

		const concurrency = 50
		var okCount int64
		var failCount int64
		var wg sync.WaitGroup
		wg.Add(concurrency)
		barrier := make(chan struct{})
		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				c := &Client{HTTP: newHTTPClientTimeout(30 * time.Second), Token: token}
				<-barrier
				resp, _, _ := c.POST("/api/video/merge", mergeBody)
				if sc(resp) == 200 {
					atomic.AddInt64(&okCount, 1)
				} else {
					atomic.AddInt64(&failCount, 1)
				}
			}()
		}
		close(barrier)
		wg.Wait()

		totalReq += concurrency
		total200 += int(okCount)
		totalNon200 += int(failCount)
		fmt.Printf("round=%d 200=%d/%d non200=%d\n", r, okCount, concurrency, failCount)
	}

	fmt.Printf("\n[SAME-HASH] total=%d 200=%d non200=%d elapsed=%v\n", totalReq, total200, totalNon200, time.Since(start).Round(time.Second))
	fmt.Println("注: FOR UPDATE SKIP LOCKED 位于 outbox 消费流程；本场景直接验证 Finalize 幂等与唯一约束在高压下的行为。")
}

// RunStressTests keeps the original all-in-one stress flow for compatibility.
func RunStressTests() {
	fmt.Println("\n══════════════════════════════════════════════════════")
	fmt.Println("  PHASE 4 — 压力测试（全量）")
	fmt.Println("══════════════════════════════════════════════════════")
	AppendRunLog(issueLogPath, "==== 新一轮压测开始 ====")

	_, _, loginHealthy := RunStressS2S3()
	if loginHealthy {
		RunStressS4S7()
	} else {
		fmt.Println("⚠️  S3 未恢复正常，按规则跳过 S4-S7")
	}
	RunSameHashMerge50(10)
	fmt.Println("\n✅ 压力测试完成")
}
