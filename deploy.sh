#!/bin/bash
# 一键部署 myapp - 纯 Argo 隧道模式
# 支持 Linux (Xray) 和 FreeBSD (Sing-box)
# 版本: v5.3 - 添加修改配置菜单

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
print_error() { echo -e "${RED}[ERROR]${NC} $1"; }
print_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
print_question() { echo -e "${BLUE}[?]${NC} $1"; }

# 固定工作目录
WORKDIR="$HOME/myapp"
VERSION="v1.0.0.12"

# 检测系统架构
detect_arch() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        armv7l|armhf) echo "arm64" ;;
        *) print_error "不支持的架构: $ARCH"; exit 1 ;;
    esac
}

# 检测操作系统
detect_os() {
    OS=$(uname -s)
    case $OS in
        Linux) echo "linux" ;;
        FreeBSD) echo "freebsd" ;;
        Darwin) echo "darwin" ;;
        *) print_error "不支持的操作系统: $OS"; exit 1 ;;
    esac
}

# 检查依赖
check_dependencies() {
    local os=$1
    if ! command -v curl &> /dev/null && ! command -v wget &> /dev/null; then
        if [ "$os" = "freebsd" ]; then
            pkg install -y curl || { print_error "安装 curl 失败"; exit 1; }
        else
            print_error "请安装 curl 或 wget"
            exit 1
        fi
    fi
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
        print_error "未找到 curl 或 wget"
        exit 1
    fi
    if [ $? -eq 0 ] && [ -f "$output" ]; then
        print_info "下载成功: $output"
        chmod +x "$output"
    else
        print_error "下载失败: $url"
        exit 1
    fi
}

# 生成 UUID
generate_uuid() {
    if command -v uuidgen &> /dev/null; then
        uuidgen
    elif [ -f /proc/sys/kernel/random/uuid ]; then
        cat /proc/sys/kernel/random/uuid
    else
        echo "9afd1229-b893-40c1-84dd-51e7ce204913"
    fi
}

# 交互式配置（根据系统调整 ARGO_PORT 处理）
configure_env() {
    local config_file=$1
    local mode=$2
    local os=$(detect_os)
    
    print_info "开始配置环境变量（纯 Argo 隧道模式）..."
    echo ""
    
    # UUID
    read -p "请输入 UUID (留空将自动生成): " input_uuid
    if [ -z "$input_uuid" ]; then
        UUID=$(generate_uuid)
        print_info "自动生成 UUID: $UUID"
    else
        UUID="$input_uuid"
    fi
    
    # HTTP 服务端口
    read -p "请输入 HTTP 服务端口 (留空使用默认 7860): " input_port
    SERVER_PORT="${input_port:-7860}"
    
    # 订阅路径
    read -p "请输入订阅路径 (留空使用默认 sub): " input_sub_path
    SUB_PATH="${input_sub_path:-sub}"
    
    # Cloudflare 优选配置
    echo ""
    read -p "请输入优选域名/IP (留空使用默认 cf.877774.xyz): " input_cfip
    CFIP="${input_cfip:-cf.877774.xyz}"
    
    read -p "请输入端口 (留空使用默认 443): " input_cfport
    CFPORT="${input_cfport:-443}"
    
    # 节点名称
    echo ""
    read -p "请输入节点名称前缀 (留空使用自动获取): " input_name
    NAME="$input_name"
    
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
            if [[ "$NEZHA_SERVER" == *":"* ]]; then
                NEZHA_PORT=""
            else
                read -p "请输入哪吒端口 (留空使用默认): " input_nezha_port
                NEZHA_PORT="${input_nezha_port:-5555}"
            fi
        else
            print_warn "跳过哪吒监控"
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
        if [ -z "$UPLOAD_URL" ] || [ -z "$PROJECT_URL" ]; then
            print_warn "跳过自动上传"
            use_upload="n"
        fi
    fi
    
    # 自动保活配置
    echo ""
    print_question "是否开启自动保活? (y/n, 默认 n): "
    read use_auto_access
    if [[ "$use_auto_access" =~ ^[Yy]$ ]] && [ -n "$PROJECT_URL" ]; then
        AUTO_ACCESS="true"
    else
        AUTO_ACCESS="false"
    fi
    
    # 写入配置文件
    cat > "$config_file" << EOF
# myapp 配置文件 - 纯 Argo 隧道模式
UUID=$UUID
CFIP=$CFIP
CFPORT=$CFPORT
NAME=$NAME
SERVER_PORT=$SERVER_PORT
SUB_PATH=$SUB_PATH
FILE_PATH=./tmp
AUTO_ACCESS=$AUTO_ACCESS
EOF
    
    # 仅在 Linux 下询问并写入 ARGO_PORT
    if [ "$os" = "linux" ]; then
        read -p "请输入 Argo 本地端口 (留空使用默认 8001): " input_argo_port
        ARGO_PORT="${input_argo_port:-8001}"
        echo "ARGO_PORT=$ARGO_PORT" >> "$config_file"
    else
        ARGO_PORT=""
    fi
    
    if [[ "$use_fixed_tunnel" =~ ^[Yy]$ ]]; then
        cat >> "$config_file" << EOF
ARGO_DOMAIN=$ARGO_DOMAIN
ARGO_AUTH=$ARGO_AUTH
EOF
    fi
    
    if [[ "$use_nezha" =~ ^[Yy]$ ]]; then
        cat >> "$config_file" << EOF
NEZHA_SERVER=$NEZHA_SERVER
NEZHA_PORT=$NEZHA_PORT
NEZHA_KEY=$NEZHA_KEY
EOF
    fi
    
    if [[ "$use_upload" =~ ^[Yy]$ ]]; then
        cat >> "$config_file" << EOF
UPLOAD_URL=$UPLOAD_URL
PROJECT_URL=$PROJECT_URL
EOF
    fi
    
    print_info "配置完成"
}

