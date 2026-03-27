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
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// 版本信息
var (
	Version   = "1.0.0"
	BuildDate = time.Now().Format("2006-01-02")
)

// 环境变量配置（带默认值）
var (
	uploadURL   = getEnv("UPLOAD_URL", "")
	projectURL  = getEnv("PROJECT_URL", "")
	autoAccess  = getEnvBool("AUTO_ACCESS", false)
	filePath    = getEnv("FILE_PATH", "./tmp")
	subPath     = getEnv("SUB_PATH", "sub")
	port        = getEnvInt("SERVER_PORT", 7860)
	uuid        = getEnv("UUID", "")
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
	webName     = generateRandomName()
	botName     = generateRandomName()
	nezhaName   = generateRandomName()
	webPath     string
	botPath     string
	nezhaPath   string
	subFilePath string
	bootLogPath string
	configPath  string

	processes    []*os.Process
	processMutex sync.Mutex
	httpServer   *http.Server
	subContent   string
	subContentMu sync.RWMutex
	subReady     bool
	subReadyMu   sync.RWMutex
)

// 辅助函数
func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
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

	webPath = filepath.Join(filePath, webName)
	botPath = filepath.Join(filePath, botName)
	nezhaPath = filepath.Join(filePath, nezhaName)
	subFilePath = filepath.Join(filePath, "sub.txt")
	bootLogPath = filepath.Join(filePath, "boot.log")
	configPath = filepath.Join(filePath, "config.json")
}

func getSystemOS() string {
	return runtime.GOOS
}

func getSystemArchitecture() string {
	switch runtime.GOARCH {
	case "arm", "arm64", "aarch64":
		return "arm64"
	default:
		return "amd64"
	}
}

func getDomain() string {
	hostname, _ := os.Hostname()
	if strings.Contains(hostname, "ct8") {
		return "ct8.pl"
	} else if strings.Contains(hostname, "hostuno") {
		return "useruno.com"
	}
	return "serv00.net"
}

// 获取本机所有非回环 IPv4 地址
func getLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ips = append(ips, ipnet.IP.String())
		}
	}
	return ips
}

// 等待本地端口监听就绪
func waitForPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("端口 %d 在 %v 内未就绪", port, timeout)
}

// 生成配置（根据操作系统选择不同核心）
func generateConfig() error {
	if runtime.GOOS == "freebsd" {
		return generateSingBoxConfig()
	}
	return generateXrayConfig()
}

