package main

import (
	"log"
	"os"
	"strconv"
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
}

// LoadConfig 从环境变量加载配置
func LoadConfig() *Config {
	config := &Config{
		// 默认值
		ServerPort:  "8080",
		UploadDir:   "./uploads", // 废弃，但保留向后兼容
		TemplateDir: "./templates",
		PicsDir:     "./pics",   // 原始图片保存目录
		WebpDir:     "./webp",   // WebP图片保存目录
		WebPQuality: 80,
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

	// 确保上传目录存在（为了向后兼容）
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
