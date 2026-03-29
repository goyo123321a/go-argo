package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// 版本信息
var (
	Version   = "dev"
	BuildDate = "unknown"
)

// 环境变量配置
var (
	uploadURL   = getEnv("UPLOAD_URL", "")
	projectURL  = getEnv("PROJECT_URL", "")
	autoAccess  = getEnvBool("AUTO_ACCESS", false)
	filePath    = getEnv("FILE_PATH", "./tmp")
	subPath     = getEnv("SUB_PATH", "sub")
	port        = getEnvInt("SERVER_PORT", 7860)
	uuid        = getEnv("UUID", "9afd1229-b893-40c1-84dd-51e7ce204913")
	nezhaServer = getEnv("NEZHA_SERVER", "")
	nezhaPort   = getEnv("NEZHA_PORT", "")
	nezhaKey    = getEnv("NEZHA_KEY", "")
	argoDomain  = getEnv("ARGO_DOMAIN", "")
	argoAuth    = getEnv("ARGO_AUTH", "")
	argoPort    = getEnvInt("ARGO_PORT", 8001)
	cfip        = getEnv("CFIP", "cf.877774.xyz")
	cfport      = getEnvInt("CFPORT", 443)
	name        = getEnv("NAME", "")
)

// 全局变量
var (
	npmName      = generateRandomName()
	webName      = generateRandomName()
	botName      = generateRandomName()
	phpName      = generateRandomName()
	npmPath      string
	phpPath      string
	webPath      string
	botPath      string
	subFilePath  string
	listFilePath string
	bootLogPath  string
	configPath   string

	processes    []*os.Process
	processMutex sync.Mutex
	httpServer   *http.Server
	subContent   string
	subContentMu sync.RWMutex
	subReady     bool
	subReadyMu   sync.RWMutex
)

// ========== Xray 配置结构体（Linux） ==========
type XrayConfig struct {
	Log       XrayLog        `json:"log"`
	Inbounds  []XrayInbound  `json:"inbounds"`
	DNS       XrayDNS        `json:"dns"`
	Outbounds []XrayOutbound `json:"outbounds"`
}

type XrayLog struct {
	Access   string `json:"access"`
	Error    string `json:"error"`
	Loglevel string `json:"loglevel"`
}

type XrayInbound struct {
	Port           int                `json:"port"`
	Listen         string             `json:"listen,omitempty"`
	Protocol       string             `json:"protocol"`
	Settings       interface{}        `json:"settings"`
	StreamSettings XrayStreamSettings `json:"streamSettings"`
	Sniffing       *XraySniffing      `json:"sniffing,omitempty"`
}

type XrayStreamSettings struct {
	Network    string          `json:"network"`
	Security   string          `json:"security,omitempty"`
	WSSettings *XrayWSSettings `json:"wsSettings,omitempty"`
}

type XrayWSSettings struct {
	Path string `json:"path"`
}

type XraySniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride"`
	MetadataOnly bool     `json:"metadataOnly"`
}

type XrayDNS struct {
	Servers []string `json:"servers"`
}

type XrayOutbound struct {
	Protocol string      `json:"protocol"`
	Tag      string      `json:"tag"`
	Settings interface{} `json:"settings,omitempty"`
}

type VlessSettings struct {
	Clients    []VlessClient `json:"clients"`
	Decryption string        `json:"decryption"`
	Fallbacks  []Fallback    `json:"fallbacks,omitempty"`
}

type VlessClient struct {
	ID    string `json:"id"`
	Flow  string `json:"flow,omitempty"`
	Level int    `json:"level,omitempty"`
}

type Fallback struct {
	Dest int    `json:"dest,omitempty"`
	Path string `json:"path,omitempty"`
}

type VmessSettings struct {
	Clients []VmessClient `json:"clients"`
}

type VmessClient struct {
	ID      string `json:"id"`
	AlterID int    `json:"alterId"`
}

type TrojanSettings struct {
	Clients []TrojanClient `json:"clients"`
}

type TrojanClient struct {
	Password string `json:"password"`
}

// ========== Sing-box 配置结构体（FreeBSD） ==========
type SingBoxConfig struct {
	Log       SingBoxLog        `json:"log"`
	DNS       SingBoxDNS        `json:"dns"`
	Inbounds  []SingBoxInbound  `json:"inbounds"`
	Outbounds []SingBoxOutbound `json:"outbounds"`
	Route     SingBoxRoute      `json:"route"`
}

type SingBoxLog struct {
	Level     string `json:"level"`
	Timestamp bool   `json:"timestamp"`
}

type SingBoxDNS struct {
	Servers []SingBoxDNSServer `json:"servers"`
}

type SingBoxDNSServer struct {
	Address         string `json:"address"`
	AddressResolver string `json:"address_resolver,omitempty"`
	Tag             string `json:"tag,omitempty"`
}

type SingBoxInbound struct {
	Type       string            `json:"type"`
	Tag        string            `json:"tag"`
	Listen     string            `json:"listen,omitempty"`
	ListenPort int               `json:"listen_port"`
	Users      []SingBoxUser     `json:"users"`
	Transport  *SingBoxTransport `json:"transport,omitempty"`
	Sniffing   *SingBoxSniffing  `json:"sniffing,omitempty"`
}

