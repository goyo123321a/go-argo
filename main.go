package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

// 配置结构体
type Config struct {
	UploadURL   string `env:"UPLOAD_URL"`
	ProjectURL  string `env:"PROJECT_URL"`
	AutoAccess  bool   `env:"AUTO_ACCESS"`
	FilePath    string `env:"FILE_PATH"`
	SubPath     string `env:"SUB_PATH"`
	Port        string `env:"SERVER_PORT"`
	UUID        string `env:"UUID"`
	NezhaServer string `env:"NEZHA_SERVER"`
	NezhaPort   string `env:"NEZHA_PORT"`
	NezhaKey    string `env:"NEZHA_KEY"`
	ArgoDomain  string `env:"ARGO_DOMAIN"`
	ArgoAuth    string `env:"ARGO_AUTH"`
	ArgoPort    string `env:"ARGO_PORT"`
	CFIP        string `env:"CFIP"`
	CFPort      string `env:"CFPORT"`
	Name        string `env:"NAME"`
}

// 进程管理器
type ProcessManager struct {
	mu        sync.Mutex
	processes map[string]*exec.Cmd
}

// 应用结构体
type App struct {
	config       *Config
	pm           *ProcessManager
	filePath     string
	subPath      string
	webPath      string
	botPath      string
	npmPath      string
	phpPath      string
	bootLogPath  string
	configPath   string
	subFilePath  string
	listFilePath string
	indexPath    string
	ctx          context.Context
	cancel       context.CancelFunc
	router       *gin.Engine
}

// Xray配置
type XrayConfig struct {
	Log       XrayLog        `json:"log"`
	DNS       XrayDNS        `json:"dns"`
	Inbounds  []XrayInbound  `json:"inbounds"`
	Outbounds []XrayOutbound `json:"outbounds"`
	Routing   XrayRouting    `json:"routing"`
}

type XrayLog struct {
	Access   string `json:"access"`
	Error    string `json:"error"`
	Loglevel string `json:"loglevel"`
}

type XrayDNS struct {
	Servers []string `json:"servers"`
}

type XrayInbound struct {
	Port           int                    `json:"port"`
	Protocol       string                 `json:"protocol"`
	Listen         string                 `json:"listen,omitempty"`
	Settings       map[string]interface{} `json:"settings"`
	StreamSettings map[string]interface{} `json:"streamSettings"`
	Sniffing       map[string]interface{} `json:"sniffing,omitempty"`
}

type XrayOutbound struct {
	Protocol string                 `json:"protocol"`
	Tag      string                 `json:"tag"`
	Settings map[string]interface{} `json:"settings,omitempty"`
}

type XrayRouting struct {
	DomainStrategy string        `json:"domainStrategy"`
	Rules          []interface{} `json:"rules"`
}

