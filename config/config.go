package config

import (
	"log"
	"os"
	"strconv"
	"time"
)

// Config 存储应用程序配置信息
type Config struct {
	// 服务器配置
	ServerPort string

	// 文件路径配置
	UploadDir   string // 废弃，但保留向后兼容
	TemplateDir string
	PicsDir     string // 原始图片目录
	WebpDir     string // WebP图片目录

	// 图片转换配置
	WebPQuality int // WebP质量 (1-100)

	// 安全配置
	AccessPassword    string        // 页面访问密码
	JWTSecret         string        // JWT 密钥
	JWTExpirationTime time.Duration // JWT 过期时间
	MaxLoginAttempts  int           // 最大登录尝试次数
	LockoutDuration   time.Duration // 锁定时间
}

// LoadConfig 从环境变量加载配置
func LoadConfig() *Config {
	config := &Config{
		// 默认值
		ServerPort:        "8080",
		UploadDir:         "./uploads", // 废弃，但保留向后兼容
		TemplateDir:       "./templates",
		PicsDir:           "./uploads/pics", // 修改为uploads目录内的pics子目录
		WebpDir:           "./uploads/webp", // 修改为uploads目录内的webp子目录
		WebPQuality:       80,
		AccessPassword:    "webpimg",                       // 默认页面访问密码
		JWTSecret:         "webpimg-secure-jwt-secret-key", // 默认JWT密钥
		JWTExpirationTime: 24 * time.Hour,                  // JWT默认过期时间为24小时
		MaxLoginAttempts:  5,                               // 默认最大登录尝试次数
		LockoutDuration:   1 * time.Hour,                   // 默认锁定时间为1小时
	}

	// 从环境变量读取配置，如果设置了则覆盖默认值
	if port := os.Getenv("WEBP_SERVER_PORT"); port != "" {
		config.ServerPort = port
	}

	if uploadDir := os.Getenv("WEBP_UPLOAD_DIR"); uploadDir != "" {
		config.UploadDir = uploadDir
	}

	if templateDir := os.Getenv("WEBP_TEMPLATE_DIR"); templateDir != "" {
		config.TemplateDir = templateDir
	}

	if picsDir := os.Getenv("WEBP_PICS_DIR"); picsDir != "" {
		config.PicsDir = picsDir
	}

	if webpDir := os.Getenv("WEBP_WEBP_DIR"); webpDir != "" {
		config.WebpDir = webpDir
	}

	if qualityStr := os.Getenv("WEBP_QUALITY"); qualityStr != "" {
		if quality, err := strconv.Atoi(qualityStr); err == nil {
			// 确保质量值在有效范围内
			if quality < 1 {
				quality = 1
			} else if quality > 100 {
				quality = 100
			}
			config.WebPQuality = quality
		} else {
			log.Printf("警告: WEBP_QUALITY 环境变量无法解析为整数: %v, 将使用默认值 %d", err, config.WebPQuality)
		}
	}

	// 安全配置
	if accessPwd := os.Getenv("WEBP_ACCESS_PASSWORD"); accessPwd != "" {
		config.AccessPassword = accessPwd
	}

	// JWT 配置
	if jwtSecret := os.Getenv("WEBP_JWT_SECRET"); jwtSecret != "" {
		config.JWTSecret = jwtSecret
	}

	if jwtExpStr := os.Getenv("WEBP_JWT_EXPIRATION_HOURS"); jwtExpStr != "" {
		if jwtExp, err := strconv.Atoi(jwtExpStr); err == nil && jwtExp > 0 {
			config.JWTExpirationTime = time.Duration(jwtExp) * time.Hour
		}
	}

	// 登录尝试限制配置
	if attemptsStr := os.Getenv("WEBP_MAX_LOGIN_ATTEMPTS"); attemptsStr != "" {
		if attempts, err := strconv.Atoi(attemptsStr); err == nil && attempts > 0 {
			config.MaxLoginAttempts = attempts
		}
	}

	if lockoutStr := os.Getenv("WEBP_LOCKOUT_MINUTES"); lockoutStr != "" {
		if lockout, err := strconv.Atoi(lockoutStr); err == nil && lockout > 0 {
			config.LockoutDuration = time.Duration(lockout) * time.Minute
		}
	}

	// 确保上传目录存在
	if err := os.MkdirAll(config.UploadDir, 0755); err != nil {
		log.Fatalf("无法创建上传目录 %s: %v", config.UploadDir, err)
	}

	// 确保原始图片目录存在
	if err := os.MkdirAll(config.PicsDir, 0755); err != nil {
		log.Fatalf("无法创建原始图片目录 %s: %v", config.PicsDir, err)
	}

	// 确保WebP图片目录存在
	if err := os.MkdirAll(config.WebpDir, 0755); err != nil {
		log.Fatalf("无法创建WebP图片目录 %s: %v", config.WebpDir, err)
	}

	log.Printf("加载配置: 端口=%s, 模板目录=%s, 原始图片目录=%s, WebP图片目录=%s, WebP质量=%d",
		config.ServerPort, config.TemplateDir, config.PicsDir, config.WebpDir, config.WebPQuality)

	return config
}
