# 使用多阶段构建以减小镜像大小
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装构建依赖
RUN apk add --no-cache git

# 复制go模块文件
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

# 构建应用
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-w -s \
    -X 'main.Version=${VERSION}' \
    -X 'main.BuildDate=${BUILD_DATE}'" \
    -o app .

# 最终镜像
FROM alpine:3.18

# 设置工作目录
WORKDIR /app

# 安装运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    curl \
    wget \
    bash \
    jq \
    && update-ca-certificates

# 创建非root用户
RUN addgroup -g 1000 -S app && \
    adduser -u 1000 -S app -G app

# 复制可执行文件
COPY --from=builder --chown=app:app /app/app /app/

# 复制配置文件（如果有）
COPY --chown=app:app index.html /app/

# 创建必要的目录
RUN mkdir -p /app/tmp && chown -R app:app /app/tmp

# 切换用户
USER app

# 暴露端口
EXPOSE 7860

# 设置卷
VOLUME ["/app/tmp"]

# 启动命令
CMD ["/app/app"]