// 生成 Sing-box 配置（FreeBSD，无 TLS）
func generateSingBoxConfig() error {
	config := map[string]interface{}{
		"log": map[string]interface{}{
			"level": "info",
		},
		"inbounds": []map[string]interface{}{
			{
				"type":        "vless",
				"tag":         "vless-in",
				"listen":      "::",
				"listen_port": argoPort,
				"users": []map[string]interface{}{
					{"uuid": uuid},
				},
				"transport": map[string]interface{}{
					"type": "ws",
					"path": "/vless-argo",
				},
				"sniffing": map[string]interface{}{
					"enabled":       true,
					"dest_override": []string{"http", "tls"},
				},
			},
		},
		"outbounds": []map[string]interface{}{
			{"type": "direct", "tag": "direct"},
			{"type": "block", "tag": "block"},
		},
		"route": map[string]interface{}{
			"rules": []map[string]interface{}{},
			"final": "direct",
		},
	}

	// s14/s15 添加 warp 出站
	hostname, _ := os.Hostname()
	if strings.Contains(hostname, "s14") || strings.Contains(hostname, "s15") {
		config["outbounds"] = append(config["outbounds"].([]map[string]interface{}), map[string]interface{}{
			"type":            "wireguard",
			"tag":             "wireguard-out",
			"server":          "162.159.192.200",
			"server_port":     4500,
			"local_address":   []string{"172.16.0.2/32", "2606:4700:110:8f77:1ca9:f086:846c:5f9e/128"},
			"private_key":     "wIxszdR2nMdA7a2Ul3XQcniSfSZqdqjPb6w6opvf5AU=",
			"peer_public_key": "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
			"reserved":        []int{126, 246, 173},
		})

		config["route"] = map[string]interface{}{
			"rule_set": []map[string]interface{}{
				{
					"tag":             "youtube",
					"type":            "remote",
					"format":          "binary",
					"url":             "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo-lite/geosite/youtube.srs",
					"download_detour": "direct",
				},
				{
					"tag":             "google",
					"type":            "remote",
					"format":          "binary",
					"url":             "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo-lite/geosite/google.srs",
					"download_detour": "direct",
				},
			},
			"rules": []map[string]interface{}{
				{
					"rule_set": []string{"google", "youtube"},
					"outbound": "wireguard-out",
				},
			},
			"final": "direct",
		}
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// 生成 Xray 配置（Linux，无 TLS）
func generateXrayConfig() error {
	config := map[string]interface{}{
		"log": map[string]interface{}{
			"loglevel": "warning",
		},
		"inbounds": []map[string]interface{}{
			{
				"port":     argoPort,
				"listen":   "127.0.0.1",
				"protocol": "vless",
				"settings": map[string]interface{}{
					"clients": []map[string]interface{}{
						{"id": uuid},
					},
					"decryption": "none",
				},
				"streamSettings": map[string]interface{}{
					"network": "ws",
					"wsSettings": map[string]interface{}{
						"path": "/vless-argo",
					},
				},
				"sniffing": map[string]interface{}{
					"enabled":      true,
					"destOverride": []string{"http", "tls"},
				},
			},
		},
		"outbounds": []map[string]interface{}{
			{
				"protocol": "freedom",
				"tag":      "direct",
			},
			{
				"protocol": "blackhole",
				"tag":      "block",
			},
		},
		"routing": map[string]interface{}{
			"rules": []map[string]interface{}{},
		},
	}

	hostname, _ := os.Hostname()
	if strings.Contains(hostname, "s14") || strings.Contains(hostname, "s15") {
		log.Println("Linux 环境暂不支持 warp 出站")
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// 下载文件（支持重试）
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

		if err := os.Chmod(filePath, 0755); err != nil {
			log.Printf("设置权限失败: %v", err)
			os.Remove(filePath)
			continue
		}

		log.Printf("✓ 下载成功: %s (%.2f MB)", filePath, float64(info.Size())/1024/1024)
		return nil
	}
	return fmt.Errorf("下载失败，已重试 %d 次", maxRetries)
}

// 获取需要下载的文件列表
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
		files = append(files,
			struct{ path string; url string }{webPath, baseURL + "/sb"},
			struct{ path string; url string }{botPath, baseURL + "/server"},
		)

		if nezhaServer != "" && nezhaKey != "" {
			if nezhaPort != "" {
				files = append(files, struct{ path string; url string }{nezhaPath, baseURL + "/npm"})
			} else {
				files = append(files, struct{ path string; url string }{nezhaPath, baseURL + "/v1"})
			}
		}
	} else {
		// Linux
		if arch == "arm64" {
			files = append(files,
				struct{ path string; url string }{webPath, "https://github.com/eooce/test/releases/download/linux-arm64/web"},
				struct{ path string; url string }{botPath, "https://github.com/eooce/test/releases/download/linux-arm64/bot"},
			)
		} else {
			files = append(files,
				struct{ path string; url string }{webPath, "https://github.com/eooce/test/releases/download/linux/web"},
				struct{ path string; url string }{botPath, "https://github.com/eooce/test/releases/download/linux/bot"},
			)
		}

		if nezhaServer != "" && nezhaKey != "" {
			// 注意：哪吒 agent 下载后需解压，这里简化处理，暂不实现解压
			log.Println("Linux 下哪吒监控需要手动解压或另行安装")
		}
	}
	return files
}