# 创建 systemd 服务 (Linux)
create_systemd_service() {
    local bin_path=$1
    local config_file=$2
    local service_file="/etc/systemd/system/myapp.service"
    local current_user=$(whoami)
    
    # 检查是否有权限写入 /etc/systemd/system
    if [ ! -w "/etc/systemd/system" ] && ! command -v sudo &> /dev/null; then
        print_warn "无权限创建 systemd 服务，跳过"
        return 1
    fi
    
    cat << EOF | sudo tee "$service_file" > /dev/null
[Unit]
Description=myapp Argo Tunnel Service
After=network.target

[Service]
Type=simple
User=$current_user
WorkingDirectory=$WORKDIR
EnvironmentFile=$config_file
ExecStart=$bin_path
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF
    print_info "systemd 服务已创建"
    return 0
}

# 创建 FreeBSD rc.d 服务
create_rc_service() {
    local bin_path=$1
    local config_file=$2
    local rc_file="/usr/local/etc/rc.d/myapp"
    local current_user=$(whoami)
    
    # 检查是否有权限写入 /usr/local/etc/rc.d
    if [ ! -w "/usr/local/etc/rc.d" ] && ! command -v sudo &> /dev/null; then
        print_warn "无权限创建 rc.d 服务，跳过"
        return 1
    fi
    
    cat << EOF | sudo tee "$rc_file" > /dev/null
#!/bin/sh
#
# PROVIDE: myapp
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="myapp"
rcvar="myapp_enable"

load_rc_config \$name

: \${myapp_enable:=NO}
: \${myapp_user:=$current_user}
: \${myapp_dir:=$WORKDIR}

pidfile="/var/run/myapp.pid"
command="/usr/sbin/daemon"
command_args="-p \${pidfile} -u \${myapp_user} -f ${bin_path}"

start_precmd="export_env"
stop_postcmd="cleanup_pid"

export_env() {
    if [ -f "${config_file}" ]; then
        while IFS='=' read -r key value; do
            case "\$key" in
                ''|\#*) continue ;;
                *) export "\$key=\$value" ;;
            esac
        done < "${config_file}"
    fi
}

cleanup_pid() {
    rm -f "\${pidfile}"
}

run_rc_command "\$1"
EOF
    sudo chmod +x "$rc_file"
    print_info "rc.d 服务已创建"
    return 0
}

