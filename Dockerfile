# 简化版 Dockerfile
FROM golang:1.21-alpine

# 设置工作目录
WORKDIR /app

# 安装必要工具
RUN apk add --no-cache git

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 直接构建（不压缩，不优化）
RUN go build -o myapp main.go

# 暴露端口
EXPOSE 3000

# 运行
CMD ["./myapp"]
