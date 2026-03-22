package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
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

// 环境变量配置
var (
	uploadURL    = getEnv("UPLOAD_URL", "")
	projectURL   = getEnv("PROJECT_URL", "")
	autoAccess   = getEnvBool("AUTO_ACCESS", false)
	filePath     = getEnv("FILE_PATH", ".tmp")
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
	npmName           = generateRandomName()
	webName           = generateRandomName()
	botName           = generateRandomName()
	phpName           = generateRandomName()
	npmPath           string
	phpPath           string
	webPath           string
	botPath           string
	subFilePath       string
	listFilePath      string
	bootLogPath       string
	configPath        string

	// 进程管理
	nezhaProcess      *os.Process
	xrayProcess       *os.Process
	cloudflaredProcess *os.Process
	processMutex      sync.Mutex
	processWg         sync.WaitGroup
)

// HTTP 客户端
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
	},
}

// 配置结构
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

type FreedomSettings struct {
	DomainStrategy string `json:"domainStrategy"`
}

type VMESS struct {
	V    string `json:"v"`
	Ps   string `json:"ps"`
	Add  string `json:"add"`
	Port int    `json:"port"`
	ID   string `json:"id"`
	Aid  string `json:"aid"`
	Scy  string `json:"scy"`
	Net  string `json:"net"`
	Type string `json:"type"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	Sni  string `json:"sni"`
	Alpn string `json:"alpn"`
	Fp   string `json:"fp"`
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
		return value == "true" || value == "1"
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
	npmPath = filepath.Join(filePath, npmName)
	phpPath = filepath.Join(filePath, phpName)
	webPath = filepath.Join(filePath, webName)
	botPath = filepath.Join(filePath, botName)
	subFilePath = filepath.Join(filePath, "sub.txt")
	listFilePath = filepath.Join(filePath, "list.txt")
	bootLogPath = filepath.Join(filePath, "boot.log")
	configPath = filepath.Join(filePath, "config.json")
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
	httpClient.Post(uploadURL+"/api/delete-nodes", "application/json", bytes.NewBuffer(data))
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
				Settings: FreedomSettings{
					DomainStrategy: "UseIP",
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
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	log.Println("Xray configuration generated successfully")
	return nil
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
	resp, err := httpClient.Get(fileURL)
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
	return err
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

// 启动进程的辅助函数
func startProcess(cmd *exec.Cmd, name string) (*os.Process, error) {
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	processWg.Add(1)
	go func() {
		defer processWg.Done()
		err := cmd.Wait()
		if err != nil {
			log.Printf("%s process exited with error: %v", name, err)
		} else {
			log.Printf("%s process exited normally", name)
		}
	}()

	return cmd.Process, nil
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

		proc, err := startProcess(cmd, phpName)
		if err != nil {
			return err
		}

		processMutex.Lock()
		nezhaProcess = proc
		processMutex.Unlock()

		log.Printf("%s is running (PID: %d)", phpName, proc.Pid)
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

		proc, err := startProcess(cmd, npmName)
		if err != nil {
			return err
		}

		processMutex.Lock()
		nezhaProcess = proc
		processMutex.Unlock()

		log.Printf("%s is running (PID: %d)", npmName, proc.Pid)
		time.Sleep(1 * time.Second)
	}
	return nil
}

// 运行 Xray
func runXray() error {
	cmd := exec.Command(webPath, "-c", configPath)
	cmd.Dir = filePath

	proc, err := startProcess(cmd, webName)
	if err != nil {
		return err
	}

	processMutex.Lock()
	xrayProcess = proc
	processMutex.Unlock()

	log.Printf("%s is running (PID: %d)", webName, proc.Pid)
	time.Sleep(1 * time.Second)
	return nil
}

// 运行 Cloudflared
func runCloudflared() error {
	if !fileExists(botPath) {
		return nil
	}

	args := []string{"tunnel", "--edge-ip-version", "auto", "--no-autoupdate", "--protocol", "http2"}

	if argoAuth != "" && len(argoAuth) >= 120 && len(argoAuth) <= 250 {
		args = append(args, "run", "--token", argoAuth)
	} else if argoAuth != "" && strings.Contains(argoAuth, "TunnelSecret") {
		tunnelYamlPath := filepath.Join(filePath, "tunnel.yml")
		args = append(args, "--config", tunnelYamlPath, "run")
	} else {
		args = append(args, "--logfile", bootLogPath, "--loglevel", "info",
			"--url", fmt.Sprintf("http://localhost:%d", argoPort))
	}

	cmd := exec.Command(botPath, args...)
	cmd.Dir = filePath

	proc, err := startProcess(cmd, botName)
	if err != nil {
		return err
	}

	processMutex.Lock()
	cloudflaredProcess = proc
	processMutex.Unlock()

	log.Printf("%s is running (PID: %d)", botName, proc.Pid)
	time.Sleep(2 * time.Second)
	return nil
}

// 下载并运行所有依赖
func downloadFilesAndRun() error {
	arch := getSystemArchitecture()
	files := getFilesForArchitecture(arch)

	// 下载文件
	for _, f := range files {
		log.Printf("Downloading %s...", f.url)
		if err := downloadFile(f.path, f.url); err != nil {
			return fmt.Errorf("download %s failed: %v", f.url, err)
		}
		// 设置可执行权限
		if err := os.Chmod(f.path, 0775); err != nil {
			return fmt.Errorf("chmod %s failed: %v", f.path, err)
		}
		log.Printf("Downloaded %s successfully", filepath.Base(f.path))
	}

	// 运行哪吒 - 错误不中断流程
	if err := runNezha(); err != nil {
		log.Printf("Nezha running error: %v", err)
	}

	// 运行 Xray - 必须成功
	if err := runXray(); err != nil {
		return fmt.Errorf("Xray running error: %v", err)
	}

	// 运行 Cloudflared - 错误不中断流程
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
				log.Println("Tunnel YAML configuration generated")
			}
		}
	} else {
		log.Println("ARGO_AUTH mismatch TunnelSecret, use token connect to tunnel")
	}
}

// 获取 ISP 信息
func getMetaInfo() (string, error) {
	// 尝试 ipapi.co
	resp, err := httpClient.Get("https://ipapi.co/json/")
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
	resp, err = httpClient.Get("http://ip-api.com/json/")
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

	vmess := VMESS{
		V:    "2",
		Ps:   nodeName,
		Add:  cfip,
		Port: cfport,
		ID:   uuid,
		Aid:  "0",
		Scy:  "auto",
		Net:  "ws",
		Type: "none",
		Host: argoDomain,
		Path: "/vmess-argo?ed=2560",
		TLS:  "tls",
		Sni:  argoDomain,
		Alpn: "",
		Fp:   "firefox",
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
	log.Printf("Subscription content:\n%s", encoded)

	if err := os.WriteFile(subFilePath, []byte(encoded), 0644); err != nil {
		return err
	}
	log.Printf("%s/sub.txt saved successfully", filePath)

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

		resp, err := httpClient.Post(uploadURL+"/api/add-subscriptions", "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("Failed to upload subscription: %v", err)
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
		resp, err := httpClient.Post(uploadURL+"/api/add-nodes", "application/json", bytes.NewBuffer(jsonData))
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
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		if fileExists(bootLogPath) {
			break
		}
		time.Sleep(1 * time.Second)
	}

	content, err := os.ReadFile(bootLogPath)
	if err != nil {
		return fmt.Errorf("failed to read boot.log: %v", err)
	}

	// 编译正则表达式
	re := regexp.MustCompile(`https?://([^ ]*trycloudflare\.com)/?`)
	matches := re.FindAllStringSubmatch(string(content), -1)

	if len(matches) > 0 && len(matches[0]) > 1 {
		argoDomain := matches[0][1]
		log.Println("Found ArgoDomain:", argoDomain)
		return generateLinks(argoDomain)
	}

	log.Println("ArgoDomain not found, re-running bot to obtain ArgoDomain")
	os.Remove(bootLogPath)

	// 停止现有进程
	processMutex.Lock()
	if cloudflaredProcess != nil {
		cloudflaredProcess.Kill()
		cloudflaredProcess = nil
	}
	processMutex.Unlock()
	time.Sleep(3 * time.Second)

	// 重新启动
	args := []string{"tunnel", "--edge-ip-version", "auto", "--no-autoupdate", "--protocol", "http2",
		"--logfile", bootLogPath, "--loglevel", "info",
		"--url", fmt.Sprintf("http://localhost:%d", argoPort)}

	cmd := exec.Command(botPath, args...)
	cmd.Dir = filePath

	proc, err := startProcess(cmd, botName)
	if err != nil {
		return fmt.Errorf("failed to restart cloudflared: %v", err)
	}

	processMutex.Lock()
	cloudflaredProcess = proc
	processMutex.Unlock()

	time.Sleep(5 * time.Second)
	return extractDomains() // 递归重试
}

