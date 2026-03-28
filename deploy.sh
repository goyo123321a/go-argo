#!/bin/bash
# 一键部署 myapp - 纯 Argo 隧道模式
# 支持 Linux (Xray) 和 FreeBSD (Sing-box)
# 版本: v5.6 - FreeBSD 支持 ARGO_PORT 配置

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
LOG_FILE="$WORKDIR/myapp.log"
MAX_LOG_SIZE=10485760  # 10MB

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

# 安全加载环境变量
load_env_file() {
    local config_file=$1
    if [ ! -f "$config_file" ]; then
        return 1
    fi
    
    # 逐行读取并导出变量
    while IFS='=' read -r key value || [ -n "$key" ]; do
        # 跳过注释和空行
        [[ "$key" =~ ^#.*$ ]] && continue
        [ -z "$key" ] && continue
        
        # 去除值中的引号和空格
        value=$(echo "$value" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
        value=$(echo "$value" | sed 's/^["\x27]//;s/["\x27]$//')
        
        # 导出变量
        export "$key=$value"
    done < "$config_file"
    
    return 0
}

# 安全写入配置
write_config_value() {
    local file=$1
    local key=$2
    local value=$3
    
    # 如果值包含特殊字符，用引号包裹
    if [[ "$value" =~ [^a-zA-Z0-9_\-\.] ]]; then
        echo "$key=\"$value\"" >> "$file"
    else
        echo "$key=$value" >> "$file"
    fi
}

# 日志轮转
rotate_log() {
    if [ -f "$LOG_FILE" ]; then
        local size=$(stat -c%s "$LOG_FILE" 2>/dev/null || stat -f%z "$LOG_FILE" 2>/dev/null)
        if [ "$size" -gt "$MAX_LOG_SIZE" ]; then
            mv "$LOG_FILE" "$LOG_FILE.old"
            print_info "日志已轮转"
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

# 交互式配置
configure_env() {
    local config_file=$1
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
    
    # Argo 端口（Linux 和 FreeBSD 都需要）
    read -p "请输入 Argo 本地端口 (留空使用默认 8001): " input_argo_port
    ARGO_PORT="${input_argo_port:-8001}"
    
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
# 生成时间: $(date)
EOF
    
    write_config_value "$config_file" "UUID" "$UUID"
    write_config_value "$config_file" "CFIP" "$CFIP"
    write_config_value "$config_file" "CFPORT" "$CFPORT"
    
    [ -n "$NAME" ] && write_config_value "$config_file" "NAME" "$NAME"
    write_config_value "$config_file" "SERVER_PORT" "$SERVER_PORT"
    write_config_value "$config_file" "SUB_PATH" "$SUB_PATH"
    write_config_value "$config_file" "FILE_PATH" "./tmp"
    write_config_value "$config_file" "AUTO_ACCESS" "$AUTO_ACCESS"
    
    # Linux 和 FreeBSD 都需要 ARGO_PORT
    [ -n "$ARGO_PORT" ] && write_config_value "$config_file" "ARGO_PORT" "$ARGO_PORT"
    
    if [[ "$use_fixed_tunnel" =~ ^[Yy]$ ]]; then
        write_config_value "$config_file" "ARGO_DOMAIN" "$ARGO_DOMAIN"
        write_config_value "$config_file" "ARGO_AUTH" "$ARGO_AUTH"
    fi
    
    if [[ "$use_nezha" =~ ^[Yy]$ ]]; then
        write_config_value "$config_file" "NEZHA_SERVER" "$NEZHA_SERVER"
        [ -n "$NEZHA_PORT" ] && write_config_value "$config_file" "NEZHA_PORT" "$NEZHA_PORT"
        write_config_value "$config_file" "NEZHA_KEY" "$NEZHA_KEY"
    fi
    
    if [[ "$use_upload" =~ ^[Yy]$ ]]; then
        write_config_value "$config_file" "UPLOAD_URL" "$UPLOAD_URL"
        write_config_value "$config_file" "PROJECT_URL" "$PROJECT_URL"
    fi
    
    print_info "配置已保存到: $config_file"
}

# 创建 systemd 服务 (Linux)
create_systemd_service() {
    local bin_path=$1
    local config_file=$2
    local service_file="/etc/systemd/system/myapp.service"
    local current_user=$(whoami)
    
    # 检查是否有权限写入
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
StandardOutput=append:$LOG_FILE
StandardError=append:$LOG_FILE

[Install]
WantedBy=multi-user.target
EOF
    print_info "systemd 服务已创建: $service_file"
    return 0
}

# 创建 FreeBSD rc.d 服务
create_rc_service() {
    local bin_path=$1
    local config_file=$2
    local rc_file="/usr/local/etc/rc.d/myapp"
    local current_user=$(whoami)
    
    # 检查是否有权限写入
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
    print_info "rc.d 服务已创建: $rc_file"
    return 0
}

# 停止现有服务
stop_existing() {
    print_info "停止现有服务..."
    
    # 通过 PID 文件停止
    if [ -f "$WORKDIR/myapp.pid" ]; then
        OLD_PID=$(cat "$WORKDIR/myapp.pid")
        if kill -0 $OLD_PID 2>/dev/null; then
            print_info "停止进程: $OLD_PID"
            kill $OLD_PID 2>/dev/null || true
            sleep 3
            # 强制终止如果还在运行
            if kill -0 $OLD_PID 2>/dev/null; then
                kill -9 $OLD_PID 2>/dev/null || true
            fi
        fi
        rm -f "$WORKDIR/myapp.pid"
    fi
    
    # 通过进程名停止
    pkill -f "$WORKDIR/myapp" 2>/dev/null || true
    
    # 清理临时文件
    if [ -d "$WORKDIR/tmp" ]; then
        rm -rf "$WORKDIR/tmp"/*
    fi
    
    print_info "服务已停止"
}

# 启动服务
start_myapp() {
    local bin_path=$1
    local config_file=$2
    
    if [ ! -f "$config_file" ]; then
        print_error "配置文件不存在: $config_file"
        return 1
    fi
    
    # 日志轮转
    rotate_log
    
    # 安全加载环境变量
    load_env_file "$config_file"
    
    # 检查必要变量
    if [ -z "$UUID" ]; then
        print_error "UUID 未配置"
        return 1
    fi
    
    # 设置默认值
    SERVER_PORT=${SERVER_PORT:-7860}
    SUB_PATH=${SUB_PATH:-sub}
    CFIP=${CFIP:-cf.877774.xyz}
    CFPORT=${CFPORT:-443}
    ARGO_PORT=${ARGO_PORT:-8001}
    FILE_PATH=${FILE_PATH:-./tmp}
    AUTO_ACCESS=${AUTO_ACCESS:-false}
    
    # 创建必要目录
    mkdir -p "$FILE_PATH"
    
    # 构建环境变量列表
    local env_vars=""
    env_vars="$env_vars UUID=$UUID"
    env_vars="$env_vars CFIP=$CFIP"
    env_vars="$env_vars CFPORT=$CFPORT"
    [ -n "$NAME" ] && env_vars="$env_vars NAME=$NAME"
    env_vars="$env_vars SERVER_PORT=$SERVER_PORT"
    env_vars="$env_vars SUB_PATH=$SUB_PATH"
    env_vars="$env_vars FILE_PATH=$FILE_PATH"
    env_vars="$env_vars AUTO_ACCESS=$AUTO_ACCESS"
    env_vars="$env_vars ARGO_PORT=$ARGO_PORT"  # Linux 和 FreeBSD 都需要
    
    # 可选变量
    [ -n "$ARGO_DOMAIN" ] && env_vars="$env_vars ARGO_DOMAIN=$ARGO_DOMAIN"
    [ -n "$ARGO_AUTH" ] && env_vars="$env_vars ARGO_AUTH=$ARGO_AUTH"
    [ -n "$NEZHA_SERVER" ] && env_vars="$env_vars NEZHA_SERVER=$NEZHA_SERVER"
    [ -n "$NEZHA_PORT" ] && env_vars="$env_vars NEZHA_PORT=$NEZHA_PORT"
    [ -n "$NEZHA_KEY" ] && env_vars="$env_vars NEZHA_KEY=$NEZHA_KEY"
    [ -n "$UPLOAD_URL" ] && env_vars="$env_vars UPLOAD_URL=$UPLOAD_URL"
    [ -n "$PROJECT_URL" ] && env_vars="$env_vars PROJECT_URL=$PROJECT_URL"
    
    # 启动服务
    print_info "启动 myapp (监听端口: $ARGO_PORT)..."
    eval "env $env_vars nohup $bin_path >> $LOG_FILE 2>&1 &"
    local new_pid=$!
    echo $new_pid > "$WORKDIR/myapp.pid"
    
    # 等待并验证启动
    sleep 5
    
    if kill -0 $new_pid 2>/dev/null; then
        print_info "✓ myapp 启动成功 (PID: $new_pid)"
        
        # 检查日志是否有错误
        if [ -f "$LOG_FILE" ] && grep -q "ERROR\|FATAL\|panic" "$LOG_FILE" 2>/dev/null; then
            print_warn "日志中有错误信息:"
            grep "ERROR\|FATAL\|panic" "$LOG_FILE" | tail -5
        fi
        
        return 0
    else
        print_error "启动失败，进程已退出"
        print_info "日志输出:"
        tail -20 "$LOG_FILE" 2>/dev/null || echo "无日志"
        return 1
    fi
}

# 健康检查
health_check() {
    local server_port=$1
    
    if curl -s -f -m 5 "http://127.0.0.1:$server_port/health" > /dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# 显示订阅链接
show_subscription() {
    local server_port=$1
    local sub_path=$2
    
    echo ""
    print_info "订阅信息:"
    
    # 健康检查
    if health_check "$server_port"; then
        print_info "健康检查: ✓ 正常"
    else
        print_warn "健康检查: ✗ 异常"
    fi
    
    echo ""
    echo "  本地订阅: http://127.0.0.1:$server_port/$sub_path"
    echo "  下载订阅: http://127.0.0.1:$server_port/$sub_path/download"
    echo "  原始节点: http://127.0.0.1:$server_port/$sub_path/raw"
    echo "  状态查看: http://127.0.0.1:$server_port/status"
    echo ""
    
    # 显示 Argo 端口信息
    if [ -f "$WORKDIR/.env" ]; then
        local argo_port=$(grep "^ARGO_PORT=" "$WORKDIR/.env" | cut -d'=' -f2 | sed 's/^["\x27]//;s/["\x27]$//')
        if [ -n "$argo_port" ]; then
            echo "  Argo 本地端口: $argo_port (Cloudflared 目标)"
            echo ""
        fi
    fi
    
    # 如果配置了汇聚订阅
    if [ -f "$WORKDIR/.env" ]; then
        local project_url=$(grep "^PROJECT_URL=" "$WORKDIR/.env" | cut -d'=' -f2 | sed 's/^["\x27]//;s/["\x27]$//')
        if [ -n "$project_url" ]; then
            echo "  汇聚订阅: ${project_url}/${sub_path}"
            echo ""
        fi
    fi
}

# 安装主流程
install() {
    echo ""
    print_info "检测系统环境..."
    OS=$(detect_os)
    ARCH=$(detect_arch)
    print_info "系统: $OS | 架构: $ARCH"
    echo ""
    
    check_dependencies "$OS"
    
    mkdir -p "$WORKDIR"
    cd "$WORKDIR" || exit 1
    
    BIN_PATH="$WORKDIR/myapp"
    CONFIG_FILE="$WORKDIR/.env"
    RESTART_SCRIPT="$WORKDIR/restart.sh"
    
    # 停止现有服务
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
    
    # 配置环境变量
    if [ -f "$CONFIG_FILE" ]; then
        print_question "检测到已有配置，是否重新配置? (y/n, 默认 n): "
        read reconfigure
        if [[ "$reconfigure" =~ ^[Yy]$ ]]; then
            # 备份旧配置
            backup_file="${CONFIG_FILE}.backup.$(date +%Y%m%d_%H%M%S)"
            cp "$CONFIG_FILE" "$backup_file"
            print_info "已备份旧配置: $backup_file"
            configure_env "$CONFIG_FILE"
        fi
    else
        configure_env "$CONFIG_FILE"
    fi
    
    # 创建重启脚本
    cat > "$RESTART_SCRIPT" << 'EOF'
#!/bin/bash
# myapp 重启脚本

WORKDIR="$HOME/myapp"
cd "$WORKDIR" || exit 1

# 停止服务
if [ -f myapp.pid ]; then
    PID=$(cat myapp.pid)
    if kill -0 $PID 2>/dev/null; then
        echo "停止进程: $PID"
        kill $PID
        sleep 3
        if kill -0 $PID 2>/dev/null; then
            kill -9 $PID
        fi
    fi
    rm -f myapp.pid
fi

# 清理临时文件
rm -rf tmp/*

# 加载环境变量
if [ -f .env ]; then
    while IFS='=' read -r key value || [ -n "$key" ]; do
        [[ "$key" =~ ^#.*$ ]] && continue
        [ -z "$key" ] && continue
        value=$(echo "$value" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')
        value=$(echo "$value" | sed 's/^["\x27]//;s/["\x27]$//')
        export "$key=$value"
    done < .env
fi

# 检查必要变量
if [ -z "$UUID" ]; then
    echo "错误: UUID 未配置"
    exit 1
fi

# 设置默认值
SERVER_PORT=${SERVER_PORT:-7860}
SUB_PATH=${SUB_PATH:-sub}
CFIP=${CFIP:-cf.877774.xyz}
CFPORT=${CFPORT:-443}
ARGO_PORT=${ARGO_PORT:-8001}
FILE_PATH=${FILE_PATH:-./tmp}
AUTO_ACCESS=${AUTO_ACCESS:-false}

# 创建必要目录
mkdir -p "$FILE_PATH"

# 构建环境变量
env_vars=""
env_vars="$env_vars UUID=$UUID"
env_vars="$env_vars CFIP=$CFIP"
env_vars="$env_vars CFPORT=$CFPORT"
[ -n "$NAME" ] && env_vars="$env_vars NAME=$NAME"
env_vars="$env_vars SERVER_PORT=$SERVER_PORT"
env_vars="$env_vars SUB_PATH=$SUB_PATH"
env_vars="$env_vars FILE_PATH=$FILE_PATH"
env_vars="$env_vars AUTO_ACCESS=$AUTO_ACCESS"
env_vars="$env_vars ARGO_PORT=$ARGO_PORT"

[ -n "$ARGO_DOMAIN" ] && env_vars="$env_vars ARGO_DOMAIN=$ARGO_DOMAIN"
[ -n "$ARGO_AUTH" ] && env_vars="$env_vars ARGO_AUTH=$ARGO_AUTH"
[ -n "$NEZHA_SERVER" ] && env_vars="$env_vars NEZHA_SERVER=$NEZHA_SERVER"
[ -n "$NEZHA_PORT" ] && env_vars="$env_vars NEZHA_PORT=$NEZHA_PORT"
[ -n "$NEZHA_KEY" ] && env_vars="$env_vars NEZHA_KEY=$NEZHA_KEY"
[ -n "$UPLOAD_URL" ] && env_vars="$env_vars UPLOAD_URL=$UPLOAD_URL"
[ -n "$PROJECT_URL" ] && env_vars="$env_vars PROJECT_URL=$PROJECT_URL"

# 启动服务
echo "启动 myapp (监听端口: $ARGO_PORT)..."
eval "env $env_vars nohup ./myapp >> myapp.log 2>&1 &"
NEW_PID=$!
echo $NEW_PID > myapp.pid

# 等待并检查
sleep 5
if kill -0 $NEW_PID 2>/dev/null; then
    echo "✓ myapp 已重启，PID: $NEW_PID"
    
    # 显示订阅地址
    echo ""
    echo "订阅地址: http://127.0.0.1:$SERVER_PORT/$SUB_PATH"
    echo "Argo 本地端口: $ARGO_PORT"
else
    echo "✗ 启动失败，查看日志:"
    tail -20 myapp.log
    exit 1
fi
EOF
    chmod +x "$RESTART_SCRIPT"
    
    # 创建状态查看脚本
    cat > "$WORKDIR/status.sh" << EOF
#!/bin/bash
cd "$WORKDIR"
if [ -f myapp.pid ]; then
    PID=\$(cat myapp.pid)
    if kill -0 \$PID 2>/dev/null; then
        echo "✓ myapp 运行中 (PID: \$PID)"
        echo ""
        echo "最近日志:"
        tail -15 myapp.log
    else
        echo "✗ myapp 未运行"
    fi
else
    echo "✗ myapp 未运行"
fi
EOF
    chmod +x "$WORKDIR/status.sh"
    
    # 启动服务
    if start_myapp "$BIN_PATH" "$CONFIG_FILE"; then
        source "$CONFIG_FILE"
        show_subscription "$SERVER_PORT" "$SUB_PATH"
        
        # 显示启动日志
        echo ""
        print_info "启动日志（最后 20 行）："
        tail -20 "$LOG_FILE" 2>/dev/null || echo "无日志"
        echo ""
        print_info "实时查看日志: tail -f $LOG_FILE"
        
        print_info "管理命令:"
        print_info "  重启服务: $RESTART_SCRIPT"
        print_info "  停止服务: kill \$(cat $WORKDIR/myapp.pid)"
        print_info "  查看状态: $WORKDIR/status.sh"
        echo ""
        
        # 询问是否安装系统服务
        print_question "是否安装为系统服务? (y/n, 默认 n): "
        read install_service
        if [[ "$install_service" =~ ^[Yy]$ ]]; then
            if [ "$OS" = "freebsd" ]; then
                create_rc_service "$BIN_PATH" "$CONFIG_FILE"
                echo ""
                print_info "启用服务命令:"
                echo "  sudo sysrc myapp_enable=YES"
                echo "  sudo service myapp start"
            else
                create_systemd_service "$BIN_PATH" "$CONFIG_FILE"
                echo ""
                print_info "启用服务命令:"
                echo "  sudo systemctl enable myapp"
                echo "  sudo systemctl start myapp"
            fi
        fi
    else
        print_error "安装失败"
        exit 1
    fi
}

# 主函数
main() {
    clear
    echo "=========================================="
    echo "        myapp 一键部署脚本 v5.6"
    echo "        纯 Argo 隧道模式"
    echo "=========================================="
    echo ""
    
    install
}

# 清理函数
cleanup() {
    print_info "脚本被中断"
    exit 1
}

trap cleanup INT TERM
main "$@"
