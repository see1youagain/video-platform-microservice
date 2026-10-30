ARG GO_IMAGE=golang:1.24.10-bookworm
FROM ${GO_IMAGE}
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg curl && rm -rf /var/lib/apt/lists/*
WORKDIR /workspace
COPY common ./common
COPY rpc-user ./rpc-user
COPY rpc-videoUpload ./rpc-videoUpload
COPY rpc-videoManager ./rpc-videoManager
COPY rpc-videoTranscode ./rpc-videoTranscode
COPY gateway ./gateway
COPY clients ./clients
COPY tests ./tests
RUN sed -i 's/\r$//' tests/integration/run.sh
CMD ["bash", "tests/integration/run.sh"]
