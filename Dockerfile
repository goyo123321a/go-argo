# 多阶段构建 Dockerfile
# 支持 amd64, arm64, arm/v7

# 构建阶段
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

# 构建参数
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG BUILD_DATE
ARG VERSION
ARG COMMIT

# 设置标签
LABEL build_date=${BUILD_DATE}
LABEL version=${VERSION}
LABEL commit=${COMMIT}

# 设置工作目录
WORKDIR /app

# 安装构建依赖
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata \
    upx \
    make

# 复制 go mod 文件
COPY go.mod go.sum ./

# 下载依赖（利用缓存）
RUN go mod download && go mod verify

# 复制源代码
COPY . .

# 编译应用
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    GOARM=${TARGETVARIANT} \
    go build -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildTime=${BUILD_DATE} -X main.GitCommit=${COMMIT}" \
    -o proxy-app \
    main.go

# 可选：压缩二进制文件
RUN if command -v upx > /dev/null 2>&1; then \
        upx --best --lzma proxy-app || true; \
    fi

# 运行阶段
FROM alpine:latest

# 安装运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    bash \
    curl \
    wget \
    jq \
    && update-ca-certificates

# 设置时区
ENV TZ=Asia/Shanghai

# 创建非 root 用户
RUN addgroup -g 1000 -S appuser && \
    adduser -u 1000 -S appuser -G appuser

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/proxy-app /app/proxy-app

# 复制配置文件（如果存在）
COPY --chown=appuser:appuser index.html /app/index.html 2>/dev/null || true

# 创建数据目录
RUN mkdir -p /app/.tmp && \
    chown -R appuser:appuser /app && \
    chmod +x /app/proxy-app

# 切换到非 root 用户
USER appuser

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:3000/ || exit 1

# 暴露端口
EXPOSE 3000

# 运行应用
CMD ["./proxy-app"]