# 停止现有服务
stop_existing() {
    if [ -f "$WORKDIR/myapp.pid" ]; then
        OLD_PID=$(cat "$WORKDIR/myapp.pid")
        if ps -p $OLD_PID > /dev/null 2>&1; then
            print_info "停止现有进程: $OLD_PID"
            kill $OLD_PID 2>/dev/null || true
            sleep 2
        fi
        rm -f "$WORKDIR/myapp.pid"
    fi
    pkill -f "$WORKDIR/myapp" 2>/dev/null || true
    if [ -d "$WORKDIR/tmp" ]; then
        rm -rf "$WORKDIR/tmp"/*
    fi
}

# 启动服务
start_myapp() {
    local bin_path=$1
    local config_file=$2
    
    if [ ! -f "$config_file" ]; then
        print_error "配置文件不存在"
        return 1
    fi
    
    set -a
    source "$config_file"
    set +a
    
    if [ -z "$UUID" ]; then
        print_error "UUID 未配置"
        return 1
    fi
    
    SERVER_PORT=${SERVER_PORT:-7860}
    SUB_PATH=${SUB_PATH:-sub}
    CFIP=${CFIP:-cf.877774.xyz}
    CFPORT=${CFPORT:-443}
    
    mkdir -p ./tmp
    
    local env_vars=""
    env_vars="$env_vars UUID=$UUID"
    env_vars="$env_vars CFIP=$CFIP"
    env_vars="$env_vars CFPORT=$CFPORT"
    env_vars="$env_vars NAME=$NAME"
    env_vars="$env_vars SERVER_PORT=$SERVER_PORT"
    env_vars="$env_vars SUB_PATH=$SUB_PATH"
    env_vars="$env_vars FILE_PATH=./tmp"
    env_vars="$env_vars AUTO_ACCESS=$AUTO_ACCESS"
    
    # 仅在 Linux 且 ARGO_PORT 非空时传递
    if [ -n "$ARGO_PORT" ]; then
        env_vars="$env_vars ARGO_PORT=$ARGO_PORT"
    fi
    
    [ -n "$ARGO_DOMAIN" ] && env_vars="$env_vars ARGO_DOMAIN=$ARGO_DOMAIN"
    [ -n "$ARGO_AUTH" ] && env_vars="$env_vars ARGO_AUTH=$ARGO_AUTH"
    [ -n "$NEZHA_SERVER" ] && env_vars="$env_vars NEZHA_SERVER=$NEZHA_SERVER"
    [ -n "$NEZHA_PORT" ] && env_vars="$env_vars NEZHA_PORT=$NEZHA_PORT"
    [ -n "$NEZHA_KEY" ] && env_vars="$env_vars NEZHA_KEY=$NEZHA_KEY"
    [ -n "$UPLOAD_URL" ] && env_vars="$env_vars UPLOAD_URL=$UPLOAD_URL"
    [ -n "$PROJECT_URL" ] && env_vars="$env_vars PROJECT_URL=$PROJECT_URL"
    
    eval "env $env_vars nohup $bin_path > ./myapp.log 2>&1 &"
    echo $! > ./myapp.pid
    
    sleep 5
    
    if ps -p $(cat ./myapp.pid) > /dev/null 2>&1; then
        return 0
    else
        print_error "启动失败"
        tail -20 ./myapp.log
        return 1
    fi
}

# 显示订阅链接
show_subscription() {
    local server_port=$1
    local sub_path=$2
    
    # 尝试从日志中提取 IP
    if [ -f "$WORKDIR/myapp.log" ]; then
        # 从日志中提取服务器 IP
        local ips=$(grep -oE '服务器 IP 地址: [0-9.]+(,[0-9.]+)*' "$WORKDIR/myapp.log" | head -1 | sed 's/服务器 IP 地址: //')
        if [ -n "$ips" ]; then
            print_info "服务器 IP 地址: $ips"
            IFS=',' read -ra IP_LIST <<< "$ips"
            for ip in "${IP_LIST[@]}"; do
                echo "  本地订阅: http://${ip}:$server_port/$sub_path"
            done
        else
            echo "  本地订阅: http://127.0.0.1:$server_port/$sub_path"
        fi
    else
        echo "  本地订阅: http://127.0.0.1:$server_port/$sub_path"
    fi
    echo "  下载订阅: http://127.0.0.1:$server_port/$sub_path/download"
    echo "  状态查看: http://127.0.0.1:$server_port/status"
    echo ""
    
    # 如果配置了汇聚订阅
    if [ -f "$WORKDIR/.env" ]; then
        source "$WORKDIR/.env"
        if [ -n "$PROJECT_URL" ]; then
            echo "汇聚订阅: ${PROJECT_URL}/${SUB_PATH:-sub}"
            echo ""
        fi
    fi
}

# 修改配置
reconfigure() {
    if [ ! -d "$WORKDIR" ]; then
        print_error "未安装 myapp，请先安装"
        menu
        return
    fi
    cd "$WORKDIR" || exit
    if [ ! -f ".env" ]; then
        print_error "配置文件不存在，请先安装"
        menu
        return
    fi
    print_info "开始重新配置..."
    # 获取当前操作系统，用于显示模式
    local os=$(detect_os)
    local mode=""
    if [ "$os" = "freebsd" ]; then
        mode="sing-box"
    else
        mode="xray"
    fi
    configure_env ".env" "$mode"
    # 重启服务
    if [ -f "restart.sh" ]; then
        ./restart.sh
    else
        stop_existing
        source .env
        export UUID CFIP CFPORT NAME SERVER_PORT SUB_PATH
        [ -n "$ARGO_PORT" ] && export ARGO_PORT
        [ -n "$ARGO_DOMAIN" ] && export ARGO_DOMAIN
        [ -n "$ARGO_AUTH" ] && export ARGO_AUTH
        [ -n "$NEZHA_SERVER" ] && export NEZHA_SERVER
        [ -n "$NEZHA_PORT" ] && export NEZHA_PORT
        [ -n "$NEZHA_KEY" ] && export NEZHA_KEY
        [ -n "$UPLOAD_URL" ] && export UPLOAD_URL
        [ -n "$PROJECT_URL" ] && export PROJECT_URL
        nohup ./myapp > myapp.log 2>&1 &
        echo $! > myapp.pid
    fi
    print_info "配置已更新并重启服务"
    sleep 2
    menu
}

# 安装主流程
install() {
    echo ""
    print_info "检测系统环境..."
    OS=$(detect_os)
    ARCH=$(detect_arch)
    print_info "系统: $OS | 架构: $ARCH"
    echo ""
    
    # FreeBSD 必须存在 devil 命令
    if [ "$OS" = "freebsd" ]; then
        if ! command -v devil &> /dev/null; then
            print_error "未检测到 devil 命令，无法在 FreeBSD 上自动分配端口。请确认你在 Serv00/ct8 环境中运行。"
            exit 1
        fi
    fi
    
    check_dependencies "$OS"
    
    mkdir -p "$WORKDIR"
    cd "$WORKDIR" || exit 1
    
    BIN_PATH="$WORKDIR/myapp"
    CONFIG_FILE="$WORKDIR/.env"
    RESTART_SCRIPT="$WORKDIR/restart.sh"
    
    stop_existing
    
    # 下载二进制文件
    if [ "$OS" = "freebsd" ]; then
        MODE="sing-box"
        DOWNLOAD_URL="https://github.com/goyo123321a/go-argo/releases/download/${VERSION}/myapp-freebsd-${ARCH}"
    elif [ "$OS" = "linux" ]; then
        MODE="xray"
        DOWNLOAD_URL="https://github.com/goyo123321a/go-argo/releases/download/${VERSION}/myapp-linux-${ARCH}"
    else
        print_error "不支持的操作系统: $OS"
        exit 1
    fi
    
    download_file "$DOWNLOAD_URL" "$BIN_PATH"
    chmod +x "$BIN_PATH"
    
    if [ -f "$CONFIG_FILE" ]; then
        print_question "检测到已有配置，是否重新配置? (y/n, 默认 n): "
        read reconfigure
        if [[ "$reconfigure" =~ ^[Yy]$ ]]; then
            configure_env "$CONFIG_FILE" "$MODE"
        fi
    else
        configure_env "$CONFIG_FILE" "$MODE"
    fi
    
    # 创建重启脚本
    cat > "$RESTART_SCRIPT" << 'EOF'
#!/bin/bash
cd ~/myapp
kill $(cat myapp.pid) 2>/dev/null
sleep 2
source .env
export UUID CFIP CFPORT NAME SERVER_PORT SUB_PATH
[ -n "$ARGO_PORT" ] && export ARGO_PORT
[ -n "$ARGO_DOMAIN" ] && export ARGO_DOMAIN
[ -n "$ARGO_AUTH" ] && export ARGO_AUTH
[ -n "$NEZHA_SERVER" ] && export NEZHA_SERVER
[ -n "$NEZHA_PORT" ] && export NEZHA_PORT
[ -n "$NEZHA_KEY" ] && export NEZHA_KEY
[ -n "$UPLOAD_URL" ] && export UPLOAD_URL
[ -n "$PROJECT_URL" ] && export PROJECT_URL
nohup ./myapp > myapp.log 2>&1 &
echo $! > myapp.pid
echo "myapp 已重启，PID: $(cat myapp.pid)"
EOF
    chmod +x "$RESTART_SCRIPT"
    
    if start_myapp "$BIN_PATH" "$CONFIG_FILE"; then
        source "$CONFIG_FILE"
        show_subscription "$SERVER_PORT" "$SUB_PATH"
        
        # 显示启动日志
        echo ""
        print_info "启动日志（最后 20 行）："
        tail -20 "$WORKDIR/myapp.log"
        echo ""
        print_info "实时查看日志：tail -f $WORKDIR/myapp.log"
        
        print_info "管理命令:"
        print_info "  查看日志: tail -f $WORKDIR/myapp.log"
        print_info "  重启服务: $RESTART_SCRIPT"
        print_info "  停止服务: kill \$(cat $WORKDIR/myapp.pid)"
        echo ""
        
        print_question "是否安装为系统服务? (y/n, 默认 n): "
        read install_service
        if [[ "$install_service" =~ ^[Yy]$ ]]; then
            if [ "$OS" = "freebsd" ]; then
                create_rc_service "$BIN_PATH" "$CONFIG_FILE"
                if [ $? -eq 0 ]; then
                    echo "启用: sudo sysrc myapp_enable=YES && sudo service myapp start"
                fi
            else
                create_systemd_service "$BIN_PATH" "$CONFIG_FILE"
                if [ $? -eq 0 ]; then
                    echo "启用: sudo systemctl enable myapp && sudo systemctl start myapp"
                fi
            fi
        fi
        
        print_info "安装完成！"
    else
        exit 1
    fi
}

# 卸载
uninstall() {
    read -p "确定要卸载吗? [y/N]: " confirm
    if [[ "$confirm" =~ ^[Yy]$ ]]; then
        OS=$(detect_os)
        
        # 清理系统服务（如果已安装）
        if [ "$OS" = "freebsd" ]; then
            if [ -f "/usr/local/etc/rc.d/myapp" ]; then
                print_info "正在停止并删除 FreeBSD rc.d 服务..."
                # 尝试停止服务（可能无权限，忽略错误）
                sudo service myapp stop 2>/dev/null || true
                sudo sysrc -x myapp_enable 2>/dev/null || true
                sudo rm -f /usr/local/etc/rc.d/myapp
                print_info "已删除 rc.d 服务文件"
            fi
        else
            if [ -f "/etc/systemd/system/myapp.service" ]; then
                print_info "正在停止并删除 systemd 服务..."
                sudo systemctl stop myapp 2>/dev/null || true
                sudo systemctl disable myapp 2>/dev/null || true
                sudo rm -f /etc/systemd/system/myapp.service
                sudo systemctl daemon-reload
                print_info "已删除 systemd 服务文件"
            fi
        fi
        
        stop_existing
        rm -rf "$WORKDIR"
        print_info "卸载完成"
    fi
    menu
}

# 重启
restart() {
    if [ ! -d "$WORKDIR" ]; then
        print_error "未安装"
        menu
        return
    fi
    
    cd "$WORKDIR" || exit
    if [ -f "restart.sh" ]; then
        ./restart.sh
    else
        stop_existing
        source .env
        export UUID CFIP CFPORT NAME SERVER_PORT SUB_PATH
        [ -n "$ARGO_PORT" ] && export ARGO_PORT
        [ -n "$ARGO_DOMAIN" ] && export ARGO_DOMAIN
        [ -n "$ARGO_AUTH" ] && export ARGO_AUTH
        [ -n "$NEZHA_SERVER" ] && export NEZHA_SERVER
        [ -n "$NEZHA_PORT" ] && export NEZHA_PORT
        [ -n "$NEZHA_KEY" ] && export NEZHA_KEY
        [ -n "$UPLOAD_URL" ] && export UPLOAD_URL
        [ -n "$PROJECT_URL" ] && export PROJECT_URL
        nohup ./myapp > myapp.log 2>&1 &
        echo $! > myapp.pid
    fi
    
    sleep 3
    source .env
    show_subscription "$SERVER_PORT" "$SUB_PATH"
    menu
}

# 查看状态
status() {
    if [ ! -d "$WORKDIR" ]; then
        print_error "未安装"
        menu
        return
    fi
    
    cd "$WORKDIR" || exit
    
    if [ -f "myapp.pid" ]; then
        PID=$(cat "myapp.pid")
        if ps -p $PID > /dev/null 2>&1; then
            print_info "运行中 - PID: $PID"
            echo ""
            tail -15 myapp.log 2>/dev/null
        else
            print_warn "未运行"
        fi
    else
        print_warn "未运行"
    fi
    
    read -p "按回车返回"
    menu
}

# 查看节点
show_nodes() {
    if [ ! -f "$WORKDIR/tmp/sub.txt" ]; then
        print_error "未找到节点信息"
        menu
        return
    fi
    
    echo ""
    print_info "节点列表"
    print_info "=========================================="
    echo ""
    
    if command -v base64 &> /dev/null; then
        cat "$WORKDIR/tmp/sub.txt" | base64 -d 2>/dev/null
    else
        cat "$WORKDIR/tmp/sub.txt"
    fi
    
    echo ""
    read -p "按回车返回"
    menu
}

# 重置系统
reset_system() {
    print_warn "仅适用于 Serv00/ct8"
    read -p "确认重置? [y/N]: " confirm
    if [[ "$confirm" =~ ^[Yy]$ ]]; then
        stop_existing
        find "$HOME" -mindepth 1 ! -name "domains" ! -name "mail" ! -name "repo" ! -name "backups" -exec rm -rf {} + 2>/dev/null || true
        
        if command -v devil &> /dev/null; then
            devil www list | awk 'NF>=2 && $1 ~ /\./ {print $1}' | while read -r domain; do
                devil www del "$domain" 2>/dev/null || true
            done
            rm -rf "$HOME/domains"/* 2>/dev/null || true
        fi
        
        print_info "重置完成"
    fi
    menu
}

# 主菜单
menu() {
    clear
    echo "=========================================="
    echo "   myapp 一键部署 v5.3"
    echo "=========================================="
    echo "  模式: 纯 Argo 隧道 (无 TLS 证书)"
    echo "  支持: Linux / FreeBSD"
    echo "=========================================="
    echo "1. 安装/更新"
    echo "2. 卸载"
    echo "3. 重启"
    echo "4. 查看状态"
    echo "5. 查看节点"
    echo "6. 重置系统"
    echo "7. 修改配置"
    echo "0. 退出"
    echo "=========================================="
    read -p "请选择 [0-7]: " choice
    case $choice in
        1) install ;;
        2) uninstall ;;
        3) restart ;;
        4) status ;;
        5) show_nodes ;;
        6) reset_system ;;
        7) reconfigure ;;
        0) exit 0 ;;
        *) print_error "无效选择"; sleep 2; menu ;;
    esac
}

# 启动菜单
menu
