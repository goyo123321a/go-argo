package main

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed index.html
var indexHTMLFS embed.FS

// ========== 全局配置 ==========

var (
	uploadURL   string
	projectURL  string
	autoAccess  bool
	filePath    string
	subPath     string
	serverPort  string
	uuid        string
	nezhaServer string
	nezhaPort   string
	nezhaKey    string
	argoDomain  string
	argoAuth    string
	argoPort    string
	s5Port      string
	hy2Port     string
	realityPort string
	cfip        string
	cfport      string
	nodeName    string
	chatID      string
	botToken    string
	showLog     bool
)

var (
	npmName, phpName, webName, botName string
	npmPath, phpPath, webPath, botPath string
	subFilePath, listPath              string
	bootLogPath, configPath            string
	certPath, keyPath, keyFilePath     string

	subContentMu sync.RWMutex
	subContent   string

	privateKey string
	publicKey  string
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

var downloadClient = &http.Client{
	Transport: &http.Transport{
		ResponseHeaderTimeout: 30 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	},
}

var reNode = regexp.MustCompile(`(vless|vmess|trojan|hysteria2|socks)://`)
var reArgoToken = regexp.MustCompile(`^[A-Za-z0-9=]{120,250}$`)
var reTunnelDomain = regexp.MustCompile(`https?://([^ ]*trycloudflare\.com)/?`)
var reMarkdownV2 = regexp.MustCompile("([_*\\[\\]()~`>#+=|{}.!\\-\\\\])")

// ========== 日志 ==========

func logf(format string, args ...interface{}) {
	if showLog {
		fmt.Printf(format+"\n", args...)
	}
}

func logerr(format string, args ...interface{}) {
	if showLog {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

func alwaysLog(msg string) {
	fmt.Println(msg)
}

// ========== subContent 线程安全访问 ==========

func setSubContent(s string) {
	subContentMu.Lock()
	subContent = s
	subContentMu.Unlock()
}

func getSubContent() string {
	subContentMu.RLock()
	defer subContentMu.RUnlock()
	return subContent
}

// ========== 环境变量 ==========

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvBool(key string, def bool) bool {
	v := strings.ToLower(os.Getenv(key))
	if v == "" {
		return def
	}
	return v == "true" || v == "1" || v == "yes"
}

func setupEnv() {
	uploadURL = getenv("UPLOAD_URL", "")
	projectURL = getenv("PROJECT_URL", "")
	autoAccess = getenvBool("AUTO_ACCESS", false)
	filePath = getenv("FILE_PATH", ".npm")
	subPath = getenv("SUB_PATH", "sub")
	serverPort = getenv("SERVER_PORT", getenv("PORT", "7860"))
	uuid = getenv("UUID", "9afd1229-b893-40c1-84dd-51e7ce204913")
	nezhaServer = getenv("NEZHA_SERVER", "")
	nezhaPort = getenv("NEZHA_PORT", "")
	nezhaKey = getenv("NEZHA_KEY", "")
	argoDomain = getenv("ARGO_DOMAIN", "")
	argoAuth = getenv("ARGO_AUTH", "")
	argoPort = getenv("ARGO_PORT", "8001")
	s5Port = getenv("S5_PORT", "")
	hy2Port = getenv("HY2_PORT", "")
	realityPort = getenv("REALITY_PORT", "")
	cfip = getenv("CFIP", "saas.sin.fan")
	cfport = getenv("CFPORT", "443")
	nodeName = getenv("NAME", "")
	chatID = getenv("CHAT_ID", "")
	botToken = getenv("BOT_TOKEN", "")

	s := strings.ToLower(getenv("SHOW_LOG", "true"))
	showLog = !(s == "false" || s == "disable" || s == "no")
}

// ========== 工具 ==========

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isValidPort(p string) bool {
	if strings.TrimSpace(p) == "" {
		return false
	}
	n, err := strconv.Atoi(strings.TrimSpace(p))
	return err == nil && n >= 1 && n <= 65535
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func randName() string {
	const chars = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 6)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return string(b)
}

func initPaths() {
	npmName = randName()
	phpName = randName()
	webName = randName()
	botName = randName()
	npmPath = filepath.Join(filePath, npmName)
	phpPath = filepath.Join(filePath, phpName)
	webPath = filepath.Join(filePath, webName)
	botPath = filepath.Join(filePath, botName)
	subFilePath = filepath.Join(filePath, "sub.txt")
	listPath = filepath.Join(filePath, "list.txt")
	bootLogPath = filepath.Join(filePath, "boot.log")
	configPath = filepath.Join(filePath, "config.json")
	certPath = filepath.Join(filePath, "cert.pem")
	keyPath = filepath.Join(filePath, "private.key")
	keyFilePath = filepath.Join(filePath, "key.txt")
}

// ========== HTTP 辅助 ==========

func httpGetString(u string) (string, error) {
	resp, err := httpClient.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func getJSON(u string, v interface{}) error {
	body, err := httpGetString(u)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(body), v)
}

// ========== 下载 ==========

func downloadFile(dest, u string) error {
	tmp := dest + ".download"
	resp, err := downloadClient.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

func downloadWithFallback(dest string, urls []string) error {
	var lastErr error
	for i, u := range urls {
		if i > 0 {
			logf("Retrying %s from backup source", filepath.Base(dest))
		}
		if err := downloadFile(dest, u); err == nil {
			logf("Download %s successfully", filepath.Base(dest))
			return nil
		} else {
			lastErr = err
			logerr("Download %s failed: %v", filepath.Base(dest), err)
		}
	}
	return lastErr
}

// ========== 密钥 / 证书 ==========

func generateOrLoadKeyPair() {
	if data, err := os.ReadFile(keyFilePath); err == nil {
		content := string(data)
		priv := reFind(content, `PrivateKey:\s*(.*)`)
		pub := reFind(content, `PublicKey:\s*(.*)`)
		if priv != "" && pub != "" {
			privateKey = priv
			publicKey = pub
			logf("Private Key: %s", privateKey)
			logf("Public Key: %s", publicKey)
			return
		}
	}
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		logerr("generate x25519 error: %v", err)
		return
	}
	privateKey = base64.RawURLEncoding.EncodeToString(priv.Bytes())
	publicKey = base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())
	content := fmt.Sprintf("PrivateKey: %s\nPublicKey: %s\n", privateKey, publicKey)
	os.WriteFile(keyFilePath, []byte(content), 0600)
	logf("Private Key: %s", privateKey)
	logf("Public Key: %s", publicKey)
}

func reFind(s, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(s)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func ensureTlsCertificates() error {
	if fileExists(certPath) && fileExists(keyPath) {
		return nil
	}
	os.MkdirAll(filepath.Dir(certPath), 0755)

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "bing.com"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(3650 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return err
	}
	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	certOut.Close()

	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return err
	}
	keyOut, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})
	keyOut.Close()
	return nil
}

