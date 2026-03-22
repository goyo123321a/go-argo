# Makefile for myapp

.PHONY: build run clean docker-build docker-run docker-stop docker-logs help

# 变量
IMAGE_NAME := myapp
CONTAINER_NAME := myapp
PORT := 3000
ARGO_PORT := 8001

help:
	@echo "Available commands:"
	@echo "  make build        - Build Go binary"
	@echo "  make run          - Run Go binary locally"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make docker-build - Build Docker image"
	@echo "  make docker-run   - Run Docker container"
	@echo "  make docker-stop  - Stop Docker container"
	@echo "  make docker-logs  - View container logs"
	@echo "  make docker-shell - Shell into container"

build:
	@echo "Building Go binary..."
	go build -o myapp .

run: build
	@echo "Running myapp..."
	./myapp

clean:
	@echo "Cleaning..."
	rm -f myapp
	rm -rf .tmp/

docker-build:
	@echo "Building Docker image..."
	docker build -t $(IMAGE_NAME) .

docker-run: docker-build
	@echo "Running Docker container..."
	docker run -d \
		--name $(CONTAINER_NAME) \
		-p $(PORT):3000 \
		-p $(ARGO_PORT):8001 \
		-e UUID=$(UUID) \
		-e NEZHA_SERVER=$(NEZHA_SERVER) \
		-e NEZHA_KEY=$(NEZHA_KEY) \
		-e ARGO_DOMAIN=$(ARGO_DOMAIN) \
		-e ARGO_AUTH=$(ARGO_AUTH) \
		--restart unless-stopped \
		$(IMAGE_NAME)

docker-stop:
	@echo "Stopping container..."
	docker stop $(CONTAINER_NAME) || true
	docker rm $(CONTAINER_NAME) || true

docker-logs:
	docker logs -f $(CONTAINER_NAME)

docker-shell:
	docker exec -it $(CONTAINER_NAME) /bin/sh

docker-clean: docker-stop
	docker rmi $(IMAGE_NAME) || true