// 添加自动访问任务
func addVisitTask() {
	if !autoAccess || projectURL == "" {
		log.Println("Skipping adding automatic access task")
		return
	}

	data := map[string]string{"url": projectURL}
	jsonData, _ := json.Marshal(data)
	resp, err := httpClient.Post("https://oooo.serv00.net/add-url", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Add automatic access task failed: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Println("Automatic access task added successfully")
}

// 停止所有进程
func stopAllProcesses() {
	log.Println("Stopping all processes...")
	processMutex.Lock()
	defer processMutex.Unlock()

	stopProcess := func(proc *os.Process, name string) {
		if proc == nil {
			return
		}
		log.Printf("Stopping %s (PID: %d)...", name, proc.Pid)

		// 先尝试优雅停止
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			log.Printf("Failed to send SIGTERM to %s: %v", name, err)
		}

		// 等待 5 秒后强制杀死
		time.AfterFunc(5*time.Second, func() {
			if err := proc.Kill(); err != nil {
				log.Printf("Failed to kill %s: %v", name, err)
			}
		})
	}

	stopProcess(nezhaProcess, "Nezha")
	stopProcess(xrayProcess, "Xray")
	stopProcess(cloudflaredProcess, "Cloudflared")

	// 清空进程引用
	nezhaProcess = nil
	xrayProcess = nil
	cloudflaredProcess = nil
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
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				log.Printf("Failed to delete %s: %v", file, err)
			}
		}

		log.Println("App is running")
		log.Println("Thank you for using this script, enjoy!")
	})
}

