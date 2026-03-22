package main

import (
	"bufio"
	"crypto/rand"
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
	"strings"
	"sync"
	"syscall"
	"time"
)

// 配置结构
type XrayConfig struct {
	Log struct {
		Access  string `json:"access"`
		Error   string `json:"error"`
		Loglevel string `json:"loglevel"`
	} `json:"log"`
	DNS struct {
		Servers []string `json:"servers"`
	} `json:"dns"`
	Inbounds  []Inbound  `json:"inbounds"`
	Outbounds []Outbound `json:"outbounds"`
}

type Inbound struct {
	Port           int                    `json:"port"`
	Listen         string                 `json:"listen,omitempty"`
	Protocol       string                 `json:"protocol"`
	Settings       map[string]interface{} `json:"settings"`
	StreamSettings map[string]interface{} `json:"streamSettings"`
	Sniffing       map[string]interface{} `json:"sniffing,omitempty"`
}

type Outbound struct {
	Protocol string `json:"protocol"`
	Tag      string `json:"tag"`
}

// 环境变量
var (
	UploadURL   = getEnv("UPLOAD_URL", "")
	ProjectURL  = getEnv("PROJECT_URL", "")
	AutoAccess  = getEnv("AUTO_ACCESS", "false") == "true"
	FilePath    = getEnv("FILE_PATH", ".tmp")
	SubPath     = getEnv("SUB_PATH", "sub")
	Port        = getEnv("SERVER_PORT", getEnv("PORT", "3000"))
	UUID        = getEnv("UUID", "9afd1229-b893-40c1-84dd-51e7ce204913")
	NezhaServer = getEnv("NEZHA_SERVER", "")
	NezhaPort   = getEnv("NEZHA_PORT", "")
	NezhaKey    = getEnv("NEZHA_KEY", "")
	ArgoDomain  = getEnv("ARGO_DOMAIN", "")
	ArgoAuth    = getEnv("ARGO_AUTH", "")
	ArgoPort    = getEnvInt("ARGO_PORT", 8001)
	CFIP        = getEnv("CFIP", "saas.sin.fan")
	CFPORT      = getEnvInt("CFPORT", 443)
	Name        = getEnv("NAME", "")
)

// 随机文件名
var (
	npmName      = generateRandomName()
	webName      = generateRandomName()
	botName      = generateRandomName()
	phpName      = generateRandomName()
	npmPath      string
	phpPath      string
	webPath      string
	botPath      string
	subPath      string
	listPath     string
	bootLogPath  string
	configPath   string
)

// 进程管理
var (
	nezhaProcess     *os.Process
	xrayProcess      *os.Process
	cloudflaredProcess *os.Process
	processMutex     sync.Mutex
)

// 订阅缓存
var subscriptionCache []byte
var cacheMutex sync.RWMutex

func main() {
	// 初始化路径
	initPaths()
	
	// 创建运行目录
	if err := os.MkdirAll(FilePath, 0755); err != nil {
		fmt.Printf("%s is created\n", FilePath)
	} else {
		fmt.Printf("%s is created\n", FilePath)
	}
	
	// 启动 HTTP 服务器
	go startHTTPServer()
	
	// 等待服务器启动
	time.Sleep(1 * time.Second)
	
	// 启动主流程
	go func() {
		if err := startServer(); err != nil {
			fmt.Printf("Error in startServer: %v\n", err)
		}
	}()
	
	// 清理文件
	cleanFiles()
	
	// 信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	fmt.Println("\nReceived shutdown signal. Stopping processes...")
	stopAllProcesses()
	os.Exit(0)
}

func initPaths() {
	npmPath = filepath.Join(FilePath, npmName)
	phpPath = filepath.Join(FilePath, phpName)
	webPath = filepath.Join(FilePath, webName)
	botPath = filepath.Join(FilePath, botName)
	subPath = filepath.Join(FilePath, "sub.txt")
	listPath = filepath.Join(FilePath, "list.txt")
	bootLogPath = filepath.Join(FilePath, "boot.log")
	configPath = filepath.Join(FilePath, "config.json")
}

func generateRandomName() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 6)
	for i := range b {
		b[i] = letters[randInt(len(letters))]
	}
	return string(b)
}

func randInt(n int) int {
	b := make([]byte, 1)
	rand.Read(b)
	return int(b[0]) % n
}

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

