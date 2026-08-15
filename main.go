package main

import (
    "bufio"
    "bytes"
    "context"
    "crypto/ecdsa"
    "crypto/elliptic"
    "crypto/rand"
    "crypto/sha256"
    "crypto/x509"
    "crypto/x509/pkix"
    "encoding/base64"
    "encoding/json"
    "encoding/pem"
    "fmt"
    "io"
    "log"
    "math/big"
    "net"
    "net/http"
    "net/url"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strconv"
    "strings"
    "sync"
    "time"

    "golang.org/x/crypto/curve25519"
)

// ========== 常量 ==========
const (
    defaultPort      = 7860
    defaultArgoPort  = 8001
    defaultCFPort    = 443
    defaultUUID      = "9afd1229-b893-40c1-84dd-51e7ce204913"
    defaultCFIP      = "saas.sin.fan"
    downloadTimeout  = 60 * time.Second
    httpTimeout      = 30 * time.Second
    maxRetries       = 3
)

// ========== 配置结构体 ==========
type Config struct {
    UploadURL    string
    ProjectURL   string
    AutoAccess   bool
    FilePath     string
    SubPath      string
    Port         int
    UUID         string
    NezhaServer  string
    NezhaPort    string
    NezhaKey     string
    ArgoDomain   string
    ArgoAuth     string
    ArgoPort     int
    S5Port       string
    HY2Port      string
    RealityPort  string
    CFIP         string
    CFPort       int
    Name         string
    ChatID       string
    BotToken     string
    ShowLog      bool
}

// ========== 全局状态 ==========
type AppState struct {
    Config      Config
    SubContent  string
    mu          sync.RWMutex
    HTTPClient  *http.Client
}

func NewAppState(cfg Config) *AppState {
    return &AppState{
        Config: cfg,
        HTTPClient: &http.Client{
            Timeout: httpTimeout,
            Transport: &http.Transport{
                DialContext: (&net.Dialer{
                    Timeout:   30 * time.Second,
                    KeepAlive: 30 * time.Second,
                }).DialContext,
                MaxIdleConns:    100,
                IdleConnTimeout: 90 * time.Second,
            },
        },
    }
}

// ========== 辅助函数 ==========
func getEnv(key, defaultVal string) string {
    if val := os.Getenv(key); val != "" {
        return val
    }
    return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
    val := os.Getenv(key)
    if val == "" {
        return defaultVal
    }
    v := strings.ToLower(val)
    return v == "true" || v == "1" || v == "yes" || v == "on"
}

func getEnvInt(key string, defaultVal int) int {
    val := os.Getenv(key)
    if val == "" {
        return defaultVal
    }
    i, err := strconv.Atoi(val)
    if err != nil {
        return defaultVal
    }
    return i
}

func loadConfig() Config {
    return Config{
        UploadURL:   getEnv("UPLOAD_URL", ""),
        ProjectURL:  getEnv("PROJECT_URL", ""),
        AutoAccess:  getEnvBool("AUTO_ACCESS", false),
        FilePath:    getEnv("FILE_PATH", ".tmp"),
        SubPath:     getEnv("SUB_PATH", "sub"),
        Port:        getEnvInt("SERVER_PORT", defaultPort),
        UUID:        getEnv("UUID", defaultUUID),
        NezhaServer: getEnv("NEZHA_SERVER", ""),
        NezhaPort:   getEnv("NEZHA_PORT", ""),
        NezhaKey:    getEnv("NEZHA_KEY", ""),
        ArgoDomain:  getEnv("ARGO_DOMAIN", ""),
        ArgoAuth:    getEnv("ARGO_AUTH", ""),
        ArgoPort:    getEnvInt("ARGO_PORT", defaultArgoPort),
        S5Port:      getEnv("S5_PORT", ""),
        HY2Port:     getEnv("HY2_PORT", ""),
        RealityPort: getEnv("REALITY_PORT", ""),
        CFIP:        getEnv("CFIP", defaultCFIP),
        CFPort:      getEnvInt("CFPORT", defaultCFPort),
        Name:        getEnv("NAME", ""),
        ChatID:      getEnv("CHAT_ID", ""),
        BotToken:    getEnv("BOT_TOKEN", ""),
        ShowLog:     getEnvBool("SHOW_LOG", true),
    }
}

func isValidPort(port string) bool {
    if port == "" {
        return false
    }
    p, err := strconv.Atoi(port)
    if err != nil || p < 1 || p > 65535 {
        return false
    }
    return true
}

func ensureDir(path string) error {
    if _, err := os.Stat(path); os.IsNotExist(err) {
        return os.MkdirAll(path, 0755)
    }
    return nil
}

// 带重试的下载
func downloadFileWithRetry(client *http.Client, url, dest string, retries int) error {
    var err error
    for i := 0; i < retries; i++ {
        err = downloadFile(client, url, dest)
        if err == nil {
            return nil
        }
        time.Sleep(time.Duration(i+1) * time.Second)
    }
    return fmt.Errorf("下载 %s 失败（重试 %d 次）: %w", url, retries, err)
}

