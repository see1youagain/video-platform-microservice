FROM golang:1.24.3-alpine AS builder

WORKDIR /build

ENV GOPROXY=https://goproxy.cn,direct

COPY common ./common
COPY rpc-videoTranscode ./rpc-videoTranscode

WORKDIR /build/rpc-videoTranscode

RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -o rpc-videoTranscode .

FROM alpine:latest

RUN apk --no-cache add tzdata ca-certificates ffmpeg
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /build/rpc-videoTranscode/rpc-videoTranscode .

EXPOSE 8084
CMD ["./rpc-videoTranscode"]