// VMESS配置
type VMESSConfig struct {
	V    string `json:"v"`
	PS   string `json:"ps"`
	Add  string `json:"add"`
	Port string `json:"port"`
	ID   string `json:"id"`
	Aid  string `json:"aid"`
	Scy  string `json:"scy"`
	Net  string `json:"net"`
	Type string `json:"type"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
	Alpn string `json:"alpn"`
	FP   string `json:"fp"`
}

// 默认配置
var defaultConfig = &Config{
	UploadURL:   "",
	ProjectURL:  "",
	AutoAccess:  false,
	FilePath:    "tmp",
	SubPath:     "sub",
	Port:        "3000",
	UUID:        "9afd1229-b893-40c1-84dd-51e7ce204913",
	NezhaServer: "",
	NezhaPort:   "",
	NezhaKey:    "",
	ArgoDomain:  "",
	ArgoAuth:    "",
	ArgoPort:    "8001",
	CFIP:        "saas.sin.fan",
	CFPort:      "443",
	Name:        "",
}

func main() {
	// 加载配置
	config := loadConfig()

	// 创建应用
	app, err := NewApp(config)
	if err != nil {
		fmt.Printf("Failed to create app: %v\n", err)
		os.Exit(1)
	}

	// 启动应用
	if err := app.Run(); err != nil {
		fmt.Printf("Failed to run app: %v\n", err)
		os.Exit(1)
	}
}

// 加载环境变量配置
func loadConfig() *Config {
	config := *defaultConfig

	if val := os.Getenv("UPLOAD_URL"); val != "" {
		config.UploadURL = val
	}
	if val := os.Getenv("PROJECT_URL"); val != "" {
		config.ProjectURL = val
	}
	if val := os.Getenv("AUTO_ACCESS"); val == "true" {
		config.AutoAccess = true
	}
	if val := os.Getenv("FILE_PATH"); val != "" {
		config.FilePath = val
	}
	if val := os.Getenv("SUB_PATH"); val != "" {
		config.SubPath = val
	}
	if val := os.Getenv("SERVER_PORT"); val != "" {
		config.Port = val
	}
	if val := os.Getenv("PORT"); val != "" && config.Port == "3000" {
		config.Port = val
	}
	if val := os.Getenv("UUID"); val != "" {
		config.UUID = val
	}
	if val := os.Getenv("NEZHA_SERVER"); val != "" {
		config.NezhaServer = val
	}
	if val := os.Getenv("NEZHA_PORT"); val != "" {
		config.NezhaPort = val
	}
	if val := os.Getenv("NEZHA_KEY"); val != "" {
		config.NezhaKey = val
	}
	if val := os.Getenv("ARGO_DOMAIN"); val != "" {
		config.ArgoDomain = val
	}
	if val := os.Getenv("ARGO_AUTH"); val != "" {
		config.ArgoAuth = val
	}
	if val := os.Getenv("ARGO_PORT"); val != "" {
		config.ArgoPort = val
	}
	if val := os.Getenv("CFIP"); val != "" {
		config.CFIP = val
	}
	if val := os.Getenv("CFPORT"); val != "" {
		config.CFPort = val
	}
	if val := os.Getenv("NAME"); val != "" {
		config.Name = val
	}

	return &config
}

// 创建新应用
func NewApp(config *Config) (*App, error) {
	ctx, cancel := context.WithCancel(context.Background())

	app := &App{
		config:       config,
		pm:           &ProcessManager{processes: make(map[string]*exec.Cmd)},
		filePath:     config.FilePath,
		subPath:      config.SubPath,
		ctx:          ctx,
		cancel:       cancel,
		subFilePath:  filepath.Join(config.FilePath, "sub.txt"),
		listFilePath: filepath.Join(config.FilePath, "list.txt"),
		bootLogPath:  filepath.Join(config.FilePath, "boot.log"),
		configPath:   filepath.Join(config.FilePath, "config.json"),
		indexPath:    "index.html",
	}

	// 生成随机文件名
	app.webPath = filepath.Join(config.FilePath, generateRandomName())
	app.botPath = filepath.Join(config.FilePath, generateRandomName())
	app.npmPath = filepath.Join(config.FilePath, generateRandomName())
	app.phpPath = filepath.Join(config.FilePath, generateRandomName())

	// 创建运行目录
	if err := os.MkdirAll(config.FilePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create file path: %w", err)
	}

	// 设置路由
	app.setupRouter()

	return app, nil
}

// 生成随机6位字符文件名
func generateRandomName() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 6)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(1 * time.Nanosecond)
	}
	return string(b)
}

// 设置路由
func (app *App) setupRouter() {
	router := gin.Default()

	// 根路由 - 与 Node.js 版本行为一致
	router.GET("/", func(c *gin.Context) {
		// 尝试读取 index.html 文件
		if data, err := os.ReadFile(app.indexPath); err == nil {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(http.StatusOK, string(data))
		} else {
			// 如果文件不存在，返回默认消息
			subPath := app.subPath
			defaultMsg := fmt.Sprintf("Hello world!<br><br>You can access /%s to get your nodes!", subPath)
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(http.StatusOK, defaultMsg)
		}
	})

	// 健康检查
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"time":   time.Now().Unix(),
		})
	})

	app.router = router
}

// 运行应用
func (app *App) Run() error {
	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal, cleaning up...")
		app.cleanup()
		app.cancel()
		os.Exit(0)
	}()

	// 启动主流程
	go func() {
		time.Sleep(1 * time.Second)
		if err := app.startServer(); err != nil {
			fmt.Printf("Error in startServer: %v\n", err)
		}
	}()

	// 清理文件
	go app.cleanFiles()

	// 启动HTTP服务
	addr := ":" + app.config.Port
	fmt.Printf("HTTP server is running on port %s!\n", app.config.Port)
	return app.router.Run(addr)
}

// 启动服务器主流程
func (app *App) startServer() error {
	// 删除历史节点
	app.deleteNodes()

	// 清理历史文件
	app.cleanupOldFiles()

	// 处理Argo隧道配置
	app.argoType()

	// 生成Xray配置
	if err := app.generateConfig(); err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	// 下载并运行依赖文件
	if err := app.downloadFilesAndRun(); err != nil {
		return fmt.Errorf("failed to download and run files: %w", err)
	}

	// 提取域名
	if err := app.extractDomains(); err != nil {
		return fmt.Errorf("failed to extract domains: %w", err)
	}

	// 添加自动访问任务
	app.addVisitTask()

	return nil
}

// 删除历史节点
func (app *App) deleteNodes() {
	if app.config.UploadURL == "" {
		return
	}

	data, err := os.ReadFile(app.subFilePath)
	if err != nil {
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return
	}

	lines := strings.Split(string(decoded), "\n")
	var nodes []string
	for _, line := range lines {
		if strings.HasPrefix(line, "vless://") ||
			strings.HasPrefix(line, "vmess://") ||
			strings.HasPrefix(line, "trojan://") {
			nodes = append(nodes, line)
		}
	}

	if len(nodes) == 0 {
		return
	}

	nodeInfo := map[string][]string{"nodes": nodes}
	jsonData, _ := json.Marshal(nodeInfo)

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("POST", app.config.UploadURL+"/api/delete-nodes", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	client.Do(req)
}

// 清理旧文件
func (app *App) cleanupOldFiles() {
	files, err := os.ReadDir(app.filePath)
	if err != nil {
		return
	}

	for _, file := range files {
		if !file.IsDir() {
			os.Remove(filepath.Join(app.filePath, file.Name()))
		}
	}
}

// 生成Xray配置
func (app *App) generateConfig() error {
	argoPort, _ := strconv.Atoi(app.config.ArgoPort)

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
				Settings: map[string]interface{}{
					"clients": []map[string]interface{}{
						{"id": app.config.UUID, "flow": "xtls-rprx-vision"},
					},
					"decryption": "none",
					"fallbacks": []map[string]interface{}{
						{"dest": 3001},
						{"path": "/vless-argo", "dest": 3002},
						{"path": "/vmess-argo", "dest": 3003},
						{"path": "/trojan-argo", "dest": 3004},
					},
				},
				StreamSettings: map[string]interface{}{
					"network": "tcp",
				},
			},
			{
				Port:     3001,
				Listen:   "127.0.0.1",
				Protocol: "vless",
				Settings: map[string]interface{}{
					"clients":    []map[string]interface{}{{"id": app.config.UUID}},
					"decryption": "none",
				},
				StreamSettings: map[string]interface{}{
					"network":  "tcp",
					"security": "none",
				},
			},
			{
				Port:     3002,
				Listen:   "127.0.0.1",
				Protocol: "vless",
				Settings: map[string]interface{}{
					"clients":    []map[string]interface{}{{"id": app.config.UUID, "level": 0}},
					"decryption": "none",
				},
				StreamSettings: map[string]interface{}{
					"network":  "ws",
					"security": "none",
					"wsSettings": map[string]interface{}{
						"path": "/vless-argo",
					},
				},
				Sniffing: map[string]interface{}{
					"enabled":      true,
					"destOverride": []string{"http", "tls", "quic"},
					"metadataOnly": false,
				},
			},
			{
				Port:     3003,
				Listen:   "127.0.0.1",
				Protocol: "vmess",
				Settings: map[string]interface{}{
					"clients": []map[string]interface{}{{"id": app.config.UUID, "alterId": 0}},
				},
				StreamSettings: map[string]interface{}{
					"network": "ws",
					"wsSettings": map[string]interface{}{
						"path": "/vmess-argo",
					},
				},
				Sniffing: map[string]interface{}{
					"enabled":      true,
					"destOverride": []string{"http", "tls", "quic"},
					"metadataOnly": false,
				},
			},
			{
				Port:     3004,
				Listen:   "127.0.0.1",
				Protocol: "trojan",
				Settings: map[string]interface{}{
					"clients": []map[string]interface{}{{"password": app.config.UUID}},
				},
				StreamSettings: map[string]interface{}{
					"network":  "ws",
					"security": "none",
					"wsSettings": map[string]interface{}{
						"path": "/trojan-argo",
					},
				},
				Sniffing: map[string]interface{}{
					"enabled":      true,
					"destOverride": []string{"http", "tls", "quic"},
					"metadataOnly": false,
				},
			},
		},
		Outbounds: []XrayOutbound{
			{Protocol: "freedom", Tag: "direct"},
			{Protocol: "blackhole", Tag: "block"},
		},
		Routing: XrayRouting{
			DomainStrategy: "IPIfNonMatch",
			Rules:          []interface{}{},
		},
	}

	jsonData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(app.configPath, jsonData, 0644)
}

// 判断系统架构
func getSystemArchitecture() string {
	switch runtime.GOARCH {
	case "arm", "arm64", "aarch64":
		return "arm"
	default:
		return "amd"
	}
}

// 下载文件
func (app *App) downloadFile(filePath, fileURL string) error {
	client := &http.Client{Timeout: 60 * time.Second}

	resp, err := client.Get(fileURL)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", fileURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download %s: status %d", fileURL, resp.StatusCode)
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

	if err := os.Chmod(filePath, 0755); err != nil {
		return err
	}

	fmt.Printf("Download %s successfully\n", filepath.Base(filePath))
	return nil
}

// 获取需要下载的文件列表
func (app *App) getFilesForArchitecture(architecture string) []struct {
	path string
	url  string
} {
	var files []struct {
		path string
		url  string
	}

	baseURL := "https://amd64.ssss.nyc.mn"
	if architecture == "arm" {
		baseURL = "https://arm64.ssss.nyc.mn"
	}

	files = append(files, struct{ path string; url string }{app.webPath, baseURL + "/web"})
	files = append(files, struct{ path string; url string }{app.botPath, baseURL + "/bot"})

	if app.config.NezhaServer != "" && app.config.NezhaKey != "" {
		if app.config.NezhaPort != "" {
			files = append([]struct{ path string; url string }{{app.npmPath, baseURL + "/agent"}}, files...)
		} else {
			files = append([]struct{ path string; url string }{{app.phpPath, baseURL + "/v1"}}, files...)
		}
	}

	return files
}

// 下载并运行依赖文件
func (app *App) downloadFilesAndRun() error {
	architecture := getSystemArchitecture()
	files := app.getFilesForArchitecture(architecture)

	var wg sync.WaitGroup
	errChan := make(chan error, len(files))

	for _, file := range files {
		wg.Add(1)
		go func(path, url string) {
			defer wg.Done()
			if err := app.downloadFile(path, url); err != nil {
				errChan <- err
			}
		}(file.path, file.url)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return err
		}
	}

	if app.config.NezhaServer != "" && app.config.NezhaKey != "" {
		if err := app.runNezha(); err != nil {
			fmt.Printf("Failed to run Nezha: %v\n", err)
		}
	}

	if err := app.runXray(); err != nil {
		return fmt.Errorf("failed to run Xray: %w", err)
	}

	if err := app.runCloudflared(); err != nil {
		fmt.Printf("Failed to run Cloudflared: %v\n", err)
	}

	return nil
}

// 运行哪吒监控
func (app *App) runNezha() error {
	if app.config.NezhaPort == "" {
		port := ""
		if strings.Contains(app.config.NezhaServer, ":") {
			parts := strings.Split(app.config.NezhaServer, ":")
			port = parts[len(parts)-1]
		}

		tlsPorts := map[string]bool{"443": true, "8443": true, "2096": true, "2087": true, "2083": true, "2053": true}
		nezhatls := "false"
		if tlsPorts[port] {
			nezhatls = "true"
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
uuid: %s`,
			app.config.NezhaKey, app.config.NezhaServer, nezhatls, app.config.UUID)

		configYamlPath := filepath.Join(app.filePath, "config.yaml")
		if err := os.WriteFile(configYamlPath, []byte(configYaml), 0644); err != nil {
			return err
		}

		cmd := exec.CommandContext(app.ctx, app.phpPath, "-c", configYamlPath)
		cmd.Dir = app.filePath
		cmd.Stdout = nil
		cmd.Stderr = nil

		if err := cmd.Start(); err != nil {
			return err
		}

		app.pm.mu.Lock()
		app.pm.processes["nezha"] = cmd
		app.pm.mu.Unlock()

		fmt.Printf("%s is running (PID: %d)\n", filepath.Base(app.phpPath), cmd.Process.Pid)
	} else {
		args := []string{
			"-s", fmt.Sprintf("%s:%s", app.config.NezhaServer, app.config.NezhaPort),
			"-p", app.config.NezhaKey,
		}

		tlsPorts := []string{"443", "8443", "2096", "2087", "2083", "2053"}
		for _, p := range tlsPorts {
			if p == app.config.NezhaPort {
				args = append(args, "--tls")
				break
			}
		}

		args = append(args, "--disable-auto-update", "--report-delay", "4", "--skip-conn", "--skip-procs")

		cmd := exec.CommandContext(app.ctx, app.npmPath, args...)
		cmd.Dir = app.filePath
		cmd.Stdout = nil
		cmd.Stderr = nil

		if err := cmd.Start(); err != nil {
			return err
		}

		app.pm.mu.Lock()
		app.pm.processes["nezha"] = cmd
		app.pm.mu.Unlock()

		fmt.Printf("%s is running (PID: %d)\n", filepath.Base(app.npmPath), cmd.Process.Pid)
	}

	time.Sleep(1 * time.Second)
	return nil
}

