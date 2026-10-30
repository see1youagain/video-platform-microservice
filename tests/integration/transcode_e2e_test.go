package integration

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type apiClient struct {
	base, token string
	http        *http.Client
}

func (c *apiClient) request(t *testing.T, method, path, contentType string, body io.Reader) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, c.base+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return resp.StatusCode, result
}
func (c *apiClient) post(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return c.request(t, "POST", path, "application/json", bytes.NewReader(encoded))
}
func (c *apiClient) get(t *testing.T, path string) (int, map[string]any) {
	t.Helper()
	return c.request(t, "GET", path, "", nil)
}
func requireSuccess(t *testing.T, status int, body map[string]any) {
	t.Helper()
	if status != 200 || body["code"] != float64(200) {
		t.Fatalf("API failed: HTTP %d body=%v", status, body)
	}
}

func requireCode(t *testing.T, want, status int, body map[string]any) {
	t.Helper()
	if status != want || body["code"] != float64(want) {
		t.Fatalf("want HTTP/business %d, got HTTP %d body=%v", want, status, body)
	}
}
func account(t *testing.T, base string) *apiClient {
	t.Helper()
	c := &apiClient{base: base, http: &http.Client{Timeout: 45 * time.Second}}
	name := fmt.Sprintf("tc%d", time.Now().UnixNano()%1000000000000000)
	status, body := c.post(t, "/api/register", map[string]string{"username": name, "password": "Transcode123!"})
	requireSuccess(t, status, body)
	status, body = c.post(t, "/api/login", map[string]string{"username": name, "password": "Transcode123!"})
	requireSuccess(t, status, body)
	c.token, _ = body["token"].(string)
	if c.token == "" {
		t.Fatalf("login omitted token: %v", body)
	}
	return c
}
func upload(t *testing.T, c *apiClient, data []byte) string {
	t.Helper()
	hash := fmt.Sprintf("%x", md5.Sum(data))
	status, body := c.post(t, "/api/video/init", map[string]any{"file_hash": hash, "filename": "integration.mp4", "file_size": len(data)})
	requireSuccess(t, status, body)
	if body["status"] == "finished" {
		return hash
	}
	id, _ := body["upload_id"].(string)
	if id == "" {
		t.Fatalf("init omitted upload_id: %v", body)
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for k, v := range map[string]string{"file_hash": hash, "upload_id": id, "index": "0"} {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("chunk", "part0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	status, body = c.request(t, "POST", "/api/video/upload_chunk", writer.FormDataContentType(), &buf)
	requireSuccess(t, status, body)
	status, body = c.post(t, "/api/video/merge", map[string]any{"file_hash": hash, "filename": "integration.mp4", "upload_id": id, "total_chunks": 1})
	requireSuccess(t, status, body)
	return hash
}
func waitTask(t *testing.T, c *apiClient, id, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	var body map[string]any
	for time.Now().Before(deadline) {
		status, b := c.get(t, "/api/video/transcode/status?task_id="+url.QueryEscape(id))
		requireSuccess(t, status, b)
		body = b
		if b["status"] == want {
			return b
		}
		if b["status"] == "failed" || b["status"] == "completed" {
			t.Fatalf("unexpected terminal status; want=%s body=%v", want, b)
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("transcode never reached %s (topic delivery, worker, or result consumer failed): %v", want, body)
	return nil
}
func TestTranscodeE2E(t *testing.T) {
	if os.Getenv("RUN_TRANSCODE_E2E") != "1" {
		t.Skip("set RUN_TRANSCODE_E2E=1 against the isolated integration stack")
	}
	base := os.Getenv("TEST_GATEWAY_URL")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	ffmpeg := os.Getenv("FFMPEG_PATH")
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	ffprobe := os.Getenv("FFPROBE_PATH")
	if ffprobe == "" {
		ffprobe = "ffprobe"
	}
	for _, exe := range []string{ffmpeg, ffprobe} {
		if _, err := exec.LookPath(exe); err != nil {
			t.Fatal(err)
		}
	}
	client := account(t, base)
	other := account(t, base)
	unauthenticated := &apiClient{base: base, http: &http.Client{Timeout: 45 * time.Second}}
	status, body := unauthenticated.post(t, "/api/video/transcode", map[string]any{"file_hash": strings.Repeat("a", 32), "resolutions": []string{"360p"}})
	requireCode(t, 401, status, body)
	root := t.TempDir()
	fixture := filepath.Join(root, "fixture.mp4")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=12", "-f", "lavfi", "-i", "sine=frequency=1000", "-t", "0.8", "-c:v", "mpeg4", "-c:a", "aac", fixture).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	video, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	// A valid MP4 tolerates trailing bytes; padding covers the current multipart
	// API minimum without making this correctness test encode minutes of video.
	video = append(video, make([]byte, 6*1024*1024-len(video))...)
	hash := upload(t, client, video)
	request := map[string]any{"file_hash": hash, "resolutions": []string{"360p", "480p", "360p"}, "request_id": "integration-" + fmt.Sprint(time.Now().UnixNano())}
	status, body = client.post(t, "/api/video/transcode", request)
	requireSuccess(t, status, body)
	id, _ := body["task_id"].(string)
	if id == "" {
		t.Fatalf("missing task ID: %v", body)
	}
	status, duplicate := client.post(t, "/api/video/transcode", request)
	requireSuccess(t, status, duplicate)
	if duplicate["task_id"] != id {
		t.Fatalf("request replay created another task: %v vs %s", duplicate, id)
	}
	status, conflict := client.post(t, "/api/video/transcode", map[string]any{"file_hash": hash, "resolutions": []string{"720p"}, "request_id": request["request_id"]})
	requireCode(t, 409, status, conflict)
	status, invalid := client.post(t, "/api/video/transcode", map[string]any{"file_hash": hash, "resolutions": []string{"4k"}})
	requireCode(t, 400, status, invalid)
	status, invalid = client.post(t, "/api/video/transcode", map[string]any{"file_hash": "short", "resolutions": []string{"360p"}})
	requireCode(t, 400, status, invalid)
	status, forbidden := other.post(t, "/api/video/transcode", map[string]any{"file_hash": hash, "resolutions": []string{"360p"}})
	requireCode(t, 404, status, forbidden)
	status, forbidden = other.get(t, "/api/video/transcode/status?task_id="+id)
	requireCode(t, 404, status, forbidden)
	finished := waitTask(t, client, id, "completed")
	if finished["progress"] != float64(1) {
		t.Fatalf("completed progress=%v", finished["progress"])
	}
	urls, ok := finished["completed_urls"].([]any)
	if !ok || len(urls) != 2 {
		t.Fatalf("deduplicated outputs missing: %v", finished)
	}
	for i, raw := range urls {
		address, ok := raw.(string)
		if !ok {
			t.Fatal(raw)
		}
		resp, err := client.http.Get(address)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("download output: status=%d err=%v", resp.StatusCode, err)
		}
		path := filepath.Join(root, fmt.Sprintf("result-%d.mp4", i))
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-xerror", "-i", path, "-f", "null", "-").CombinedOutput(); err != nil {
			t.Fatalf("output not playable: %v %s", err, out)
		}
		out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=codec_name,height", "-of", "json", path).Output()
		if err != nil {
			t.Fatal(err)
		}
		var probe struct {
			Streams []struct {
				Codec  string `json:"codec_name"`
				Height int    `json:"height"`
			}
		}
		if err := json.Unmarshal(out, &probe); err != nil {
			t.Fatal(err)
		}
		wantHeight := []int{360, 480}[i]
		if len(probe.Streams) != 1 || probe.Streams[0].Codec != "h264" || probe.Streams[0].Height != wantHeight {
			t.Fatalf("unexpected output profile: %s", out)
		}
	}
	status, info := client.get(t, "/api/video/info?file_hash="+hash)
	requireSuccess(t, status, info)
	if info["transcode_status"] != "published" {
		t.Fatalf("video metadata did not converge: %v", info)
	}
	status, replay := client.post(t, "/api/video/transcode", request)
	requireSuccess(t, status, replay)
	if replay["task_id"] != id {
		t.Fatal("completed request replay created duplicate work")
	}
	badHash := upload(t, client, []byte(strings.Repeat("corrupt-source-", 450000)))
	status, bad := client.post(t, "/api/video/transcode", map[string]any{"file_hash": badHash, "resolutions": []string{"360p"}})
	requireSuccess(t, status, bad)
	badID, _ := bad["task_id"].(string)
	failed := waitTask(t, client, badID, "failed")
	if outputs, ok := failed["completed_urls"].([]any); ok && len(outputs) > 0 {
		t.Fatalf("failed job exposed outputs: %v", failed)
	}
	status, info = client.get(t, "/api/video/info?file_hash="+badHash)
	requireSuccess(t, status, info)
	if info["transcode_status"] != "transcode_failed" {
		t.Fatalf("failed metadata did not converge: %v", info)
	}
}
