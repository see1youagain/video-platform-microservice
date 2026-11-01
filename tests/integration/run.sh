#!/usr/bin/env bash
set -euo pipefail
cd /workspace
mkdir -p /tmp/transcode-test-logs /tmp/transcode-test-bin
pids=()
cleanup() {
  local result=$?
  trap - EXIT
  for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; done
  if [ "$result" -ne 0 ]; then
    for file in /tmp/transcode-test-logs/*.log; do
      echo "Log: $file"
      tail -n 80 "$file"
    done
  fi
  exit "$result"
}
trap cleanup EXIT

# Unit, SQL-contract regression, and real FFmpeg integration tests all run here.
for module in common rpc-videoTranscode rpc-videoManager gateway rpc-user rpc-videoUpload clients; do
  (cd "$module" && go test ./... -count=1)
done
for module in common rpc-videoTranscode rpc-videoManager; do
  (cd "$module" && go test -race ./... -count=1)
done
for service in rpc-user rpc-videoUpload rpc-videoManager rpc-videoTranscode gateway; do
  (cd "$service" && go build -o "/tmp/transcode-test-bin/$service" .)
done

# AutoMigrate touches a shared outbox table. Start each service in order and
# allow its startup to finish instead of racing schema creation.
for service in rpc-user rpc-videoUpload rpc-videoManager rpc-videoTranscode gateway; do
  (cd "$service" && exec "/tmp/transcode-test-bin/$service") >"/tmp/transcode-test-logs/$service.log" 2>&1 &
  pids+=("$!")
  sleep 3
  kill -0 "${pids[-1]}"
done
for attempt in $(seq 1 30); do
  if curl -fsS http://127.0.0.1:8080/ping >/dev/null; then break; fi
  sleep 1
done
curl -fsS http://127.0.0.1:8080/ping >/dev/null
(cd tests && go test ./integration -run TestTranscodeE2E -count=1 -v -timeout=5m)
# Existing authentication/upload/download/delete suites remain the regression baseline.
(cd tests && go run ./cmd regression)