// 运行Xray
func (app *App) runXray() error {
	cmd := exec.CommandContext(app.ctx, app.webPath, "-c", app.configPath)
	cmd.Dir = app.filePath
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return err
	}

	app.pm.mu.Lock()
	app.pm.processes["xray"] = cmd
	app.pm.mu.Unlock()

	fmt.Printf("%s is running (PID: %d)\n", filepath.Base(app.webPath), cmd.Process.Pid)
	time.Sleep(1 * time.Second)

	return nil
}

// 运行Cloudflared
func (app *App) runCloudflared() error {
	if _, err := os.Stat(app.botPath); os.IsNotExist(err) {
		return nil
	}

	args := []string{"tunnel", "--edge-ip-version", "auto", "--no-autoupdate", "--protocol", "http2"}

	if app.config.ArgoAuth != "" {
		if len(app.config.ArgoAuth) >= 120 && len(app.config.ArgoAuth) <= 250 {
			args = append(args, "run", "--token", app.config.ArgoAuth)
		} else if strings.Contains(app.config.ArgoAuth, "TunnelSecret") {
			args = append(args, "--config", filepath.Join(app.filePath, "tunnel.yml"), "run")
		}
	}

	if len(args) == 4 {
		args = append(args, "--logfile", app.bootLogPath, "--loglevel", "info",
			"--url", fmt.Sprintf("http://localhost:%s", app.config.ArgoPort))
	}

	cmd := exec.CommandContext(app.ctx, app.botPath, args...)
	cmd.Dir = app.filePath
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return err
	}

	app.pm.mu.Lock()
	app.pm.processes["cloudflared"] = cmd
	app.pm.mu.Unlock()

	fmt.Printf("%s is running (PID: %d)\n", filepath.Base(app.botPath), cmd.Process.Pid)
	time.Sleep(2 * time.Second)

	return nil
}

