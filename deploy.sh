#!/bin/bash
# 一键部署 myapp - 自动下载、配置并运行
# 支持 Linux (Xray) 和 FreeBSD (Sing-box)
# 版本: v3.1

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

# 交互式配置（根据系统使用不同变量名）
configure_env() {
    local config_file=$1
    local mode=$2
    print_info "开始配置环境变量（$mode 模式）..."
    echo ""

    # HTTP 服务端口配置
    read -p "请输入 HTTP 服务端口 (留空使用默认 7860): " input_port
    SERVER_PORT="${input_port:-7860}"
    print_info "HTTP 服务端口: $SERVER_PORT"

    # 订阅路径配置
    read -p "请输入订阅路径 (留空使用默认 sub): " input_sub_path
    SUB_PATH="${input_sub_path:-sub}"
    print_info "订阅路径: $SUB_PATH"

    # 隧道端口配置（根据系统使用不同变量名）
    if [ "$mode" = "sing-box" ]; then
        read -p "请输入 VLESS 端口 (留空使用默认 8001): " input_vless_port
        VLESS_PORT="${input_vless_port:-8001}"
        print_info "VLESS 端口: $VLESS_PORT"
        ARGO_PORT="$VLESS_PORT"  # 兼容内部使用
    else
        read -p "请输入 Argo 隧道端口 (留空使用默认 8001): " input_argo_port
        ARGO_PORT="${input_argo_port:-8001}"
        print_info "Argo 隧道端口: $ARGO_PORT"
    fi

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
FILE_PATH=./tmp
AUTO_ACCESS=$AUTO_ACCESS
EOF

    # 根据模式写入不同端口变量
    if [ "$mode" = "sing-box" ]; then
        cat >> "$config_file" << EOF
VLESS_PORT=$VLESS_PORT
ARGO_PORT=$ARGO_PORT
EOF
    else
        cat >> "$config_file" << EOF
ARGO_PORT=$ARGO_PORT
EOF
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

    print_info "配置文件已保存: $config_file"
}

