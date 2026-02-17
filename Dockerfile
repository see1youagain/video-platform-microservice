FROM alpine:3.19

WORKDIR /app

# 复制已编译的二进制文件
COPY rpc-video/rpc-video .

# 复制配置文件（可选）
COPY rpc-video/.env .

EXPOSE 8889
CMD ["./rpc-video"]