// 处理Argo隧道配置
func (app *App) argoType() {
	if app.config.ArgoAuth == "" || app.config.ArgoDomain == "" {
		fmt.Println("ARGO_DOMAIN or ARGO_AUTH variable is empty, use quick tunnels")
		return
	}

	if strings.Contains(app.config.ArgoAuth, "TunnelSecret") {
		tunnelJsonPath := filepath.Join(app.filePath, "tunnel.json")
		if err := os.WriteFile(tunnelJsonPath, []byte(app.config.ArgoAuth), 0644); err != nil {
			fmt.Printf("Failed to write tunnel.json: %v\n", err)
			return
		}

		var tunnelData map[string]interface{}
		if err := json.Unmarshal([]byte(app.config.ArgoAuth), &tunnelData); err == nil {
			if tunnelID, ok := tunnelData["TunnelID"]; ok {
				tunnelYaml := fmt.Sprintf(`
tunnel: %s
credentials-file: %s
protocol: http2

ingress:
  - hostname: %s
    service: http://localhost:%s
    originRequest:
      noTLSVerify: true
  - service: http_status:404`,
					tunnelID, tunnelJsonPath, app.config.ArgoDomain, app.config.ArgoPort)

				tunnelYamlPath := filepath.Join(app.filePath, "tunnel.yml")
				if err := os.WriteFile(tunnelYamlPath, []byte(tunnelYaml), 0644); err != nil {
					fmt.Printf("Failed to write tunnel.yml: %v\n", err)
				}
			}
		}
	} else {
		fmt.Println("ARGO_AUTH mismatch TunnelSecret, use token connect to tunnel")
	}
}