func startHTTPServer() {
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/"+SubPath, handleSub)
	
	fmt.Printf("http server is running on port:%s!\n", Port)
	if err := http.ListenAndServe(":"+Port, nil); err != nil {
		fmt.Printf("HTTP server error: %v\n", err)
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	indexPath := filepath.Join(".", "index.html")
	if data, err := os.ReadFile(indexPath); err == nil {
		w.Write(data)
	} else {
		w.Write([]byte("Hello world!<br><br>You can access /" + SubPath + " to get your nodes!"))
	}
}

func handleSub(w http.ResponseWriter, r *http.Request) {
	cacheMutex.RLock()
	cache := subscriptionCache
	cacheMutex.RUnlock()
	
	if len(cache) > 0 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(cache)
		return
	}
	
	// 如果缓存为空，尝试读取文件
	if data, err := os.ReadFile(subPath); err == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(data)
	} else {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Subscription not found"))
	}
}

func startServer() error {
	fmt.Println("Starting server initialization...")
	
	// 配置 Argo
	argoType()
	
	// 删除历史节点
	deleteNodes()
	
	// 清理旧文件
	cleanupOldFiles()
	
	// 生成 Xray 配置
	if err := generateConfig(); err != nil {
		return fmt.Errorf("generate config: %w", err)
	}
	
	// 下载并运行依赖
	if err := downloadFilesAndRun(); err != nil {
		return fmt.Errorf("download and run: %w", err)
	}
	
	// 等待服务启动
	time.Sleep(5 * time.Second)
	
	// 提取域名
	if err := extractDomains(); err != nil {
		fmt.Printf("Error extracting domains: %v\n", err)
	}
	
	// 添加自动访问任务
	addVisitTask()
	
	fmt.Println("Server initialization completed")
	return nil
}

func deleteNodes() {
	if UploadURL == "" {
		return
	}
	
	data, err := os.ReadFile(subPath)
	if err != nil {
		return
	}
	
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return
	}
	
	scanner := bufio.NewScanner(strings.NewReader(string(decoded)))
	var nodes []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "vless://") || strings.Contains(line, "vmess://") ||
			strings.Contains(line, "trojan://") || strings.Contains(line, "hysteria2://") ||
			strings.Contains(line, "tuic://") {
			nodes = append(nodes, line)
		}
	}
	
	if len(nodes) == 0 {
		return
	}
	
	jsonData, _ := json.Marshal(map[string][]string{"nodes": nodes})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(UploadURL+"/api/delete-nodes", "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func cleanupOldFiles() {
	entries, err := os.ReadDir(FilePath)
	if err != nil {
		return
	}
	
	for _, entry := range entries {
		if !entry.IsDir() {
			filePath := filepath.Join(FilePath, entry.Name())
			os.Remove(filePath)
		}
	}
}

