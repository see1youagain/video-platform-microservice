FROM golang:1.24.3-alpine AS builder

WORKDIR /build

ENV GOPROXY=https://goproxy.cn,direct

COPY common ./common
COPY rpc-videoManager ./rpc-videoManager

WORKDIR /build/rpc-videoManager

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o rpc-videoManager .

FROM alpine:latest

RUN apk --no-cache add tzdata ca-certificates
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /build/rpc-videoManager/rpc-videoManager .
COPY --from=builder /build/rpc-videoManager/.env .

EXPOSE 8889
CMD ["./rpc-videoManager"]