func getCertificateFingerprint() string {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return ""
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return ""
	}
	sum := sha256.Sum256(block.Bytes)
	hexStr := strings.ToUpper(hex.EncodeToString(sum[:]))
	var parts []string
	for i := 0; i < len(hexStr); i += 2 {
		parts = append(parts, hexStr[i:i+2])
	}
	return strings.Join(parts, ":")
}

// ========== 生成 config.json ==========

func generateConfig() error {
	inbounds := []interface{}{
		map[string]interface{}{
			"tag": "vless-fallback-in", "port": parseInt(argoPort), "listen": "::",
			"protocol": "vless",
			"settings": map[string]interface{}{
				"clients":    []interface{}{map[string]interface{}{"id": uuid, "flow": "xtls-rprx-vision"}},
				"decryption": "none",
				"fallbacks": []interface{}{
					map[string]interface{}{"dest": 3001},
					map[string]interface{}{"path": "/vless-argo", "dest": 3002},
					map[string]interface{}{"path": "/vmess-argo", "dest": 3003},
					map[string]interface{}{"path": "/trojan-argo", "dest": 3004},
				},
			},
			"streamSettings": map[string]interface{}{"network": "tcp"},
		},
		map[string]interface{}{
			"tag": "vless-tcp-in", "port": 3001, "listen": "127.0.0.1", "protocol": "vless",
			"settings":       map[string]interface{}{"clients": []interface{}{map[string]interface{}{"id": uuid}}, "decryption": "none"},
			"streamSettings": map[string]interface{}{"network": "tcp", "security": "none"},
		},
		map[string]interface{}{
			"tag": "vless-ws-in", "port": 3002, "listen": "127.0.0.1", "protocol": "vless",
			"settings": map[string]interface{}{"clients": []interface{}{map[string]interface{}{"id": uuid, "level": 0}}, "decryption": "none"},
			"streamSettings": map[string]interface{}{"network": "ws", "security": "none", "wsSettings": map[string]interface{}{"path": "/vless-argo"}},
			"sniffing":       map[string]interface{}{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "metadataOnly": false},
		},
		map[string]interface{}{
			"tag": "vmess-ws-in", "port": 3003, "listen": "127.0.0.1", "protocol": "vmess",
			"settings":       map[string]interface{}{"clients": []interface{}{map[string]interface{}{"id": uuid, "alterId": 0}}},
			"streamSettings": map[string]interface{}{"network": "ws", "wsSettings": map[string]interface{}{"path": "/vmess-argo"}},
			"sniffing":       map[string]interface{}{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "metadataOnly": false},
		},
		map[string]interface{}{
			"tag": "trojan-ws-in", "port": 3004, "listen": "127.0.0.1", "protocol": "trojan",
			"settings":       map[string]interface{}{"clients": []interface{}{map[string]interface{}{"password": uuid}}},
			"streamSettings": map[string]interface{}{"network": "ws", "security": "none", "wsSettings": map[string]interface{}{"path": "/trojan-argo"}},
			"sniffing":       map[string]interface{}{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "metadataOnly": false},
		},
	}

	if isValidPort(realityPort) {
		inbounds = append(inbounds, map[string]interface{}{
			"tag": "vless-in", "listen": "::", "port": parseInt(realityPort), "protocol": "vless",
			"settings": map[string]interface{}{
				"clients":    []interface{}{map[string]interface{}{"id": uuid, "flow": "xtls-rprx-vision"}},
				"decryption": "none",
			},
			"streamSettings": map[string]interface{}{
				"network":  "raw",
				"security": "reality",
				"realitySettings": map[string]interface{}{
					"show":        false,
					"dest":        "www.iij.ad.jp:443",
					"xver":        0,
					"serverNames": []string{"www.iij.ad.jp"},
					"privateKey":  privateKey,
					"shortIds":    []string{""},
				},
			},
		})
	}

	if isValidPort(hy2Port) {
		inbounds = append(inbounds, map[string]interface{}{
			"tag": "hysteria-in", "listen": "::", "port": parseInt(hy2Port), "protocol": "hysteria",
			"settings": map[string]interface{}{
				"version": 2,
				"clients": []interface{}{map[string]interface{}{"auth": uuid}},
			},
			"streamSettings": map[string]interface{}{
				"network": "hysteria",
				"hysteriaSettings": map[string]interface{}{
					"version": 2,
					"masquerade": map[string]interface{}{
						"type": "proxy", "url": "https://bing.com",
					},
				},
				"security": "tls",
				"tlsSettings": map[string]interface{}{
					"alpn": []string{"h3"},
					"certificates": []interface{}{
						map[string]interface{}{"certificateFile": certPath, "keyFile": keyPath},
					},
				},
			},
		})
	}

	if isValidPort(s5Port) {
		user := uuid
		pass := uuid
		if len(uuid) >= 12 {
			user = uuid[:8]
			pass = uuid[len(uuid)-12:]
		}
		inbounds = append(inbounds, map[string]interface{}{
			"tag": "s5-in", "listen": "::", "port": parseInt(s5Port), "protocol": "socks",
			"settings": map[string]interface{}{
				"auth": "password",
				"accounts": []interface{}{
					map[string]interface{}{"user": user, "pass": pass},
				},
				"udp": true,
			},
		})
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{
			"access": "/dev/null", "error": "/dev/null", "loglevel": "none",
		},
		"inbounds":  inbounds,
		"dns":       map[string]interface{}{"servers": []string{"https+local://8.8.8.8/dns-query"}},
		"outbounds": []interface{}{
			map[string]interface{}{"protocol": "freedom", "tag": "direct"},
			map[string]interface{}{"protocol": "blackhole", "tag": "block"},
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// ========== 子进程 ==========

func startBackground(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

func killBotProcess() {
	if runtime.GOOS == "windows" {
		exec.Command("taskkill", "/f", "/im", botName+".exe").Run()
		return
	}
	if len(botName) > 0 {
		pattern := "[" + string(botName[0]) + "]" + botName[1:]
		exec.Command("pkill", "-f", pattern).Run()
	}
}

// ========== Cloudflare Tunnel ==========

func argoType() {
	if argoAuth == "" || argoDomain == "" {
		logf("ARGO_DOMAIN or ARGO_AUTH is empty, use quick tunnels")
		return
	}

	if strings.Contains(argoAuth, "TunnelSecret") {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(argoAuth), &parsed); err != nil {
			logerr("ARGO_AUTH is not valid JSON: %v", err)
			return
		}
		tunnelID := ""
		for _, k := range []string{"TunnelID", "tunnelId", "tunnel_id"} {
			if v, ok := parsed[k].(string); ok && v != "" {
				tunnelID = v
				break
			}
		}
		if tunnelID == "" {
			logerr("TunnelID not found in ARGO_AUTH JSON")
			return
		}
		os.WriteFile(filepath.Join(filePath, "tunnel.json"), []byte(argoAuth), 0600)
		yaml := fmt.Sprintf(`tunnel: %s
credentials-file: %s
protocol: http2

ingress:
  - hostname: %s
    service: http://localhost:%s
    originRequest:
      noTLSVerify: true
  - service: http_status:404
`, tunnelID, filepath.Join(filePath, "tunnel.json"), argoDomain, argoPort)
		os.WriteFile(filepath.Join(filePath, "tunnel.yml"), []byte(yaml), 0644)
		logf("Tunnel config written, ID: %s", tunnelID)
	} else {
		logf("Using token connect to tunnel, please set %s in cloudflare", argoPort)
	}
}

func waitForQuickTunnelLog(timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(bootLogPath); err == nil {
			if strings.Contains(string(data), "trycloudflare.com") {
				return string(data)
			}
		}
		time.Sleep(time.Second)
	}
	return ""
}

func extractDomains(maxRetries, retryCount int) error {
	if argoAuth != "" && argoDomain != "" {
		logf("ARGO_DOMAIN: %s", argoDomain)
		generateLinks(argoDomain)
		return nil
	}

	content := waitForQuickTunnelLog(30 * time.Second)
	if m := reTunnelDomain.FindStringSubmatch(content); len(m) > 1 {
		domain := m[1]
		logf("ArgoDomain: %s", domain)
		generateLinks(domain)
		return nil
	}

	if retryCount >= maxRetries {
		logerr("ArgoDomain not found after %d retries, giving up", maxRetries)
		return nil
	}

	logf("ArgoDomain not found, restarting bot (attempt %d/%d)", retryCount+1, maxRetries)
	os.Remove(bootLogPath)
	killBotProcess()
	time.Sleep(3 * time.Second)

	args := []string{
		"tunnel", "--edge-ip-version", "auto", "--no-autoupdate",
		"--protocol", "http2", "--logfile", bootLogPath,
		"--loglevel", "info", "--url", "http://localhost:" + argoPort,
	}
	if err := startBackground(botPath, args...); err != nil {
		logerr("Error executing command: %v", err)
		return err
	}
	logf("%s is running", botName)
	time.Sleep(6 * time.Second)
	return extractDomains(maxRetries, retryCount+1)
}

// ========== 元信息 ==========

func getMetaInfo() string {
	type ipSbResp struct {
		CountryCode string `json:"country_code"`
		ISP         string `json:"isp"`
	}
	var a ipSbResp
	if err := getJSON("https://api.ip.sb/geoip", &a); err == nil && a.CountryCode != "" && a.ISP != "" {
		return strings.ReplaceAll(a.CountryCode+"-"+a.ISP, " ", "_")
	}

	type ipApiResp struct {
		Status      string `json:"status"`
		CountryCode string `json:"countryCode"`
		Org         string `json:"org"`
	}
	var b ipApiResp
	if err := getJSON("http://ip-api.com/json", &b); err == nil && b.Status == "success" {
		return strings.ReplaceAll(b.CountryCode+"-"+b.Org, " ", "_")
	}
	return "Unknown"
}

func getServerIP() string {
	if body, err := httpGetString("http://ipv4.ip.sb"); err == nil {
		if ip := strings.TrimSpace(body); ip != "" {
			return ip
		}
	}
	if out, err := exec.Command("curl", "-sm", "3", "ipv4.ip.sb").Output(); err == nil {
		if ip := strings.TrimSpace(string(out)); ip != "" {
			return ip
		}
	}
	if body, err := httpGetString("http://ipv6.ip.sb"); err == nil {
		if ip := strings.TrimSpace(body); ip != "" {
			return "[" + ip + "]"
		}
	}
	if out, err := exec.Command("curl", "-sm", "3", "ipv6.ip.sb").Output(); err == nil {
		if ip := strings.TrimSpace(string(out)); ip != "" {
			return "[" + ip + "]"
		}
	}
	logerr("Failed to get IP address")
	return ""
}

// ========== 订阅生成 ==========

func generateLinks(argoDomain string) {
	isp := getMetaInfo()
	nn := isp
	if nodeName != "" {
		nn = nodeName + "-" + isp
	}
	serverIP := getServerIP()

	vmess := map[string]interface{}{
		"v": "2", "ps": nn, "add": cfip, "port": cfport, "id": uuid,
		"aid": "0", "scy": "auto", "net": "ws", "type": "none",
		"host": argoDomain, "path": "/vmess-argo?ed=2560",
		"tls": "tls", "sni": argoDomain, "alpn": "", "fp": "firefox",
	}
	vmessJSON, _ := json.Marshal(vmess)

	var sb strings.Builder
	fmt.Fprintf(&sb,
		"\nvless://%s@%s:%s?encryption=none&security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Fvless-argo%%3Fed%%3D2560#%s\n\n",
		uuid, cfip, cfport, argoDomain, argoDomain, nn)
	fmt.Fprintf(&sb, "vmess://%s\n\n", base64.StdEncoding.EncodeToString(vmessJSON))
	fmt.Fprintf(&sb,
		"trojan://%s@%s:%s?security=tls&sni=%s&fp=firefox&type=ws&host=%s&path=%%2Ftrojan-argo%%3Fed%%3D2560#%s",
		uuid, cfip, cfport, argoDomain, argoDomain, nn)

	if isValidPort(hy2Port) {
		fpParam := ""
		if fp := getCertificateFingerprint(); fp != "" {
			fpParam = "&pinSHA256=" + url.QueryEscape(fp)
		}
		fmt.Fprintf(&sb,
			"\nhysteria2://%s@%s:%s/?sni=www.bing.com&insecure=0&alpn=h3&obfs=none%s#%s",
			uuid, serverIP, hy2Port, fpParam, nn)
	}
	if isValidPort(realityPort) {
		fmt.Fprintf(&sb,
			"\nvless://%s@%s:%s?encryption=none&flow=xtls-rprx-vision&security=reality&sni=www.iij.ad.jp&fp=firefox&pbk=%s&type=tcp&headerType=none#%s",
			uuid, serverIP, realityPort, publicKey, nn)
	}
	if isValidPort(s5Port) {
		user := uuid
		pass := uuid
		if len(uuid) >= 12 {
			user = uuid[:8]
			pass = uuid[len(uuid)-12:]
		}
		auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		fmt.Fprintf(&sb, "\nsocks://%s@%s:%s#%s", auth, serverIP, s5Port, nn)
	}

	subTxt := sb.String()
	encoded := base64.StdEncoding.EncodeToString([]byte(subTxt))
	setSubContent(encoded)
	os.WriteFile(subFilePath, []byte(encoded), 0644)
	os.WriteFile(listPath, []byte(subTxt), 0644)
	logf("%s/sub.txt saved successfully", filePath)
	logf("%s", encoded)

	go uploadNodes()
}

// ========== 上传 / 删除 ==========

func readNodesFromSub() []string {
	if uploadURL == "" || !fileExists(subFilePath) {
		return nil
	}
	data, err := os.ReadFile(subFilePath)
	if err != nil {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(decoded), "\n") {
		if reNode.MatchString(line) {
			out = append(out, line)
		}
	}
	return out
}

func postDeleteNodes(nodes []string) {
	if len(nodes) == 0 {
		return
	}
	payload, _ := json.Marshal(map[string]interface{}{"nodes": nodes})
	resp, err := httpClient.Post(uploadURL+"/api/delete-nodes", "application/json", bytes.NewReader(payload))
	if err != nil {
		logerr("delete-nodes request failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		logerr("delete-nodes returned %d", resp.StatusCode)
	}
}

func uploadNodes() {
	if uploadURL != "" && projectURL != "" {
		subURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(projectURL, "/"), subPath)
		payload, _ := json.Marshal(map[string]interface{}{
			"subscription": []string{subURL},
		})
		resp, err := httpClient.Post(uploadURL+"/api/add-subscriptions", "application/json", bytes.NewReader(payload))
		if err != nil {
			logerr("add-subscriptions request failed: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			logerr("add-subscriptions returned %d", resp.StatusCode)
			return
		}
		logf("Subscription uploaded successfully")
		return
	}
	if uploadURL != "" {
		data, err := os.ReadFile(listPath)
		if err != nil {
			return
		}
		var nodes []string
		for _, line := range strings.Split(string(data), "\n") {
			if reNode.MatchString(line) {
				nodes = append(nodes, line)
			}
		}
		if len(nodes) == 0 {
			return
		}
		payload, _ := json.Marshal(map[string]interface{}{"nodes": nodes})
		resp, err := httpClient.Post(uploadURL+"/api/add-nodes", "application/json", bytes.NewReader(payload))
		if err != nil {
			logerr("add-nodes request failed: %v", err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			logerr("add-nodes returned %d", resp.StatusCode)
			return
		}
		logf("Nodes uploaded successfully")
	}
}

// ========== Telegram ==========

func escapeMarkdownV2(s string) string {
	return reMarkdownV2.ReplaceAllString(s, `\$1`)
}

func sendTelegram() {
	if botToken == "" || chatID == "" {
		logf("TG variables is empty, Skipping push nodes to TG")
		return
	}
	data, err := os.ReadFile(subFilePath)
	if err != nil {
		logerr("Read sub error: %v", err)
		return
	}
	text := fmt.Sprintf("**%s节点推送**\n```%s```", escapeMarkdownV2(nodeName), string(data))
	form := url.Values{}
	form.Set("chat_id", chatID)
	form.Set("text", text)
	form.Set("parse_mode", "MarkdownV2")

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	resp, err := httpClient.PostForm(apiURL, form)
	if err != nil {
		logerr("Failed to send Telegram message: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		logerr("Telegram returned %d: %s", resp.StatusCode, string(body))
		return
	}
	logf("Telegram message sent successfully")
}

// ========== 保活 ==========

func addVisitTask() {
	if !autoAccess || projectURL == "" {
		logf("Skipping adding automatic access task")
		return
	}
	payload, _ := json.Marshal(map[string]string{"url": projectURL})
	resp, err := httpClient.Post("https://oooo.serv00.net/add-url", "application/json", bytes.NewReader(payload))
	if err != nil {
		logerr("Add automatic access task faild: %v", err)
		return
	}
	resp.Body.Close()
	logf("automatic access task added successfully")
}

// ========== 下载并启动依赖 ==========

func downloadFilesAndRun() error {
	arch := runtime.GOARCH
	var baseURL, backupURL string
	switch arch {
	case "arm", "arm64":
		baseURL = "https://arm64.oooen.com"
		backupURL = "https://arm64.ssss.nyc.mn"
	default:
		baseURL = "https://amd64.oooen.com"
		backupURL = "https://amd64.ssss.nyc.mn"
	}

	type fileInfo struct {
		path string
		urls []string
	}
	var files []fileInfo

	if nezhaServer != "" && nezhaKey != "" {
		if nezhaPort != "" {
			files = append(files, fileInfo{npmPath, []string{baseURL + "/agent", backupURL + "/agent"}})
		} else {
			files = append(files, fileInfo{phpPath, []string{baseURL + "/v1", backupURL + "/v1"}})
		}
	}
	files = append(files,
		fileInfo{webPath, []string{baseURL + "/web", backupURL + "/web"}},
		fileInfo{botPath, []string{baseURL + "/bot", backupURL + "/bot"}},
	)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for _, f := range files {
		wg.Add(1)
		go func(fi fileInfo) {
			defer wg.Done()
			if err := downloadWithFallback(fi.path, fi.urls); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(f)
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}

	for _, p := range []string{webPath, botPath, npmPath, phpPath} {
		if fileExists(p) {
			os.Chmod(p, 0775)
		}
	}

	// 启动 nezha
	if nezhaServer != "" && nezhaKey != "" {
		if nezhaPort == "" {
			tls := "false"
			for _, p := range []string{"443", "8443", "2096", "2087", "2083", "2053"} {
				if strings.HasSuffix(nezhaServer, ":"+p) {
					tls = "true"
				}
			}
			configYaml := fmt.Sprintf(`client_secret: %s
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
uuid: %s`, nezhaKey, nezhaServer, tls, uuid)
			os.WriteFile(filepath.Join(filePath, "config.yaml"), []byte(configYaml), 0600)
			if err := startBackground(phpPath, "-c", filepath.Join(filePath, "config.yaml")); err != nil {
				logerr("php running error: %v", err)
			} else {
				logf("%s is running", phpName)
			}
			time.Sleep(time.Second)
		} else {
			args := []string{"-s", nezhaServer + ":" + nezhaPort, "-p", nezhaKey}
			for _, p := range []string{"443", "8443", "2096", "2087", "2083", "2053"} {
				if nezhaPort == p {
					args = append(args, "--tls")
				}
			}
			args = append(args, "--disable-auto-update", "--report-delay", "4", "--skip-conn", "--skip-procs")
			if err := startBackground(npmPath, args...); err != nil {
				logerr("npm running error: %v", err)
			} else {
				logf("%s is running", npmName)
			}
			time.Sleep(time.Second)
		}
	} else {
		logf("NEZHA variable is empty,skip running")
	}

	// 启动 xray
	if err := startBackground(webPath, "-c", configPath); err != nil {
		logerr("web running error: %v", err)
	} else {
		logf("%s is running", webName)
	}
	time.Sleep(time.Second)

	// 启动 cloudflared
	if fileExists(botPath) {
		var args []string
		if reArgoToken.MatchString(argoAuth) {
			args = []string{"tunnel", "--edge-ip-version", "auto", "--no-autoupdate", "--protocol", "http2", "run", "--token", argoAuth}
		} else if strings.Contains(argoAuth, "TunnelSecret") {
			args = []string{"tunnel", "--edge-ip-version", "auto", "--config", filepath.Join(filePath, "tunnel.yml"), "run"}
		} else {
			args = []string{
				"tunnel", "--edge-ip-version", "auto", "--no-autoupdate",
				"--protocol", "http2", "--logfile", bootLogPath,
				"--loglevel", "info", "--url", "http://localhost:" + argoPort,
			}
		}
		if err := startBackground(botPath, args...); err != nil {
			logerr("Error executing command: %v", err)
		} else {
			logf("%s is running", botName)
		}
		time.Sleep(2 * time.Second)
	}

	time.Sleep(5 * time.Second)
	return nil
}

// ========== 清理 ==========

func cleanupOldFiles() {
	entries, err := os.ReadDir(filePath)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		os.Remove(filepath.Join(filePath, e.Name()))
	}
}

func cleanFiles() {
	time.Sleep(90 * time.Second)
	files := []string{bootLogPath, configPath, webPath, botPath, listPath, certPath, keyPath}
	if nezhaPort != "" {
		files = append(files, npmPath)
	} else if nezhaServer != "" && nezhaKey != "" {
		files = append(files, phpPath)
	}
	for _, f := range files {
		os.Remove(f)
	}
	alwaysLog("App is running")
	alwaysLog("Thank you for using this script, enjoy!")
}

// ========== HTTP 服务 ==========

// 三级查找 index.html：
//   1. 二进制所在目录（用户可覆盖）
//   2. 二进制内嵌资源（go:embed）
//   3. 当前工作目录（兜底）
func readIndexHTML() ([]byte, error) {
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "index.html")
		if data, err := os.ReadFile(p); err == nil {
			return data, nil
		}
	}
	if data, err := indexHTMLFS.ReadFile("index.html"); err == nil {
		return data, nil
	}
	return os.ReadFile("index.html")
}

func startHTTPServer() {
	mux := http.NewServeMux()

	mux.HandleFunc("/"+subPath, func(w http.ResponseWriter, r *http.Request) {
		if c := getSubContent(); c != "" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte(c))
			return
		}
		data, err := os.ReadFile(subFilePath)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Subscription content not yet available, please try again later."))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(data)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not Found"))
			return
		}
		data, err := readIndexHTML()
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte("Hello world!<br><br>You can access /{SUB_PATH}(Default: /sub) to get your nodes!"))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	alwaysLog(fmt.Sprintf("http server is running on %s!", serverPort))
	if err := http.ListenAndServe(":"+serverPort, mux); err != nil {
		logerr("http server error: %v", err)
	}
}

// ========== 主流程 ==========

func run() {
	argoType()

	nodes := readNodesFromSub()
	cleanupOldFiles()

	if len(nodes) > 0 {
		go postDeleteNodes(nodes)
	}

	if isValidPort(realityPort) {
		generateOrLoadKeyPair()
	}
	if isValidPort(hy2Port) {
		if err := ensureTlsCertificates(); err != nil {
			logerr("tls cert error: %v", err)
		}
	}
	if err := generateConfig(); err != nil {
		logerr("Error in generateConfig: %v", err)
		return
	}
	if err := downloadFilesAndRun(); err != nil {
		logerr("Error in downloadFilesAndRun: %v", err)
		return
	}
	if err := extractDomains(3, 0); err != nil {
		logerr("Error in extractDomains: %v", err)
		return
	}
	sendTelegram()
	addVisitTask()
}

func main() {
	setupEnv()
	os.MkdirAll(filePath, 0755)
	initPaths()

	go cleanFiles()
	go run()

	startHTTPServer()
}