func generateConfig() error {
	config := XrayConfig{
		Inbounds: []Inbound{
			{
				Port:     ArgoPort,
				Protocol: "vless",
				Settings: map[string]interface{}{
					"clients": []map[string]interface{}{
						{"id": UUID, "flow": "xtls-rprx-vision"},
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
					"clients":    []map[string]interface{}{{"id": UUID}},
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
					"clients":    []map[string]interface{}{{"id": UUID, "level": 0}},
					"decryption": "none",
				},
				StreamSettings: map[string]interface{}{
					"network": "ws",
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
					"clients": []map[string]interface{}{{"id": UUID, "alterId": 0}},
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
					"clients": []map[string]interface{}{{"password": UUID}},
				},
				StreamSettings: map[string]interface{}{
					"network": "ws",
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
		DNS: struct {
			Servers []string `json:"servers"`
		}{
			Servers: []string{"https+local://8.8.8.8/dns-query"},
		},
		Outbounds: []Outbound{
			{Protocol: "freedom", Tag: "direct"},
			{Protocol: "blackhole", Tag: "block"},
		},
	}
	
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(configPath, data, 0644)
}

func getSystemArchitecture() string {
	arch := runtime.GOARCH
	if arch == "arm" || arch == "arm64" || arch == "aarch64" {
		return "arm"
	}
	return "amd"
}

func downloadFile(filePath, fileURL string) error {
	fmt.Printf("Download %s from %s\n", filepath.Base(filePath), fileURL)
	
	resp, err := http.Get(fileURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	
	out, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer out.Close()
	
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		os.Remove(filePath)
		return err
	}
	
	fmt.Printf("Download %s successfully\n", filepath.Base(filePath))
	return nil
}

func downloadFilesAndRun() error {
	arch := getSystemArchitecture()
	filesToDownload := getFilesForArchitecture(arch)
	
	// 下载文件
	for _, file := range filesToDownload {
		if err := downloadFile(file.fileName, file.fileUrl); err != nil {
			fmt.Printf("Download %s failed: %v\n", file.fileName, err)
			continue
		}
	}
	
	// 授权文件
	filesToAuthorize := []string{webPath, botPath}
	if NezhaPort != "" {
		filesToAuthorize = append(filesToAuthorize, npmPath)
	} else if NezhaServer != "" && NezhaKey != "" {
		filesToAuthorize = append(filesToAuthorize, phpPath)
	}
	
	for _, file := range filesToAuthorize {
		if err := os.Chmod(file, 0775); err != nil {
			fmt.Printf("Empowerment failed for %s: %v\n", file, err)
		} else {
			fmt.Printf("Empowerment success for %s\n", file)
		}
	}
	
	// 运行哪吒监控
	if NezhaServer != "" && NezhaKey != "" {
		if NezhaPort == "" {
			if err := runNezhaV1(); err != nil {
				fmt.Printf("Error running Nezha v1: %v\n", err)
			}
		} else {
			if err := runNezhaV0(); err != nil {
				fmt.Printf("Error running Nezha v0: %v\n", err)
			}
		}
		time.Sleep(1 * time.Second)
	} else {
		fmt.Println("NEZHA variable is empty,skip running")
	}
	
	// 运行 Xray
	if err := runXray(); err != nil {
		return fmt.Errorf("run xray: %w", err)
	}
	time.Sleep(1 * time.Second)
	
	// 运行 Cloudflared
	if err := runCloudflared(); err != nil {
		fmt.Printf("Error running cloudflared: %v\n", err)
	}
	time.Sleep(2 * time.Second)
	
	return nil
}

type fileInfo struct {
	fileName string
	fileUrl  string
}

func getFilesForArchitecture(arch string) []fileInfo {
	var baseFiles []fileInfo
	
	if arch == "arm" {
		baseFiles = []fileInfo{
			{fileName: webPath, fileUrl: "https://arm64.ssss.nyc.mn/web"},
			{fileName: botPath, fileUrl: "https://arm64.ssss.nyc.mn/bot"},
		}
	} else {
		baseFiles = []fileInfo{
			{fileName: webPath, fileUrl: "https://amd64.ssss.nyc.mn/web"},
			{fileName: botPath, fileUrl: "https://amd64.ssss.nyc.mn/bot"},
		}
	}
	
	if NezhaServer != "" && NezhaKey != "" {
		if NezhaPort != "" {
			npmUrl := "https://amd64.ssss.nyc.mn/agent"
			if arch == "arm" {
				npmUrl = "https://arm64.ssss.nyc.mn/agent"
			}
			baseFiles = append([]fileInfo{{fileName: npmPath, fileUrl: npmUrl}}, baseFiles...)
		} else {
			phpUrl := "https://amd64.ssss.nyc.mn/v1"
			if arch == "arm" {
				phpUrl = "https://arm64.ssss.nyc.mn/v1"
			}
			baseFiles = append([]fileInfo{{fileName: phpPath, fileUrl: phpUrl}}, baseFiles...)
		}
	}
	
	return baseFiles
}

func runNezhaV1() error {
	// 检测是否开启TLS
	server := NezhaServer
	port := ""
	if strings.Contains(server, ":") {
		parts := strings.Split(server, ":")
		server = parts[0]
		port = parts[1]
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
uuid: %s`, NezhaKey, NezhaServer, nezhatls, UUID)
	
	configYamlPath := filepath.Join(FilePath, "config.yaml")
	if err := os.WriteFile(configYamlPath, []byte(configYaml), 0644); err != nil {
		return err
	}
	
	cmd := exec.Command(phpPath, "-c", configYamlPath)
	cmd.Dir = FilePath
	cmd.Stdout = nil
	cmd.Stderr = nil
	
	if err := cmd.Start(); err != nil {
		return err
	}
	
	processMutex.Lock()
	nezhaProcess = cmd.Process
	processMutex.Unlock()
	
	fmt.Printf("%s is running\n", phpName)
	
	// 监控进程退出
	go func() {
		cmd.Wait()
		processMutex.Lock()
		nezhaProcess = nil
		processMutex.Unlock()
	}()
	
	return nil
}

func runNezhaV0() error {
	args := []string{
		"-s", NezhaServer + ":" + NezhaPort,
		"-p", NezhaKey,
	}
	
	tlsPorts := []string{"443", "8443", "2096", "2087", "2083", "2053"}
	for _, p := range tlsPorts {
		if NezhaPort == p {
			args = append(args, "--tls")
			break
		}
	}
	
	args = append(args, "--disable-auto-update", "--report-delay", "4", "--skip-conn", "--skip-procs")
	
	cmd := exec.Command(npmPath, args...)
	cmd.Dir = FilePath
	cmd.Stdout = nil
	cmd.Stderr = nil
	
	if err := cmd.Start(); err != nil {
		return err
	}
	
	processMutex.Lock()
	nezhaProcess = cmd.Process
	processMutex.Unlock()
	
	fmt.Printf("%s is running\n", npmName)
	
	go func() {
		cmd.Wait()
		processMutex.Lock()
		nezhaProcess = nil
		processMutex.Unlock()
	}()
	
	return nil
}

func runXray() error {
	cmd := exec.Command(webPath, "-c", configPath)
	cmd.Dir = FilePath
	cmd.Stdout = nil
	cmd.Stderr = nil
	
	if err := cmd.Start(); err != nil {
		return err
	}
	
	processMutex.Lock()
	xrayProcess = cmd.Process
	processMutex.Unlock()
	
	fmt.Printf("%s is running\n", webName)
	
	go func() {
		cmd.Wait()
		processMutex.Lock()
		xrayProcess = nil
		processMutex.Unlock()
	}()
	
	return nil
}

func runCloudflared() error {
	if _, err := os.Stat(botPath); os.IsNotExist(err) {
		return nil
	}
	
	args := []string{"tunnel", "--edge-ip-version", "auto", "--no-autoupdate", "--protocol", "http2"}
	
	if ArgoAuth != "" && len(ArgoAuth) >= 120 && len(ArgoAuth) <= 250 {
		args = append(args, "run", "--token", ArgoAuth)
	} else if ArgoAuth != "" && strings.Contains(ArgoAuth, "TunnelSecret") {
		tunnelYamlPath := filepath.Join(FilePath, "tunnel.yml")
		args = append(args, "--config", tunnelYamlPath, "run")
	} else {
		args = append(args, "--logfile", bootLogPath, "--loglevel", "info",
			"--url", fmt.Sprintf("http://localhost:%d", ArgoPort))
	}
	
	cmd := exec.Command(botPath, args...)
	cmd.Dir = FilePath
	cmd.Stdout = nil
	cmd.Stderr = nil
	
	if err := cmd.Start(); err != nil {
		return err
	}
	
	processMutex.Lock()
	cloudflaredProcess = cmd.Process
	processMutex.Unlock()
	
	fmt.Printf("%s is running\n", botName)
	
	go func() {
		cmd.Wait()
		processMutex.Lock()
		cloudflaredProcess = nil
		processMutex.Unlock()
	}()
	
	return nil
}

func argoType() {
	if ArgoAuth == "" || ArgoDomain == "" {
		fmt.Println("ARGO_DOMAIN or ARGO_AUTH variable is empty, use quick tunnels")
		return
	}
	
	if strings.Contains(ArgoAuth, "TunnelSecret") {
		tunnelJsonPath := filepath.Join(FilePath, "tunnel.json")
		if err := os.WriteFile(tunnelJsonPath, []byte(ArgoAuth), 0644); err != nil {
			fmt.Printf("Error writing tunnel.json: %v\n", err)
			return
		}
		
		// 解析 TunnelID
		var tunnelData map[string]interface{}
		if err := json.Unmarshal([]byte(ArgoAuth), &tunnelData); err != nil {
			fmt.Printf("Error parsing ArgoAuth: %v\n", err)
			return
		}
		
		tunnelID, _ := tunnelData["TunnelID"].(string)
		if tunnelID == "" {
			tunnelID = tunnelData["tunnel_id"].(string)
		}
		
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
`, tunnelID, tunnelJsonPath, ArgoDomain, ArgoPort)
		
		tunnelYamlPath := filepath.Join(FilePath, "tunnel.yml")
		if err := os.WriteFile(tunnelYamlPath, []byte(tunnelYaml), 0644); err != nil {
			fmt.Printf("Error writing tunnel.yml: %v\n", err)
		} else {
			fmt.Println("Tunnel YAML configuration generated")
		}
	} else {
		fmt.Println("ARGO_AUTH mismatch TunnelSecret,use token connect to tunnel")
	}
}

func extractDomains() error {
	var argoDomain string
	
	if ArgoAuth != "" && ArgoDomain != "" {
		argoDomain = ArgoDomain
		fmt.Println("ARGO_DOMAIN:", argoDomain)
		return generateLinks(argoDomain)
	}
	
	// 重试获取临时域名
	maxRetries := 5
	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			fmt.Printf("Retrying to get Argo domain (%d/%d)...\n", retry, maxRetries)
			time.Sleep(3 * time.Second)
		}
		
		data, err := os.ReadFile(bootLogPath)
		if err != nil {
			continue
		}
		
		content := string(data)
		lines := strings.Split(content, "\n")
		
		for _, line := range lines {
			// 查找 trycloudflare.com 域名
			start := strings.Index(line, "https://")
			if start == -1 {
				continue
			}
			start += 8
			
			end := strings.Index(line[start:], " ")
			if end == -1 {
				end = strings.Index(line[start:], "/")
			}
			if end == -1 {
				end = len(line) - start
			}
			
			if end > 0 {
				domain := line[start : start+end]
				if strings.Contains(domain, "trycloudflare.com") {
					argoDomain = domain
					break
				}
			}
		}
		
		if argoDomain != "" {
			break
		}
		
		// 如果还没获取到，尝试重启 cloudflared
		if retry == 1 {
			fmt.Println("ArgoDomain not found, re-running bot to obtain ArgoDomain")
			
			// 删除 boot.log 文件
			os.Remove(bootLogPath)
			
			// 停止现有的 cloudflared 进程
			processMutex.Lock()
			if cloudflaredProcess != nil {
				cloudflaredProcess.Kill()
				cloudflaredProcess = nil
			}
			processMutex.Unlock()
			
			time.Sleep(2 * time.Second)
			
			// 重新启动 cloudflared
			args := []string{"tunnel", "--edge-ip-version", "auto", "--no-autoupdate", "--protocol", "http2",
				"--logfile", bootLogPath, "--loglevel", "info",
				"--url", fmt.Sprintf("http://localhost:%d", ArgoPort)}
			
			cmd := exec.Command(botPath, args...)
			cmd.Dir = FilePath
			cmd.Stdout = nil
			cmd.Stderr = nil
			
			if err := cmd.Start(); err == nil {
				processMutex.Lock()
				cloudflaredProcess = cmd.Process
				processMutex.Unlock()
				fmt.Printf("%s is running\n", botName)
			}
			
			time.Sleep(3 * time.Second)
		}
	}
	
	if argoDomain == "" {
		return fmt.Errorf("failed to get Argo domain after %d retries", maxRetries)
	}
	
	fmt.Println("ArgoDomain:", argoDomain)
	return generateLinks(argoDomain)
}

func getMetaInfo() string {
	client := &http.Client{Timeout: 3 * time.Second}
	
	// 尝试 api.ip.sb
	resp, err := client.Get("https://api.ip.sb/geoip")
	if err == nil {
		defer resp.Body.Close()
		var data map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&data) == nil {
			countryCode, _ := data["country_code"].(string)
			isp, _ := data["isp"].(string)
			if countryCode != "" && isp != "" {
				return strings.ReplaceAll(countryCode+"-"+isp, " ", "_")
			}
		}
	}
	
	// 备用 ip-api.com
	resp, err = client.Get("http://ip-api.com/json")
	if err == nil {
		defer resp.Body.Close()
		var data map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&data) == nil {
			if status, _ := data["status"].(string); status == "success" {
				countryCode, _ := data["countryCode"].(string)
				org, _ := data["org"].(string)
				if countryCode != "" && org != "" {
					return strings.ReplaceAll(countryCode+"-"+org, " ", "_")
				}
			}
		}
	}
	
	return "Unknown"
}

