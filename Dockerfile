# 多阶段构建 Dockerfile - 支持多架构
# 阶段1: 构建阶段
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装必要的构建工具
RUN apk add --no-cache git ca-certificates tzdata upx

# 复制 go.mod 和 go.sum 文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download && go mod verify

# 复制源代码
COPY . .

# 构建参数
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

# 构建应用
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT} \
    go build \
    -ldflags="-w -s -extldflags '-static'" \
    -o /app/myapp \
    main.go && \
    upx --best --lzma /app/myapp || true

# 阶段2: 运行阶段
FROM alpine:latest

# 安装运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    bash \
    curl \
    && update-ca-certificates

# 设置时区
ENV TZ=Asia/Shanghai

# 创建非 root 用户
RUN addgroup -g 1000 -S appgroup && \
    adduser -u 1000 -S appuser -G appgroup

# 创建工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/myapp /app/myapp

# 复制伪装页面
COPY --from=builder /app/index.html /app/index.html

# 创建必要的目录并设置权限
RUN mkdir -p /app/tmp && \
    chown -R appuser:appgroup /app && \
    chmod +x /app/myapp

# 切换到非 root 用户
USER appuser

# 暴露端口
EXPOSE 3000

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:3000/health || exit 1

# 启动应用
ENTRYPOINT ["/app/myapp"]