type SingBoxUser struct {
	UUID string `json:"uuid"`
	Flow string `json:"flow,omitempty"`
}

type SingBoxTransport struct {
	Type                string `json:"type"`
	Path                string `json:"path,omitempty"`
	EarlyDataHeaderName string `json:"early_data_header_name,omitempty"`
	MaxEarlyData        int    `json:"max_early_data,omitempty"`
}

type SingBoxSniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"dest_override"`
}

type SingBoxOutbound struct {
	Type          string   `json:"type"`
	Tag           string   `json:"tag"`
	Server        string   `json:"server,omitempty"`
	ServerPort    int      `json:"server_port,omitempty"`
	LocalAddress  []string `json:"local_address,omitempty"`
	PrivateKey    string   `json:"private_key,omitempty"`
	PeerPublicKey string   `json:"peer_public_key,omitempty"`
	Reserved      []int    `json:"reserved,omitempty"`
}

type SingBoxRoute struct {
	Final string        `json:"final"`
	Rules []SingBoxRule `json:"rules"`
}

type SingBoxRule struct {
	Protocol []string `json:"protocol,omitempty"`
	Outbound string   `json:"outbound"`
}

// ========== 辅助函数 ==========
func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		var i int
		fmt.Sscanf(v, "%d", &i)
		return i
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "true" || v == "1" || v == "True" || v == "TRUE"
	}
	return defaultValue
}

func generateRandomName() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = chars[randInt(len(chars))]
	}
	return string(b)
}

func randInt(n int) int {
	if n <= 0 {
		return 0
	}
	b := make([]byte, 1)
	if _, err := rand.Read(b); err != nil {
		return 0
	}
	return int(b[0]) % n
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func initPaths() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal("获取工作目录失败:", err)
	}
	absFilePath := filePath
	if !filepath.IsAbs(filePath) {
		absFilePath = filepath.Join(cwd, filePath)
	}
	if err := ensureDir(absFilePath); err != nil {
		log.Fatal("创建目录失败:", err)
	}
	filePath = absFilePath

	npmPath = filepath.Join(filePath, npmName)
	phpPath = filepath.Join(filePath, phpName)
	webPath = filepath.Join(filePath, webName)
	botPath = filepath.Join(filePath, botName)
	subFilePath = filepath.Join(filePath, "sub.txt")
	listFilePath = filepath.Join(filePath, "list.txt")
	bootLogPath = filepath.Join(filePath, "boot.log")
	configPath = filepath.Join(filePath, "config.json")
}

func getSystemOS() string { return runtime.GOOS }
func getSystemArchitecture() string {
	switch runtime.GOARCH {
	case "arm", "arm64", "aarch64":
		return "arm64"
	default:
		return "amd64"
	}
}

// ========== Xray 配置生成（Linux） ==========
func generateXrayConfig() error {
	config := XrayConfig{
		Log: XrayLog{Access: "/dev/null", Error: "/dev/null", Loglevel: "none"},
		DNS: XrayDNS{Servers: []string{"https+local://8.8.8.8/dns-query"}},
		Inbounds: []XrayInbound{
			{
				Port:     argoPort,
				Protocol: "vless",
				Settings: VlessSettings{
					Clients:    []VlessClient{{ID: uuid, Flow: "xtls-rprx-vision"}},
					Decryption: "none",
					Fallbacks: []Fallback{
						{Dest: 3001},
						{Path: "/vless-argo", Dest: 3002},
						{Path: "/vmess-argo", Dest: 3003},
						{Path: "/trojan-argo", Dest: 3004},
					},
				},
				StreamSettings: XrayStreamSettings{Network: "tcp"},
			},
			{
				Port:     3001,
				Listen:   "127.0.0.1",
				Protocol: "vless",
				Settings: VlessSettings{Clients: []VlessClient{{ID: uuid}}, Decryption: "none"},
				StreamSettings: XrayStreamSettings{Network: "tcp", Security: "none"},
			},
			{
				Port:     3002,
				Listen:   "127.0.0.1",
				Protocol: "vless",
				Settings: VlessSettings{Clients: []VlessClient{{ID: uuid, Level: 0}}, Decryption: "none"},
				StreamSettings: XrayStreamSettings{
					Network:    "ws",
					Security:   "none",
					WSSettings: &XrayWSSettings{Path: "/vless-argo"},
				},
				Sniffing: &XraySniffing{Enabled: true, DestOverride: []string{"http", "tls", "quic"}},
			},
			{
				Port:     3003,
				Listen:   "127.0.0.1",
				Protocol: "vmess",
				Settings: VmessSettings{Clients: []VmessClient{{ID: uuid, AlterID: 0}}},
				StreamSettings: XrayStreamSettings{
					Network:    "ws",
					WSSettings: &XrayWSSettings{Path: "/vmess-argo"},
				},
				Sniffing: &XraySniffing{Enabled: true, DestOverride: []string{"http", "tls", "quic"}},
			},
			{
				Port:     3004,
				Listen:   "127.0.0.1",
				Protocol: "trojan",
				Settings: TrojanSettings{Clients: []TrojanClient{{Password: uuid}}},
				StreamSettings: XrayStreamSettings{
					Network:    "ws",
					Security:   "none",
					WSSettings: &XrayWSSettings{Path: "/trojan-argo"},
				},
				Sniffing: &XraySniffing{Enabled: true, DestOverride: []string{"http", "tls", "quic"}},
			},
		},
		Outbounds: []XrayOutbound{
			{Protocol: "freedom", Tag: "direct", Settings: map[string]interface{}{"domainStrategy": "UseIP"}},
			{Protocol: "blackhole", Tag: "block"},
		},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// ========== Sing-box 配置生成（FreeBSD） ==========
func generateSingBoxConfig() error {
	log.Printf("生成 Sing-box 配置，监听端口: %d", argoPort)

	if argoPort <= 0 || argoPort > 65535 {
		return fmt.Errorf("无效的端口号: %d", argoPort)
	}
	if uuid == "" {
		return fmt.Errorf("UUID 不能为空")
	}

	config := SingBoxConfig{
		Log: SingBoxLog{
			Level:     "error",
			Timestamp: true,
		},
		DNS: SingBoxDNS{
			Servers: []SingBoxDNSServer{
				{
					Address:         "8.8.8.8",
					AddressResolver: "local",
				},
				{
					Tag:     "local",
					Address: "local",
				},
			},
		},
		Inbounds: []SingBoxInbound{
			{
				Type:       "vless",
				Tag:        "vless-ws-in",
				Listen:     "::",
				ListenPort: argoPort,
				Users: []SingBoxUser{
					{
						UUID: uuid,
						Flow: "",
					},
				},
				Transport: &SingBoxTransport{
					Type:                "ws",
					Path:                "/vless-argo",
					EarlyDataHeaderName: "Sec-WebSocket-Protocol",
					MaxEarlyData:        2560,
				},
				Sniffing: &SingBoxSniffing{
					Enabled:      true,
					DestOverride: []string{"http", "tls", "quic"},
				},
			},
		},
		Outbounds: []SingBoxOutbound{
			{
				Type: "direct",
				Tag:  "direct",
			},
			{
				Type: "block",
				Tag:  "block",
			},
		},
		Route: SingBoxRoute{
			Final: "direct",
			Rules: []SingBoxRule{
				{
					Protocol: []string{"dns"},
					Outbound: "direct",
				},
			},
		},
	}

	// 只在环境变量明确设置时才添加 WireGuard
	if useWireGuard := getEnvBool("USE_WIREGUARD", false); useWireGuard {
		log.Printf("添加 WireGuard 出站配置")
		config.Outbounds = append(config.Outbounds, SingBoxOutbound{
			Type:          "wireguard",
			Tag:           "wireguard-out",
			Server:        getEnv("WG_SERVER", "162.159.192.200"),
			ServerPort:    getEnvInt("WG_PORT", 4500),
			LocalAddress:  []string{getEnv("WG_IPV4", "172.16.0.2/32"), getEnv("WG_IPV6", "2606:4700:110:8f77:1ca9:f086:846c:5f9e/128")},
			PrivateKey:    getEnv("WG_PRIVATE_KEY", ""),
			PeerPublicKey: getEnv("WG_PUBLIC_KEY", "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo="),
			Reserved:      []int{126, 246, 173},
		})
		config.Route.Rules = append(config.Route.Rules, SingBoxRule{
			Protocol: []string{"google", "youtube", "spotify"},
			Outbound: "wireguard-out",
		})
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	log.Printf("✓ Sing-box 配置已生成: %s", configPath)
	log.Printf("  协议: vless + WebSocket")
	log.Printf("  监听: [::]:%d (IPv6/IPv4)", argoPort)
	log.Printf("  路径: /vless-argo")
	log.Printf("  Early Data: 2560")

	return nil
}

// 统一配置生成入口
func generateConfig() error {
	if runtime.GOOS == "freebsd" {
		return generateSingBoxConfig()
	}
	return generateXrayConfig()
}

// ========== 文件下载与管理 ==========
func downloadFile(filePath, fileURL string) error {
	maxRetries := 3
	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			log.Printf("重试下载 (%d/%d): %s", retry+1, maxRetries, fileURL)
			time.Sleep(3 * time.Second)
		}
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Get(fileURL)
		if err != nil {
			log.Printf("下载请求失败: %v", err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			log.Printf("HTTP状态码错误: %d", resp.StatusCode)
			continue
		}
		tempFile := filePath + ".tmp"
		out, err := os.Create(tempFile)
		if err != nil {
			resp.Body.Close()
			log.Printf("创建临时文件失败: %v", err)
			continue
		}
		_, err = io.Copy(out, resp.Body)
		out.Close()
		resp.Body.Close()
		if err != nil {
			log.Printf("下载数据失败: %v", err)
			os.Remove(tempFile)
			continue
		}
		info, err := os.Stat(tempFile)
		if err != nil || info.Size() < 1024 {
			log.Printf("下载文件太小 (%d bytes)，可能不是有效二进制文件", info.Size())
			os.Remove(tempFile)
			continue
		}
		if err := os.Rename(tempFile, filePath); err != nil {
			log.Printf("重命名文件失败: %v", err)
			os.Remove(tempFile)
			continue
		}
		if err := os.Chmod(filePath, 0775); err != nil {
			log.Printf("设置权限失败: %v", err)
			os.Remove(filePath)
			continue
		}
		if !isExecutable(filePath) {
			log.Printf("文件不是有效的可执行文件")
			os.Remove(filePath)
			continue
		}
		log.Printf("✓ 下载成功: %s (%.2f MB)", filePath, float64(info.Size())/1024/1024)
		return nil
	}
	return fmt.Errorf("下载失败，已重试 %d 次", maxRetries)
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.Size() < 1024 {
		return false
	}
	if info.Mode()&0111 == 0 {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	header := make([]byte, 4)
	n, err := f.Read(header)
	if err != nil || n < 4 {
		return false
	}
	return header[0] == 0x7F && header[1] == 0x45 && header[2] == 0x4C && header[3] == 0x46
}

func getFilesForArchitecture(arch string) []struct {
	path string
	url  string
} {
	var files []struct{ path string; url string }
	osName := getSystemOS()

	if osName == "freebsd" {
		baseURL := "https://github.com/eooce/test/releases/download/freebsd"
		if arch == "arm64" {
			baseURL = "https://github.com/eooce/test/releases/download/freebsd-arm64"
		}
		files = append(files, struct{ path string; url string }{webPath, baseURL + "/sb"})
		files = append(files, struct{ path string; url string }{botPath, baseURL + "/server"})
		if nezhaServer != "" && nezhaKey != "" {
			if nezhaPort != "" {
				files = append(files, struct{ path string; url string }{npmPath, baseURL + "/npm"})
			} else {
				files = append(files, struct{ path string; url string }{phpPath, baseURL + "/v1"})
			}
		}
	} else {
		if arch == "arm64" {
			files = append(files, struct{ path string; url string }{webPath, "https://arm64.ssss.nyc.mn/web"})
			files = append(files, struct{ path string; url string }{botPath, "https://arm64.ssss.nyc.mn/bot"})
		} else {
			files = append(files, struct{ path string; url string }{webPath, "https://amd64.ssss.nyc.mn/web"})
			files = append(files, struct{ path string; url string }{botPath, "https://amd64.ssss.nyc.mn/bot"})
		}
		if nezhaServer != "" && nezhaKey != "" {
			if nezhaPort != "" {
				url := "https://amd64.ssss.nyc.mn/agent"
				if arch == "arm64" {
					url = "https://arm64.ssss.nyc.mn/agent"
				}
				files = append([]struct{ path string; url string }{{npmPath, url}}, files...)
			} else {
				url := "https://amd64.ssss.nyc.mn/v1"
				if arch == "arm64" {
					url = "https://arm64.ssss.nyc.mn/v1"
				}
				files = append([]struct{ path string; url string }{{phpPath, url}}, files...)
			}
		}
	}
	return files
}

// ========== 核心服务运行 ==========
func runCore() error {
	if !fileExists(webPath) {
		return fmt.Errorf("core binary not found: %s", webPath)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "freebsd" {
		cmd = exec.Command(webPath, "run", "-c", configPath)
		log.Printf("启动 Sing-box: %s run -c %s", webPath, configPath)
	} else {
		cmd = exec.Command(webPath, "-c", configPath)
		log.Printf("启动 Xray: %s -c %s", webPath, configPath)
	}

	cmd.Dir = filePath
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return err
	}

	processMutex.Lock()
	processes = append(processes, cmd.Process)
	processMutex.Unlock()

	time.Sleep(2 * time.Second)

	log.Printf("✓ 核心服务已启动 (PID: %d)", cmd.Process.Pid)
	return nil
}

func runCloudflared() error {
	if !fileExists(botPath) {
		log.Printf("Cloudflared 二进制文件不存在: %s", botPath)
		return nil
	}

	args := []string{"tunnel", "--edge-ip-version", "auto", "--no-autoupdate", "--protocol", "http2"}

	if argoAuth != "" && len(argoAuth) >= 120 && len(argoAuth) <= 250 {
		args = append(args, "run", "--token", argoAuth)
	} else if argoAuth != "" && strings.Contains(argoAuth, "TunnelSecret") {
		tunnelYamlPath := filepath.Join(filePath, "tunnel.yml")
		if !fileExists(tunnelYamlPath) {
			time.Sleep(2 * time.Second)
		}
		args = append(args, "--config", tunnelYamlPath, "run")
	} else {
		args = append(args, "--logfile", bootLogPath, "--loglevel", "info",
			"--url", fmt.Sprintf("http://localhost:%d", argoPort))
	}

	cmd := exec.Command(botPath, args...)
	cmd.Dir = filePath

	// 只有当需要解析临时隧道域名时才捕获输出
	if argoAuth == "" || argoDomain == "" {
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Printf("创建 stdout pipe 失败: %v", err)
		} else {
			// 使用 goroutine 读取输出，不需要复杂的退出机制
			go func() {
				buf := make([]byte, 4096)
				for {
					n, err := stdout.Read(buf)
					if n > 0 {
						f, _ := os.OpenFile(bootLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
						if f != nil {
							f.Write(buf[:n])
							f.Close()
						}
					}
					if err != nil {
						break
					}
				}
			}()
		}
	} else {
		cmd.Stdout = nil
		cmd.Stderr = nil
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	processMutex.Lock()
	processes = append(processes, cmd.Process)
	processMutex.Unlock()

	log.Printf("✓ Cloudflared 已启动 (PID: %d)", cmd.Process.Pid)
	return nil
}

func runNezha() error {
	if nezhaServer == "" || nezhaKey == "" {
		return nil
	}

	if nezhaPort == "" {
		if !fileExists(phpPath) {
			return fmt.Errorf("哪吒客户端不存在: %s", phpPath)
		}
		portStr := ""
		if strings.Contains(nezhaServer, ":") {
			parts := strings.Split(nezhaServer, ":")
			portStr = parts[len(parts)-1]
		}
		tlsPorts := map[string]bool{"443": true, "8443": true, "2096": true, "2087": true, "2083": true, "2053": true}
		nezhaTLS := "false"
		if tlsPorts[portStr] {
			nezhaTLS = "true"
		}
		configYaml := fmt.Sprintf(`
