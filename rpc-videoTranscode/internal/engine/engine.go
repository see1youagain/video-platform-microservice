// Package engine encodes real videos without depending on RPC, Kafka or a database.
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/see1youagain/video-platform-microservice/common/events"
)

type Store interface {
	Download(context.Context, string, string) error
	Upload(context.Context, string, string) (string, error)
}

type Config struct {
	FFmpegPath  string
	FFprobePath string
	TempDir     string
	Timeout     time.Duration
	Threads     int
}

type Engine struct {
	config Config
	store  Store
}

func New(config Config, store Store) (*Engine, error) {
	if store == nil {
		return nil, fmt.Errorf("object store is required")
	}
	if config.FFmpegPath == "" {
		config.FFmpegPath = "ffmpeg"
	}
	if config.FFprobePath == "" {
		config.FFprobePath = "ffprobe"
	}
	var err error
	if config.FFmpegPath, err = exec.LookPath(config.FFmpegPath); err != nil {
		return nil, fmt.Errorf("find ffmpeg: %w", err)
	}
	if config.FFprobePath, err = exec.LookPath(config.FFprobePath); err != nil {
		return nil, fmt.Errorf("find ffprobe: %w", err)
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Minute
	}
	if config.Threads <= 0 {
		config.Threads = 2
	}
	return &Engine{config: config, store: store}, nil
}

type profile struct {
	width, height int
	bitrate       string
}

var profiles = map[string]profile{
	"360p":  {640, 360, "800k"},
	"480p":  {854, 480, "1400k"},
	"720p":  {1280, 720, "2800k"},
	"1080p": {1920, 1080, "5000k"},
}

// Transcode downloads the original once and uploads a playable MP4 for each profile.
// Progress is 0..100; the caller owns the final database commit at 100 percent.
func (e *Engine) Transcode(ctx context.Context, taskID, fileHash string, resolutions []string, onProgress func(int32) error) ([]string, error) {
	resolutions, err := events.ValidateTranscodeRequest(fileHash, resolutions)
	if err != nil {
		return nil, err
	}
	if !events.ValidTaskID(taskID) {
		return nil, fmt.Errorf("invalid task_id")
	}
	ctx, cancel := context.WithTimeout(ctx, e.config.Timeout)
	defer cancel()
	dir, err := os.MkdirTemp(e.config.TempDir, "transcode-")
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, "source.mp4")
	if err := e.store.Download(ctx, "videos/raw/"+fileHash+".mp4", input); err != nil {
		return nil, fmt.Errorf("download original: %w", err)
	}
	if err := e.probe(ctx, input, false); err != nil {
		return nil, fmt.Errorf("invalid source video: %w", err)
	}
	progress := func(value int32) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if onProgress != nil {
			return onProgress(value)
		}
		return nil
	}
	if err := progress(5); err != nil {
		return nil, err
	}
	urls := make([]string, 0, len(resolutions))
	var uploaded []string
	success := false
	defer func() {
		// A failed multi-profile job must never expose a partial result as success.
		if !success {
			if store, ok := e.store.(interface {
				Delete(context.Context, string) error
			}); ok {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				for _, name := range uploaded {
					_ = store.Delete(cleanupCtx, name)
				}
			}
		}
	}()
	for i, name := range resolutions {
		p := profiles[name]
		output := filepath.Join(dir, name+".mp4")
		filter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease:force_divisible_by=2,setsar=1", p.width, p.height)
		args := []string{
			"-hide_banner", "-loglevel", "error", "-nostdin", "-y",
			"-xerror", "-protocol_whitelist", "file,pipe",
			"-format_whitelist", "mov,matroska,webm,avi,mpegts,flv,ogg",
			"-threads", fmt.Sprint(e.config.Threads), "-i", input,
			"-map", "0:v:0", "-map", "0:a:0?", "-sn", "-dn",
			"-vf", filter, "-filter_threads", "1", "-c:v", "libx264",
			"-preset", "fast", "-pix_fmt", "yuv420p", "-b:v", p.bitrate,
			"-threads", fmt.Sprint(e.config.Threads), "-c:a", "aac", "-b:a", "128k",
			"-movflags", "+faststart", "-map_metadata", "-1", output,
		}
		if err := run(ctx, e.config.FFmpegPath, args...); err != nil {
			return nil, fmt.Errorf("encode %s: %w", name, err)
		}
		if err := e.probe(ctx, output, true); err != nil {
			return nil, fmt.Errorf("verify %s: %w", name, err)
		}
		object := fmt.Sprintf("videos/%s/%s/%s.mp4", fileHash, taskID, name)
		// Include the attempted key so a failed upload/signing step is also cleaned up.
		uploaded = append(uploaded, object)
		url, err := e.store.Upload(ctx, object, output)
		if err != nil {
			return nil, fmt.Errorf("upload %s: %w", name, err)
		}
		if url == "" {
			return nil, fmt.Errorf("upload %s returned an empty URL", name)
		}
		urls = append(urls, url)
		if err := progress(int32(5 + (i+1)*90/len(resolutions))); err != nil {
			return nil, err
		}
	}
	success = true
	return urls, nil
}

func (e *Engine) probe(ctx context.Context, path string, encoded bool) error {
	cmd := exec.CommandContext(ctx, e.config.FFprobePath,
		"-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm,avi,mpegts,flv,ogg",
		"-show_entries", "stream=codec_type,codec_name,width,height", "-of", "json", path)
	var stdout, stderr tailBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffprobe: %w: %s", err, stderr.String())
	}
	var media struct {
		Streams []struct {
			Type   string `json:"codec_type"`
			Codec  string `json:"codec_name"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(stdout.data, &media); err != nil {
		return fmt.Errorf("parse ffprobe output: %w", err)
	}
	found := false
	for _, s := range media.Streams {
		if s.Type == "video" && s.Width > 0 && s.Height > 0 {
			found = true
			if encoded && (s.Codec != "h264" || s.Width%2 != 0 || s.Height%2 != 0) {
				return fmt.Errorf("output is not an even-sized H264 video")
			}
		}
		if encoded && s.Type == "audio" && s.Codec != "aac" {
			return fmt.Errorf("output audio is not AAC")
		}
	}
	if !found {
		return fmt.Errorf("no video stream")
	}
	return nil
}

// Keep subprocess diagnostics bounded even for a corrupt or very large input.
type tailBuffer struct{ data []byte }

func (b *tailBuffer) Write(p []byte) (int, error) {
	const limit = 32 * 1024
	n := len(p)
	if n >= limit {
		b.data = append(b.data[:0], p[n-limit:]...)
	} else {
		if len(b.data)+n > limit {
			b.data = b.data[len(b.data)+n-limit:]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

func (b *tailBuffer) String() string { return strings.TrimSpace(string(b.data)) }

func run(ctx context.Context, executable string, args ...string) error {
	cmd := exec.CommandContext(ctx, executable, args...)
	var stderr tailBuffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}