func downloadFile(client *http.Client, url, dest string) error {
    ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
    defer cancel()
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return err
    }
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("HTTP %d", resp.StatusCode)
    }
    out, err := os.Create(dest)
    if err != nil {
        return err
    }
    defer out.Close()
    _, err = io.Copy(out, resp.Body)
    if err != nil {
        return err
    }
    // 设置可执行权限
    if err := os.Chmod(dest, 0755); err != nil {
        return err
    }
    return nil
}

// 自动检测系统架构
func getArch() string {
    arch := runtime.GOARCH
    switch arch {
    case "arm64", "aarch64":
        return "arm64"
    case "amd64", "x86_64", "x86":
        return "amd64"
    default:
        log.Printf("[WARN] 未知架构 %s，使用 amd64", arch)
        return "amd64"
    }
}

// ========== 日志辅助 ==========
var showLog bool

func logInfo(msg string) {
    if showLog {
        log.Println("[INFO]", msg)
    }
}

func logWarn(msg string) {
    log.Println("[WARN]", msg)
}

func logError(msg string) {
    log.Println("[ERROR]", msg)
}

// ========== 清空目录 ==========
func cleanupOldFiles(filePath string) error {
    dir := filePath
    if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
        return err
    }
    return os.MkdirAll(dir, 0755)
}

// ========== 删除上游旧节点 ==========
func deleteNodes(state *AppState) error {
    if state.Config.UploadURL == "" {
        return nil
    }
    subPath := filepath.Join(state.Config.FilePath, "sub.txt")
    if _, err := os.Stat(subPath); os.IsNotExist(err) {
        return nil
    }
    data, err := os.ReadFile(subPath)
    if err != nil {
        return err
    }
    decoded, err := base64.StdEncoding.DecodeString(string(data))
    if err != nil {
        return err
    }
    lines := strings.Split(string(decoded), "\n")
    var nodes []string
    for _, line := range lines {
        if strings.Contains(line, "://") {
            nodes = append(nodes, line)
        }
    }
    if len(nodes) == 0 {
        return nil
    }
    payload := map[string]interface{}{"nodes": nodes}
    jsonData, _ := json.Marshal(payload)
    resp, err := state.HTTPClient.Post(state.Config.UploadURL+"/api/delete-nodes", "application/json", bytes.NewReader(jsonData))
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode == http.StatusOK {
        logInfo("旧节点删除成功")
    } else {
        logWarn("旧节点删除失败: " + resp.Status)
    }
    return nil
}

// ========== 生成 X25519 密钥对（保存到文件） ==========
func generateOrLoadKeypair(filePath string) (priv, pub string, err error) {
    keyFile := filepath.Join(filePath, "key.txt")
    if _, err := os.Stat(keyFile); err == nil {
        data, err := os.ReadFile(keyFile)
        if err == nil {
            lines := strings.Split(string(data), "\n")
            var privKey, pubKey string
            for _, line := range lines {
                if strings.HasPrefix(line, "PrivateKey:") {
                    privKey = strings.TrimSpace(strings.TrimPrefix(line, "PrivateKey:"))
                }
                if strings.HasPrefix(line, "PublicKey:") {
                    pubKey = strings.TrimSpace(strings.TrimPrefix(line, "PublicKey:"))
                }
            }
            if privKey != "" && pubKey != "" {
                return privKey, pubKey, nil
            }
        }
    }
    // 生成新密钥
    var privKey [32]byte
    if _, err := rand.Read(privKey[:]); err != nil {
        return "", "", err
    }
    var pubKey [32]byte
    curve25519.ScalarBaseMult(&pubKey, &privKey)
    privB64 := base64.URLEncoding.EncodeToString(privKey[:])
    pubB64 := base64.URLEncoding.EncodeToString(pubKey[:])
    content := fmt.Sprintf("PrivateKey: %s\nPublicKey: %s\n", privB64, pubB64)
    if err := os.WriteFile(keyFile, []byte(content), 0644); err != nil {
        return "", "", err
    }
    return privB64, pubB64, nil
}

// ========== 生成自签名证书（用于 Hysteria2） ==========
func generateCertAndKey(certPath, keyPath string) error {
    privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    if err != nil {
        return err
    }
    template := &x509.Certificate{
        SerialNumber: big.NewInt(1),
        Subject: pkix.Name{
            CommonName: "bing.com",
        },
        NotBefore: time.Now(),
        NotAfter:  time.Now().Add(3650 * 24 * time.Hour),
        KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
        ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
        DNSNames:  []string{"bing.com"},
    }
    derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
    if err != nil {
        return err
    }
    certOut, err := os.Create(certPath)
    if err != nil {
        return err
    }
    defer certOut.Close()
    pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

    keyOut, err := os.Create(keyPath)
    if err != nil {
        return err
    }
    defer keyOut.Close()
    privBytes, err := x509.MarshalECPrivateKey(privKey)
    if err != nil {
        return err
    }
    pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})
    return nil
}

