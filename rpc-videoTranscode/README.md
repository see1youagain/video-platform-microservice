# Real video transcoding

`rpc-videoTranscode` downloads `videos/raw/{file_hash}.mp4` from MinIO,
encodes H.264 video and optional AAC audio with FFmpeg, checks the result with
ffprobe, and uploads MP4 files with `faststart`. It supports `360p`, `480p`,
`720p`, and `1080p`; each profile is a bounding box, preserves aspect ratio,
and uses even dimensions. Silent and portrait videos are supported.

## Request and state flow

1. Authenticate and upload the original through the existing multipart API.
2. `POST /api/video/transcode` with `file_hash`, `resolutions` and an optional
   `request_id` (up to 64 bytes).
3. VideoManager commits the owned video's state, `transcode_tasks`, and an
   `outbox_events` record in one MySQL transaction.
4. The relay publishes `video.transcode.tasks`. The worker service also consumes
   the old `video.transcode.requested` topic for in-flight upgrade messages.
5. Messages are acknowledged only after insertion into `transcode_jobs`.
   Fixed database workers claim jobs using `FOR UPDATE SKIP LOCKED`.
6. The worker downloads, encodes, verifies, and uploads every requested profile.
   A single failed profile makes the job fail; it never substitutes a fake URL.
7. Terminal job state and `video.transcode.finished` are committed together.
   VideoManager applies the result to both task and video in one transaction,
   then acknowledges the message.
8. Poll `GET /api/video/transcode/status?task_id=...` until `completed` or `failed`.
   Progress is a fraction from 0 to 1; completed tasks include `completed_urls`.
   A failed task's `msg` includes its processing error.

Example body:

```json
{"file_hash":"<MD5 or SHA256 hex digest>","resolutions":["360p","720p"],"request_id":"my-upload-transcode-1"}
```

Reusing a `request_id` with the same user and parameters returns the same task,
including after completion. Changing parameters with the same ID returns 409.
Requests without an ID reuse matching active work. A failed task may be retried
with a new ID. Task creation and status queries enforce the authenticated user.
The legacy direct `CreateTask` RPC reads the canonical MinIO object derived
from `file_hash`; its historical `minio_url` argument is not fetched.

## Runtime

Install FFmpeg with the `libx264` encoder and ffprobe. The service fails startup
if either executable or MinIO is unavailable. The supplied Dockerfile installs
FFmpeg and listens on 8084; provide secrets through environment variables.

| Variable | Default | Meaning |
|---|---|---|
| `FFMPEG_PATH` | `ffmpeg` | Encoder executable |
| `FFPROBE_PATH` | `ffprobe` | Media inspection executable |
| `TRANSCODE_WORKERS` | `2` | Concurrent jobs per replica, 1–64 |
| `TRANSCODE_THREADS` | `2` | Codec threads per job, 1–64 |
| `TRANSCODE_TIMEOUT` | `30m` | Whole-job timeout, including object I/O |
| `TRANSCODE_TEMP_DIR` | OS temporary directory | Parent for isolated job directories |
| `VIDEOTRANSCODE_ADDR` | `:8084` | RPC listen address |

MySQL 8 is required for `SKIP LOCKED`. Services currently share the database
configured by `DB_*`; VideoManager reads `transcode_jobs` for live progress.
Schema additions are applied by GORM AutoMigrate. Back up production data before
upgrading; existing fake output from older versions must be regenerated.

Provision the Kafka topics before starting the relays: `video.file.uploaded`,
`video.transcode.tasks`, `video.transcode.requested` (upgrade compatibility), and
`video.transcode.finished`. The isolated test stack includes a topic initializer.

Workers renew a 90-second lease every 30 seconds. A crashed worker's job becomes
eligible after expiry; a stale lease cannot commit terminal state. Shutdown
cancels FFmpeg and returns owned work to pending. Each lease uses distinct output
keys, preventing stale encoders from overwriting replacement outputs. Temporary
directories are removed after processing; partial uploaded objects are cleaned up
on a best-effort basis. A hard process crash can leave orphan objects, so storage
lifecycle/garbage collection is still an operational responsibility.

Outbox rows abandoned in `processing` are reclaimed after five minutes. Delivery
is at least once; consumers deduplicate by task ID. Object URLs are signed for
seven days by the existing storage layer; they are not permanent public URLs.

## Tests

Run unit and real-media integration tests with Go 1.24+ and FFmpeg on PATH:

```bash
cd common && go test ./...
cd ../rpc-videoTranscode && go test ./... -count=1
cd ../rpc-videoManager && go test ./... -count=1
```

For the complete HTTP → RPC → MySQL/Outbox → Kafka → FFmpeg → MinIO → status
round trip and existing regression suite, use the disposable stack:

```bash
docker compose -f tests/integration/compose.yml build
docker compose -f tests/integration/compose.yml run --rm runner
docker compose -f tests/integration/compose.yml down -v
```

It uses its own network and temporary database/object storage. It exposes no host
ports. Media tests explicitly skip when their tools are absent, and HTTP E2E
requires `RUN_TRANSCODE_E2E=1`; a skipped integration test is not verification.
