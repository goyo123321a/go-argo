<div align="center">
  <h2>
    <img src="https://cdn.nodeimage.com/i/NXz3ah3zTwikq3AdQOU0dYw3uyaBiGVj.webp" width="40" height="40" style="vertical-align: middle;"/> 
    go-argo - Argo隧道代理工具
  </h2>
  go-argo是一个强大的Argo隧道部署工具，专为Linux服务器和容器环境设计。它支持多种代理协议（VLESS、VMess、Trojan等），并集成了哪吒探针功能。

---

Telegram交流反馈群组：https://t.me/eooceu
</div>

## 郑重声明
* 本项目自2025年10月29日15时45分起,已更改开源协议,并包含以下特定要求
* 此项目仅限个人使用，禁止用于商业行为(包括但不限于：youtube,bilibili,tiktok,facebook..等等)
* 禁止新建项目将代码复制到自己仓库中用做商业行为
* 请遵守当地法律法规,禁止滥用做公共代理行为
* 如有违反以上条款者将追究法律责任

## 说明 （部署前请仔细阅读）

* 本项目是为Linux服务器和容器环境设计的Argo隧道部署工具，采用Go语言编写，性能更优，资源占用更低。
* 支持一键部署脚本，自动检测系统架构（amd64/arm64），下载对应版本并后台运行。
* 不填写ARGO_DOMAIN和ARGO_AUTH两个变量即启用临时隧道，反之则使用固定隧道。
* 哪吒v0/v1可选,当哪吒端口为{443,8443,2096,2087,2083,2053}其中之一时，自动开启tls。
* 订阅地址返回base64编码的节点信息，支持直接查看或下载。

## 📋 环境变量

| 变量名 | 是否必须 | 默认值 | 说明 |
|--------|----------|--------|------|
| UPLOAD_URL | 否 | - | 订阅上传地址（Merge-sub项目地址） |
| PROJECT_URL | 否 | - | 项目分配的域名 |
| AUTO_ACCESS | 否 | false | 是否开启自动访问保活 |
| SERVER_PORT | 否 | 7860 | HTTP服务监听端口 |
| ARGO_PORT | 否 | 8001 | Argo隧道端口（内部使用） |
| UUID | 否 | 9afd1229-b893-40c1-84dd-51e7ce204913 | 用户UUID |
| NEZHA_SERVER | 否 | - | 哪吒面板域名（v1填域名:端口） |
| NEZHA_PORT | 否 | - | 哪吒端口（v0使用，v1这里留空） |
| NEZHA_KEY | 否 | - | 哪吒密钥 |
| ARGO_DOMAIN | 否 | - | Argo固定隧道域名 |
| ARGO_AUTH | 否 | - | Argo固定隧道密钥（Token或JSON） |
| CFIP | 否 | saas.sin.fan | 节点优选域名或IP |
| CFPORT | 否 | 443 | 节点端口 |
| NAME | 否 | - | 节点名称前缀（留空自动获取ISP信息） |
| FILE_PATH | 否 | /app/tmp | 运行目录 |
| SUB_PATH | 否 | sub | 订阅路径 |

## 🌐 订阅地址

- 标准端口：`https://your-domain.com/sub`
- 非标端口：`http://your-domain.com:port/sub`
- 下载地址：`http://your-domain.com:port/sub/download`
- 原始节点：`http://your-domain.com:port/sub/raw`
- 状态查看：`http://your-domain.com:port/status`
- 健康检查：`http://your-domain.com:port/health`

---

## 🚀 快速部署

### 一键部署脚本（推荐）

```bash
# 下载并运行一键部署脚本
bash <(curl -sL https://raw.githubusercontent.com/goyo123321a/go-argo/main/deploy.sh)

脚本会自动：

· 检测系统架构（amd64/arm64）
· 下载对应版本的二进制文件
· 生成随机6位字母文件名
· 交互式配置环境变量
· 后台运行服务
· 可选安装为systemd服务

手动部署

```bash
# 1. 下载对应版本
# amd64
wget https://github.com/goyo123321a/go-argo/releases/download/latest/myapp-linux-amd64 -O myapp

# arm64
wget https://github.com/goyo123321a/go-argo/releases/download/latest/myapp-linux-arm64 -O myapp

# 2. 赋予执行权限
chmod +x myapp

# 3. 设置环境变量
export UUID="your-uuid-here"
export NEZHA_SERVER="nz.your-domain.com:8008"
export NEZHA_KEY="your-nezha-key"
export SERVER_PORT=7860

# 4. 创建临时目录
mkdir -p ./tmp

# 5. 运行
./myapp
```

🐳 Docker 部署

使用 Docker 运行

```bash
# 拉取镜像
docker pull ghcr.io/goyo123321a/go-argo:latest

