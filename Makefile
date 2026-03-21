# Makefile for myapp

.PHONY: build run clean test docker-build docker-push docker-buildx help

# 变量
IMAGE_NAME := myapp
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")
REGISTRY ?= ghcr.io/$(shell whoami)

# 颜色输出
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
NC := \033[0m # No Color

help:
	@echo "$(GREEN)Available targets:$(NC)"
	@echo "  build        - Build the binary"
	@echo "  run          - Run the application"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-push  - Push Docker image to registry"
	@echo "  docker-buildx- Build multi-architecture Docker image"
	@echo "  help         - Show this help"

# 构建二进制文件
build:
	@echo "$(GREEN)Building binary...$(NC)"
	go build -ldflags="-w -s" -o myapp main.go
	@echo "$(GREEN)Build complete: myapp$(NC)"

# 运行
run:
	@echo "$(GREEN)Running application...$(NC)"
	go run main.go

# 清理
clean:
	@echo "$(YELLOW)Cleaning...$(NC)"
	rm -f myapp
	rm -rf tmp/
	@echo "$(GREEN)Clean complete$(NC)"

# 测试
test:
	@echo "$(GREEN)Running tests...$(NC)"
	go test -v ./...

# 构建 Docker 镜像（单架构）
docker-build:
	@echo "$(GREEN)Building Docker image...$(NC)"
	docker build -t $(IMAGE_NAME):$(VERSION) -t $(IMAGE_NAME):latest .
	@echo "$(GREEN)Image built: $(IMAGE_NAME):$(VERSION)$(NC)"

# 推送镜像（单架构）
docker-push: docker-build
	@echo "$(GREEN)Pushing Docker image...$(NC)"
	docker tag $(IMAGE_NAME):$(VERSION) $(REGISTRY)/$(IMAGE_NAME):$(VERSION)
	docker push $(REGISTRY)/$(IMAGE_NAME):$(VERSION)
	docker tag $(IMAGE_NAME):latest $(REGISTRY)/$(IMAGE_NAME):latest
	docker push $(REGISTRY)/$(IMAGE_NAME):latest
	@echo "$(GREEN)Image pushed successfully$(NC)"

# 多架构构建并推送
docker-buildx:
	@echo "$(GREEN)Building multi-architecture image...$(NC)"
	docker buildx create --name multiarch --use || true
	docker buildx inspect --bootstrap
	docker buildx build \
		--platform linux/amd64,linux/arm64,linux/arm/v7 \
		--tag $(REGISTRY)/$(IMAGE_NAME):$(VERSION) \
		--tag $(REGISTRY)/$(IMAGE_NAME):latest \
		--push \
		-f Dockerfile \
		.
	@echo "$(GREEN)Multi-architecture image built and pushed successfully$(NC)"
	@echo "$(GREEN)Tags:$(NC)"
	@echo "  - $(REGISTRY)/$(IMAGE_NAME):$(VERSION)"
	@echo "  - $(REGISTRY)/$(IMAGE_NAME):latest"

# 运行 Docker 容器
docker-run:
	@echo "$(GREEN)Running Docker container...$(NC)"
	docker run -d \
		--name $(IMAGE_NAME) \
		-p 3000:3000 \
		-e UUID=$(UUID) \
		-e NEZHA_SERVER=$(NEZHA_SERVER) \
		-e NEZHA_KEY=$(NEZHA_KEY) \
		$(IMAGE_NAME):latest
	@echo "$(GREEN)Container started$(NC)"

# 停止并删除容器
docker-stop:
	@echo "$(YELLOW)Stopping container...$(NC)"
	docker stop $(IMAGE_NAME) || true
	docker rm $(IMAGE_NAME) || true
	@echo "$(GREEN)Container stopped and removed$(NC)"

# 查看日志
logs:
	docker logs -f $(IMAGE_NAME)

# 进入容器
shell:
	docker exec -it $(IMAGE_NAME) /bin/sh