func generateLinks(argoDomain string) error {
	isp := getMetaInfo()
	nodeName := isp
	if Name != "" {
		nodeName = Name + "-" + isp
	}
	
	// 生成 VMESS 配置
	vmessConfig := map[string]interface{}{
		"v":   "2",
		"ps":  nodeName,
		"add": CFIP,
		"port": CFPORT,
		"id":  UUID,
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
	
	vmessJSON, _ := json.Marshal(vmessConfig)
	vmessBase64 := base64.StdEncoding.EncodeToString(vmessJSON)
	
	subTxt := fmt.Sprintf(`
vless://%s@%s:%d?encryption=none&security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Fvless-argo%%3Fed%%3D2560#%s

vmess://%s

trojan://%s@%s:%d?security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Ftrojan-argo%%3Fed%%3D2560#%s
`,
		UUID, CFIP, CFPORT, argoDomain, argoDomain, nodeName,
		vmessBase64,
		UUID, CFIP, CFPORT, argoDomain, argoDomain, nodeName)
	
	// 保存 base64 编码的订阅
	encoded := base64.StdEncoding.EncodeToString([]byte(subTxt))
	
	// 更新缓存
	cacheMutex.Lock()
	subscriptionCache = []byte(encoded)
	cacheMutex.Unlock()
	
	if err := os.WriteFile(subPath, []byte(encoded), 0644); err != nil {
		return err
	}
	
	// 打印订阅内容（与 Node.js 版本一致）
	fmt.Println(string(encoded))
	fmt.Printf("%s/sub.txt saved successfully\n", FilePath)
	
	// 上传节点
	uploadNodes()
	
	return nil
}

func uploadNodes() {
	if UploadURL == "" {
		return
	}
	
	if UploadURL != "" && ProjectURL != "" {
		subscriptionURL := ProjectURL + "/" + SubPath
		jsonData := map[string][]string{
			"subscription": {subscriptionURL},
		}
		data, _ := json.Marshal(jsonData)
		
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(UploadURL+"/api/add-subscriptions", "application/json", strings.NewReader(string(data)))
		if err != nil {
			return
		}
		defer resp.Body.Close()
		
		if resp.StatusCode == 200 {
			fmt.Println("Subscription uploaded successfully")
		}
	} else if UploadURL != "" {
		// 上传节点（如果 list.txt 存在）
		data, err := os.ReadFile(listPath)
		if err != nil {
			return
		}
		
		content := string(data)
		scanner := bufio.NewScanner(strings.NewReader(content))
		var nodes []string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "vless://") || strings.Contains(line, "vmess://") ||
				strings.Contains(line, "trojan://") {
				nodes = append(nodes, line)
			}
		}
		
		if len(nodes) == 0 {
			return
		}
		
		jsonData, _ := json.Marshal(map[string][]string{"nodes": nodes})
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(UploadURL+"/api/add-nodes", "application/json", strings.NewReader(string(jsonData)))
		if err != nil {
			return
		}
		defer resp.Body.Close()
		
		if resp.StatusCode == 200 {
			fmt.Println("Nodes uploaded successfully")
		}
	}
}

