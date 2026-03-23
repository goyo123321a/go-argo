#!/bin/bash

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的信息
print_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_question() {
    echo -e "${BLUE}[?]${NC} $1"
}

# 生成随机6位数文件名
generate_random_name() {
    cat /dev/urandom | tr -dc 'a-z' | fold -w 6 | head -n 1
}

# 检测系统架构
detect_arch() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64|amd64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        armv7l|armhf)
            echo "arm64"
            ;;
        *)
            print_error "不支持的架构: $ARCH"
            exit 1
            ;;
    esac
}

# 检测操作系统
detect_os() {
    OS=$(uname -s)
    case $OS in
        Linux)
            echo "linux"
            ;;
        Darwin)
            echo "darwin"
            ;;
        *)
            print_error "不支持的操作系统: $OS"
            exit 1
            ;;
    esac
}

# 下载文件
download_file() {
    local url=$1
    local output=$2
    
    print_info "正在下载: $url"
    
    if command -v curl &> /dev/null; then
        curl -L -o "$output" "$url" --progress-bar
    elif command -v wget &> /dev/null; then
        wget -O "$output" "$url" -q --show-progress
    else
        print_error "未找到 curl 或 wget，请安装其中之一"
        exit 1
    fi
    
    if [ $? -eq 0 ] && [ -f "$output" ]; then
        print_info "下载成功: $output"
        return 0
    else
        print_error "下载失败: $url"
        return 1
    fi
}

