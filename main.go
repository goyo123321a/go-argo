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
	uploadURL    = getEnv("UPLOAD_URL", "")
	projectURL   = getEnv("PROJECT_URL", "")
	autoAccess   = getEnvBool("AUTO_ACCESS", false)
	filePath     = getEnv("FILE_PATH", "./tmp")
	subPath      = getEnv("SUB_PATH", "sub")
	port         = getEnvInt("SERVER_PORT", 7860)
	uuid         = getEnv("UUID", "9afd1229-b893-40c1-84dd-51e7ce204913")
	nezhaServer  = getEnv("NEZHA_SERVER", "")
	nezhaPort    = getEnv("NEZHA_PORT", "")
	nezhaKey     = getEnv("NEZHA_KEY", "")
	argoDomain   = getEnv("ARGO_DOMAIN", "")
	argoAuth     = getEnv("ARGO_AUTH", "")
	argoPort     = getEnvInt("ARGO_PORT", 8001)
	cfip         = getEnv("CFIP", "cf.877774.xyz")
	cfport       = getEnvInt("CFPORT", 443)
	name         = getEnv("NAME", "")
)

// 全局变量
var (
	npmName   = generateRandomName()
	webName   = generateRandomName()
	botName   = generateRandomName()
	phpName   = generateRandomName()
	npmPath   string
	phpPath   string
	webPath   string
	botPath   string
	subFilePath string
	listFilePath string
	bootLogPath  string
	configPath   string

	// 进程管理
	processes      []*os.Process
	processMutex   sync.Mutex
	httpServer     *http.Server
	subContent     string
	subContentMutex sync.RWMutex
	subReady       bool
	subReadyMutex  sync.RWMutex

	// 工作目录
	workDir string
)

// Xray 配置结构体
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
	Network    string           `json:"network"`
	Security   string           `json:"security,omitempty"`
	WSSettings *XrayWSSettings  `json:"wsSettings,omitempty"`
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

// 辅助函数
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intVal int
		fmt.Sscanf(value, "%d", &intVal)
		return intVal
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "True" || value == "TRUE"
	}
	return defaultValue
}

func generateRandomName() string {
	const chars = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 6)
	for i := range b {
		b[i] = chars[randInt(len(chars))]
	}
	return string(b)
}

func randInt(n int) int {
	b := make([]byte, 1)
	rand.Read(b)
	return int(b[0]) % n
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// 初始化路径
func initPaths() {
	var err error
	workDir, err = os.Getwd()
	if err != nil {
		workDir = "."
	}

	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(workDir, filePath)
	}

	if err := ensureDir(filePath); err != nil {
		log.Printf("⚠ 创建目录失败: %v, 使用当前目录", err)
		filePath = workDir
	}

	npmPath = filepath.Join(filePath, npmName)
	phpPath = filepath.Join(filePath, phpName)
	webPath = filepath.Join(filePath, webName)
	botPath = filepath.Join(filePath, botName)
	subFilePath = filepath.Join(filePath, "sub.txt")
	listFilePath = filepath.Join(filePath, "list.txt")
	bootLogPath = filepath.Join(filePath, "boot.log")
	configPath = filepath.Join(filePath, "config.json")

	log.Printf("📁 工作目录: %s", workDir)
	log.Printf("📁 文件目录: %s", filePath)
}

// 检测操作系统
func getSystemOS() string {
	return runtime.GOOS
}

// 检测系统架构
func getSystemArchitecture() string {
	switch runtime.GOARCH {
	case "arm", "arm64", "aarch64":
		return "arm64"
	default:
		return "amd64"
	}
}