client_secret: %s
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
tls: %s
use_gitee_to_upgrade: false
use_ipv6_country_code: false
uuid: %s`, nezhaKey, nezhaServer, nezhaTLS, uuid)

		configYamlPath := filepath.Join(filePath, "config.yaml")
		if err := os.WriteFile(configYamlPath, []byte(configYaml), 0644); err != nil {
			return err
		}

		cmd := exec.Command(phpPath, "-c", configYamlPath)
		cmd.Dir = filePath
		cmd.Stdout = nil
		cmd.Stderr = nil

		if err := cmd.Start(); err != nil {
			return err
		}

		processMutex.Lock()
		processes = append(processes, cmd.Process)
		processMutex.Unlock()

		log.Printf("✓ 哪吒监控(v1)已启动 (PID: %d)", cmd.Process.Pid)
	} else {
		if !fileExists(npmPath) {
			return fmt.Errorf("哪吒客户端不存在: %s", npmPath)
		}
		args := []string{"-s", nezhaServer + ":" + nezhaPort, "-p", nezhaKey}
		tlsPorts := []string{"443", "8443", "2096", "2087", "2083", "2053"}
		for _, p := range tlsPorts {
			if nezhaPort == p {
				args = append(args, "--tls")
				break
			}
		}
		args = append(args, "--disable-auto-update", "--report-delay", "4", "--skip-conn", "--skip-procs")

		cmd := exec.Command(npmPath, args...)
		cmd.Dir = filePath
		cmd.Stdout = nil
		cmd.Stderr = nil

		if err := cmd.Start(); err != nil {
			return err
		}

		processMutex.Lock()
		processes = append(processes, cmd.Process)
		processMutex.Unlock()

		log.Printf("✓ 哪吒监控(agent)已启动 (PID: %d)", cmd.Process.Pid)
	}

	time.Sleep(2 * time.Second)
	return nil
}

func downloadFilesAndRun() error {
	arch := getSystemArchitecture()
	files := getFilesForArchitecture(arch)

	log.Printf("开始下载依赖文件...")
	for _, f := range files {
		if fileExists(f.path) {
			log.Printf("文件已存在: %s", f.path)
			continue
		}
		log.Printf("正在下载: %s", f.url)
		if err := downloadFile(f.path, f.url); err != nil {
			log.Printf("⚠ 下载失败 %s: %v", f.url, err)
			continue
		}
	}

	if !fileExists(webPath) {
		return fmt.Errorf("核心二进制文件不存在: %s", webPath)
	}

	log.Printf("✓ 核心二进制文件已就绪: %s", webPath)

	if err := runNezha(); err != nil {
		log.Printf("⚠ 哪吒监控启动失败: %v", err)
	}

	if err := runCore(); err != nil {
		return fmt.Errorf("代理运行失败: %v", err)
	}

	if err := runCloudflared(); err != nil {
		log.Printf("⚠ Cloudflared启动失败: %v", err)
	}

	time.Sleep(5 * time.Second)

	log.Printf("✓ 所有服务已启动")
	if runtime.GOOS == "freebsd" {
		log.Printf("  Sing-box: %s (监听端口 %d)", webName, argoPort)
	} else {
		log.Printf("  Xray: %s (监听端口 %d)", webName, argoPort)
	}
	log.Printf("  Tunnel: %s", botName)
	if nezhaServer != "" && nezhaKey != "" {
		if nezhaPort != "" {
			log.Printf("  哪吒: %s", npmName)
		} else {
			log.Printf("  哪吒: %s", phpName)
		}
	}

	return nil
}

// ========== Argo 隧道配置 ==========
func argoType() {
	if argoAuth == "" || argoDomain == "" {
		return
	}

	if strings.Contains(argoAuth, "TunnelSecret") {
		tunnelJsonPath := filepath.Join(filePath, "tunnel.json")
		if err := os.WriteFile(tunnelJsonPath, []byte(argoAuth), 0644); err != nil {
			log.Printf("写入 tunnel.json 失败: %v", err)
			return
		}

		var tunnelConfig map[string]interface{}
		if err := json.Unmarshal([]byte(argoAuth), &tunnelConfig); err == nil {
			if tunnelID, ok := tunnelConfig["TunnelID"]; ok {
				tunnelYaml := fmt.Sprintf(`
