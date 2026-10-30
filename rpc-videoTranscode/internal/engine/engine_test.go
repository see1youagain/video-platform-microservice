package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testHash = "0123456789abcdef0123456789abcdef"

// The test binary acts as an executable dependency, exercising real subprocess
// cancellation and failure handling without requiring FFmpeg in the unit suite.
func init() {
	if os.Getenv("ENGINE_TEST_SUBPROCESS") != "1" {
		return
	}
	mode := os.Getenv("ENGINE_TEST_MODE")
	if mode == "hang" {
		time.Sleep(time.Minute)
		os.Exit(1)
	}
	probe := false
	for _, arg := range os.Args[1:] {
		if arg == "-show_entries" {
			probe = true
		}
	}
	if probe {
		if mode == "probe-error" {
			fmt.Fprintln(os.Stderr, "invalid input")
			os.Exit(2)
		}
		if mode == "probe-json" {
			fmt.Print("not JSON")
			os.Exit(0)
		}
		if mode == "audio-only" {
			fmt.Print(`{"streams":[{"codec_type":"audio","codec_name":"aac"}]}`)
			os.Exit(0)
		}
		codec := "h264"
		if mode == "bad-codec" && !strings.HasSuffix(os.Args[len(os.Args)-1], "source.mp4") {
			codec = "mpeg4"
		}
		fmt.Printf(`{"streams":[{"codec_type":"video","codec_name":%q,"width":640,"height":360},{"codec_type":"audio","codec_name":"aac"}]}`, codec)
		os.Exit(0)
	}
	if mode == "encode-error" {
		fmt.Fprintln(os.Stderr, "encoder failed")
		os.Exit(3)
	}
	if err := os.WriteFile(os.Args[len(os.Args)-1], []byte("encoded fixture"), 0600); err != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

type recordingStore struct {
	downloadErr, uploadErr      error
	emptyURL                    bool
	downloads, uploads, deleted []string
	paths                       []string
}

func (s *recordingStore) Download(_ context.Context, key, path string) error {
	s.downloads = append(s.downloads, key)
	s.paths = append(s.paths, path)
	if s.downloadErr != nil {
		return s.downloadErr
	}
	return os.WriteFile(path, []byte("source fixture"), 0600)
}
func (s *recordingStore) Upload(_ context.Context, key, path string) (string, error) {
	s.uploads = append(s.uploads, key)
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	if s.uploadErr != nil {
		return "", s.uploadErr
	}
	if s.emptyURL {
		return "", nil
	}
	return "https://objects.test/" + key, nil
}
func (s *recordingStore) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

func testEngine(t *testing.T, mode string, store *recordingStore) *Engine {
	t.Helper()
	t.Setenv("ENGINE_TEST_SUBPROCESS", "1")
	t.Setenv("ENGINE_TEST_MODE", mode)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(Config{FFmpegPath: executable, FFprobePath: executable, TempDir: t.TempDir(), Timeout: 10 * time.Second}, store)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func assertWorkspaceClean(t *testing.T, e *Engine) {
	t.Helper()
	entries, err := os.ReadDir(e.config.TempDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary workspaces leaked: %v", entries)
	}
}

func TestNewValidatesDependencies(t *testing.T) {
	if _, err := New(Config{}, nil); err == nil {
		t.Fatal("nil store accepted")
	}
	if _, err := New(Config{FFmpegPath: filepath.Join(t.TempDir(), "missing")}, &recordingStore{}); err == nil {
		t.Fatal("missing ffmpeg accepted")
	}
	executable, _ := os.Executable()
	if _, err := New(Config{FFmpegPath: executable, FFprobePath: filepath.Join(t.TempDir(), "missing")}, &recordingStore{}); err == nil {
		t.Fatal("missing ffprobe accepted")
	}
}

func TestTranscodeUnitSuccessDeduplicatesAndReportsProgress(t *testing.T) {
	s := &recordingStore{}
	e := testEngine(t, "", s)
	var progress []int32
	urls, err := e.Transcode(context.Background(), "task-unit", testHash, []string{"360p", "480p", "360p"}, func(p int32) error { progress = append(progress, p); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 2 || len(s.downloads) != 1 || len(s.uploads) != 2 {
		t.Fatalf("urls=%v downloads=%v uploads=%v", urls, s.downloads, s.uploads)
	}
	if s.downloads[0] != "videos/raw/"+testHash+".mp4" {
		t.Fatalf("unexpected source key %q", s.downloads[0])
	}
	if !reflect.DeepEqual(progress, []int32{5, 50, 95}) {
		t.Fatalf("progress=%v", progress)
	}
	for i, name := range []string{"360p", "480p"} {
		want := "videos/" + testHash + "/task-unit/" + name + ".mp4"
		if s.uploads[i] != want || urls[i] != "https://objects.test/"+want {
			t.Fatalf("unexpected output %v %v", s.uploads, urls)
		}
	}
	assertWorkspaceClean(t, e)
}

func TestTranscodeRejectsUnsafeAndInvalidRequests(t *testing.T) {
	cases := []struct {
		task, hash  string
		resolutions []string
	}{
		{"task", "../source", []string{"360p"}},
		{"../../escape", testHash, []string{"360p"}},
		{"task", testHash, nil},
		{"task", testHash, []string{"999p"}},
	}
	for _, tc := range cases {
		s := &recordingStore{}
		e := testEngine(t, "", s)
		if _, err := e.Transcode(context.Background(), tc.task, tc.hash, tc.resolutions, nil); err == nil {
			t.Fatalf("accepted invalid request %+v", tc)
		}
		if len(s.downloads) != 0 {
			t.Fatal("invalid request reached storage")
		}
		assertWorkspaceClean(t, e)
	}
}

func TestTranscodePropagatesFailuresAndCleansWorkspace(t *testing.T) {
	cases := []struct {
		name, mode, want string
		store            recordingStore
	}{
		{"download", "", "download original", recordingStore{downloadErr: errors.New("storage unavailable")}},
		{"probe", "probe-error", "invalid source video", recordingStore{}},
		{"probeJSON", "probe-json", "parse ffprobe output", recordingStore{}},
		{"noVideo", "audio-only", "no video stream", recordingStore{}},
		{"encode", "encode-error", "encode 360p", recordingStore{}},
		{"verify", "bad-codec", "not an even-sized H264", recordingStore{}},
		{"upload", "", "upload 360p", recordingStore{uploadErr: errors.New("write denied")}},
		{"emptyURL", "", "empty URL", recordingStore{emptyURL: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := testEngine(t, tc.mode, &tc.store)
			urls, err := e.Transcode(context.Background(), "task-fail", testHash, []string{"360p"}, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) || len(urls) != 0 {
				t.Fatalf("urls=%v err=%v; want %q", urls, err, tc.want)
			}
			if len(tc.store.uploads) > 0 && !reflect.DeepEqual(tc.store.uploads, tc.store.deleted) {
				t.Fatalf("unremoved output keys %v vs %v", tc.store.uploads, tc.store.deleted)
			}
			assertWorkspaceClean(t, e)
		})
	}
}

func TestTranscodeProgressFailureRemovesPartialOutputs(t *testing.T) {
	s := &recordingStore{}
	e := testEngine(t, "", s)
	want := errors.New("lease lost")
	_, err := e.Transcode(context.Background(), "task-progress", testHash, []string{"360p", "720p"}, func(p int32) error {
		if p > 5 {
			return want
		}
		return nil
	})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v", err)
	}
	if len(s.uploads) != 1 || !reflect.DeepEqual(s.deleted, s.uploads) {
		t.Fatalf("uploads=%v deleted=%v", s.uploads, s.deleted)
	}
	assertWorkspaceClean(t, e)
}

func TestTranscodeTimeoutTerminatesSubprocess(t *testing.T) {
	s := &recordingStore{}
	e := testEngine(t, "hang", s)
	e.config.Timeout = 100 * time.Millisecond
	start := time.Now()
	_, err := e.Transcode(context.Background(), "task-timeout", testHash, []string{"360p"}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("subprocess did not terminate promptly")
	}
	assertWorkspaceClean(t, e)
}

func TestTranscodeCanceledContext(t *testing.T) {
	e := testEngine(t, "", &recordingStore{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Transcode(ctx, "task-cancel", testHash, []string{"360p"}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	assertWorkspaceClean(t, e)
}

func TestTailBufferBoundsDiagnosticMemory(t *testing.T) {
	var b tailBuffer
	input := bytes.Repeat([]byte("x"), 64*1024)
	if n, err := b.Write(input); err != nil || n != len(input) {
		t.Fatalf("n=%d err=%v", n, err)
	}
	b.Write([]byte("END"))
	if len(b.data) != 32*1024 || !strings.HasSuffix(b.String(), "END") {
		t.Fatalf("len=%d", len(b.data))
	}
}
