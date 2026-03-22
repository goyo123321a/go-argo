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

// 版本信息（通过 ldflags 注入）
var (
	Version   = "dev"
	BuildDate = "unknown"
)

// 环境变量配置
var (
	uploadURL    = getEnv("UPLOAD_URL", "")
	projectURL   = getEnv("PROJECT_URL", "")
	autoAccess   = getEnvBool("AUTO_ACCESS", false)
	filePath     = getEnv("FILE_PATH", "/app/tmp")
	subPath      = getEnv("SUB_PATH", "sub")
	port         = getEnvInt("SERVER_PORT", 7860)
	uuid         = getEnv("UUID", "9afd1229-b893-40c1-84dd-51e7ce204913")
	nezhaServer  = getEnv("NEZHA_SERVER", "")
	nezhaPort    = getEnv("NEZHA_PORT", "")
	nezhaKey     = getEnv("NEZHA_KEY", "")
	argoDomain   = getEnv("ARGO_DOMAIN", "")
	argoAuth     = getEnv("ARGO_AUTH", "")
	argoPort     = getEnvInt("ARGO_PORT", 8001)
	cfip         = getEnv("CFIP", "saas.sin.fan")
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

func initPaths() {
	npmPath = filepath.Join(filePath, npmName)
	phpPath = filepath.Join(filePath, phpName)
	webPath = filepath.Join(filePath, webName)
	botPath = filepath.Join(filePath, botName)
	subFilePath = filepath.Join(filePath, "sub.txt")
	listFilePath = filepath.Join(filePath, "list.txt")
	bootLogPath = filepath.Join(filePath, "boot.log")
	configPath = filepath.Join(filePath, "config.json")
}

// 生成 Xray 配置
func generateConfig() error {
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

// 获取系统架构
func getSystemArchitecture() string {
	switch runtime.GOARCH {
	case "arm", "arm64", "aarch64":
		return "arm"
	default:
		return "amd"
	}
}

// 下载文件
func downloadFile(filePath, fileURL string) error {
	log.Printf("Downloading %s from %s", filepath.Base(filePath), fileURL)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(fileURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	// 设置可执行权限
	if err := os.Chmod(filePath, 0775); err != nil {
		return err
	}

	return nil
}

// 获取需要下载的文件列表
func getFilesForArchitecture(architecture string) []struct {
	path string
	url  string
} {
	var files []struct{ path string; url string }

	if architecture == "arm" {
		files = append(files, struct{ path string; url string }{webPath, "https://arm64.ssss.nyc.mn/web"})
		files = append(files, struct{ path string; url string }{botPath, "https://arm64.ssss.nyc.mn/bot"})
	} else {
		files = append(files, struct{ path string; url string }{webPath, "https://amd64.ssss.nyc.mn/web"})
		files = append(files, struct{ path string; url string }{botPath, "https://amd64.ssss.nyc.mn/bot"})
	}

	if nezhaServer != "" && nezhaKey != "" {
		if nezhaPort != "" {
			url := "https://amd64.ssss.nyc.mn/agent"
			if architecture == "arm" {
				url = "https://arm64.ssss.nyc.mn/agent"
			}
			files = append([]struct{ path string; url string }{{npmPath, url}}, files...)
		} else {
			url := "https://amd64.ssss.nyc.mn/v1"
			if architecture == "arm" {
				url = "https://arm64.ssss.nyc.mn/v1"
			}
			files = append([]struct{ path string; url string }{{phpPath, url}}, files...)
		}
	}

	return files
}

// 运行哪吒监控
func runNezha() error {
	if nezhaServer == "" || nezhaKey == "" {
		return nil
	}

	if nezhaPort == "" {
		// 哪吒 v1
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
		log.Printf("%s is running (PID: %d)", phpName, cmd.Process.Pid)
		time.Sleep(1 * time.Second)
	} else {
		// 哪吒 v0
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
		log.Printf("%s is running (PID: %d)", npmName, cmd.Process.Pid)
		time.Sleep(1 * time.Second)
	}
	return nil
}

// 运行 Xray
func runXray() error {
	if !fileExists(webPath) {
		return fmt.Errorf("xray binary not found: %s", webPath)
	}

	cmd := exec.Command(webPath, "-c", configPath)
	cmd.Dir = filePath
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	processMutex.Lock()
	processes = append(processes, cmd.Process)
	processMutex.Unlock()
	log.Printf("%s is running (PID: %d)", webName, cmd.Process.Pid)
	time.Sleep(1 * time.Second)
	return nil
}

// 运行 Cloudflared
func runCloudflared() error {
	if !fileExists(botPath) {
		log.Printf("Cloudflared binary not found: %s", botPath)
		return nil
	}

	args := []string{"tunnel", "--edge-ip-version", "auto", "--no-autoupdate", "--protocol", "http2"}

	if argoAuth != "" && len(argoAuth) >= 120 && len(argoAuth) <= 250 {
		args = append(args, "run", "--token", argoAuth)
	} else if argoAuth != "" && strings.Contains(argoAuth, "TunnelSecret") {
		tunnelYamlPath := filepath.Join(filePath, "tunnel.yml")
		if !fileExists(tunnelYamlPath) {
			log.Println("Tunnel YAML config not found, waiting...")
			time.Sleep(2 * time.Second)
		}
		args = append(args, "--config", tunnelYamlPath, "run")
	} else {
		args = append(args, "--logfile", bootLogPath, "--loglevel", "info",
			"--url", fmt.Sprintf("http://localhost:%d", argoPort))
	}

	log.Printf("Starting cloudflared with args: %v", args)
	cmd := exec.Command(botPath, args...)
	cmd.Dir = filePath
	cmd.Stdout = nil
	cmd.Stderr = nil

	// 对于临时隧道，需要捕获输出
	if argoAuth == "" || argoDomain == "" {
		stdout, err := cmd.StdoutPipe()
		if err == nil {
			go func() {
				buf := make([]byte, 4096)
				for {
					n, err := stdout.Read(buf)
					if n > 0 {
						// 追加到日志文件
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
	log.Printf("%s is running (PID: %d)", botName, cmd.Process.Pid)
	time.Sleep(2 * time.Second)
	return nil
}

// 下载并运行所有依赖
func downloadFilesAndRun() error {
	arch := getSystemArchitecture()
	files := getFilesForArchitecture(arch)

	// 下载文件
	for _, f := range files {
		if err := downloadFile(f.path, f.url); err != nil {
			log.Printf("Download %s failed: %v", f.url, err)
			continue
		}
		log.Printf("Downloaded %s successfully", filepath.Base(f.path))
	}

	// 运行哪吒
	if err := runNezha(); err != nil {
		log.Printf("Nezha running error: %v", err)
	}

	// 运行 Xray
	if err := runXray(); err != nil {
		return fmt.Errorf("Xray running error: %v", err)
	}

	// 运行 Cloudflared
	if err := runCloudflared(); err != nil {
		log.Printf("Cloudflared running error: %v", err)
	}

	time.Sleep(5 * time.Second)
	return nil
}

// 配置 Argo 隧道
func argoType() {
	if argoAuth == "" || argoDomain == "" {
		log.Println("ARGO_DOMAIN or ARGO_AUTH variable is empty, use quick tunnels")
		return
	}

	if strings.Contains(argoAuth, "TunnelSecret") {
		tunnelJsonPath := filepath.Join(filePath, "tunnel.json")
		if err := os.WriteFile(tunnelJsonPath, []byte(argoAuth), 0644); err != nil {
			log.Printf("Error writing tunnel.json: %v", err)
			return
		}

		// 解析 tunnel ID
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
				log.Println("Tunnel YAML config generated")
			}
		}
	} else {
		log.Println("ARGO_AUTH mismatch TunnelSecret, use token connect to tunnel")
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

	// 尝试 ipapi.co
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

	// 备用 ip-api.com
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

	subTxt := fmt.Sprintf(`
vless://%s@%s:%d?encryption=none&security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Fvless-argo%%3Fed%%3D2560#%s

vmess://%s

trojan://%s@%s:%d?security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Ftrojan-argo%%3Fed%%3D2560#%s
`,
		uuid, cfip, cfport, argoDomain, argoDomain, nodeName,
		vmessBase64,
		uuid, cfip, cfport, argoDomain, argoDomain, nodeName)

	// Base64 编码
	encoded := base64.StdEncoding.EncodeToString([]byte(subTxt))
	log.Printf("Subscription generated (length: %d)", len(encoded))

	// 保存到文件
	if err := os.WriteFile(subFilePath, []byte(encoded), 0644); err != nil {
		return err
	}
	log.Printf("%s/sub.txt saved successfully", filePath)

	// 更新内存中的订阅内容
	subContentMutex.Lock()
	subContent = subTxt
	subContentMutex.Unlock()

	// 上传节点
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
			log.Printf("Upload subscription failed: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			log.Println("Subscription uploaded successfully")
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
				log.Println("Nodes uploaded successfully")
			}
		}
	}
}

// 提取临时隧道域名
func extractDomains() error {
	if argoAuth != "" && argoDomain != "" {
		log.Println("Using fixed domain:", argoDomain)
		return generateLinks(argoDomain)
	}

	// 等待 boot.log 生成
	log.Println("Waiting for tunnel to start...")
	time.Sleep(5 * time.Second)

	// 尝试多次读取
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		if fileExists(bootLogPath) {
			content, err := os.ReadFile(bootLogPath)
			if err == nil {
				// 查找 trycloudflare.com 域名
				re := regexp.MustCompile(`https?://([^ ]*trycloudflare\.com)/?`)
				matches := re.FindAllStringSubmatch(string(content), -1)
				if len(matches) > 0 && len(matches[0]) > 1 {
					argoDomain := matches[0][1]
					log.Println("Found ArgoDomain:", argoDomain)
					return generateLinks(argoDomain)
				}
			}
		}
		log.Printf("Waiting for domain (attempt %d/%d)...", i+1, maxRetries)
		time.Sleep(3 * time.Second)
	}

	log.Println("ArgoDomain not found, continuing without tunnel domain")
	return nil
}

// 添加自动访问任务
func addVisitTask() {
	if !autoAccess || projectURL == "" {
		log.Println("Skipping adding automatic access task")
		return
	}

	data := map[string]string{"url": projectURL}
	jsonData, _ := json.Marshal(data)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post("https://oooo.serv00.net/add-url", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Add automatic access task failed: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Println("Automatic access task added successfully")
}

// 清理文件（90秒后）
func cleanFiles() {
	time.AfterFunc(90*time.Second, func() {
		filesToDelete := []string{bootLogPath, configPath, webPath, botPath}

		if nezhaPort != "" {
			filesToDelete = append(filesToDelete, npmPath)
		} else if nezhaServer != "" && nezhaKey != "" {
			filesToDelete = append(filesToDelete, phpPath)
		}

		for _, file := range filesToDelete {
			if err := os.Remove(file); err == nil {
				log.Printf("Deleted: %s", file)
			}
		}

		log.Println("App is running")
		log.Println("Thank you for using this script, enjoy!")
	})
}

// 停止所有进程
func stopAllProcesses() {
	log.Println("Stopping all processes...")
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

	// 根路由
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 检查是否有 index.html
		indexPath := "index.html"
		if fileExists(indexPath) {
			http.ServeFile(w, r, indexPath)
			return
		}

		// 返回默认页面
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "Hello world!<br><br>You can access /%s to get your nodes!", subPath)
	})

	// 订阅路由
	mux.HandleFunc("/"+subPath, func(w http.ResponseWriter, r *http.Request) {
		subContentMutex.RLock()
		content := subContent
		subContentMutex.RUnlock()

		if content != "" {
			// 返回 base64 编码的内容
			encoded := base64.StdEncoding.EncodeToString([]byte(content))
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte(encoded))
			return
		}

		// 尝试从文件读取
		if fileExists(subFilePath) {
			fileContent, err := os.ReadFile(subFilePath)
			if err == nil {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Write(fileContent)
				return
			}
		}

		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Subscription not found"))
	})

	// 版本信息路由
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"version":    Version,
			"build_date": BuildDate,
			"go_version": runtime.Version(),
		})
	})

	// 健康检查路由
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

	log.Printf("HTTP server is running on port: %d", port)
	log.Printf("Version: %s, Build Date: %s", Version, BuildDate)

	// 在 goroutine 中启动服务器
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("Starting VPS Agent v%s (built: %s)", Version, BuildDate)

	// 打印环境信息
	log.Printf("Runtime: %s/%s", runtime.GOOS, runtime.GOARCH)
	log.Printf("Go Version: %s", runtime.Version())
	log.Printf("Working directory: %s", filePath)

	// 初始化目录
	if err := ensureDir(filePath); err != nil {
		log.Fatal("Failed to create directory:", err)
	}
	initPaths()

	// 信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received shutdown signal")
		stopAllProcesses()
		if httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			httpServer.Shutdown(ctx)
		}
		os.Exit(0)
	}()

	// 启动 HTTP 服务（非阻塞）
	startHTTPServer()

	// 等待服务启动
	time.Sleep(2 * time.Second)

	// 启动前清理
	deleteNodes()
	cleanupOldFiles()

	// 配置 Argo 隧道
	argoType()

	// 生成 Xray 配置
	if err := generateConfig(); err != nil {
		log.Printf("Failed to generate config: %v", err)
	} else {
		log.Println("Xray config generated successfully")
	}

	// 下载并运行依赖
	if err := downloadFilesAndRun(); err != nil {
		log.Printf("Failed to download and run files: %v", err)
	}

	// 提取域名
	if err := extractDomains(); err != nil {
		log.Printf("Failed to extract domains: %v", err)
	}

	// 添加自动访问任务
	addVisitTask()

	// 清理文件
	cleanFiles()

	log.Println("VPS Agent started successfully")
	log.Println("Waiting for requests...")

	// 保持程序运行
	select {}
}
