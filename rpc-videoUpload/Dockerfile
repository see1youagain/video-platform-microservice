FROM golang:1.24.3-alpine AS builder

WORKDIR /build

ENV GOPROXY=https://goproxy.cn,direct

COPY common ./common
COPY rpc-videoUpload ./rpc-videoUpload

WORKDIR /build/rpc-videoUpload

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o rpc-videoUpload .

FROM alpine:latest

RUN apk --no-cache add tzdata ca-certificates
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /build/rpc-videoUpload/rpc-videoUpload .
COPY --from=builder /build/rpc-videoUpload/.env .

EXPOSE 8888
CMD ["./rpc-videoUpload"]
