# 多阶段构建 Dockerfile
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装构建依赖
RUN apk add --no-cache git ca-certificates

# 复制 go 模块文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 设置构建参数
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=latest
ARG BUILD_DATE
ARG COMMIT

# 构建应用
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-w -s \
    -X 'main.Version=${VERSION}' \
    -X 'main.BuildDate=${BUILD_DATE}' \
    -X 'main.GitCommit=${COMMIT}'" \
    -o app main.go

# 最终镜像
FROM alpine:3.18

# 设置工作目录
WORKDIR /app

# 安装运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl \
    wget \
    bash \
    jq \
    && update-ca-certificates

# 设置时区
ENV TZ=Asia/Shanghai

# 创建非 root 用户
RUN addgroup -g 1000 -S appuser && \
    adduser -u 1000 -S appuser -G appuser

# 复制可执行文件
COPY --from=builder --chown=appuser:appuser /app/app /app/proxy-app

# 复制配置文件（如果存在）
COPY --chown=appuser:appuser index.html /app/ 2>/dev/null || true

# 创建必要的目录
RUN mkdir -p /app/.tmp && \
    chown -R appuser:appuser /app && \
    chmod +x /app/proxy-app

# 切换用户
USER appuser

# 暴露端口
EXPOSE 3000

# 设置卷
VOLUME ["/app/.tmp"]

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:3000/ || exit 1

# 启动命令
CMD ["/app/proxy-app"]