# 交互式配置
configure_env() {
    local config_file=$1
    
    print_info "开始配置环境变量..."
    echo ""
    
    # UUID 配置
    read -p "请输入 UUID (留空使用默认值): " input_uuid
    if [ -z "$input_uuid" ]; then
        UUID="9afd1229-b893-40c1-84dd-51e7ce204913"
        print_info "使用默认 UUID: $UUID"
    else
        UUID="$input_uuid"
        print_info "使用自定义 UUID: $UUID"
    fi
    
    # CFIP 和 CFPORT 配置
    echo ""
    read -p "请输入优选域名/IP (留空使用默认 saas.sin.fan): " input_cfip
    if [ -z "$input_cfip" ]; then
        CFIP="saas.sin.fan"
        print_info "使用默认 CFIP: $CFIP"
    else
        CFIP="$input_cfip"
        print_info "使用自定义 CFIP: $CFIP"
    fi
    
    read -p "请输入端口 (留空使用默认 443): " input_cfport
    if [ -z "$input_cfport" ]; then
        CFPORT="443"
        print_info "使用默认 CFPORT: $CFPORT"
    else
        CFPORT="$input_cfport"
        print_info "使用自定义 CFPORT: $CFPORT"
    fi
    
    # 节点名称
    echo ""
    read -p "请输入节点名称 (留空使用自动获取): " input_name
    if [ -n "$input_name" ]; then
        NAME="$input_name"
        print_info "使用节点名称: $NAME"
    fi
    
    # Argo Tunnel 配置
    echo ""
    print_question "是否使用固定隧道? (y/n, 默认 n): "
    read use_fixed_tunnel
    
    if [[ "$use_fixed_tunnel" =~ ^[Yy]$ ]]; then
        read -p "请输入 Argo 域名: " ARGO_DOMAIN
        read -p "请输入 Argo Token 或 Tunnel JSON: " ARGO_AUTH
        
        if [ -z "$ARGO_DOMAIN" ] || [ -z "$ARGO_AUTH" ]; then
            print_warn "域名或Token为空，将使用临时隧道"
            use_fixed_tunnel="n"
        else
            print_info "已配置固定隧道"
        fi
    fi
    
    # 哪吒监控配置
    echo ""
    print_question "是否使用哪吒监控? (y/n, 默认 n): "
    read use_nezha
    
    if [[ "$use_nezha" =~ ^[Yy]$ ]]; then
        read -p "请输入哪吒服务器地址 (格式: nz.abc.com:8008): " NEZHA_SERVER
        read -p "请输入哪吒密钥: " NEZHA_KEY
        
        if [ -n "$NEZHA_SERVER" ] && [ -n "$NEZHA_KEY" ]; then
            # 检测是否包含端口，判断版本
            if [[ "$NEZHA_SERVER" == *":"* ]]; then
                NEZHA_PORT=""
                print_info "使用哪吒 v1 版本"
            else
                read -p "请输入哪吒端口 (留空使用默认): " input_nezha_port
                NEZHA_PORT="${input_nezha_port:-5555}"
                print_info "使用哪吒 v0 版本，端口: $NEZHA_PORT"
            fi
            print_info "已配置哪吒监控"
        else
            print_warn "服务器地址或密钥为空，跳过哪吒监控"
            use_nezha="n"
        fi
    fi
    
    # 自动上传配置
    echo ""
    print_question "是否自动上传订阅? (y/n, 默认 n): "
    read use_upload
    
    if [[ "$use_upload" =~ ^[Yy]$ ]]; then
        read -p "请输入 Merge-sub 项目地址: " UPLOAD_URL
        read -p "请输入项目分配的 URL: " PROJECT_URL
        
        if [ -n "$UPLOAD_URL" ] && [ -n "$PROJECT_URL" ]; then
            print_info "已配置自动上传"
        else
            print_warn "地址为空，跳过自动上传"
            use_upload="n"
        fi
    fi
    
    # 自动保活配置
    echo ""
    print_question "是否开启自动保活? (y/n, 默认 n): "
    read use_auto_access
    
    if [[ "$use_auto_access" =~ ^[Yy]$ ]]; then
        if [ -n "$PROJECT_URL" ]; then
            AUTO_ACCESS="true"
            print_info "已开启自动保活"
        else
            print_warn "未配置 PROJECT_URL，无法开启自动保活"
            AUTO_ACCESS="false"
        fi
    else
        AUTO_ACCESS="false"
    fi
    
    # 写入配置文件
    cat > "$config_file" << EOF
# myapp 配置文件
UUID=$UUID
CFIP=$CFIP
CFPORT=$CFPORT
NAME=$NAME
SERVER_PORT=7860
ARGO_PORT=8001
FILE_PATH=./tmp
SUB_PATH=sub
AUTO_ACCESS=$AUTO_ACCESS
EOF
    
    # 添加 Argo 配置
    if [[ "$use_fixed_tunnel" =~ ^[Yy]$ ]]; then
        cat >> "$config_file" << EOF
ARGO_DOMAIN=$ARGO_DOMAIN
ARGO_AUTH=$ARGO_AUTH
EOF
    fi
    
    # 添加哪吒配置
    if [[ "$use_nezha" =~ ^[Yy]$ ]]; then
        cat >> "$config_file" << EOF
NEZHA_SERVER=$NEZHA_SERVER
NEZHA_PORT=$NEZHA_PORT
NEZHA_KEY=$NEZHA_KEY
EOF
    fi
    
    # 添加上传配置
    if [[ "$use_upload" =~ ^[Yy]$ ]]; then
        cat >> "$config_file" << EOF
UPLOAD_URL=$UPLOAD_URL
PROJECT_URL=$PROJECT_URL
EOF
    fi
    
    print_info "配置文件已保存: $config_file"
}

# 创建 systemd 服务文件
create_systemd_service() {
    local bin_path=$1
    local config_file=$2
    local service_name=$3
    local service_file="/etc/systemd/system/${service_name}.service"
    
    # 获取当前用户
    local current_user=$(whoami)
    local current_dir=$(pwd)
    
    cat > "$service_file" << EOF
[Unit]
Description=myapp Service
After=network.target

[Service]
Type=simple
User=$current_user
WorkingDirectory=$current_dir
EnvironmentFile=$config_file
ExecStart=$bin_path
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
    
    print_info "systemd 服务文件已创建: $service_file"
}