// ========== 生成 Xray 配置文件 ==========
func generateConfig(state *AppState) error {
    configPath := filepath.Join(state.Config.FilePath, "config.json")
    inbounds := []map[string]interface{}{
        {
            "tag":      "vless-fallback-in",
            "port":     state.Config.ArgoPort,
            "listen":   "::",
            "protocol": "vless",
            "settings": map[string]interface{}{
                "clients": []map[string]interface{}{
                    {"id": state.Config.UUID, "flow": "xtls-rprx-vision"},
                },
                "decryption": "none",
                "fallbacks": []map[string]interface{}{
                    {"dest": 3001},
                    {"path": "/vless-argo", "dest": 3002},
                    {"path": "/vmess-argo", "dest": 3003},
                    {"path": "/trojan-argo", "dest": 3004},
                },
            },
            "streamSettings": map[string]interface{}{
                "network": "tcp",
            },
        },
        {
            "tag":      "vless-tcp-in",
            "port":     3001,
            "listen":   "127.0.0.1",
            "protocol": "vless",
            "settings": map[string]interface{}{
                "clients": []map[string]interface{}{{"id": state.Config.UUID}},
                "decryption": "none",
            },
            "streamSettings": map[string]interface{}{
                "network": "tcp", "security": "none",
            },
        },
        {
            "tag":      "vless-ws-in",
            "port":     3002,
            "listen":   "127.0.0.1",
            "protocol": "vless",
            "settings": map[string]interface{}{
                "clients": []map[string]interface{}{{"id": state.Config.UUID, "level": 0}},
                "decryption": "none",
            },
            "streamSettings": map[string]interface{}{
                "network": "ws",
                "security": "none",
                "wsSettings": map[string]interface{}{
                    "path": "/vless-argo",
                },
            },
            "sniffing": map[string]interface{}{
                "enabled": true,
                "destOverride": []string{"http", "tls", "quic"},
                "metadataOnly": false,
            },
        },
        {
            "tag":      "vmess-ws-in",
            "port":     3003,
            "listen":   "127.0.0.1",
            "protocol": "vmess",
            "settings": map[string]interface{}{
                "clients": []map[string]interface{}{
                    {"id": state.Config.UUID, "alterId": 0},
                },
            },
            "streamSettings": map[string]interface{}{
                "network": "ws",
                "wsSettings": map[string]interface{}{
                    "path": "/vmess-argo",
                },
            },
            "sniffing": map[string]interface{}{
                "enabled": true,
                "destOverride": []string{"http", "tls", "quic"},
                "metadataOnly": false,
            },
        },
        {
            "tag":      "trojan-ws-in",
            "port":     3004,
            "listen":   "127.0.0.1",
            "protocol": "trojan",
            "settings": map[string]interface{}{
                "clients": []map[string]interface{}{
                    {"password": state.Config.UUID},
                },
            },
            "streamSettings": map[string]interface{}{
                "network": "ws",
                "security": "none",
                "wsSettings": map[string]interface{}{
                    "path": "/trojan-argo",
                },
            },
            "sniffing": map[string]interface{}{
                "enabled": true,
                "destOverride": []string{"http", "tls", "quic"},
                "metadataOnly": false,
            },
        },
    }

    // Reality
    if isValidPort(state.Config.RealityPort) {
        priv, pub, err := generateOrLoadKeypair(state.Config.FilePath)
        if err != nil {
            return err
        }
        pubPath := filepath.Join(state.Config.FilePath, "public_key.txt")
        os.WriteFile(pubPath, []byte(pub), 0644)
        port, _ := strconv.Atoi(state.Config.RealityPort)
        inbounds = append(inbounds, map[string]interface{}{
            "tag":      "vless-in",
            "listen":   "::",
            "port":     port,
            "protocol": "vless",
            "settings": map[string]interface{}{
                "clients": []map[string]interface{}{
                    {"id": state.Config.UUID, "flow": "xtls-rprx-vision"},
                },
                "decryption": "none",
            },
            "streamSettings": map[string]interface{}{
                "network": "raw",
                "security": "reality",
                "realitySettings": map[string]interface{}{
                    "show": false,
                    "dest": "www.iij.ad.jp:443",
                    "xver": 0,
                    "serverNames": []string{"www.iij.ad.jp"},
                    "privateKey": priv,
                    "shortIds": []string{""},
                },
            },
        })
    }

    // Hysteria2
    if isValidPort(state.Config.HY2Port) {
        certPath := filepath.Join(state.Config.FilePath, "cert.pem")
        keyPath := filepath.Join(state.Config.FilePath, "private.key")
        if err := generateCertAndKey(certPath, keyPath); err != nil {
            return err
        }
        port, _ := strconv.Atoi(state.Config.HY2Port)
        inbounds = append(inbounds, map[string]interface{}{
            "tag":      "hysteria-in",
            "listen":   "::",
            "port":     port,
            "protocol": "hysteria",
            "settings": map[string]interface{}{
                "version": 2,
                "clients": []map[string]interface{}{
                    {"auth": state.Config.UUID},
                },
            },
            "streamSettings": map[string]interface{}{
                "network": "hysteria",
                "hysteriaSettings": map[string]interface{}{
                    "version": 2,
                    "masquerade": map[string]interface{}{
                        "type": "proxy",
                        "url":  "https://bing.com",
                    },
                },
                "security": "tls",
                "tlsSettings": map[string]interface{}{
                    "alpn": []string{"h3"},
                    "certificates": []map[string]interface{}{
                        {
                            "certificateFile": certPath,
                            "keyFile":         keyPath,
                        },
                    },
                },
            },
        })
    }

    // Socks5
    if isValidPort(state.Config.S5Port) {
        port, _ := strconv.Atoi(state.Config.S5Port)
        inbounds = append(inbounds, map[string]interface{}{
            "tag":      "s5-in",
            "listen":   "::",
            "port":     port,
            "protocol": "socks",
            "settings": map[string]interface{}{
                "auth": "password",
                "accounts": []map[string]interface{}{
                    {
                        "user": state.Config.UUID[:8],
                        "pass": state.Config.UUID[12:],
                    },
                },
                "udp": true,
            },
        })
    }

    config := map[string]interface{}{
        "log": map[string]interface{}{
            "access":   "/dev/null",
            "error":    "/dev/null",
            "loglevel": "none",
        },
        "inbounds": inbounds,
        "dns": map[string]interface{}{
            "servers": []string{"https+local://8.8.8.8/dns-query"},
        },
        "outbounds": []map[string]interface{}{
            {"protocol": "freedom", "tag": "direct"},
            {"protocol": "blackhole", "tag": "block"},
        },
    }
    jsonData, err := json.MarshalIndent(config, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(configPath, jsonData, 0644)
}

// ========== 下载并运行外部二进制 ==========
func downloadAndRun(state *AppState) error {
    arch := getArch()
    filePath := state.Config.FilePath
    client := state.HTTPClient

    // web 和 bot
    webUrl := fmt.Sprintf("https://%s.ssss.nyc.mn/web", arch)
    botUrl := fmt.Sprintf("https://%s.ssss.nyc.mn/bot", arch)
    webDest := filepath.Join(filePath, "web")
    botDest := filepath.Join(filePath, "bot")

    // 并行下载
    var wg sync.WaitGroup
    wg.Add(2)
    go func() {
        defer wg.Done()
        if err := downloadFileWithRetry(client, webUrl, webDest, maxRetries); err != nil {
            logError("下载 web 失败: " + err.Error())
        }
    }()
    go func() {
        defer wg.Done()
        if err := downloadFileWithRetry(client, botUrl, botDest, maxRetries); err != nil {
            logError("下载 bot 失败: " + err.Error())
        }
    }()
    wg.Wait()

    // Nezha
    if state.Config.NezhaServer != "" && state.Config.NezhaKey != "" {
        nezhaPort := state.Config.NezhaPort
        if nezhaPort != "" {
            agentUrl := fmt.Sprintf("https://%s.ssss.nyc.mn/agent", arch)
            agentDest := filepath.Join(filePath, "agent")
            if err := downloadFileWithRetry(client, agentUrl, agentDest, maxRetries); err != nil {
                logError("下载 agent 失败: " + err.Error())
            } else {
                // 启动 agent
                cmd := exec.Command(agentDest,
                    "-s", state.Config.NezhaServer+":"+nezhaPort,
                    "-p", state.Config.NezhaKey,
                    "--disable-auto-update",
                    "--report-delay", "4",
                    "--skip-conn",
                    "--skip-procs",
                )
                tlsPorts := []string{"443", "8443", "2096", "2087", "2083", "2053"}
                for _, p := range tlsPorts {
                    if p == nezhaPort {
                        cmd.Args = append(cmd.Args, "--tls")
                        break
                    }
                }
                cmd.Stdout = nil
                cmd.Stderr = nil
                if err := cmd.Start(); err != nil {
                    logError("启动 agent 失败: " + err.Error())
                } else {
                    logInfo("agent 启动")
                }
            }
        } else {
            v1Url := fmt.Sprintf("https://%s.ssss.nyc.mn/v1", arch)
            v1Dest := filepath.Join(filePath, "v1")
            if err := downloadFileWithRetry(client, v1Url, v1Dest, maxRetries); err != nil {
                logError("下载 v1 失败: " + err.Error())
            } else {
                // 生成 config.yaml
                tlsFlag := strings.Contains(state.Config.NezhaServer, "443") ||
                    strings.Contains(state.Config.NezhaServer, "8443") ||
                    strings.Contains(state.Config.NezhaServer, "2096") ||
                    strings.Contains(state.Config.NezhaServer, "2087") ||
                    strings.Contains(state.Config.NezhaServer, "2083") ||
                    strings.Contains(state.Config.NezhaServer, "2053")
                yamlContent := fmt.Sprintf(`client_secret: %s
debug: false
disable_auto_update: true
disable_command_execute: false
disable_force_update: true
disable_nat: false
disable_send_query: false
gpu: false
insecure_tls: true
ip_report_period: 1800
report_delay: 4
server: %s
skip_connection_count: true
skip_procs_count: true
temperature: false
tls: %v
use_gitee_to_upgrade: false
use_ipv6_country_code: false
uuid: %s
`,
                    state.Config.NezhaKey,
                    state.Config.NezhaServer,
                    tlsFlag,
                    state.Config.UUID,
                )
                yamlPath := filepath.Join(filePath, "config.yaml")
                os.WriteFile(yamlPath, []byte(yamlContent), 0644)
                cmd := exec.Command(v1Dest, "-c", yamlPath)
                cmd.Stdout = nil
                cmd.Stderr = nil
                if err := cmd.Start(); err != nil {
                    logError("启动 v1 失败: " + err.Error())
                } else {
                    logInfo("v1 启动")
                }
            }
        }
        time.Sleep(1 * time.Second)
    }

    // 启动 xray
    if _, err := os.Stat(webDest); err == nil {
        configPath := filepath.Join(filePath, "config.json")
        cmd := exec.Command(webDest, "-c", configPath)
        cmd.Stdout = nil
        cmd.Stderr = nil
        if err := cmd.Start(); err != nil {
            logError("启动 xray 失败: " + err.Error())
        } else {
            logInfo("xray 启动")
        }
        time.Sleep(1 * time.Second)
    }

    // 启动 cloudflared
    if _, err := os.Stat(botDest); err == nil {
        cmd := exec.Command(botDest)
        if strings.HasPrefix(state.Config.ArgoAuth, "TunnelSecret") {
            tunnelJsonPath := filepath.Join(filePath, "tunnel.json")
            os.WriteFile(tunnelJsonPath, []byte(state.Config.ArgoAuth), 0644)
            var tunnelID string
            var jsonData map[string]interface{}
            if err := json.Unmarshal([]byte(state.Config.ArgoAuth), &jsonData); err == nil {
                if tid, ok := jsonData["TunnelSecret"]; ok {
                    if m, ok := tid.(map[string]interface{}); ok {
                        if id, ok := m["TunnelID"]; ok {
                            tunnelID = id.(string)
                        }
                    }
                }
            }
            tunnelYaml := fmt.Sprintf(`tunnel: %s
credentials-file: %s
protocol: http2
ingress:
  - hostname: %s
    service: http://localhost:%d
    originRequest:
      noTLSVerify: true
  - service: http_status:404
`,
                tunnelID,
                tunnelJsonPath,
                state.Config.ArgoDomain,
                state.Config.ArgoPort,
            )
            yamlPath := filepath.Join(filePath, "tunnel.yml")
            os.WriteFile(yamlPath, []byte(tunnelYaml), 0644)
            cmd.Args = append(cmd.Args, "tunnel",
                "--edge-ip-version", "auto",
                "--no-autoupdate",
                "--protocol", "http2",
                "--config", yamlPath,
                "run",
            )
        } else if len(state.Config.ArgoAuth) > 120 {
            cmd.Args = append(cmd.Args, "tunnel",
                "--edge-ip-version", "auto",
                "--no-autoupdate",
                "--protocol", "http2",
                "run",
                "--token", state.Config.ArgoAuth,
            )
        } else {
            logPath := filepath.Join(filePath, "boot.log")
            cmd.Args = append(cmd.Args, "tunnel",
                "--edge-ip-version", "auto",
                "--no-autoupdate",
                "--protocol", "http2",
                "--logfile", logPath,
                "--loglevel", "info",
                "--url", fmt.Sprintf("http://localhost:%d", state.Config.ArgoPort),
            )
        }
        cmd.Stdout = nil
        cmd.Stderr = nil
        if err := cmd.Start(); err != nil {
            logError("启动 cloudflared 失败: " + err.Error())
        } else {
            logInfo("cloudflared 启动")
        }
        time.Sleep(2 * time.Second)
    }
    return nil
}

// ========== 提取 Argo 域名 ==========
func extractArgoDomain(state *AppState) string {
    if state.Config.ArgoDomain != "" && state.Config.ArgoAuth != "" {
        return state.Config.ArgoDomain
    }
    logPath := filepath.Join(state.Config.FilePath, "boot.log")
    if data, err := os.ReadFile(logPath); err == nil {
        scanner := bufio.NewScanner(strings.NewReader(string(data)))
        for scanner.Scan() {
            line := scanner.Text()
            if strings.Contains(line, "trycloudflare.com") {
                parts := strings.Fields(line)
                for _, part := range parts {
                    if strings.Contains(part, "trycloudflare.com") {
                        if strings.HasPrefix(part, "https://") {
                            domain := strings.TrimPrefix(part, "https://")
                            domain = strings.TrimSuffix(domain, "/")
                            return domain
                        }
                    }
                }
            }
        }
    }
    return "localhost"
}

// ========== 获取证书指纹 ==========
func getCertFingerprint(certPath string) (string, error) {
    data, err := os.ReadFile(certPath)
    if err != nil {
        return "", err
    }
    block, _ := pem.Decode(data)
    if block == nil {
        return "", fmt.Errorf("failed to parse PEM")
    }
    cert, err := x509.ParseCertificate(block.Bytes)
    if err != nil {
        return "", err
    }
    hash := sha256.Sum256(cert.Raw)
    hex := fmt.Sprintf("%x", hash)
    var b strings.Builder
    for i, c := range hex {
        if i > 0 && i%2 == 0 {
            b.WriteByte(':')
        }
        b.WriteByte(byte(c))
    }
    return strings.ToUpper(b.String()), nil
}

// ========== 获取 Meta 信息 ==========
func getMetaInfo(client *http.Client) (string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    req, _ := http.NewRequestWithContext(ctx, "GET", "http://ip-api.com/json", nil)
    resp, err := client.Do(req)
    if err == nil && resp.StatusCode == http.StatusOK {
        defer resp.Body.Close()
        var data map[string]interface{}
        if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
            if status, ok := data["status"].(string); ok && status == "success" {
                country := data["countryCode"].(string)
                isp := data["isp"].(string)
                isp = strings.ReplaceAll(isp, " ", "_")
                return country + "-" + isp, nil
            }
        }
    }
    // 备用 api.ip.sb
    req2, _ := http.NewRequestWithContext(ctx, "GET", "https://api.ip.sb/geoip", nil)
    resp2, err := client.Do(req2)
    if err == nil && resp2.StatusCode == http.StatusOK {
        defer resp2.Body.Close()
        var data map[string]interface{}
        if err := json.NewDecoder(resp2.Body).Decode(&data); err == nil {
            country := data["country_code"].(string)
            isp := data["isp"].(string)
            isp = strings.ReplaceAll(isp, " ", "_")
            return country + "-" + isp, nil
        }
    }
    return "Unknown", nil
}