tunnel: %s
credentials-file: %s
protocol: http2

ingress:
  - hostname: %s
    service: http://localhost:%d
    originRequest:
      noTLSVerify: true
  - service: http_status:404
`, tunnelID, tunnelJsonPath, argoDomain, argoPort)
				tunnelYamlPath := filepath.Join(filePath, "tunnel.yml")
				os.WriteFile(tunnelYamlPath, []byte(tunnelYaml), 0644)
				log.Printf("✓ 固定隧道配置已生成")
			}
		}
	}
}

// ========== 订阅管理 ==========
func getMetaInfo() (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}

	resp, err := client.Get("https://ipapi.co/json/")
	if err == nil {
		defer resp.Body.Close()
		var data map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&data) == nil {
			if countryCode, ok := data["country_code"].(string); ok {
				if org, ok := data["org"].(string); ok {
					return fmt.Sprintf("%s_%s", countryCode, strings.ReplaceAll(org, " ", "_")), nil
				}
			}
		}
	}

	resp, err = client.Get("http://ip-api.com/json/")
	if err == nil {
		defer resp.Body.Close()
		var data map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&data) == nil {
			if status, ok := data["status"].(string); ok && status == "success" {
				countryCode, _ := data["countryCode"].(string)
				org, _ := data["org"].(string)
				if countryCode != "" && org != "" {
					return fmt.Sprintf("%s_%s", countryCode, strings.ReplaceAll(org, " ", "_")), nil
				}
			}
		}
	}

	return "Unknown", nil
}

// 生成订阅链接（统一 VLESS 链接格式）
func generateLinks(argoDomain string) error {
	isp, _ := getMetaInfo()
	nodeName := isp
	if name != "" {
		nodeName = name + "-" + isp
	}

	// 验证必要参数
	if cfip == "" {
		cfip = argoDomain
	}
	if uuid == "" {
		return fmt.Errorf("UUID 不能为空")
	}

	// 统一的 VLESS 链接基础格式
	baseVlessLink := fmt.Sprintf("vless://%s@%s:%d?encryption=none&security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Fvless-argo%%3Fed%%3D2560",
		uuid, cfip, cfport, argoDomain, argoDomain)

	var encoded string

	if runtime.GOOS == "freebsd" {
		// FreeBSD 使用 Sing-box - 只生成 VLESS 节点
		vlessLink := baseVlessLink + "#" + nodeName
		encoded = base64.StdEncoding.EncodeToString([]byte(vlessLink))

		subContentMu.Lock()
		subContent = vlessLink
		subContentMu.Unlock()

		log.Printf("✓ 订阅已生成 (Sing-box - VLESS节点)")
		log.Printf("  链接: vless://%s@%s:%d", uuid[:8]+"...", cfip, cfport)
	} else {
		// Linux 使用 Xray - 生成三个节点
		vlessLink := baseVlessLink + "&flow=xtls-rprx-vision#" + nodeName

		// VMESS 节点
		vmess := map[string]interface{}{
			"v":    "2",
			"ps":   nodeName,
			"add":  cfip,
			"port": cfport,
			"id":   uuid,
			"aid":  "0",
			"net":  "ws",
			"type": "none",
			"host": argoDomain,
			"path": "/vmess-argo?ed=2560",
			"tls":  "tls",
			"sni":  argoDomain,
			"fp":   "firefox",
		}
		vmessJSON, err := json.Marshal(vmess)
		if err != nil {
			return fmt.Errorf("生成 VMESS 配置失败: %v", err)
		}
		vmessBase64 := base64.StdEncoding.EncodeToString(vmessJSON)

		// Trojan 节点
		trojanLink := fmt.Sprintf("trojan://%s@%s:%d?security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Ftrojan-argo%%3Fed%%3D2560#%s",
			uuid, cfip, cfport, argoDomain, argoDomain, nodeName)

		subTxt := strings.Join([]string{vlessLink, "vmess://" + vmessBase64, trojanLink}, "\n\n")
		encoded = base64.StdEncoding.EncodeToString([]byte(subTxt))

		subContentMu.Lock()
		subContent = subTxt
		subContentMu.Unlock()

		log.Printf("✓ 订阅已生成 (Xray - 三节点)")
		log.Printf("  VLESS: %s@%s:%d", uuid[:8]+"...", cfip, cfport)
		log.Printf("  VMESS: %s@%s:%d", uuid[:8]+"...", cfip, cfport)
		log.Printf("  Trojan: %s@%s:%d", uuid[:8]+"...", cfip, cfport)
	}

	// 写入文件
	if err := os.WriteFile(subFilePath, []byte(encoded), 0644); err != nil {
		return fmt.Errorf("保存订阅文件失败: %v", err)
	}

	subReadyMu.Lock()
	subReady = true
	subReadyMu.Unlock()

	log.Printf("  隧道域名: %s", argoDomain)
	log.Printf("  节点名称: %s", nodeName)

	uploadNodes()
	return nil
}

func uploadNodes() {
	if uploadURL == "" {
		return
	}

	if uploadURL != "" && projectURL != "" {
		subscriptionURL := fmt.Sprintf("%s/%s", projectURL, subPath)
		data := map[string][]string{"subscription": {subscriptionURL}}
		jsonData, _ := json.Marshal(data)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(uploadURL+"/api/add-subscriptions", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("上传订阅失败: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			log.Println("✓ 订阅已上传")
		}
	} else if uploadURL != "" && fileExists(listFilePath) {
		content, err := os.ReadFile(listFilePath)
		if err != nil {
			return
		}
		lines := strings.Split(string(content), "\n")
		var nodes []string
		for _, line := range lines {
			if strings.Contains(line, "vless://") || strings.Contains(line, "vmess://") ||
				strings.Contains(line, "trojan://") || strings.Contains(line, "hysteria2://") ||
				strings.Contains(line, "tuic://") {
				nodes = append(nodes, line)
			}
		}
		if len(nodes) == 0 {
			return
		}
		data := map[string][]string{"nodes": nodes}
		jsonData, _ := json.Marshal(data)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(uploadURL+"/api/add-nodes", "application/json", bytes.NewBuffer(jsonData))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				log.Println("✓ 节点已上传")
			}
		}
	}
}

func extractDomains() error {
	if argoAuth != "" && argoDomain != "" {
		return generateLinks(argoDomain)
	}

	time.Sleep(5 * time.Second)
	for i := 0; i < 10; i++ {
		if fileExists(bootLogPath) {
			content, err := os.ReadFile(bootLogPath)
			if err == nil {
				re := regexp.MustCompile(`https?://([^ ]*trycloudflare\.com)/?`)
				matches := re.FindAllStringSubmatch(string(content), -1)
				if len(matches) > 0 && len(matches[0]) > 1 {
					argoDomain := matches[0][1]
					return generateLinks(argoDomain)
				}
			}
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("未能获取到 Argo 隧道域名")
}