// 运行核心代理
func runCore() error {
	if !fileExists(webPath) {
		return fmt.Errorf("核心二进制文件不存在: %s", webPath)
	}

	var cmd *exec.Cmd
	if getSystemOS() == "freebsd" {
		cmd = exec.Command(webPath, "run", "-c", configPath)
	} else {
		cmd = exec.Command(webPath, "-c", configPath)
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

	// 等待核心代理监听端口
	if err := waitForPort(argoPort, 10*time.Second); err != nil {
		// 启动失败，尝试终止进程
		cmd.Process.Kill()
		cmd.Process.Wait()
		return fmt.Errorf("核心代理启动后端口未就绪: %v", err)
	}

	log.Printf("✓ 核心代理已启动 (监听本地端口 %d)", argoPort)
	return nil
}

// 运行 Cloudflared
func runCloudflared() error {
	if !fileExists(botPath) {
		return fmt.Errorf("cloudflared 二进制文件不存在: %s", botPath)
	}

	var args []string

	if argoAuth != "" && len(argoAuth) >= 120 && len(argoAuth) <= 250 {
		args = []string{"tunnel", "--edge-ip-version", "auto", "--no-autoupdate",
			"--protocol", "http2", "run", "--token", argoAuth}
		log.Printf("使用 Token 模式，隧道域名: %s", argoDomain)
	} else if argoAuth != "" && strings.Contains(argoAuth, "TunnelSecret") {
		tunnelYamlPath := filepath.Join(filePath, "tunnel.yml")
		args = []string{"tunnel", "--edge-ip-version", "auto", "--config", tunnelYamlPath, "run"}
		log.Printf("使用 JSON 密钥模式，隧道域名: %s", argoDomain)
	} else {
		args = []string{"tunnel", "--edge-ip-version", "auto", "--no-autoupdate",
			"--protocol", "http2", "--logfile", bootLogPath, "--loglevel", "info",
			"--url", fmt.Sprintf("http://127.0.0.1:%d", argoPort)}
		log.Printf("使用临时隧道模式")
	}

	cmd := exec.Command(botPath, args...)
	cmd.Dir = filePath
	cmd.Stdout = nil
	cmd.Stderr = nil

	// 临时隧道需要捕获日志以获取域名
	if argoAuth == "" {
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

	time.Sleep(3 * time.Second)
	log.Printf("✓ Cloudflared 已启动")
	return nil
}

// 运行哪吒监控
func runNezha() error {
	if nezhaServer == "" || nezhaKey == "" {
		return nil
	}

	if !fileExists(nezhaPath) {
		return nil
	}

	if nezhaPort == "" {
		// v1 版本
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

		cmd := exec.Command(nezhaPath, "-c", configYamlPath)
		cmd.Dir = filePath
		cmd.Stdout = nil
		cmd.Stderr = nil

		if err := cmd.Start(); err != nil {
			return err
		}
		processMutex.Lock()
		processes = append(processes, cmd.Process)
		processMutex.Unlock()

		log.Printf("✓ 哪吒监控(v1)已启动")
	} else {
		// v0 版本
		args := []string{"-s", nezhaServer + ":" + nezhaPort, "-p", nezhaKey}
		tlsPorts := []string{"443", "8443", "2096", "2087", "2083", "2053"}
		for _, p := range tlsPorts {
			if nezhaPort == p {
				args = append(args, "--tls")
				break
			}
		}
		args = append(args, "--disable-auto-update", "--report-delay", "4", "--skip-conn", "--skip-procs")

		cmd := exec.Command(nezhaPath, args...)
		cmd.Dir = filePath
		cmd.Stdout = nil
		cmd.Stderr = nil

		if err := cmd.Start(); err != nil {
			return err
		}
		processMutex.Lock()
		processes = append(processes, cmd.Process)
		processMutex.Unlock()

		log.Printf("✓ 哪吒监控(v0)已启动")
	}
	time.Sleep(1 * time.Second)
	return nil
}

// 下载并运行所有服务
func downloadAndRun() error {
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
		}
	}

	if !fileExists(webPath) {
		return fmt.Errorf("核心二进制文件不存在: %s", webPath)
	}

	if err := runCore(); err != nil {
		return fmt.Errorf("核心代理启动失败: %v", err)
	}
	if err := runCloudflared(); err != nil {
		log.Printf("⚠ Cloudflared 启动失败: %v", err)
	}
	if err := runNezha(); err != nil {
		log.Printf("⚠ 哪吒监控启动失败: %v", err)
	}

	return nil
}

// 配置固定隧道
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
    service: http://127.0.0.1:%d
    originRequest:
      noTLSVerify: true
  - service: http_status:404
