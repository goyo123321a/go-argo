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
	"github.com/google/uuid"
)

// 配置结构体
type Config struct {
	UploadURL   string
	ProjectURL  string
	AutoAccess  bool
	FilePath    string
	SubPath     string
	Port        string
	UUID        string
	NezhaServer string
	NezhaPort   string
	NezhaKey    string
	ArgoDomain  string
	ArgoAuth    string
	ArgoPort    string
	CFIP        string
	CFPort      string
	Name        string
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

	// 根路由
	router.GET("/", func(c *gin.Context) {
		if data, err := os.ReadFile(app.indexPath); err == nil {
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.String(http.StatusOK, string(data))
		} else {
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
		app.cancel()
		os.Exit(0)
	}()

	// 启动HTTP服务
	addr := ":" + app.config.Port
	fmt.Printf("HTTP server is running on port %s!\n", app.config.Port)
	
	// 添加订阅路由
	app.router.GET("/"+app.subPath, func(c *gin.Context) {
		// 简单的订阅内容
		subContent := "vless://example@example.com:443?encryption=none#Example"
		encoded := base64.StdEncoding.EncodeToString([]byte(subContent))
		c.String(http.StatusOK, encoded)
	})
	
	return app.router.Run(addr)
}
