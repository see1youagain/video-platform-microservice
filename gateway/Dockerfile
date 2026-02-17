FROM golang:1.24.3-alpine AS builder

WORKDIR /build

ENV GOPROXY=https://goproxy.cn,direct

COPY common ./common
COPY rpc-user ./rpc-user
COPY rpc-video ./rpc-video
COPY gateway ./gateway

WORKDIR /build/gateway

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o gateway .

FROM alpine:latest

WORKDIR /app
COPY --from=builder /build/gateway/gateway .
COPY --from=builder /build/gateway/.env .

EXPOSE 8080
CMD ["./gateway"]