// 生成 Xray 配置（Linux 系统）
func generateXrayConfig() error {
	config := XrayConfig{
		Log: XrayLog{
			Access:   "/dev/null",
			Error:    "/dev/null",
			Loglevel: "none",
		},
		DNS: XrayDNS{
			Servers: []string{"https+local://8.8.8.8/dns-query"},
		},
		Inbounds: []XrayInbound{
			{
				Port:     argoPort,
				Protocol: "vless",
				Settings: VlessSettings{
					Clients: []VlessClient{
						{ID: uuid, Flow: "xtls-rprx-vision"},
					},
					Decryption: "none",
					Fallbacks: []Fallback{
						{Dest: 3001},
						{Path: "/vless-argo", Dest: 3002},
						{Path: "/vmess-argo", Dest: 3003},
						{Path: "/trojan-argo", Dest: 3004},
					},
				},
				StreamSettings: XrayStreamSettings{
					Network: "tcp",
				},
			},
			{
				Port:     3001,
				Listen:   "127.0.0.1",
				Protocol: "vless",
				Settings: VlessSettings{
					Clients:    []VlessClient{{ID: uuid}},
					Decryption: "none",
				},
				StreamSettings: XrayStreamSettings{
					Network:  "tcp",
					Security: "none",
				},
			},
			{
				Port:     3002,
				Listen:   "127.0.0.1",
				Protocol: "vless",
				Settings: VlessSettings{
					Clients:    []VlessClient{{ID: uuid, Level: 0}},
					Decryption: "none",
				},
				StreamSettings: XrayStreamSettings{
					Network:  "ws",
					Security: "none",
					WSSettings: &XrayWSSettings{
						Path: "/vless-argo",
					},
				},
				Sniffing: &XraySniffing{
					Enabled:      true,
					DestOverride: []string{"http", "tls", "quic"},
					MetadataOnly: false,
				},
			},
			{
				Port:     3003,
				Listen:   "127.0.0.1",
				Protocol: "vmess",
				Settings: VmessSettings{
					Clients: []VmessClient{{ID: uuid, AlterID: 0}},
				},
				StreamSettings: XrayStreamSettings{
					Network: "ws",
					WSSettings: &XrayWSSettings{
						Path: "/vmess-argo",
					},
				},
				Sniffing: &XraySniffing{
					Enabled:      true,
					DestOverride: []string{"http", "tls", "quic"},
					MetadataOnly: false,
				},
			},
			{
				Port:     3004,
				Listen:   "127.0.0.1",
				Protocol: "trojan",
				Settings: TrojanSettings{
					Clients: []TrojanClient{{Password: uuid}},
				},
				StreamSettings: XrayStreamSettings{
					Network:  "ws",
					Security: "none",
					WSSettings: &XrayWSSettings{
						Path: "/trojan-argo",
					},
				},
				Sniffing: &XraySniffing{
					Enabled:      true,
					DestOverride: []string{"http", "tls", "quic"},
					MetadataOnly: false,
				},
			},
		},
		Outbounds: []XrayOutbound{
			{
				Protocol: "freedom",
				Tag:      "direct",
				Settings: map[string]interface{}{
					"domainStrategy": "UseIP",
				},
			},
			{
				Protocol: "blackhole",
				Tag:      "block",
			},
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// 生成 Sing-box 配置（单端口多路径 + DNS）
func generateSingBoxConfig() error {
	config := map[string]interface{}{
		"log": map[string]interface{}{
			"disabled":  true,
			"level":     "info",
			"timestamp": true,
		},
		"dns": map[string]interface{}{
			"servers": []map[string]interface{}{
				{
					"tag":     "google",
					"address": "https://8.8.8.8/dns-query",
					"detour":  "direct",
				},
				{
					"tag":     "local",
					"address": "local",
					"detour":  "direct",
				},
				{
					"tag":     "block",
					"address": "rcode://success",
				},
			},
			"rules": []map[string]interface{}{
				{
					"domain_suffix": []string{
						"cn",
						"baidu.com",
						"taobao.com",
						"jd.com",
						"qq.com",
						"weixin.com",
					},
					"server": "local",
				},
				{
					"geosite":       []string{"category-ads-all"},
					"server":        "block",
					"disable_cache": true,
				},
			},
			"final": "google",
		},
		"inbounds": []map[string]interface{}{
			{
				"type":        "vmess",
				"tag":         "vmess-ws",
				"listen":      "::",
				"listen_port": argoPort,
				"users": []map[string]interface{}{
					{
						"uuid": uuid,
					},
				},
				"transport": map[string]interface{}{
					"type":                   "ws",
					"path":                   "/vmess-argo",
					"early_data_header_name": "Sec-WebSocket-Protocol",
				},
			},
			{
				"type":        "vless",
				"tag":         "vless-ws",
				"listen":      "::",
				"listen_port": argoPort,
				"users": []map[string]interface{}{
					{
						"uuid": uuid,
						"flow": "xtls-rprx-vision",
					},
				},
				"transport": map[string]interface{}{
					"type": "ws",
					"path": "/vless-argo",
				},
			},
			{
				"type":        "trojan",
				"tag":         "trojan-ws",
				"listen":      "::",
				"listen_port": argoPort,
				"users": []map[string]interface{}{
					{
						"password": uuid,
					},
				},
				"transport": map[string]interface{}{
					"type": "ws",
					"path": "/trojan-argo",
				},
			},
		},
		"outbounds": []map[string]interface{}{
			{
				"type": "direct",
				"tag":  "direct",
			},
			{
				"type": "block",
				"tag":  "block",
			},
		},
		"route": map[string]interface{}{
			"rules": []map[string]interface{}{
				{
					"protocol": []string{"quic"},
					"outbound": "block",
				},
			},
			"final": "direct",
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// 生成配置（根据系统选择）
func generateConfig() error {
	if runtime.GOOS == "freebsd" {
		return generateSingBoxConfig()
	}
	return generateXrayConfig()
}

// 获取需要下载的文件列表
func getFilesForArchitecture(arch string) []struct {
	path string
	url  string
} {
	var files []struct{ path string; url string }

	osName := getSystemOS()

	if osName == "freebsd" {
		var baseURL string
		if arch == "arm64" {
			baseURL = "https://github.com/eooce/test/releases/download/freebsd-arm64"
		} else {
			baseURL = "https://github.com/eooce/test/releases/download/freebsd"
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

// 下载文件
func downloadFile(filePath, fileURL string) error {
	if err := ensureDir(filepath.Dir(filePath)); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(fileURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败: %s", resp.Status)
	}

	tempFile := filePath + ".tmp"
	out, err := os.Create(tempFile)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		os.Remove(tempFile)
		return err
	}
	out.Close()

	if err := os.Rename(tempFile, filePath); err != nil {
		return err
	}

	if err := os.Chmod(filePath, 0755); err != nil {
		return err
	}

	return nil
}

// 运行核心代理
func runCore() error {
	if !fileExists(webPath) {
		altPath := filepath.Join(workDir, filepath.Base(webPath))
		if fileExists(altPath) {
			webPath = altPath
		} else {
			return fmt.Errorf("核心文件不存在: %s", webPath)
		}
	}

	var cmd *exec.Cmd

	if runtime.GOOS == "freebsd" {
		log.Printf("🚀 启动 Sing-box: %s", webPath)
		log.Printf("📝 配置文件: %s", configPath)
		cmd = exec.Command(webPath, "run", "-c", configPath)
	} else {
		log.Printf("🚀 启动 Xray: %s", webPath)
		cmd = exec.Command(webPath, "-c", configPath)
	}

	cmd.Dir = filePath
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动失败: %v", err)
	}

	processMutex.Lock()
	processes = append(processes, cmd.Process)
	processMutex.Unlock()

	log.Printf("✓ 核心进程已启动 (PID: %d)", cmd.Process.Pid)
	time.Sleep(2 * time.Second)
	return nil
}

// 运行 Cloudflared
func runCloudflared() error {
	if !fileExists(botPath) {
		altPath := filepath.Join(workDir, filepath.Base(botPath))
		if fileExists(altPath) {
			botPath = altPath
		} else {
			log.Printf("⚠ Cloudflared 不存在: %s", botPath)
			return nil
		}
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

	log.Printf("🚀 启动 Cloudflared: %s", botPath)
	cmd := exec.Command(botPath, args...)
	cmd.Dir = filePath
	cmd.Stdout = nil
	cmd.Stderr = nil

	if argoAuth == "" || argoDomain == "" {
		stdout, err := cmd.StdoutPipe()
		if err == nil {
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
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	processMutex.Lock()
	processes = append(processes, cmd.Process)
	processMutex.Unlock()

	log.Printf("✓ Cloudflared 已启动 (PID: %d)", cmd.Process.Pid)
	time.Sleep(2 * time.Second)
	return nil
}

// 运行哪吒监控
func runNezha() error {
	if nezhaServer == "" || nezhaKey == "" {
		return nil
	}

	if nezhaPort == "" {
		if !fileExists(phpPath) {
			return nil
		}

		port := ""
		if strings.Contains(nezhaServer, ":") {
			parts := strings.Split(nezhaServer, ":")
			port = parts[len(parts)-1]
		}
		tlsPorts := map[string]bool{"443": true, "8443": true, "2096": true, "2087": true, "2083": true, "2053": true}
		nezhaTLS := "false"
		if tlsPorts[port] {
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
insecure_tls: false
ip_report_period: 1800
report_delay: 1
server: %s
skip_connection_count: false
skip_procs_count: false
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
		time.Sleep(1 * time.Second)
	} else {
		if !fileExists(npmPath) {
			return nil
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
		time.Sleep(1 * time.Second)
	}
	return nil
}

// 下载并运行所有依赖
func downloadFilesAndRun() error {
	arch := getSystemArchitecture()
	files := getFilesForArchitecture(arch)

	log.Printf("📥 准备下载 %d 个文件", len(files))

	for _, f := range files {
		if err := downloadFile(f.path, f.url); err != nil {
			log.Printf("⚠ 下载失败 %s: %v", f.url, err)
			continue
		}
		log.Printf("✓ 下载成功: %s", filepath.Base(f.path))
	}

	log.Printf("📁 文件列表:")
	entries, _ := os.ReadDir(filePath)
	for _, entry := range entries {
		if !entry.IsDir() {
			if info, err := entry.Info(); err == nil {
				log.Printf("   - %s (%d bytes)", entry.Name(), info.Size())
			}
		}
	}

	if err := runNezha(); err != nil {
		log.Printf("⚠ 哪吒启动失败: %v", err)
	}

	if err := runCore(); err != nil {
		return fmt.Errorf("代理运行失败: %v", err)
	}

	if err := runCloudflared(); err != nil {
		log.Printf("⚠ Cloudflared 启动失败: %v", err)
	}

	time.Sleep(5 * time.Second)

	log.Printf("✓ 核心服务已启动")
	if runtime.GOOS == "freebsd" {
		log.Printf("  Sing-box: %s", filepath.Base(webPath))
	} else {
		log.Printf("  Xray: %s", filepath.Base(webPath))
	}
	log.Printf("  Tunnel: %s", filepath.Base(botPath))

	return nil
}

// 配置 Argo 隧道
func argoType() {
	if argoAuth == "" || argoDomain == "" {
		return
	}

	if strings.Contains(argoAuth, "TunnelSecret") {
		tunnelJsonPath := filepath.Join(filePath, "tunnel.json")
		if err := os.WriteFile(tunnelJsonPath, []byte(argoAuth), 0644); err != nil {
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
			}
		}
	}
}

// 删除历史节点
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

// 清理历史文件
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

// 获取 ISP 信息
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

// 生成订阅链接
func generateLinks(argoDomain string) error {
	isp, _ := getMetaInfo()
	nodeName := isp
	if name != "" {
		nodeName = name + "-" + isp
	}

	vmess := map[string]interface{}{
		"v":    "2",
		"ps":   nodeName,
		"add":  cfip,
		"port": cfport,
		"id":   uuid,
		"aid":  "0",
		"scy":  "auto",
		"net":  "ws",
		"type": "none",
		"host": argoDomain,
		"path": "/vmess-argo?ed=2560",
		"tls":  "tls",
		"sni":  argoDomain,
		"alpn": "",
		"fp":   "firefox",
	}

	vmessJSON, _ := json.Marshal(vmess)
	vmessBase64 := base64.StdEncoding.EncodeToString(vmessJSON)

	subTxt := fmt.Sprintf(`vless://%s@%s:%d?encryption=none&security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Fvless-argo%%3Fed%%3D2560#%s

vmess://%s

trojan://%s@%s:%d?security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Ftrojan-argo%%3Fed%%3D2560#%s`,
		uuid, cfip, cfport, argoDomain, argoDomain, nodeName,
		vmessBase64,
		uuid, cfip, cfport, argoDomain, argoDomain, nodeName)

	subTxt = strings.TrimSpace(subTxt)

	encoded := base64.StdEncoding.EncodeToString([]byte(subTxt))

	if err := os.WriteFile(subFilePath, []byte(encoded), 0644); err != nil {
		return fmt.Errorf("保存订阅文件失败: %v", err)
	}

	subContentMutex.Lock()
	subContent = subTxt
	subContentMutex.Unlock()

	subReadyMutex.Lock()
	subReady = true
	subReadyMutex.Unlock()

	log.Printf("✓ 订阅已生成")
	log.Printf("✓ 隧道域名: %s", argoDomain)
	log.Printf("✓ 节点名称: %s", nodeName)

	uploadNodes()

	return nil
}

// 上传节点或订阅
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

// 提取临时隧道域名
func extractDomains() error {
	if argoAuth != "" && argoDomain != "" {
		return generateLinks(argoDomain)
	}

	time.Sleep(5 * time.Second)

	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
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

	return nil
}

// 添加自动访问任务
func addVisitTask() {
	if !autoAccess || projectURL == "" {
		return
	}

	data := map[string]string{"url": projectURL}
	jsonData, _ := json.Marshal(data)
	client := &http.Client{Timeout: 10 * time.Second}
	client.Post("https://oooo.serv00.net/add-url", "application/json", bytes.NewBuffer(jsonData))
}

// 清理文件
func cleanFiles() {
	time.AfterFunc(90*time.Second, func() {
		filesToDelete := []string{bootLogPath, configPath, webPath, botPath}

		if nezhaPort != "" {
			filesToDelete = append(filesToDelete, npmPath)
		} else if nezhaServer != "" && nezhaKey != "" {
			filesToDelete = append(filesToDelete, phpPath)
		}

		for _, file := range filesToDelete {
			os.Remove(file)
		}
	})
}

// 停止所有进程
func stopAllProcesses() {
	processMutex.Lock()
	defer processMutex.Unlock()

	for _, proc := range processes {
		if proc != nil {
			proc.Kill()
		}
	}
	processes = nil
}

// HTTP 服务
func startHTTPServer() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		indexPath := "index.html"
		if fileExists(indexPath) {
			http.ServeFile(w, r, indexPath)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "myapp 运行中<br><br>订阅地址: /%s", subPath)
	})

	mux.HandleFunc("/"+subPath, func(w http.ResponseWriter, r *http.Request) {
		var responseData []byte

		subReadyMutex.RLock()
		ready := subReady
		subReadyMutex.RUnlock()

		if ready {
			subContentMutex.RLock()
			content := subContent
			subContentMutex.RUnlock()

			if content != "" {
				responseData = []byte(base64.StdEncoding.EncodeToString([]byte(content)))
			}
		}

		if len(responseData) == 0 && fileExists(subFilePath) {
			fileContent, err := os.ReadFile(subFilePath)
			if err == nil && len(fileContent) > 0 {
				responseData = fileContent
			}
		}

		if len(responseData) > 0 {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write(responseData)
			return
		}

		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("订阅未就绪，请稍后重试"))
	})

	mux.HandleFunc("/"+subPath+"/download", func(w http.ResponseWriter, r *http.Request) {
		var responseData []byte

		subReadyMutex.RLock()
		ready := subReady
		subReadyMutex.RUnlock()

		if ready {
			subContentMutex.RLock()
			content := subContent
			subContentMutex.RUnlock()

			if content != "" {
				responseData = []byte(base64.StdEncoding.EncodeToString([]byte(content)))
			}
		}

		if len(responseData) == 0 && fileExists(subFilePath) {
			fileContent, err := os.ReadFile(subFilePath)
			if err == nil && len(fileContent) > 0 {
				responseData = fileContent
			}
		}

		if len(responseData) > 0 {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", "attachment; filename=sub.txt")
			w.Write(responseData)
			return
		}

		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("订阅未就绪，请稍后重试"))
	})

	mux.HandleFunc("/"+subPath+"/raw", func(w http.ResponseWriter, r *http.Request) {
		var responseData []byte

		subReadyMutex.RLock()
		ready := subReady
		subReadyMutex.RUnlock()

		if ready {
			subContentMutex.RLock()
			content := subContent
			subContentMutex.RUnlock()

			if content != "" {
				responseData = []byte(content)
			}
		}

		if len(responseData) == 0 && fileExists(subFilePath) {
			fileContent, err := os.ReadFile(subFilePath)
			if err == nil {
				decoded, err := base64.StdEncoding.DecodeString(string(fileContent))
				if err == nil {
					responseData = decoded
				}
			}
		}

		if len(responseData) > 0 {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write(responseData)
			return
		}

		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("无订阅数据"))
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		subReadyMutex.RLock()
		ready := subReady
		subReadyMutex.RUnlock()

		status := map[string]interface{}{
			"status":    "running",
			"version":   Version,
			"sub_ready": ready,
			"sub_path":  subPath,
			"os":        runtime.GOOS,
			"arch":      runtime.GOARCH,
			"workdir":   workDir,
			"filepath":  filePath,
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
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP服务错误: %v", err)
		}
	}()
}

func main() {
	log.SetFlags(log.LstdFlags)

	log.Printf("🚀 myapp 启动 (版本: %s)", Version)
	log.Printf("💻 系统: %s/%s", runtime.GOOS, runtime.GOARCH)

	initPaths()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("🛑 正在关闭服务...")
		stopAllProcesses()
		if httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			httpServer.Shutdown(ctx)
		}
		log.Println("✓ 服务已关闭")
		os.Exit(0)
	}()

	startHTTPServer()
	log.Printf("✓ HTTP服务已启动 (端口: %d)", port)

	time.Sleep(1 * time.Second)

	deleteNodes()
	cleanupOldFiles()

	argoType()

	if err := generateConfig(); err != nil {
		log.Printf("⚠ 生成配置失败: %v", err)
	}

	if err := downloadFilesAndRun(); err != nil {
		log.Printf("⚠ 启动失败: %v", err)
	}

	if err := extractDomains(); err != nil {
		log.Printf("⚠ 获取隧道域名失败: %v", err)
	}

	addVisitTask()

	cleanFiles()

	log.Printf("✓ myapp 运行中")
	log.Printf("  订阅: /%s", subPath)
	log.Printf("  下载: /%s/download", subPath)
	log.Printf("  状态: /status")
	log.Printf("  目录: %s", workDir)

	select {}
}