// ========== 获取服务器公网 IP ==========
func getServerIP(client *http.Client) (string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    urls := []string{"https://ipv4.ip.sb", "https://api.ipify.org"}
    for _, u := range urls {
        req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
        resp, err := client.Do(req)
        if err == nil && resp.StatusCode == http.StatusOK {
            defer resp.Body.Close()
            body, err := io.ReadAll(resp.Body)
            if err == nil {
                ip := strings.TrimSpace(string(body))
                if ip != "" {
                    return ip, nil
                }
            }
        }
    }
    // IPv6
    req, _ := http.NewRequestWithContext(ctx, "GET", "https://ipv6.ip.sb", nil)
    resp, err := client.Do(req)
    if err == nil && resp.StatusCode == http.StatusOK {
        defer resp.Body.Close()
        body, err := io.ReadAll(resp.Body)
        if err == nil {
            ip := strings.TrimSpace(string(body))
            if ip != "" {
                return "[" + ip + "]", nil
            }
        }
    }
    return "127.0.0.1", nil
}

// ========== 生成订阅内容 ==========
func generateSubscription(state *AppState, argoDomain string) (string, error) {
    meta, _ := getMetaInfo(state.HTTPClient)
    nodeName := meta
    if state.Config.Name != "" {
        nodeName = state.Config.Name + "-" + meta
    }
    serverIP, _ := getServerIP(state.HTTPClient)

    var lines []string

    lines = append(lines, fmt.Sprintf(
        "vless://%s@%s:%d?encryption=none&security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Fvless-argo%%3Fed%%3D2560#%s",
        state.Config.UUID, state.Config.CFIP, state.Config.CFPort, argoDomain, argoDomain, nodeName,
    ))

    vmess := map[string]interface{}{
        "v":   "2",
        "ps":  nodeName,
        "add": state.Config.CFIP,
        "port": state.Config.CFPort,
        "id":  state.Config.UUID,
        "aid": "0",
        "scy": "auto",
        "net": "ws",
        "type": "none",
        "host": argoDomain,
        "path": "/vmess-argo?ed=2560",
        "tls": "tls",
        "sni": argoDomain,
        "alpn": "",
        "fp": "firefox",
    }
    vmessJSON, _ := json.Marshal(vmess)
    vmessB64 := base64.StdEncoding.EncodeToString(vmessJSON)
    lines = append(lines, "vmess://"+vmessB64)

    lines = append(lines, fmt.Sprintf(
        "trojan://%s@%s:%d?security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Ftrojan-argo%%3Fed%%3D2560#%s",
        state.Config.UUID, state.Config.CFIP, state.Config.CFPort, argoDomain, argoDomain, nodeName,
    ))

    if isValidPort(state.Config.HY2Port) {
        certPath := filepath.Join(state.Config.FilePath, "cert.pem")
        fingerprint, err := getCertFingerprint(certPath)
        pin := ""
        if err == nil && fingerprint != "" {
            pin = "&pinSHA256=" + fingerprint
        }
        lines = append(lines, fmt.Sprintf(
            "hysteria2://%s@%s:%s?sni=www.bing.com&insecure=0&alpn=h3&obfs=none%s#%s",
            state.Config.UUID, serverIP, state.Config.HY2Port, pin, nodeName,
        ))
    }

    if isValidPort(state.Config.RealityPort) {
        pubPath := filepath.Join(state.Config.FilePath, "public_key.txt")
        pubKey := ""
        if data, err := os.ReadFile(pubPath); err == nil {
            pubKey = strings.TrimSpace(string(data))
        }
        if pubKey == "" {
            _, pub, _ := generateOrLoadKeypair(state.Config.FilePath)
            pubKey = pub
        }
        lines = append(lines, fmt.Sprintf(
            "vless://%s@%s:%s?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.iij.ad.jp&fp=firefox&pbk=%s&type=tcp&headerType=none#%s",
            state.Config.UUID, serverIP, state.Config.RealityPort, pubKey, nodeName,
        ))
    }

    if isValidPort(state.Config.S5Port) {
        auth := base64.StdEncoding.EncodeToString([]byte(state.Config.UUID[:8] + ":" + state.Config.UUID[12:]))
        lines = append(lines, fmt.Sprintf(
            "socks://%s@%s:%s#%s",
            auth, serverIP, state.Config.S5Port, nodeName,
        ))
    }

    subText := strings.Join(lines, "\n")
    subB64 := base64.StdEncoding.EncodeToString([]byte(subText))
    subPath := filepath.Join(state.Config.FilePath, "sub.txt")
    os.WriteFile(subPath, []byte(subB64), 0644)
    listPath := filepath.Join(state.Config.FilePath, "list.txt")
    os.WriteFile(listPath, []byte(subText), 0644)
    return subB64, nil
}

