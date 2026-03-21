.PHONY: build run stop clean test

# 变量定义
IMAGE_NAME=proxy-app
VERSION=latest

# 构建镜像
build:
	docker build -t $(IMAGE_NAME):$(VERSION) .

# 多架构构建
buildx:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(IMAGE_NAME):$(VERSION) --push .

# 运行容器
run:
	docker run -d \
		--name $(IMAGE_NAME) \
		-p 3000:3000 \
		-p 8001:8001 \
		--env-file .env \
		-v $(PWD)/data:/app/.tmp \
		$(IMAGE_NAME):$(VERSION)

# 停止容器
stop:
	docker stop $(IMAGE_NAME) || true
	docker rm $(IMAGE_NAME) || true

# 查看日志
logs:
	docker logs -f $(IMAGE_NAME)

# 进入容器
shell:
	docker exec -it $(IMAGE_NAME) /bin/sh

# 清理
clean: stop
	docker rmi $(IMAGE_NAME):$(VERSION) || true
	rm -rf data/

# 测试
test:
	go test -v ./...

# 本地运行（不使用 Docker）
run-local:
	go run main.go

# 构建二进制
build-local:
	go build -o proxy-app main.go
