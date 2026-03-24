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

# 生成随机6位数文件名
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

# 检查并安装依赖 (针对 FreeBSD)
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
        # 显示文件信息
        ls -la "$output" 2>/dev/null || true
        file "$output" 2>/dev/null || true
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
    local current_dir=$WORKDIR

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

# 创建 FreeBSD rc.d 服务
create_rc_service() {
    local bin_path=$1
    local config_file=$2
    local service_name=$3
    local rc_file="/usr/local/etc/rc.d/${service_name}"
    local current_user=$(whoami)
    local current_dir=$WORKDIR
    
    # 转换相对路径为绝对路径
    bin_path=$(realpath "$bin_path" 2>/dev/null || readlink -f "$bin_path" 2>/dev/null || echo "$bin_path")
    config_file=$(realpath "$config_file" 2>/dev/null || readlink -f "$config_file" 2>/dev/null || echo "$config_file")
    
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
: \${${service_name}_dir:=$current_dir}

pidfile="/var/run/\${name}.pid"
command="/usr/sbin/daemon"
command_args="-p \${pidfile} -u \${${service_name}_user} -f ${bin_path}"

start_precmd="export_env"
stop_postcmd="cleanup_pid"

export_env() {
    # 加载环境变量
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
    if [ -f "$WORKDIR/myapp.pid" ]; then
        OLD_PID=$(cat "$WORKDIR/myapp.pid")
        if ps -p $OLD_PID > /dev/null 2>&1; then
            print_info "停止现有进程: $OLD_PID"
            kill $OLD_PID 2>/dev/null || true
            sleep 2
        fi
    fi
    
    # 杀掉相关进程
    pkill -f "$BIN_NAME" 2>/dev/null || true
}

# 主函数
main() {
    echo "=========================================="
    echo "        myapp 一键部署脚本 v1.0"
    echo "=========================================="
    echo ""

    # 固定工作目录
    WORKDIR="$HOME/myapp"
    
    # 检测环境
    print_info "检测系统环境..."
    OS=$(detect_os)
    ARCH=$(detect_arch)
    print_info "操作系统: $OS"
    print_info "系统架构: $ARCH"
    echo ""
    
    # 检查并安装依赖
    check_dependencies "$OS"

    # 生成随机文件名
    BIN_NAME=$(generate_random_name)
    print_info "二进制文件名: $BIN_NAME"
    
    # 创建固定目录
    mkdir -p "$WORKDIR"
    cd "$WORKDIR" || exit 1
    print_info "工作目录: $(pwd)"
    
    # 使用绝对路径
    BIN_PATH="$(pwd)/$BIN_NAME"
    CONFIG_FILE="$(pwd)/.env"
    
    print_info "二进制文件路径: $BIN_PATH"
    print_info "配置文件路径: $CONFIG_FILE"

    # 停止现有服务
    stop_existing

    # 下载对应版本 - 使用您提供的地址
    if [ "$OS" = "freebsd" ]; then
        if [ "$ARCH" = "amd64" ]; then
            DOWNLOAD_URL="https://github.com/goyo123321a/go-argo/releases/download/v1.0.0.12/myapp-freebsd-amd64"
        else
            print_error "FreeBSD 系统暂不支持 arm64 架构"
            print_info "支持的架构: amd64"
            exit 1
        fi
    elif [ "$OS" = "linux" ]; then
        if [ "$ARCH" = "amd64" ]; then
            DOWNLOAD_URL="https://github.com/goyo123321a/go-argo/releases/download/v1.0.0.12/myapp-linux-amd64"
        elif [ "$ARCH" = "arm64" ]; then
            DOWNLOAD_URL="https://github.com/goyo123321a/go-argo/releases/download/v1.0.0.12/myapp-linux-arm64"
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
    
    # 显示文件信息
    print_info "文件信息:"
    ls -la "$BIN_PATH"
    file "$BIN_PATH"

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

    # 启动程序
    print_info "启动 myapp..."
    
    # 加载环境变量并启动
    set -a
    source "$CONFIG_FILE"
    set +a
    
    nohup "$BIN_PATH" > ./myapp.log 2>&1 &
    
    APP_PID=$!
    echo $APP_PID > ./myapp.pid

    print_info "myapp 已启动，PID: $APP_PID"
    print_info "日志文件: $(pwd)/myapp.log"
    print_info "配置文件: $CONFIG_FILE"

    # 等待几秒检查进程
    sleep 3

    if ps -p $APP_PID > /dev/null 2>&1; then
        print_info "✓ myapp 运行正常"
        echo ""
        print_info "=========================================="
        print_info "服务已启动"
        print_info "  订阅地址: http://localhost:$SERVER_PORT/$SUB_PATH"
        print_info "  下载地址: http://localhost:$SERVER_PORT/$SUB_PATH/download"
        print_info "  状态查看: http://localhost:$SERVER_PORT/status"
        echo ""
        print_info "管理命令:"
        print_info "  查看日志: tail -f $(pwd)/myapp.log"
        print_info "  停止服务: kill $APP_PID"
        print_info "  重启服务: kill $APP_PID && ./$BIN_NAME"
        print_info "  查看配置: cat $CONFIG_FILE"
        echo ""

        print_question "是否安装为系统服务? (y/n, 默认 n): "
        read install_service
        if [[ "$install_service" =~ ^[Yy]$ ]]; then
            SERVICE_NAME="myapp"
            create_service "$(pwd)/$BIN_NAME" "$CONFIG_FILE" "$SERVICE_NAME" "$OS"
        fi
        
        print_info "=========================================="
    else
        print_error "myapp 启动失败，请检查日志"
        echo "=== 日志内容 ==="
        cat ./myapp.log 2>/dev/null || echo "无日志文件"
        echo "=== 二进制文件信息 ==="
        file "$BIN_PATH" 2>/dev/null || echo "无法获取文件信息"
        exit 1
    fi
}

trap 'print_info "脚本被中断"; exit 1' INT TERM
main "$@"