// HTTP 服务
func startHTTPServer() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		indexPath := "index.html"
		if fileExists(indexPath) {
			http.ServeFile(w, r, indexPath)
		} else {
			w.Write([]byte("Hello world!<br><br>You can access /" + subPath + " to get your nodes!"))
		}
	})

	// 订阅路由
	http.HandleFunc("/"+subPath, func(w http.ResponseWriter, r *http.Request) {
		if fileExists(subFilePath) {
			content, err := os.ReadFile(subFilePath)
			if err == nil {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.Write(content)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Subscription not found"))
	})

	log.Printf("HTTP server is running on port: %d", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatal("HTTP server error:", err)
	}
}

func main() {
	// 设置日志格式
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 初始化
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

		// 等待所有进程退出
		done := make(chan struct{})
		go func() {
			processWg.Wait()
			close(done)
		}()

		select {
		case <-done:
			log.Println("All processes stopped gracefully")
		case <-time.After(10 * time.Second):
			log.Println("Timeout waiting for processes to stop")
		}

		os.Exit(0)
	}()

	// 启动前清理
	deleteNodes()
	cleanupOldFiles()

	// 配置 Argo 隧道
	argoType()

	// 生成 Xray 配置
	if err := generateConfig(); err != nil {
		log.Fatal("Failed to generate config:", err)
	}

	// 下载并运行依赖
	if err := downloadFilesAndRun(); err != nil {
		log.Fatal("Failed to download and run files:", err)
	}

	// 提取域名（带重试）
	for i := 0; i < 3; i++ {
		if err := extractDomains(); err == nil {
			break
		} else if i == 2 {
			log.Printf("Failed to extract domains after retries: %v", err)
		} else {
			log.Printf("Retry %d: %v", i+1, err)
			time.Sleep(5 * time.Second)
		}
	}

	// 添加自动访问任务
	addVisitTask()

	// 清理文件
	cleanFiles()

	// 启动 HTTP 服务
	startHTTPServer()
}