func addVisitTask() {
	if !AutoAccess || ProjectURL == "" {
		fmt.Println("Skipping adding automatic access task")
		return
	}
	
	jsonData := map[string]string{"url": ProjectURL}
	data, _ := json.Marshal(jsonData)
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post("https://oooo.serv00.net/add-url", "application/json", strings.NewReader(string(data)))
	if err != nil {
		fmt.Printf("Add automatic access task faild: %v\n", err)
		return
	}
	defer resp.Body.Close()
	
	fmt.Println("automatic access task added successfully")
}

func stopAllProcesses() {
	processMutex.Lock()
	defer processMutex.Unlock()
	
	if nezhaProcess != nil {
		fmt.Println("Stopping Nezha process...")
		nezhaProcess.Kill()
	}
	if xrayProcess != nil {
		fmt.Println("Stopping Xray process...")
		xrayProcess.Kill()
	}
	if cloudflaredProcess != nil {
		fmt.Println("Stopping Cloudflared process...")
		cloudflaredProcess.Kill()
	}
}

func cleanFiles() {
	go func() {
		time.Sleep(90 * time.Second)
		
		filesToDelete := []string{bootLogPath, configPath, webPath, botPath}
		
		if NezhaPort != "" {
			filesToDelete = append(filesToDelete, npmPath)
		} else if NezhaServer != "" && NezhaKey != "" {
			filesToDelete = append(filesToDelete, phpPath)
		}
		
		for _, file := range filesToDelete {
			if err := os.Remove(file); err == nil {
				fmt.Printf("Deleted: %s\n", file)
			}
		}
		
		fmt.Println("App is running")
		fmt.Println("Thank you for using this script, enjoy!")
	}()
}