# 创建 systemd 服务 (Linux)
create_systemd_service() {
    local bin_path=$1
    local config_file=$2
    local service_file="/etc/systemd/system/myapp.service"
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
    local rc_file="/usr/local/etc/rc.d/myapp"
    local current_user=$(whoami)
    cat > "$rc_file" << EOF
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
        . "${config_file}"
        export UUID CFIP CFPORT NAME SERVER_PORT SUB_PATH FILE_PATH AUTO_ACCESS
        # 根据系统导出不同端口变量
        if [ -n "\$VLESS_PORT" ]; then
            export VLESS_PORT ARGO_PORT
        else
            export ARGO_PORT
        fi
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

# 显示状态
show_status() {
    local server_port=$1
    local sub_path=$2
    local tunnel_port=$3
    local mode=$4
    echo ""
    print_info "=========================================="
    print_info "服务已启动 ($mode 模式)"
    print_info "  订阅地址: http://localhost:$server_port/$sub_path"
    print_info "  下载地址: http://localhost:$server_port/$sub_path/download"
    print_info "  状态查看: http://localhost:$server_port/status"
    if [ "$mode" = "sing-box" ]; then
        print_info "  VLESS 端口: $tunnel_port (直连和 Argo 隧道目标)"
    else
        print_info "  Argo 端口: $tunnel_port"
    fi
    echo ""
    print_info "管理命令:"
    print_info "  查看日志: tail -f $WORKDIR/myapp.log"
    print_info "  停止服务: kill \$(cat $WORKDIR/myapp.pid)"
    print_info "  重启服务: $WORKDIR/myapp"
    echo ""
    sleep 3
    if [ -f "$WORKDIR/tmp/sub.txt" ]; then
        print_info "✓ 订阅文件已生成"
    fi
    if [ -f "$WORKDIR/tmp/boot.log" ]; then
        ARGO_URL=$(grep -oE 'https?://[^ ]*trycloudflare\.com' "$WORKDIR/tmp/boot.log" 2>/dev/null | head -1)
        if [ -n "$ARGO_URL" ]; then
            print_info "✓ Argo 隧道地址: $ARGO_URL"
            print_info "  外网订阅: $ARGO_URL/$sub_path"
        fi
    fi
}

# 安装主流程
install() {
    echo ""
    print_info "检测系统环境..."
    OS=$(detect_os)
    ARCH=$(detect_arch)
    print_info "操作系统: $OS"
    print_info "系统架构: $ARCH"
    echo ""

    check_dependencies "$OS"

    mkdir -p "$WORKDIR"
    cd "$WORKDIR" || exit 1

    BIN_PATH="$WORKDIR/myapp"
    CONFIG_FILE="$WORKDIR/.env"

    stop_existing

    # 选择模式（自动识别）
    MODE=""
    if [ "$OS" = "freebsd" ]; then
        MODE="sing-box"
        print_info "检测到 FreeBSD，将使用 Sing-box 模式"
        if [ "$ARCH" != "amd64" ]; then
            print_error "FreeBSD 仅支持 amd64 架构"
            exit 1
        fi
        DOWNLOAD_URL="https://github.com/goyo123321a/go-argo/releases/download/${VERSION}/myapp-freebsd-amd64"
    elif [ "$OS" = "linux" ]; then
        MODE="xray"
        print_info "检测到 Linux，将使用 Xray 模式"
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

    if [ ! -f "$BIN_PATH" ]; then
        print_error "下载失败，文件不存在: $BIN_PATH"
        exit 1
    fi

    chmod +x "$BIN_PATH"
    print_info "已赋予执行权限"

    if [ -f "$CONFIG_FILE" ]; then
        print_question "检测到已有配置文件，是否重新配置? (y/n, 默认 n): "
        read reconfigure
        if [[ "$reconfigure" =~ ^[Yy]$ ]]; then
            configure_env "$CONFIG_FILE" "$MODE"
        else
            print_info "使用现有配置文件"
            set -a
            source "$CONFIG_FILE"
            set +a
        fi
    else
        configure_env "$CONFIG_FILE" "$MODE"
    fi

    set -a
    source "$CONFIG_FILE"
    set +a

    mkdir -p ./tmp
    print_info "已创建 tmp 目录"

    print_info "启动 myapp..."
    nohup "$BIN_PATH" > ./myapp.log 2>&1 &
    APP_PID=$!
    echo $APP_PID > ./myapp.pid

    print_info "myapp 已启动，PID: $APP_PID"
    sleep 5

    if ps -p $APP_PID > /dev/null 2>&1; then
        print_info "✓ myapp 运行正常"
        
        # 获取隧道端口用于显示
        if [ "$MODE" = "sing-box" ]; then
            TUNNEL_PORT="${VLESS_PORT:-8001}"
        else
            TUNNEL_PORT="${ARGO_PORT:-8001}"
        fi
        show_status "$SERVER_PORT" "$SUB_PATH" "$TUNNEL_PORT" "$MODE"

        print_question "是否安装为系统服务? (y/n, 默认 n): "
        read install_service
        if [[ "$install_service" =~ ^[Yy]$ ]]; then
            if [ "$OS" = "freebsd" ]; then
                create_rc_service "$BIN_PATH" "$CONFIG_FILE"
                print_info "启用服务命令: sudo sysrc myapp_enable=YES && sudo service myapp start"
            else
                create_systemd_service "$BIN_PATH" "$CONFIG_FILE"
                print_info "启用服务命令: sudo systemctl enable myapp && sudo systemctl start myapp"
            fi
        fi
        print_info "=========================================="
        echo ""
        print_info "最近日志 (最后 10 行):"
        tail -10 ./myapp.log 2>/dev/null || echo "无日志"
    else
        print_error "myapp 启动失败，请检查日志"
        cat ./myapp.log 2>/dev/null
        exit 1
    fi
}

# 卸载
uninstall() {
    read -p "确定要卸载吗? [y/N]: " confirm
    if [[ "$confirm" =~ ^[Yy]$ ]]; then
        stop_existing
        rm -rf "$WORKDIR"
        if [ "$OS" = "freebsd" ]; then
            rm -f /usr/local/etc/rc.d/myapp 2>/dev/null
        else
            rm -f /etc/systemd/system/myapp.service 2>/dev/null
            systemctl daemon-reload 2>/dev/null
        fi
        print_info "myapp 已卸载"
    fi
    menu
}

# 重启
restart() {
    stop_existing
    cd "$WORKDIR" || { print_error "工作目录不存在"; menu; }
    nohup ./myapp > myapp.log 2>&1 &
    echo $! > myapp.pid
    print_info "myapp 已重启"
    sleep 2
    status
}

# 查看状态
status() {
    if [ -f "$WORKDIR/myapp.pid" ]; then
        PID=$(cat "$WORKDIR/myapp.pid")
        if ps -p $PID > /dev/null 2>&1; then
            print_info "myapp 正在运行，PID: $PID"
            tail -20 "$WORKDIR/myapp.log"
        else
            print_warn "myapp 未运行"
        fi
    else
        print_warn "myapp 未运行"
    fi
    read -p "按回车返回菜单"
    menu
}

# 重置系统
reset_system() {
    read -p "危险操作！确认重置系统? [y/N]: " confirm
    if [[ "$confirm" =~ ^[Yy]$ ]]; then
        stop_existing
        find "$HOME" -mindepth 1 ! -name "domains" ! -name "mail" ! -name "repo" ! -name "backups" -exec rm -rf {} + 2>/dev/null
        devil www list | awk 'NF>=2 && $1 ~ /\./ {print $1}' | while read -r domain; do devil www del "$domain"; done
        rm -rf "$HOME/domains"/* 2>/dev/null
        print_info "系统已重置（保留 domains, mail, repo, backups 目录）"
    fi
    menu
}

# 主菜单
menu() {
    clear
    echo "=========================================="
    echo "        myapp 一键部署脚本 v3.1"
    echo "=========================================="
    echo "1. 安装/更新 myapp"
    echo "2. 卸载 myapp"
    echo "3. 重启 myapp"
    echo "4. 查看状态"
    echo "5. 重置系统（谨慎）"
    echo "0. 退出"
    echo "=========================================="
    read -p "请选择 [0-5]: " choice
    case $choice in
        1) install ;;
        2) uninstall ;;
        3) restart ;;
        4) status ;;
        5) reset_system ;;
        0) exit 0 ;;
        *) print_error "无效选择"; sleep 2; menu ;;
    esac
}

# 启动菜单
menu