// ========== 辅助功能 ==========
func addVisitTask() {
	if !autoAccess || projectURL == "" {
		return
	}
	data := map[string]string{"url": projectURL}
	jsonData, _ := json.Marshal(data)
	client := &http.Client{Timeout: 10 * time.Second}
	client.Post("https://oooo.serv00.net/add-url", "application/json", bytes.NewBuffer(jsonData))
}

func deleteNodes() {
	if uploadURL == "" || !fileExists(subFilePath) {
		return
	}
	content, err := os.ReadFile(subFilePath)
	if err != nil {
		return
	}
	decoded, err := base64.StdEncoding.DecodeString(string(content))
	if err != nil {
		return
	}
	lines := strings.Split(string(decoded), "\n")
	var nodes []string
	for _, line := range lines {
		if strings.Contains(line, "vless://") || strings.Contains(line, "vmess://") ||
			strings.Contains(line, "trojan://") || strings.Contains(line, "hysteria2://") ||
			strings.Contains(line, "tuic://") {
			nodes = append(nodes, line)
		}
	}
	if len(nodes) == 0 {
		return
	}
	data, _ := json.Marshal(map[string][]string{"nodes": nodes})
	client := &http.Client{Timeout: 10 * time.Second}
	client.Post(uploadURL+"/api/delete-nodes", "application/json", bytes.NewBuffer(data))
}

