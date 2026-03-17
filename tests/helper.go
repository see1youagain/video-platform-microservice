package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const BaseURL = "http://127.0.0.1:8080"

// Client is a simple HTTP test client
type Client struct {
	HTTP  *http.Client
	Token string
}

func NewClient() *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 20 * time.Second},
	}
}

// ----- JSON request helpers -----

func (c *Client) POST(path string, body interface{}) (*http.Response, map[string]interface{}, error) {
	data, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", BaseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(raw, &result)
	return resp, result, nil
}

func (c *Client) GET(path string, params map[string]string) (*http.Response, map[string]interface{}, error) {
	u := BaseURL + path
	if len(params) > 0 {
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		q := url.Values{}
		for _, k := range keys {
			q.Set(k, params[k])
		}
		u += "?" + q.Encode()
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(raw, &result)
	return resp, result, nil
}

func (c *Client) GETNoRedirect(path string, params map[string]string) (*http.Response, map[string]interface{}, error) {
	u := BaseURL + path
	if len(params) > 0 {
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		q := url.Values{}
		for _, k := range keys {
			q.Set(k, params[k])
		}
		u += "?" + q.Encode()
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	nrClient := &http.Client{
		Timeout: c.HTTP.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := nrClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(raw, &result)
	return resp, result, nil
}

// ----- Multipart upload helpers -----

func (c *Client) UploadFile(path string, fieldName string, filename string, data []byte, extraFields map[string]string) (*http.Response, map[string]interface{}, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range extraFields {
		w.WriteField(k, v)
	}
	part, err := w.CreateFormFile(fieldName, filename)
	if err != nil {
		return nil, nil, err
	}
	part.Write(data)
	w.Close()

	req, err := http.NewRequest("POST", BaseURL+path, &buf)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(raw, &result)
	return resp, result, nil
}

// ----- Body parse helpers -----

func ExtractToken(body map[string]interface{}) string {
	if body == nil {
		return ""
	}
	if token, ok := body["token"].(string); ok && token != "" {
		return token
	}
	if data, ok := body["data"].(map[string]interface{}); ok {
		if token, ok := data["token"].(string); ok {
			return token
		}
	}
	return ""
}

// ----- Test assertion helpers -----

type TestCase struct {
	Name string
	Pass bool
	Msg  string
}

type TestSuite struct {
	Name  string
	Cases []TestCase
	Start time.Time
}

func NewSuite(name string) *TestSuite {
	return &TestSuite{Name: name, Start: time.Now()}
}

func (s *TestSuite) Check(name string, condition bool, msg string) {
	s.Cases = append(s.Cases, TestCase{Name: name, Pass: condition, Msg: msg})
	status := "✅ PASS"
	if !condition {
		status = "❌ FAIL"
	}
	fmt.Printf("  %s  %s", status, name)
	if msg != "" && !condition {
		fmt.Printf("  →  %s", msg)
	}
	fmt.Println()
}

func (s *TestSuite) Summary() (passed, failed int) {
	for _, c := range s.Cases {
		if c.Pass {
			passed++
		} else {
			failed++
		}
	}
	elapsed := time.Since(s.Start)
	fmt.Printf("\n[%s] %d passed, %d failed  (%.2fs)\n", s.Name, passed, failed, elapsed.Seconds())
	return
}

// ----- Stress test helpers -----

type StressResult struct {
	Name       string
	Total      int
	Success    int
	Fail       int
	Elapsed    time.Duration
	LatencyP50 time.Duration
	LatencyP99 time.Duration
	RPS        float64
}

func (r StressResult) Print() {
	fmt.Printf("┌─────────────────────────────────────────────────────\n")
	fmt.Printf("│  Stress: %-40s\n", r.Name)
	fmt.Printf("│  Total:  %d  Success: %d  Fail: %d\n", r.Total, r.Success, r.Fail)
	fmt.Printf("│  Elapsed: %v   RPS: %.1f req/s\n", r.Elapsed.Round(time.Millisecond), r.RPS)
	fmt.Printf("│  Latency  P50: %v   P99: %v\n", r.LatencyP50.Round(time.Millisecond), r.LatencyP99.Round(time.Millisecond))
	fmt.Printf("└─────────────────────────────────────────────────────\n")
}

// MakeTempFile creates a temp file with given content
func MakeTempFile(prefix string, content []byte) (string, func()) {
	f, _ := os.CreateTemp("", prefix)
	f.Write(content)
	f.Close()
	return f.Name(), func() { os.Remove(filepath.Base(f.Name())); os.Remove(f.Name()) }
}

// WaitForReady probes the gateway until it responds or times out
func WaitForReady(maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(BaseURL + "/ping")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return true
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

// Percentile returns the p-th percentile from a sorted slice
func Percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p / 100.0)
	return sorted[idx]
}

func nowMs() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func AppendRunLog(path string, msg string) {
	line := fmt.Sprintf("[%s] %s\n", time.Now().Format(time.RFC3339), strings.TrimSpace(msg))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(line)
}

// newHTTPClientTimeout creates an *http.Client with a custom timeout.
// Used by concurrent tests where large-chunk uploads may exceed the default 20s.
func newHTTPClientTimeout(d time.Duration) *http.Client {
	return &http.Client{Timeout: d}
}