// 提取域名
func (app *App) extractDomains() error {
	var argoDomain string

	if app.config.ArgoAuth != "" && app.config.ArgoDomain != "" {
		argoDomain = app.config.ArgoDomain
		fmt.Println("ARGO_DOMAIN:", argoDomain)
		return app.generateLinks(argoDomain)
	}

	data, err := os.ReadFile(app.bootLogPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	var domains []string

	for _, line := range lines {
		start := strings.Index(line, "https://")
		if start != -1 {
			end := strings.Index(line[start:], " ")
			if end == -1 {
				end = len(line) - start
			}
			urlStr := line[start : start+end]
			if strings.Contains(urlStr, "trycloudflare.com") {
				domain := strings.TrimPrefix(urlStr, "https://")
				domain = strings.TrimSuffix(domain, "/")
				domains = append(domains, domain)
			}
		}
	}

	if len(domains) > 0 {
		argoDomain = domains[0]
		fmt.Println("ArgoDomain:", argoDomain)
		return app.generateLinks(argoDomain)
	}

	fmt.Println("ArgoDomain not found, re-running bot to obtain ArgoDomain")
	os.Remove(app.bootLogPath)

	app.pm.mu.Lock()
	if cmd, ok := app.pm.processes["cloudflared"]; ok {
		cmd.Process.Kill()
		delete(app.pm.processes, "cloudflared")
	}
	app.pm.mu.Unlock()

	time.Sleep(3 * time.Second)

	args := []string{"tunnel", "--edge-ip-version", "auto", "--no-autoupdate", "--protocol", "http2",
		"--logfile", app.bootLogPath, "--loglevel", "info",
		"--url", fmt.Sprintf("http://localhost:%s", app.config.ArgoPort)}

	cmd := exec.CommandContext(app.ctx, app.botPath, args...)
	cmd.Dir = app.filePath
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return err
	}

	app.pm.mu.Lock()
	app.pm.processes["cloudflared"] = cmd
	app.pm.mu.Unlock()

	fmt.Printf("%s is running (PID: %d)\n", filepath.Base(app.botPath), cmd.Process.Pid)
	time.Sleep(3 * time.Second)

	return app.extractDomains()
}