// ========== 上传节点/订阅 ==========
func uploadNodes(state *AppState) error {
    if state.Config.UploadURL == "" {
        return nil
    }
    if state.Config.ProjectURL != "" {
        subURL := state.Config.ProjectURL + "/" + state.Config.SubPath
        payload := map[string]interface{}{"subscription": []string{subURL}}
        jsonData, _ := json.Marshal(payload)
        resp, err := state.HTTPClient.Post(state.Config.UploadURL+"/api/add-subscriptions", "application/json", bytes.NewReader(jsonData))
        if err != nil {
            return err
        }
        defer resp.Body.Close()
        if resp.StatusCode == http.StatusOK {
            logInfo("订阅上传成功")
        } else {
            logWarn("订阅上传失败: " + resp.Status)
        }
        return nil
    }
    listPath := filepath.Join(state.Config.FilePath, "list.txt")
    if _, err := os.Stat(listPath); os.IsNotExist(err) {
        return nil
    }
    data, err := os.ReadFile(listPath)
    if err != nil {
        return err
    }
    lines := strings.Split(string(data), "\n")
    var nodes []string
    for _, line := range lines {
        if strings.Contains(line, "://") {
            nodes = append(nodes, line)
        }
    }
    if len(nodes) == 0 {
        return nil
    }
    payload := map[string]interface{}{"nodes": nodes}
    jsonData, _ := json.Marshal(payload)
    resp, err := state.HTTPClient.Post(state.Config.UploadURL+"/api/add-nodes", "application/json", bytes.NewReader(jsonData))
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode == http.StatusOK {
        logInfo("节点上传成功")
    } else {
        logWarn("节点上传失败: " + resp.Status)
    }
    return nil
}