`, tunnelID, tunnelJsonPath, argoDomain, argoPort)
				tunnelYamlPath := filepath.Join(filePath, "tunnel.yml")
				os.WriteFile(tunnelYamlPath, []byte(tunnelYaml), 0644)
				log.Printf("✓ 固定隧道配置已生成 (连接本地端口 %d)", argoPort)
			}
		}
	}
}

// 获取 ISP 信息
func getISPInfo() string {
	client := &http.Client{Timeout: 3 * time.Second}

	resp, err := client.Get("https://ipapi.co/json/")
	if err == nil {
		defer resp.Body.Close()
		var data map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&data) == nil {
			if country, ok := data["country_code"].(string); ok {
				if org, ok := data["org"].(string); ok {
					return fmt.Sprintf("%s_%s", country, strings.ReplaceAll(org, " ", "_"))
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
				country, _ := data["countryCode"].(string)
				org, _ := data["org"].(string)
				if country != "" && org != "" {
					return fmt.Sprintf("%s_%s", country, strings.ReplaceAll(org, " ", "_"))
				}
			}
		}
	}
	return "Unknown"
}

// 生成节点链接
func generateLinks(argoDomain string) error {
	isp := getISPInfo()
	nodeName := isp
	if name != "" {
		nodeName = name + "-" + isp
	}

	var subTxt string

	vlessLink := fmt.Sprintf(`vless://%s@%s:%d?encryption=none&type=ws&host=%s&path=%%2Fvless-argo%%3Fed%%3D2560&security=tls&sni=%s&fp=chrome#%s`,
		uuid, cfip, cfport, argoDomain, argoDomain, nodeName)

	if getSystemOS() == "freebsd" {
		subTxt = vlessLink
	} else {
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
			"fp":   "chrome",
		}
		vmessJSON, _ := json.Marshal(vmess)
		vmessBase64 := base64.StdEncoding.EncodeToString(vmessJSON)

		trojanLink := fmt.Sprintf(`trojan://%s@%s:%d?security=tls&sni=%s&fp=chrome&type=ws&host=%s&path=%%2Ftrojan-argo%%3Fed%%3D2560#%s`,
			uuid, cfip, cfport, argoDomain, argoDomain, nodeName)

		subTxt = fmt.Sprintf("%s\n\nvmess://%s\n\n%s", vlessLink, vmessBase64, trojanLink)
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(subTxt))
	if err := os.WriteFile(subFilePath, []byte(encoded), 0644); err != nil {
		return fmt.Errorf("保存订阅文件失败: %v", err)
	}

	subContentMu.Lock()
	subContent = subTxt
	subContentMu.Unlock()
	subReadyMu.Lock()
	subReady = true
	subReadyMu.Unlock()

	log.Printf("✓ 订阅已生成")
	log.Printf("✓ SNI/Host: %s (Argo 隧道域名)", argoDomain)
	log.Printf("✓ 节点名称: %s", nodeName)

	return nil
}

// 提取临时隧道域名
func extractTunnelDomain() error {
	if argoAuth != "" && argoDomain != "" {
		return generateLinks(argoDomain)
	}

	time.Sleep(5 * time.Second)

	for i := 0; i < 10; i++ {
		if fileExists(bootLogPath) {
			content, err := os.ReadFile(bootLogPath)
			if err == nil {
				re := regexp.MustCompile(`https?://([a-zA-Z0-9.-]+\.trycloudflare\.com)/?`)
				matches := re.FindAllStringSubmatch(string(content), -1)
				if len(matches) > 0 && len(matches[0]) > 1 {
					argoDomain := matches[0][1]
					return generateLinks(argoDomain)
				}
			}
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("未能获取到临时隧道域名")
}

// 上传订阅
func uploadSubscription() {
	if uploadURL == "" {
		return
	}
	if uploadURL != "" && projectURL != "" {
		subscriptionURL := fmt.Sprintf("%s/%s", projectURL, subPath)
		data := map[string][]string{"subscription": {subscriptionURL}}
		jsonData, _ := json.Marshal(data)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Post(uploadURL+"/api/add-subscriptions", "application/json", bytes.NewBuffer(jsonData))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				log.Println("✓ 订阅已上传到汇聚器")
			}
		}
	}
}

// 添加自动访问任务
func addAutoVisit() {
	if !autoAccess || projectURL == "" {
		return
	}
	data := map[string]string{"url": projectURL}
	jsonData, _ := json.Marshal(data)
	client := &http.Client{Timeout: 10 * time.Second}
	client.Post("https://oooo.serv00.net/add-url", "application/json", bytes.NewBuffer(jsonData))
}

// 清理临时文件
func cleanFiles() {
	time.AfterFunc(90*time.Second, func() {
		filesToDelete := []string{bootLogPath, configPath}
		for _, file := range filesToDelete {
			os.Remove(file)
		}
	})
}

// 停止所有进程并清理
func stopAllProcesses() {
	processMutex.Lock()
	defer processMutex.Unlock()
	for _, proc := range processes {
		if proc != nil {
			proc.Kill()
			proc.Wait() // 避免僵尸进程
		}
	}
	processes = nil
}

// 启动进程清理器
func startProcessCleaner() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			processMutex.Lock()
			alive := make([]*os.Process, 0, len(processes))
			for _, p := range processes {
				if p != nil {
					if err := p.Signal(syscall.Signal(0)); err == nil {
						alive = append(alive, p)
					}
				}
			}
			processes = alive
			processMutex.Unlock()
		}
	}()
}

