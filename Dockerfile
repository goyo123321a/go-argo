# 多阶段构建 Dockerfile
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装构建依赖
RUN apk add --no-cache git ca-certificates tzdata

# 设置构建参数
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG VERSION=latest
ARG BUILD_DATE
ARG COMMIT

# 设置 Go 环境变量（使用国内代理加速）
ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,direct \
    CGO_ENABLED=0

# 复制 go mod 文件
COPY go.mod ./
# 注意：如果 go.sum 有问题，可以先不复制，让 go mod tidy 重新生成
RUN if [ -f go.sum ]; then cp go.sum .; fi

# 下载依赖（如果 go.sum 有问题，使用 -mod=mod 跳过验证）
RUN go mod download -mod=mod 2>/dev/null || go mod tidy

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    GOARM=${TARGETVARIANT} \
    go build -mod=mod -buildvcs=false \
    -ldflags="-w -s \
    -X 'main.Version=${VERSION}' \
    -X 'main.BuildDate=${BUILD_DATE}' \
    -X 'main.GitCommit=${COMMIT}'" \
    -o proxy-app main.go

# 可选：压缩二进制文件（减小镜像大小）
RUN if command -v upx >/dev/null 2>&1; then \
        upx --best --lzma proxy-app 2>/dev/null || true; \
    else \
        apk add --no-cache upx && upx --best --lzma proxy-app || true; \
    fi

# 最终镜像
FROM alpine:3.19

# 设置标签
LABEL maintainer="Your Name" \
      version="${VERSION}" \
      description="Proxy Application with Xray and Cloudflare Tunnel"

# 设置工作目录
WORKDIR /app

# 安装运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl \
    wget \
    bash \
    && update-ca-certificates \
    && rm -rf /var/cache/apk/*

# 设置时区
ENV TZ=Asia/Shanghai

# 创建非 root 用户
RUN addgroup -g 1000 -S appuser && \
    adduser -u 1000 -S appuser -G appuser

# 复制可执行文件
COPY --from=builder --chown=appuser:appuser /app/proxy-app /app/proxy-app

# 复制 index.html（如果存在，不存在则跳过）
COPY --chown=appuser:appuser index.html /app/ 2>/dev/null || true

# 创建必要的目录（与代码中的 FILE_PATH 保持一致）
RUN mkdir -p /app/.tmp && \
    chown -R appuser:appuser /app && \
    chmod +x /app/proxy-app

# 切换用户
USER appuser

# 暴露端口（根据代码中的实际端口）
EXPOSE 3000

# 设置卷（与代码中的 FILE_PATH 保持一致）
VOLUME ["/app/.tmp"]

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:3000/ || exit 1

# 启动命令
CMD ["/app/proxy-app"]
