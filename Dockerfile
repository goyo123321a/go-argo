# 多阶段构建
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

WORKDIR /app

# 复制源代码
COPY . .

# 设置构建参数
ARG TARGETOS
ARG TARGETARCH

# 构建应用
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-w -s" -o myapp .

# 运行镜像
FROM alpine:3.18

WORKDIR /app

# 安装运行时依赖
RUN apk add --no-cache ca-certificates curl wget && \
    update-ca-certificates

# 创建用户和目录
RUN addgroup -g 1000 -S appuser && \
    adduser -u 1000 -S appuser -G appuser && \
    mkdir -p /app/.tmp && \
    chown -R appuser:appuser /app

# 复制文件
COPY --from=builder --chown=appuser:appuser /app/myapp /app/
COPY --chown=appuser:appuser index.html /app/

USER appuser

# 暴露端口
EXPOSE 7860

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=30s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3000/ || exit 1

CMD ["/app/myapp"]
