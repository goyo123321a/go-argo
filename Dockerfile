# 最简单的 Dockerfile
FROM golang:1.21

WORKDIR /app

# 复制所有文件
COPY . .

# 下载依赖并构建
RUN go mod download && \
    go build -o myapp main.go

# 暴露端口
EXPOSE 3000

# 运行
CMD ["./myapp"]