// 启动 HTTP 服务
func startHTTPServer() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head><title>Argo Tunnel Proxy</title></head>
<body>
<h1>Argo Tunnel Proxy Running</h1>
<p>订阅地址: <a href="/%s">/%s</a></p>
<p>下载订阅: <a href="/%s/download">/%s/download</a></p>
<p>原始订阅: <a href="/%s/raw">/%s/raw</a></p>
</body>
</html>`, subPath, subPath, subPath, subPath, subPath, subPath)
	})

	mux.HandleFunc("/"+subPath, func(w http.ResponseWriter, r *http.Request) {
		subReadyMu.RLock()
		ready := subReady
		subReadyMu.RUnlock()

		var data []byte
		if ready {
			subContentMu.RLock()
			content := subContent
			subContentMu.RUnlock()
			if content != "" {
				data = []byte(base64.StdEncoding.EncodeToString([]byte(content)))
			}
		}
		if len(data) == 0 && fileExists(subFilePath) {
			data, _ = os.ReadFile(subFilePath)
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
		subReadyMu.RLock()
		ready := subReady
		subReadyMu.RUnlock()

		var data []byte
		if ready {
			subContentMu.RLock()
			content := subContent
			subContentMu.RUnlock()
			if content != "" {
				data = []byte(base64.StdEncoding.EncodeToString([]byte(content)))
			}
		}
		if len(data) == 0 && fileExists(subFilePath) {
			data, _ = os.ReadFile(subFilePath)
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
		subReadyMu.RLock()
		ready := subReady
		subReadyMu.RUnlock()

		var data []byte
		if ready {
			subContentMu.RLock()
			content := subContent
			subContentMu.RUnlock()
			if content != "" {
				data = []byte(content)
			}
		}
		if len(data) == 0 && fileExists(subFilePath) {
			encoded, _ := os.ReadFile(subFilePath)
			if len(encoded) > 0 {
				data, _ = base64.StdEncoding.DecodeString(string(encoded))
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
			"status":    "running",
			"version":   Version,
			"build":     BuildDate,
			"sub_ready": ready,
			"sub_path":  subPath,
			"os":        runtime.GOOS,
			"arch":      runtime.GOARCH,
			"argo_port": argoPort,
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
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if uuid == "" {
		log.Fatal("UUID 环境变量未设置")
	}

	initPaths()

	// 信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("正在关闭服务...")
		stopAllProcesses()
		if httpServer != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			httpServer.Shutdown(ctx)
		}
		log.Println("服务已关闭")
		os.Exit(0)
	}()

	startHTTPServer()
	log.Printf("✓ HTTP 服务已启动 (端口: %d)", port)

	// 显示服务器 IP 和订阅链接
	ips := getLocalIPs()
	if len(ips) > 0 {
		log.Printf("服务器 IP 地址: %s", strings.Join(ips, ", "))
		for _, ip := range ips {
			log.Printf("本地订阅: http://%s:%d/%s", ip, port, subPath)
		}
	} else {
		log.Printf("本地订阅: http://127.0.0.1:%d/%s", port, subPath)
	}
	log.Printf("订阅路径: /%s", subPath)

	time.Sleep(1 * time.Second)

	// 启动进程清理器
	startProcessCleaner()

	// 配置固定隧道
	argoType()

	// 生成核心配置
	if err := generateConfig(); err != nil {
		log.Printf("⚠ 生成配置失败: %v", err)
	}

	// 下载并运行服务
	if err := downloadAndRun(); err != nil {
		log.Printf("⚠ 服务启动失败: %v", err)
	}

	// 提取隧道域名并生成订阅
	if err := extractTunnelDomain(); err != nil {
		log.Printf("⚠ 获取隧道域名失败: %v", err)
	}

	// 上传订阅
	uploadSubscription()

	// 添加自动访问任务
	addAutoVisit()

	// 清理临时文件
	cleanFiles()

	log.Printf("✓ 所有服务已启动")
	log.Printf("  工作目录: %s", filePath)
	log.Printf("  核心代理监听: 127.0.0.1:%d (无 TLS)", argoPort)
	log.Printf("  系统: %s/%s", runtime.GOOS, runtime.GOARCH)
	log.Printf("  模式: 纯 Argo 隧道模式")

	select {}
}