// ========== Telegram 推送 ==========
func escapeMarkdownV2(text string) string {
    special := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "=", "|", "{", "}", ".", "!", "-", "\\"}
    for _, s := range special {
        text = strings.ReplaceAll(text, s, "\\"+s)
    }
    return text
}

func sendTelegram(state *AppState) error {
    if state.Config.BotToken == "" || state.Config.ChatID == "" {
        return nil
    }
    subPath := filepath.Join(state.Config.FilePath, "sub.txt")
    if _, err := os.Stat(subPath); os.IsNotExist(err) {
        return nil
    }
    data, err := os.ReadFile(subPath)
    if err != nil {
        return err
    }
    content := string(data)
    escapedName := escapeMarkdownV2(state.Config.Name)
    text := fmt.Sprintf("**%s节点推送**\n```\n%s\n```", escapedName, content)
    params := url.Values{}
    params.Set("chat_id", state.Config.ChatID)
    params.Set("text", text)
    params.Set("parse_mode", "MarkdownV2")
    resp, err := state.HTTPClient.PostForm("https://api.telegram.org/bot"+state.Config.BotToken+"/sendMessage", params)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode == http.StatusOK {
        logInfo("Telegram 推送成功")
    } else {
        logWarn("Telegram 推送失败: " + resp.Status)
    }
    return nil
}

