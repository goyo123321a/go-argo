#!/bin/bash
# 一键部署 myapp - 自动下载、配置并运行

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
VERSION="v1.0.0.13"

# 生成随机文件名
generate_random_name() {
    if command -v openssl &> /dev/null; then
        openssl rand -hex 3 2>/dev/null | tr -d '\n' || echo "myapp"
    elif [ -f /dev/urandom ]; then
        cat /dev/urandom 2>/dev/null | tr -dc 'a-z0-9' | fold -w 6 | head -n 1 || echo "myapp"
    else
        echo "myapp"
    fi
}

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

# 检查并安装依赖
check_dependencies() {
    local os=$1
    
    if [ "$os" = "freebsd" ]; then
        if ! command -v pkg &> /dev/null; then
            print_error "FreeBSD 系统未安装 pkg 包管理器"
            print_info "请先运行: pkg bootstrap"
            exit 1
        fi
        
        if ! command -v curl &> /dev/null && ! command -v wget &> /dev/null; then
            print_info "正在安装 curl..."
            pkg install -y curl || {
                print_error "安装 curl 失败"
                exit 1
            }
        fi
    elif [ "$os" = "linux" ]; then
        if ! command -v curl &> /dev/null && ! command -v wget &> /dev/null; then
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

# 交互式配置
configure_env() {
    local config_file=$1
    print_info "开始配置环境变量..."
    echo ""

    # HTTP 服务端口配置
    read -p "请输入 HTTP 服务端口 (留空使用默认 7860): " input_port
    SERVER_PORT="${input_port:-7860}"
    print_info "HTTP 服务端口: $SERVER_PORT"

    # 订阅路径配置
    read -p "请输入订阅路径 (留空使用默认 sub): " input_sub_path
    SUB_PATH="${input_sub_path:-sub}"
    print_info "订阅路径: $SUB_PATH"

    # Argo 端口配置
    read -p "请输入 Argo 隧道端口 (留空使用默认 8001): " input_argo_port
    ARGO_PORT="${input_argo_port:-8001}"
    print_info "Argo 隧道端口: $ARGO_PORT"

    # UUID 配置
    echo ""
    read -p "请输入 UUID (留空使用默认值): " input_uuid
    if [ -z "$input_uuid" ]; then
        UUID="9afd1229-b893-40c1-84dd-51e7ce204913"
        print_info "使用默认 UUID: $UUID"
    else
        UUID="$input_uuid"
    fi

    # CFIP 和 CFPORT 配置
    echo ""
    read -p "请输入优选域名/IP (留空使用默认 cf.877774.xyz): " input_cfip
    if [ -z "$input_cfip" ]; then
        CFIP="cf.877774.xyz"
    else
        CFIP="$input_cfip"
    fi

    read -p "请输入端口 (留空使用默认 443): " input_cfport
    CFPORT="${input_cfport:-443}"

    # 节点名称
    echo ""
    read -p "请输入节点名称 (留空使用自动获取): " input_name
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
                print_info "使用哪吒 v1 版本"
            else
                read -p "请输入哪吒端口 (留空使用默认): " input_nezha_port
                NEZHA_PORT="${input_nezha_port:-5555}"
                print_info "使用哪吒 v0 版本，端口: $NEZHA_PORT"
            fi
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
        if [ -z "$UPLOAD_URL" ] || [ -z "$PROJECT_URL" ]; then
            print_warn "地址为空，跳过自动上传"
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
# myapp 配置文件
UUID=$UUID
CFIP=$CFIP
CFPORT=$CFPORT
NAME=$NAME
SERVER_PORT=$SERVER_PORT
SUB_PATH=$SUB_PATH
ARGO_PORT=$ARGO_PORT
FILE_PATH=./tmp
AUTO_ACCESS=$AUTO_ACCESS
EOF

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

    print_info "配置文件已保存: $config_file"
}

# 创建 systemd 服务 (Linux)
create_systemd_service() {
    local bin_path=$1
    local config_file=$2
    local service_name=$3
    local service_file="/etc/systemd/system/${service_name}.service"
    local current_user=$(whoami)

    cat > "$service_file" << EOF
[Unit]
Description=myapp Service
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
    print_info "systemd 服务文件已创建: $service_file"
}

# 创建 FreeBSD rc.d 服务
create_rc_service() {
    local bin_path=$1
    local config_file=$2
    local service_name=$3
    local rc_file="/usr/local/etc/rc.d/${service_name}"
    local current_user=$(whoami)
    
    cat > "$rc_file" << EOF
#!/bin/sh
#
# PROVIDE: $service_name
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="$service_name"
rcvar="${service_name}_enable"

load_rc_config "\$name"

: \${${service_name}_enable:=NO}
: \${${service_name}_user:=$current_user}
: \${${service_name}_dir:=$WORKDIR}

pidfile="/var/run/\${name}.pid"
command="/usr/sbin/daemon"
command_args="-p \${pidfile} -u \${${service_name}_user} -f ${bin_path}"

start_precmd="export_env"
stop_postcmd="cleanup_pid"

export_env() {
    if [ -f "${config_file}" ]; then
        . "${config_file}"
        export UUID CFIP CFPORT NAME SERVER_PORT SUB_PATH ARGO_PORT FILE_PATH AUTO_ACCESS
        [ -n "\$ARGO_DOMAIN" ] && export ARGO_DOMAIN
        [ -n "\$ARGO_AUTH" ] && export ARGO_AUTH
        [ -n "\$NEZHA_SERVER" ] && export NEZHA_SERVER
        [ -n "\$NEZHA_PORT" ] && export NEZHA_PORT
        [ -n "\$NEZHA_KEY" ] && export NEZHA_KEY
        [ -n "\$UPLOAD_URL" ] && export UPLOAD_URL
        [ -n "\$PROJECT_URL" ] && export PROJECT_URL
    fi
}

cleanup_pid() {
    rm -f "\${pidfile}"
}

run_rc_command "\$1"
EOF
    
    chmod +x "$rc_file"
    print_info "FreeBSD rc.d 服务文件已创建: $rc_file"
    print_info "启用服务命令:"
    echo "  sudo sysrc ${service_name}_enable=YES"
    echo "  sudo service ${service_name} start"
}

# 创建服务 (根据系统选择)
create_service() {
    local bin_path=$1
    local config_file=$2
    local service_name=$3
    local os=$4
    
    if [ "$os" = "freebsd" ]; then
        create_rc_service "$bin_path" "$config_file" "$service_name"
    else
        create_systemd_service "$bin_path" "$config_file" "$service_name"
    fi
}

# 停止现有服务
stop_existing() {
    # 保存旧二进制文件名用于清理
    OLD_BIN_NAME=""
    if [ -f "$WORKDIR/.bin_name" ]; then
        OLD_BIN_NAME=$(cat "$WORKDIR/.bin_name")
    fi
    
    # 停止通过 PID 文件记录的进程
    if [ -f "$WORKDIR/myapp.pid" ]; then
        OLD_PID=$(cat "$WORKDIR/myapp.pid")
        if ps -p $OLD_PID > /dev/null 2>&1; then
            print_info "停止现有进程: $OLD_PID"
            kill $OLD_PID 2>/dev/null || true
            sleep 2
        fi
        rm -f "$WORKDIR/myapp.pid"
    fi
    
    # 杀掉所有可能的进程
    if [ -n "$OLD_BIN_NAME" ] && [ -f "$WORKDIR/$OLD_BIN_NAME" ]; then
        pkill -f "$WORKDIR/$OLD_BIN_NAME" 2>/dev/null || true
    fi
    pkill -f "$WORKDIR/myapp" 2>/dev/null || true
    
    # 清理旧二进制文件
    if [ -n "$OLD_BIN_NAME" ] && [ -f "$WORKDIR/$OLD_BIN_NAME" ]; then
        print_info "删除旧二进制文件: $OLD_BIN_NAME"
        rm -f "$WORKDIR/$OLD_BIN_NAME"
    fi
    
    # 清理 tmp 目录中的旧文件（但保留配置）
    if [ -d "$WORKDIR/tmp" ]; then
        print_info "清理 tmp 目录..."
        # 保留 .env 和 .bin_name，删除其他文件
        find "$WORKDIR/tmp" -type f ! -name ".env" ! -name ".bin_name" -delete 2>/dev/null || true
    fi
}

# 检查程序运行状态
check_process() {
    local pid=$1
    
    if ps -p $pid > /dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# 获取本机 IP
get_local_ip() {
    # 尝试获取本机 IP
    if command -v ip &> /dev/null; then
        ip addr show | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | grep -v 127.0.0.1 | head -1
    elif command -v ifconfig &> /dev/null; then
        ifconfig | grep -oE 'inet [0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' | grep -v 127.0.0.1 | head -1 | awk '{print $2}'
    else
        echo "localhost"
    fi
}

# 显示服务状态
show_status() {
    local server_port=$1
    local sub_path=$2
    local log_file="$WORKDIR/myapp.log"
    local bin_name=$(cat "$WORKDIR/.bin_name" 2>/dev/null || echo "myapp")
    local local_ip=$(get_local_ip)
    
    echo ""
    print_info "=========================================="
    print_info "✓ myapp 运行正常"
    print_info ""
    print_info "本地订阅地址:"
    print_info "  http://localhost:$server_port/$sub_path"
    if [ -n "$local_ip" ] && [ "$local_ip" != "localhost" ]; then
        print_info "  http://$local_ip:$server_port/$sub_path"
    fi
    print_info ""
    print_info "状态查看: http://localhost:$server_port/status"
    echo ""
    
    # 显示 Argo 隧道域名
    if [ -f "$WORKDIR/tmp/boot.log" ]; then
        # 从日志中提取 Argo 域名
        ARGO_DOMAIN=$(grep -oE '隧道域名: [^ ]+' "$WORKDIR/tmp/boot.log" 2>/dev/null | head -1 | sed 's/隧道域名: //')
        if [ -z "$ARGO_DOMAIN" ]; then
            ARGO_DOMAIN=$(grep -oE 'https?://([^ ]*trycloudflare\.com)' "$WORKDIR/tmp/boot.log" 2>/dev/null | head -1 | sed 's|https://||' | sed 's|http://||')
        fi
        if [ -n "$ARGO_DOMAIN" ]; then
            print_info "Argo 隧道域名: $ARGO_DOMAIN"
        fi
    fi
    
    echo ""
    print_info "管理命令:"
    print_info "  查看日志: tail -f $log_file"
    print_info "  停止服务: kill \$(cat $WORKDIR/myapp.pid)"
    print_info "  重启服务: $WORKDIR/$bin_name"
    echo ""
    
    # 显示订阅文件信息
    if [ -f "$WORKDIR/tmp/sub.txt" ]; then
        print_info "✓ 订阅文件已生成 ($(ls -lh "$WORKDIR/tmp/sub.txt" | awk '{print $5}'))"
    fi
    
    # 显示系统信息
    if [ -f "$log_file" ]; then
        SYSTEM_INFO=$(grep -oE '系统: [^ ]+' "$log_file" 2>/dev/null | tail -1 | sed 's/系统: //')
        if [ -n "$SYSTEM_INFO" ]; then
            print_info "运行环境: $SYSTEM_INFO"
        fi
    fi
}

# 主函数
main() {
    echo "=========================================="
    echo "        myapp 一键部署脚本 v1.0"
    echo "=========================================="
    echo ""

    # 检测环境
    print_info "检测系统环境..."
    OS=$(detect_os)
    ARCH=$(detect_arch)
    print_info "操作系统: $OS"
    print_info "系统架构: $ARCH"
    echo ""
    
    # 检查并安装依赖
    check_dependencies "$OS"

    # 创建固定目录
    mkdir -p "$WORKDIR"
    cd "$WORKDIR" || exit 1
    print_info "工作目录: $(pwd)"
    
    # 生成随机二进制文件名
    BIN_NAME=$(generate_random_name)
    BIN_PATH="$WORKDIR/$BIN_NAME"
    CONFIG_FILE="$WORKDIR/.env"
    
    print_info "二进制文件名: $BIN_NAME"
    print_info "二进制文件路径: $BIN_PATH"
    print_info "配置文件路径: $CONFIG_FILE"

    # 停止现有服务
    stop_existing

    # 下载对应版本
    if [ "$OS" = "freebsd" ]; then
        if [ "$ARCH" = "amd64" ]; then
            DOWNLOAD_URL="https://github.com/goyo123321a/go-argo/releases/download/${VERSION}/myapp-freebsd-amd64"
        else
            print_error "FreeBSD 系统暂不支持 arm64 架构"
            print_info "支持的架构: amd64"
            exit 1
        fi
    elif [ "$OS" = "linux" ]; then
        if [ "$ARCH" = "amd64" ]; then
            DOWNLOAD_URL="https://github.com/goyo123321a/go-argo/releases/download/${VERSION}/myapp-linux-amd64"
        elif [ "$ARCH" = "arm64" ]; then
            DOWNLOAD_URL="https://github.com/goyo123321a/go-argo/releases/download/${VERSION}/myapp-linux-arm64"
        else
            print_error "不支持的架构: $ARCH"
            exit 1
        fi
    else
        print_error "不支持的操作系统: $OS"
        exit 1
    fi
    
    print_info "下载地址: $DOWNLOAD_URL"
    download_file "$DOWNLOAD_URL" "$BIN_PATH"

    # 验证下载的文件
    if [ ! -f "$BIN_PATH" ]; then
        print_error "下载失败，文件不存在: $BIN_PATH"
        exit 1
    fi

    # 赋予执行权限
    chmod +x "$BIN_PATH"
    print_info "已赋予执行权限"
    
    # 保存二进制文件名
    echo "$BIN_NAME" > "$WORKDIR/.bin_name"
    
    # 显示文件信息
    print_info "文件信息:"
    ls -la "$BIN_PATH"
    file "$BIN_PATH" 2>/dev/null || true

    # 配置文件
    if [ -f "$CONFIG_FILE" ]; then
        print_question "检测到已有配置文件，是否重新配置? (y/n, 默认 n): "
        read reconfigure
        if [[ "$reconfigure" =~ ^[Yy]$ ]]; then
            configure_env "$CONFIG_FILE"
        else
            print_info "使用现有配置文件"
        fi
    else
        configure_env "$CONFIG_FILE"
    fi

    # 创建必要目录
    mkdir -p ./tmp
    print_info "已创建 tmp 目录"

    # 加载环境变量
    set -a
    source "$CONFIG_FILE"
    set +a

    # 启动程序
    print_info "启动 myapp..."
    nohup "$BIN_PATH" > ./myapp.log 2>&1 &
    
    APP_PID=$!
    echo $APP_PID > ./myapp.pid

    print_info "myapp 已启动，PID: $APP_PID"
    print_info "日志文件: $(pwd)/myapp.log"
    print_info "配置文件: $CONFIG_FILE"
    print_info "临时目录: $(pwd)/tmp"
    print_info "二进制文件: $BIN_PATH"

    # 等待程序启动
    sleep 5

    # 检查进程
    if check_process $APP_PID; then
        # 显示服务状态
        show_status "$SERVER_PORT" "$SUB_PATH"
        
        # 询问是否安装系统服务
        print_question "是否安装为系统服务? (y/n, 默认 n): "
        read install_service
        if [[ "$install_service" =~ ^[Yy]$ ]]; then
            SERVICE_NAME="myapp"
            create_service "$BIN_PATH" "$CONFIG_FILE" "$SERVICE_NAME" "$OS"
            print_info "系统服务已安装，可使用以下命令管理:"
            if [ "$OS" = "freebsd" ]; then
                echo "  sudo service myapp start"
                echo "  sudo service myapp stop"
                echo "  sudo service myapp status"
                echo "  sudo sysrc myapp_enable=YES  # 开机自启"
            else
                echo "  sudo systemctl start myapp"
                echo "  sudo systemctl stop myapp"
                echo "  sudo systemctl status myapp"
                echo "  sudo systemctl enable myapp  # 开机自启"
            fi
        fi
        
        print_info "=========================================="
        
        # 显示最近的日志
        echo ""
        print_info "最近日志 (最后 15 行):"
        tail -15 ./myapp.log 2>/dev/null || echo "无日志"
        
    else
        print_error "myapp 启动失败，请检查日志"
        echo ""
        echo "=== 日志内容 ==="
        cat ./myapp.log 2>/dev/null || echo "无日志文件"
        echo ""
        echo "=== 二进制文件信息 ==="
        file "$BIN_PATH" 2>/dev/null || echo "无法获取文件信息"
        echo ""
        echo "=== 配置文件内容 ==="
        cat "$CONFIG_FILE" 2>/dev/null || echo "无法读取配置文件"
        exit 1
    fi
}

# 清理函数
cleanup() {
    print_info "脚本被中断"
    exit 1
}

trap cleanup INT TERM
main "$@"
