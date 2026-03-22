# 多阶段构建 Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

# 设置 Go 代理（可选，加速下载）
ENV GOPROXY=https://goproxy.cn,direct

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o myapp .

# 运行阶段
FROM alpine:latest

# 安装运行时依赖
RUN apk add --no-cache ca-certificates tzdata wget && \
    update-ca-certificates

# 创建非 root 用户
RUN addgroup -g 1000 -S appuser && \
    adduser -u 1000 -S appuser -G appuser

WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/myapp .

# 创建临时目录
RUN mkdir -p /app/.tmp && \
    chown -R appuser:appuser /app

USER appuser

EXPOSE 3000 8001

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3000/ || exit 1

CMD ["./myapp"]