// ========== 自动访问任务 ==========
func addVisitTask(state *AppState) error {
    if !state.Config.AutoAccess || state.Config.ProjectURL == "" {
        return nil
    }
    payload := map[string]interface{}{"url": state.Config.ProjectURL}
    jsonData, _ := json.Marshal(payload)
    resp, err := state.HTTPClient.Post("https://oooo.serv00.net/add-url", "application/json", bytes.NewReader(jsonData))
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode == http.StatusOK {
        logInfo("自动访问任务添加成功")
    } else {
        logWarn("自动访问任务添加失败: " + resp.Status)
    }
    return nil
}

// ========== 延迟清理任务 ==========
func cleanupTask(state *AppState) {
    time.Sleep(90 * time.Second)
    files := []string{
        "boot.log", "config.json", "web", "bot", "list.txt",
        "cert.pem", "private.key", "agent", "v1",
        "tunnel.json", "tunnel.yml", "config.yaml",
        "key.txt", "public_key.txt",
    }
    for _, f := range files {
        path := filepath.Join(state.Config.FilePath, f)
        os.Remove(path)
    }
    fmt.Print("\x1B[2J\x1B[1;1H")
    logInfo("App is running")
}

// ========== HTTP 服务器 ==========
func startHTTPServer(state *AppState) {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        data, err := os.ReadFile("index.html")
        if err != nil {
            w.Header().Set("Content-Type", "text/html; charset=utf-8")
            fmt.Fprint(w, "Hello world!<br><br>You can access /sub to get your nodes!")
            return
        }
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.Write(data)
    })
    http.HandleFunc("/"+state.Config.SubPath, func(w http.ResponseWriter, r *http.Request) {
        state.mu.RLock()
        content := state.SubContent
        state.mu.RUnlock()
        if content == "" {
            w.WriteHeader(http.StatusServiceUnavailable)
            fmt.Fprint(w, "Subscription content not yet available")
            return
        }
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        fmt.Fprint(w, content)
    })
    addr := fmt.Sprintf(":%d", state.Config.Port)
    logInfo("HTTP 服务器监听 " + addr)
    if err := http.ListenAndServe(addr, nil); err != nil {
        logError("HTTP 服务器启动失败: " + err.Error())
    }
}