# 运行容器
docker run -d \
  --name go-argo \
  -p 7860:7860 \
  -v ./data:/app/tmp \
  -e UUID="your-uuid" \
  -e NEZHA_SERVER="nz.example.com:8008" \
  -e NEZHA_KEY="your-key" \
  -e ARGO_DOMAIN="your-domain.com" \
  -e ARGO_AUTH="your-token" \
  ghcr.io/goyo123321a/go-argo:latest
```

使用 Docker Compose

创建 docker-compose.yml：

```yaml
version: '3.8'

services:
  go-argo:
    image: ghcr.io/goyo123321a/go-argo:latest
    container_name: go-argo
    restart: unless-stopped
    ports:
      - "7860:7860"
    volumes:
      - ./data:/app/tmp
    environment:
      - UUID=9afd1229-b893-40c1-84dd-51e7ce204913
      - NEZHA_SERVER=nz.example.com:8008
      - NEZHA_KEY=your-secret-key
      - SERVER_PORT=7860
      - ARGO_PORT=8001
      - CFIP=saas.sin.fan
      - CFPORT=443
```

启动服务：

```bash
docker-compose up -d
```

🔧 环境变量配置

使用 .env 文件

创建 .env 文件：

```bash
# myapp 配置文件
UUID=9afd1229-b893-40c1-84dd-51e7ce204913
CFIP=saas.sin.fan
CFPORT=443
NAME=My-VPS
SERVER_PORT=7860
SUB_PATH=sub
ARGO_PORT=8001
FILE_PATH=./tmp
AUTO_ACCESS=false
```

使用 export 设置

```bash
export UUID="your-uuid-here"
export NEZHA_SERVER="nz.your-domain.com:8008"
export NEZHA_KEY="your-nezha-key"
export SERVER_PORT=7860
./myapp
```

🔧 后台运行

使用 nohup（简单方式）

```bash
nohup ./myapp > myapp.log 2>&1 &
echo $! > myapp.pid
```

使用 screen（推荐）

```bash
# 创建screen会话
screen -S argo

# 运行应用
./myapp

# 按 Ctrl+A 然后按 D 分离会话
# 重新连接：screen -r argo
```

使用 tmux

```bash
# 创建tmux会话
tmux new-session -d -s argo

# 运行应用
tmux send-keys -t argo "./myapp" Enter

# 分离会话：tmux detach -s argo
# 重新连接：tmux attach -t argo
```

使用 PM2

```bash
# 安装PM2
npm install -g pm2

# 启动应用
pm2 start ./myapp --name "argo-service"

# 管理应用
pm2 status
pm2 logs argo-service
pm2 restart argo-service
pm2 stop argo-service
```

使用 systemd（Linux系统服务）

创建服务文件 /etc/systemd/system/go-argo.service：

```ini
[Unit]
Description=Go Argo Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/root/go-argo
Environment="UUID=9afd1229-b893-40c1-84dd-51e7ce204913"
Environment="SERVER_PORT=7860"
Environment="ARGO_PORT=8001"
ExecStart=/root/go-argo/myapp
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl start go-argo
sudo systemctl enable go-argo
sudo systemctl status go-argo
```

🔄 更新

```bash
# 停止当前服务
kill $(cat myapp.pid)

# 下载最新版本
wget -O myapp https://github.com/goyo123321a/go-argo/releases/download/latest/myapp-linux-$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')

# 赋予执行权限
chmod +x myapp

# 重新启动
nohup ./myapp > myapp.log 2>&1 &
echo $! > myapp.pid
```

📊 服务端点

端点 功能 说明
/sub 查看订阅 返回base64编码的节点信息，浏览器直接显示
/sub/download 下载订阅 下载sub.txt文件
/sub/raw 原始节点 查看未编码的节点配置
/status 服务状态 JSON格式返回运行状态
/health 健康检查 返回"OK"
/version 版本信息 返回版本和构建信息

## 主要修改说明

| 修改项 | 说明 |
|--------|------|
| **项目名称** | nodejs-argo → go-argo |
| **环境变量** | PORT → SERVER_PORT，添加 SUB_PATH、ARGO_PORT 等 |
| **部署方式** | npm全局安装 → 二进制文件下载 |
| **架构检测** | 自动检测 amd64/arm64 |
| **订阅端点** | 添加 /sub/download、/sub/raw、/status、/health |
| **后台运行** | 添加 nohup、screen、tmux、PM2、systemd 等多种方式 |
| **Docker部署** | 添加 Docker 和 Docker Compose 部署说明 |
| **一键脚本** | 添加 curl 一键部署命令 |

这个 README 完整适配了 Go 项目的特点，包括二进制部署、多架构支持、Docker 容器化等特性。

📚 更多信息

· GitHub仓库
· 问题反馈
· Releases下载
