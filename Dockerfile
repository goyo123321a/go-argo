# 多阶段构建
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

WORKDIR /app

# 设置 Go 代理
ENV GOPROXY=https://goproxy.cn,direct
ENV CGO_ENABLED=0

# 复制 go.mod 和 go.sum（如果存在）
COPY go.mod go.sum* ./
RUN if [ -f go.mod ]; then go mod download; fi

# 复制所有源代码
COPY . .

# 设置构建参数
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=latest
ARG BUILD_TIME

# 构建应用，输出名为 app
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-w -s -extldflags '-static'" \
    -o app .

# 运行镜像
FROM alpine:3.18

# 安装运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    curl \
    && update-ca-certificates \
    && rm -rf /var/cache/apk/*

# 创建 app 用户和目录
RUN addgroup -g 1000 -S app && \
    adduser -u 1000 -S app -G app && \
    mkdir -p /app/.tmp /app/logs && \
    chown -R app:app /app

# 复制可执行文件
COPY --from=builder --chown=app:app /app/app /app/app

# 复制配置文件（如果有）
COPY --chown=app:app index.html /app/

# 切换用户
USER app

# 设置工作目录
WORKDIR /app

# 暴露端口
EXPOSE 7860

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:7860/health || exit 1

# 启动应用
CMD ["/app/app"]