// ========== 主函数 ==========
func main() {
    cfg := loadConfig()
    showLog = cfg.ShowLog
    if !showLog {
        log.SetOutput(io.Discard)
    }
    if err := ensureDir(cfg.FilePath); err != nil {
        log.Fatal("创建目录失败: ", err)
    }
    state := NewAppState(cfg)

    // 1. 删除旧节点
    if err := deleteNodes(state); err != nil {
        logError("删除旧节点失败: " + err.Error())
    }

    // 2. 清理旧文件
    if err := cleanupOldFiles(cfg.FilePath); err != nil {
        logError("清理旧文件失败: " + err.Error())
    }

    // 3. 生成配置
    if err := generateConfig(state); err != nil {
        logError("生成配置失败: " + err.Error())
    }

    // 4. 下载并运行二进制
    if err := downloadAndRun(state); err != nil {
        logError("下载运行失败: " + err.Error())
    }

    // 5. 提取 Argo 域名
    argoDomain := extractArgoDomain(state)
    logInfo("Argo 域名: " + argoDomain)

    // 6. 生成订阅
    subContent, err := generateSubscription(state, argoDomain)
    if err != nil {
        logError("生成订阅失败: " + err.Error())
    } else {
        state.mu.Lock()
        state.SubContent = subContent
        state.mu.Unlock()
    }

    // 7. 上传节点
    if err := uploadNodes(state); err != nil {
        logError("上传节点失败: " + err.Error())
    }

    // 8. Telegram 推送
    if err := sendTelegram(state); err != nil {
        logError("Telegram 推送失败: " + err.Error())
    }

    // 9. 自动访问任务
    if err := addVisitTask(state); err != nil {
        logError("添加自动访问任务失败: " + err.Error())
    }

    // 10. 延迟清理
    go cleanupTask(state)

    // 11. HTTP 服务器（阻塞）
    startHTTPServer(state)
}
