FROM golang:1.24.3-alpine AS builder

WORKDIR /build

# 设置国内镜像源（可选，加快下载）
ENV GOPROXY=https://goproxy.cn,direct

COPY common ./common
COPY rpc-video ./rpc-video

WORKDIR /build/rpc-video

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o rpc-video .

FROM alpine:latest

WORKDIR /app
COPY --from=builder /build/rpc-video/rpc-video .
COPY --from=builder /build/rpc-video/.env .

EXPOSE 8889
CMD ["./rpc-video"]