// 获取ISP信息
func (app *App) getMetaInfo() (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}

	resp, err := client.Get("https://ipapi.co/json/")
	if err == nil {
		defer resp.Body.Close()
		var data struct {
			CountryCode string `json:"country_code"`
			Org         string `json:"org"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil && data.CountryCode != "" && data.Org != "" {
			return fmt.Sprintf("%s_%s", data.CountryCode, strings.ReplaceAll(data.Org, " ", "_")), nil
		}
	}

	resp, err = client.Get("http://ip-api.com/json/")
	if err == nil {
		defer resp.Body.Close()
		var data struct {
			Status      string `json:"status"`
			CountryCode string `json:"countryCode"`
			Org         string `json:"org"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil && data.Status == "success" {
			return fmt.Sprintf("%s_%s", data.CountryCode, strings.ReplaceAll(data.Org, " ", "_")), nil
		}
	}

	return "Unknown", nil
}

// 生成订阅链接
func (app *App) generateLinks(argoDomain string) error {
	isp, _ := app.getMetaInfo()
	nodeName := isp
	if app.config.Name != "" {
		nodeName = fmt.Sprintf("%s-%s", app.config.Name, isp)
	}

	vmess := VMESSConfig{
		V:    "2",
		PS:   nodeName,
		Add:  app.config.CFIP,
		Port: app.config.CFPort,
		ID:   app.config.UUID,
		Aid:  "0",
		Scy:  "auto",
		Net:  "ws",
		Type: "none",
		Host: argoDomain,
		Path: "/vmess-argo?ed=2560",
		TLS:  "tls",
		SNI:  argoDomain,
		Alpn: "",
		FP:   "firefox",
	}

	vmessJSON, _ := json.Marshal(vmess)
	vmessBase64 := base64.StdEncoding.EncodeToString(vmessJSON)

	subTxt := fmt.Sprintf(`
vless://%s@%s:%s?encryption=none&security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Fvless-argo%%3Fed%%3D2560#%s

vmess://%s

trojan://%s@%s:%s?security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Ftrojan-argo%%3Fed%%3D2560#%s`,
		app.config.UUID, app.config.CFIP, app.config.CFPort, argoDomain, argoDomain, nodeName,
		vmessBase64,
		app.config.UUID, app.config.CFIP, app.config.CFPort, argoDomain, argoDomain, nodeName)

	encoded := base64.StdEncoding.EncodeToString([]byte(subTxt))
	if err := os.WriteFile(app.subFilePath, []byte(encoded), 0644); err != nil {
		return err
	}
	fmt.Printf("%s/sub.txt saved successfully\n", app.filePath)
	fmt.Println(encoded)

	// 添加订阅路由
	app.router.GET("/"+app.subPath, func(c *gin.Context) {
		c.String(http.StatusOK, encoded)
	})

	app.uploadNodes()

	return nil
}

