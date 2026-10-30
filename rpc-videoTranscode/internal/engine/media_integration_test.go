package engine

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

type diskStore struct {
	source, root string
	keys         []string
}

func (s *diskStore) Download(_ context.Context, _ string, path string) error {
	data, err := os.ReadFile(s.source)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
func (s *diskStore) Upload(_ context.Context, key, path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(s.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(dest, data, 0600); err != nil {
		return "", err
	}
	s.keys = append(s.keys, key)
	return dest, nil
}
func (s *diskStore) Delete(_ context.Context, key string) error {
	return os.Remove(filepath.Join(s.root, filepath.FromSlash(key)))
}

func mediaTools(t *testing.T) (string, string) {
	t.Helper()
	find := func(env, fallback string) string {
		v := os.Getenv(env)
		if v == "" {
			v = fallback
		}
		p, err := exec.LookPath(v)
		if err != nil {
			t.Skipf("real-media integration requires %s or %s on PATH", env, fallback)
		}
		return p
	}
	return find("FFMPEG_PATH", "ffmpeg"), find("FFPROBE_PATH", "ffprobe")
}

func TestRealMediaIntegration(t *testing.T) {
	ffmpeg, ffprobe := mediaTools(t)
	cases := []struct {
		name, size string
		audio      bool
	}{{"landscapeWithAudio", "320x180", true}, {"portraitSilent", "180x320", false}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source.mp4")
			args := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc2=size=" + tc.size + ":rate=12"}
			if tc.audio {
				args = append(args, "-f", "lavfi", "-i", "sine=frequency=1000:sample_rate=44100")
			}
			args = append(args, "-t", "0.6", "-c:v", "mpeg4", "-pix_fmt", "yuv420p")
			if tc.audio {
				args = append(args, "-c:a", "aac")
			}
			args = append(args, source)
			if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
				t.Fatalf("generate fixture: %v %s", err, out)
			}
			work := filepath.Join(root, "work")
			if err := os.Mkdir(work, 0700); err != nil {
				t.Fatal(err)
			}
			store := &diskStore{source: source, root: filepath.Join(root, "objects")}
			e, err := New(Config{FFmpegPath: ffmpeg, FFprobePath: ffprobe, TempDir: work, Timeout: time.Minute, Threads: 1}, store)
			if err != nil {
				t.Fatal(err)
			}
			var progress []int32
			urls, err := e.Transcode(context.Background(), "real-media-test", testHash, []string{"360p", "480p", "720p", "1080p"}, func(p int32) error { progress = append(progress, p); return nil })
			if err != nil {
				t.Fatal(err)
			}
			if len(urls) != 4 {
				t.Fatalf("outputs=%v", urls)
			}
			for i, path := range urls {
				out, err := exec.Command(ffprobe, "-v", "error", "-show_streams", "-show_format", "-of", "json", path).Output()
				if err != nil {
					t.Fatal(err)
				}
				var media struct {
					Streams []struct {
						Type   string `json:"codec_type"`
						Codec  string `json:"codec_name"`
						Width  int    `json:"width"`
						Height int    `json:"height"`
					}
					Format struct {
						Duration string `json:"duration"`
					}
				}
				if err := json.Unmarshal(out, &media); err != nil {
					t.Fatal(err)
				}
				hasVideo, hasAudio := false, false
				profile := profiles[[]string{"360p", "480p", "720p", "1080p"}[i]]
				for _, stream := range media.Streams {
					if stream.Type == "video" {
						hasVideo = true
						if stream.Codec != "h264" || stream.Width <= 0 || stream.Height <= 0 || stream.Width%2 != 0 || stream.Height%2 != 0 || stream.Width > profile.width || stream.Height > profile.height {
							t.Fatalf("invalid encoded stream %+v", stream)
						}
						if tc.name == "portraitSilent" && stream.Width >= stream.Height {
							t.Fatal("portrait aspect ratio lost")
						}
						if tc.name == "landscapeWithAudio" && stream.Width <= stream.Height {
							t.Fatal("landscape aspect ratio lost")
						}
					}
					if stream.Type == "audio" {
						hasAudio = true
						if stream.Codec != "aac" {
							t.Fatalf("audio codec=%s", stream.Codec)
						}
					}
				}
				if !hasVideo || hasAudio != tc.audio {
					t.Fatalf("video=%v audio=%v", hasVideo, hasAudio)
				}
				duration, err := strconv.ParseFloat(media.Format.Duration, 64)
				if err != nil || duration < 0.4 || duration > 1.5 {
					t.Fatalf("invalid output duration %q", media.Format.Duration)
				}
				if decoded, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-xerror", "-i", path, "-f", "null", "-").CombinedOutput(); err != nil {
					t.Fatalf("output is not decodable: %v %s", err, decoded)
				}
			}
			if len(progress) != 5 || progress[len(progress)-1] != 95 {
				t.Fatalf("progress=%v", progress)
			}
			assertWorkspaceClean(t, e)
		})
	}
}

func TestRealMediaRejectsCorruptSource(t *testing.T) {
	ffmpeg, ffprobe := mediaTools(t)
	root := t.TempDir()
	source := filepath.Join(root, "corrupt.mp4")
	if err := os.WriteFile(source, []byte("this is not a video"), 0600); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "work")
	os.Mkdir(work, 0700)
	store := &diskStore{source: source, root: filepath.Join(root, "objects")}
	e, err := New(Config{FFmpegPath: ffmpeg, FFprobePath: ffprobe, TempDir: work}, store)
	if err != nil {
		t.Fatal(err)
	}
	if urls, err := e.Transcode(context.Background(), "corrupt-source", testHash, []string{"360p"}, nil); err == nil || len(urls) != 0 {
		t.Fatalf("corrupt source accepted: urls=%v err=%v", urls, err)
	}
	if len(store.keys) != 0 {
		t.Fatal("corrupt input uploaded outputs")
	}
	assertWorkspaceClean(t, e)
}
