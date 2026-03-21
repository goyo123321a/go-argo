#!/bin/bash

# 多架构构建脚本
set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

# 镜像名称
IMAGE_NAME="myapp"
REGISTRY="${REGISTRY:-ghcr.io/$(whoami)}"

# 版本标签
VERSION=${1:-latest}

echo -e "${GREEN}Building multi-architecture Docker image for myapp${NC}"
echo -e "${YELLOW}Image: ${REGISTRY}/${IMAGE_NAME}:${VERSION}${NC}"
echo -e "${YELLOW}Platforms: linux/amd64, linux/arm64, linux/arm/v7${NC}"

# 检查 Docker Buildx
if ! docker buildx version > /dev/null 2>&1; then
    echo -e "${RED}Docker Buildx not available. Please install Docker Buildx.${NC}"
    exit 1
fi

# 创建并启用 Buildx builder
echo -e "${GREEN}Setting up Docker Buildx...${NC}"
docker buildx create --name multiarch --use || true
docker buildx inspect --bootstrap

# 构建多架构镜像
echo -e "${GREEN}Building multi-architecture image...${NC}"
docker buildx build \
    --platform linux/amd64,linux/arm64,linux/arm/v7 \
    --tag ${REGISTRY}/${IMAGE_NAME}:${VERSION} \
    --tag ${REGISTRY}/${IMAGE_NAME}:latest \
    --push \
    -f Dockerfile \
    .

echo -e "${GREEN}Multi-architecture image built and pushed successfully!${NC}"
echo -e "${GREEN}Tags:${NC}"
echo -e "  - ${REGISTRY}/${IMAGE_NAME}:${VERSION}"
echo -e "  - ${REGISTRY}/${IMAGE_NAME}:latest"

# 显示镜像信息
echo -e "\n${GREEN}Image manifests:${NC}"
docker buildx imagetools inspect ${REGISTRY}/${IMAGE_NAME}:${VERSION}