func cleanupOldFiles() {
	files, err := os.ReadDir(filePath)
	if err != nil {
		return
	}
	for _, file := range files {
		if !file.IsDir() {
			os.Remove(filepath.Join(filePath, file.Name()))
		}
	}
}

func cleanFiles() {
	time.AfterFunc(90*time.Second, func() {
		filesToDelete := []string{bootLogPath, configPath}
		if nezhaPort != "" {
			filesToDelete = append(filesToDelete, npmPath)
		} else if nezhaServer != "" && nezhaKey != "" {
			filesToDelete = append(filesToDelete, phpPath)
		}
		for _, file := range filesToDelete {
			if err := os.Remove(file); err == nil {
				log.Printf("已清理: %s", filepath.Base(file))
			}
		}
	})
}

func stopAllProcesses() {
	processMutex.Lock()
	defer processMutex.Unlock()
	for _, proc := range processes {
		if proc != nil {
			if err := proc.Kill(); err != nil {
				log.Printf("停止进程失败: %v", err)
			}
		}
	}
	processes = nil
	log.Println("所有进程已停止")
}

// ========== HTTP 服务 ==========
func startHTTPServer() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head><title>Proxy Service</title></head>
<body>
<h1>Proxy Service Running</h1>
<p>系统: %s/%s</p>
<p>核心: %s</p>
<p>订阅地址: <a href="/%s">/%s</a></p>
<p>下载订阅: <a href="/%s/download">/%s/download</a></p>
<p>原始订阅: <a href="/%s/raw">/%s/raw</a></p>
</body>
</html>`, runtime.GOOS, runtime.GOARCH, getCoreType(), subPath, subPath, subPath, subPath, subPath, subPath)
	})

	mux.HandleFunc("/"+subPath, func(w http.ResponseWriter, r *http.Request) {
		var data []byte
		subReadyMu.RLock()
		ready := subReady
		subReadyMu.RUnlock()

		if ready {
			subContentMu.RLock()
			content := subContent
			subContentMu.RUnlock()
			if content != "" {
				data = []byte(base64.StdEncoding.EncodeToString([]byte(content)))
			}
		}

		if len(data) == 0 && fileExists(subFilePath) {
			fileContent, err := os.ReadFile(subFilePath)
			if err == nil && len(fileContent) > 0 {
				data = fileContent
			}
		}

		if len(data) > 0 {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write(data)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("订阅未就绪，请稍后重试"))
	})

	mux.HandleFunc("/"+subPath+"/download", func(w http.ResponseWriter, r *http.Request) {
		var data []byte
		subReadyMu.RLock()
		ready := subReady
		subReadyMu.RUnlock()

		if ready {
			subContentMu.RLock()
			content := subContent
			subContentMu.RUnlock()
			if content != "" {
				data = []byte(base64.StdEncoding.EncodeToString([]byte(content)))
			}
		}

		if len(data) == 0 && fileExists(subFilePath) {
			fileContent, err := os.ReadFile(subFilePath)
			if err == nil && len(fileContent) > 0 {
				data = fileContent
			}
		}

		if len(data) > 0 {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", "attachment; filename=sub.txt")
			w.Write(data)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("订阅未就绪，请稍后重试"))
	})

	mux.HandleFunc("/"+subPath+"/raw", func(w http.ResponseWriter, r *http.Request) {
		var data []byte
		subReadyMu.RLock()
		ready := subReady
		subReadyMu.RUnlock()

		if ready {
			subContentMu.RLock()
			content := subContent
			subContentMu.RUnlock()
			if content != "" {
				data = []byte(content)
			}
		}

		if len(data) == 0 && fileExists(subFilePath) {
			fileContent, err := os.ReadFile(subFilePath)
			if err == nil {
				decoded, err := base64.StdEncoding.DecodeString(string(fileContent))
				if err == nil {
					data = decoded
				}
			}
		}

		if len(data) > 0 {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write(data)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("无订阅数据"))
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		subReadyMu.RLock()
		ready := subReady
		subReadyMu.RUnlock()

		status := map[string]interface{}{
			"status":     "running",
			"version":    Version,
			"build_date": BuildDate,
			"sub_ready":  ready,
			"sub_path":   subPath,
			"os":         runtime.GOOS,
			"arch":       runtime.GOARCH,
			"core_type":  getCoreType(),
			"uptime":     time.Now().Unix(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		log.Printf("HTTP 服务启动在端口 %d", port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP服务错误: %v", err)
		}
	}()
}

func getCoreType() string {
	if runtime.GOOS == "freebsd" {
		return "sing-box"
	}
	return "xray"
}

// ========== 配置验证 ==========
func validateConfig() error {
	if uuid == "" {
		return fmt.Errorf("UUID 不能为空，请设置环境变量 UUID")
	}

	if argoPort <= 0 || argoPort > 65535 {
		return fmt.Errorf("ARGO_PORT 必须设置在 1-65535 之间，当前值: %d", argoPort)
	}

	if port <= 0 || port > 65535 {
		return fmt.Errorf("SERVER_PORT 必须设置在 1-65535 之间，当前值: %d", port)
	}

	return nil
}

// ========== 主函数 ==========
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("启动代理服务 - 版本: %s, 构建日期: %s", Version, BuildDate)

	// 验证配置
	if err := validateConfig(); err != nil {
		log.Fatalf("配置验证失败: %v", err)
	}

	initPaths()
	log.Printf("工作目录: %s", filePath)
	log.Printf("系统信息: %s/%s", runtime.GOOS, runtime.GOARCH)

	// 打印配置信息（隐藏敏感信息）
	log.Printf("配置信息:")
	log.Printf("  UUID: %s...", uuid[:8])
	log.Printf("  HTTP端口: %d", port)
	log.Printf("  代理端口: %d", argoPort)
	log.Printf("  CF IP: %s", cfip)
	log.Printf("  CF端口: %d", cfport)
	if argoDomain != "" {
		log.Printf("  Argo域名: %s", argoDomain)
	}
	if nezhaServer != "" {
		log.Printf("  哪吒监控: %s", nezhaServer)
	}

	// 信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		sig := <-sigChan
		log.Printf("收到信号: %v, 正在关闭服务...", sig)

		// 设置超时
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		done := make(chan struct{})
		go func() {
			stopAllProcesses()
			if httpServer != nil {
				httpServer.Shutdown(ctx)
			}
			close(done)
		}()

		select {
		case <-done:
			log.Println("服务已正常关闭")
		case <-ctx.Done():
			log.Println("关闭超时，强制退出")
		}

		os.Exit(0)
	}()

	// 启动 HTTP 服务
	startHTTPServer()
	time.Sleep(1 * time.Second)

	// 初始化
	deleteNodes()
	cleanupOldFiles()
	argoType()

	// 生成配置
	if err := generateConfig(); err != nil {
		log.Printf("⚠ 生成配置失败: %v", err)
	} else {
		log.Printf("✓ 配置文件已生成: %s", configPath)
	}

	// 下载并运行服务
	if err := downloadFilesAndRun(); err != nil {
		log.Printf("⚠ 启动服务失败: %v", err)
		log.Println("请检查网络连接和文件下载地址")
	}

	// 获取隧道域名
	if err := extractDomains(); err != nil {
		log.Printf("⚠ 获取隧道域名失败: %v", err)
	}

	// 其他任务
	addVisitTask()
	cleanFiles()

	// 打印运行信息
	log.Printf("========================================")
	log.Printf("✓ 服务运行中")
	log.Printf("  系统: %s/%s", runtime.GOOS, runtime.GOARCH)
	log.Printf("  核心: %s", getCoreType())
	log.Printf("  HTTP端口: %d", port)
	log.Printf("  代理端口: %d", argoPort)
	log.Printf("  订阅地址: http://localhost:%d/%s", port, subPath)
	log.Printf("  原始订阅: http://localhost:%d/%s/raw", port, subPath)
	log.Printf("========================================")

	// 保持运行
	select {}
}