# 主函数
main() {
    echo "=========================================="
    echo "        myapp 自动部署脚本 v1.0"
    echo "=========================================="
    echo ""
    
    # 检测环境
    print_info "检测系统环境..."
    OS=$(detect_os)
    ARCH=$(detect_arch)
    print_info "操作系统: $OS"
    print_info "系统架构: $ARCH"
    echo ""
    
    # 创建临时目录
    WORK_DIR="./myapp_$(date +%s)"
    mkdir -p "$WORK_DIR"
    cd "$WORK_DIR" || exit 1
    print_info "工作目录: $(pwd)"
    
    # 生成随机文件名
    BIN_NAME=$(generate_random_name)
    BIN_PATH="./$BIN_NAME"
    print_info "二进制文件名: $BIN_NAME"
    
    # 下载对应版本
    DOWNLOAD_URL="https://github.com/goyo123321a/go-argo/releases/download/v1.0.0.12/myapp-linux-${ARCH}"
    print_info "下载地址: $DOWNLOAD_URL"
    
    if ! download_file "$DOWNLOAD_URL" "$BIN_PATH"; then
        print_error "下载失败，请检查网络连接"
        exit 1
    fi
    
    # 赋予执行权限
    chmod +x "$BIN_PATH"
    print_info "已赋予执行权限"
    
    # 配置文件
    CONFIG_FILE="./.env"
    configure_env "$CONFIG_FILE"
    
    # 创建临时目录
    mkdir -p ./tmp
    
    # 启动程序
    print_info "正在启动 myapp..."
    
    # 使用 nohup 后台运行
    nohup "$BIN_PATH" > ./myapp.log 2>&1 &
    APP_PID=$!
    
    # 保存 PID
    echo $APP_PID > ./myapp.pid
    
    print_info "myapp 已启动，PID: $APP_PID"
    print_info "日志文件: $(pwd)/myapp.log"
    print_info "配置文件: $(pwd)/.env"
    
    # 等待启动
    sleep 3
    
    # 检查进程是否运行
    if ps -p $APP_PID > /dev/null 2>&1; then
        print_info "✓ myapp 运行正常"
        
        # 获取订阅地址
        echo ""
        print_info "=========================================="
        print_info "服务已启动，访问地址:"
        print_info "  订阅地址: http://localhost:7860/sub"
        print_info "  下载地址: http://localhost:7860/sub/download"
        print_info "  状态查看: http://localhost:7860/status"
        print_info "  健康检查: http://localhost:7860/health"
        echo ""
        
        # 询问是否安装为系统服务
        print_question "是否安装为系统服务 (systemd)? (y/n, 默认 n): "
        read install_service
        
        if [[ "$install_service" =~ ^[Yy]$ ]]; then
            SERVICE_NAME="myapp_${BIN_NAME}"
            create_systemd_service "$(pwd)/$BIN_NAME" "$(pwd)/.env" "$SERVICE_NAME"
            
            print_info "请执行以下命令启用服务:"
            echo "  sudo systemctl daemon-reload"
            echo "  sudo systemctl enable ${SERVICE_NAME}.service"
            echo "  sudo systemctl start ${SERVICE_NAME}.service"
        fi
        
        print_info "=========================================="
        print_info "管理命令:"
        print_info "  查看日志: tail -f $(pwd)/myapp.log"
        print_info "  停止服务: kill $APP_PID"
        print_info "  重启服务: kill $APP_PID && nohup $BIN_PATH > ./myapp.log 2>&1 &"
        print_info "=========================================="
        
    else
        print_error "myapp 启动失败，请检查日志"
        cat ./myapp.log
        exit 1
    fi
}

# 捕获退出信号
trap 'print_info "脚本被中断"; exit 1' INT TERM

# 运行主函数
main "$@"