// 上传节点
func (app *App) uploadNodes() {
	client := &http.Client{Timeout: 10 * time.Second}

	if app.config.UploadURL != "" && app.config.ProjectURL != "" {
		subscriptionURL := fmt.Sprintf("%s/%s", app.config.ProjectURL, app.subPath)
		subInfo := map[string][]string{"subscription": {subscriptionURL}}
		jsonData, _ := json.Marshal(subInfo)

		req, _ := http.NewRequest("POST", app.config.UploadURL+"/api/add-subscriptions", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			fmt.Println("Subscription uploaded successfully")
		}
		if resp != nil {
			defer resp.Body.Close()
		}
	} else if app.config.UploadURL != "" {
		data, err := os.ReadFile(app.listFilePath)
		if err != nil {
			return
		}

		lines := strings.Split(string(data), "\n")
		var nodes []string
		for _, line := range lines {
			if strings.HasPrefix(line, "vless://") ||
				strings.HasPrefix(line, "vmess://") ||
				strings.HasPrefix(line, "trojan://") {
				nodes = append(nodes, line)
			}
		}

		if len(nodes) == 0 {
			return
		}

		nodeInfo := map[string][]string{"nodes": nodes}
		jsonData, _ := json.Marshal(nodeInfo)

		req, _ := http.NewRequest("POST", app.config.UploadURL+"/api/add-nodes", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			fmt.Println("Nodes uploaded successfully")
		}
		if resp != nil {
			defer resp.Body.Close()
		}
	}
}

// 添加自动访问任务
func (app *App) addVisitTask() {
	if !app.config.AutoAccess || app.config.ProjectURL == "" {
		fmt.Println("Skipping adding automatic access task")
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	data := map[string]string{"url": app.config.ProjectURL}
	jsonData, _ := json.Marshal(data)

	resp, err := client.Post("https://oooo.serv00.net/add-url", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Add automatic access task failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Println("automatic access task added successfully")
}

// 清理文件
func (app *App) cleanFiles() {
	time.Sleep(90 * time.Second)

	filesToDelete := []string{
		app.bootLogPath,
		app.configPath,
		app.webPath,
		app.botPath,
	}

	if app.config.NezhaPort != "" {
		filesToDelete = append(filesToDelete, app.npmPath)
	} else if app.config.NezhaServer != "" && app.config.NezhaKey != "" {
		filesToDelete = append(filesToDelete, app.phpPath)
	}

	for _, file := range filesToDelete {
		os.Remove(file)
	}

	fmt.Println("App is running")
	fmt.Println("Thank you for using this script, enjoy!")
}

// 清理所有资源
func (app *App) cleanup() {
	app.pm.mu.Lock()
	defer app.pm.mu.Unlock()

	for name, cmd := range app.pm.processes {
		if cmd.Process != nil {
			fmt.Printf("Stopping %s process...\n", name)
			cmd.Process.Kill()
		}
	}
}