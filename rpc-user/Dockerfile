FROM golang:1.24.3-alpine AS builder

WORKDIR /build

ENV GOPROXY=https://goproxy.cn,direct

COPY common ./common
COPY rpc-user ./rpc-user

WORKDIR /build/rpc-user

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o rpc-user .

FROM alpine:latest

WORKDIR /app
COPY --from=builder /build/rpc-user/rpc-user .
COPY --from=builder /build/rpc-user/.env .

EXPOSE 8888
CMD ["./rpc-user"]