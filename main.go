package main

import (
	"bytes"
	"fmt"
	"image/gif"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	// Import our local config package
	cfg "github.com/suixinio/webp-img/config"
	"github.com/suixinio/webp-img/security"
)

// 全局配置
var config *cfg.Config

func main() {
	// 加载配置
	config = cfg.LoadConfig()

	// 如果启用了自动转换现有图片功能，则启动转换
	if config.ConvertExistingImages {
		go convertExistingImages()
	}

	// 设置Gin路由器
	router := gin.Default()

	// 加载HTML模板，但排除css目录
	router.LoadHTMLGlob(filepath.Join(config.TemplateDir, "*.html"))

	// 定义路由
	router.GET("/login", loginPageHandler)
	router.POST("/login", loginHandler)

	// 使用JWT中间件保护的路由
	router.GET("/", security.AuthMiddleware(config), homeHandler)
	router.GET("/gallery", security.AuthMiddleware(config), galleryHandler)
	router.GET("/api/images", security.AuthMiddleware(config), listImagesHandler)
	router.POST("/upload", security.AuthMiddleware(config), uploadHandler)
	router.GET("/download/webp/*filename", downloadWebpHandler)                                          // 下载WebP图片，无需权限校验
	router.GET("/download/original/*filename", security.AuthMiddleware(config), downloadOriginalHandler) // 下载原图需要权限校验
	router.GET("/img/*filename", imageHandler)                                                           // 保留原有的/img/路径用于向后兼容

	// 设置静态文件服务
	router.Static("/uploads", config.UploadDir)

	// 设置CSS静态文件服务
	router.Static("/css", filepath.Join(config.TemplateDir, "css"))

	// 启动服务器
	log.Printf("服务器在端口 %s 上启动...\n", config.ServerPort)
	log.Printf("可以通过 /pics/ 和 /webp/ 直接访问图片\n")
	router.Run(":" + config.ServerPort)
}

// 登录页面处理函数
func loginPageHandler(c *gin.Context) {
	// 检查是否已经有有效的JWT令牌
	tokenCookie, err := c.Cookie("auth_token")
	if err == nil {
		// 验证令牌有效性
		valid, _ := security.ValidateToken(tokenCookie, config)
		if valid {
			// 如果令牌有效，重定向到主页
			c.Redirect(http.StatusFound, "/")
			return
		}
	}

	// 为表单生成CSRF令牌
	csrfToken := security.GenerateCSRFToken()
	c.SetCookie("csrf_token", csrfToken, 3600, "/", "", false, true)

	// 显示登录页面
	c.HTML(http.StatusOK, "password.html", gin.H{
		"message":   "",
		"csrfToken": csrfToken,
	})
}

// 登录处理函数
func loginHandler(c *gin.Context) {
	// 获取客户端IP，用于登录限速
	clientIP := c.ClientIP()

	// 检查是否被锁定
	if !security.CheckLoginAttempts(clientIP, config) {
		c.HTML(http.StatusTooManyRequests, "password.html", gin.H{
			"message": "登录尝试次数过多，请稍后再试",
		})
		return
	}

	// 验证CSRF令牌
	formToken := c.PostForm("csrf_token")
	csrfCookie, err := c.Cookie("csrf_token")
	if err != nil || formToken != csrfCookie {
		c.HTML(http.StatusForbidden, "password.html", gin.H{
			"message": "安全验证失败，请刷新页面重试",
		})
		return
	}

	// 获取和验证密码
	password := c.PostForm("password")
	if password == config.AccessPassword {
		// 密码正确，生成JWT令牌
		tokenString, err := security.GenerateToken(config)
		if err == nil {
			// 设置包含JWT的Cookie
			c.SetCookie("auth_token", tokenString, int(config.JWTExpirationTime.Seconds()), "/", "", false, true)

			// 记录成功的登录尝试
			security.RecordLoginAttempt(clientIP, true, config)

			// 重定向到主页
			c.Redirect(http.StatusFound, "/")
			return
		} else {
			log.Printf("生成JWT令牌失败: %v", err)
		}
	}

	// 记录失败的登录尝试
	security.RecordLoginAttempt(clientIP, false, config)

	// 密码错误或其他错误，返回登录页面
	// 重新生成CSRF令牌
	csrfToken := security.GenerateCSRFToken()
	c.SetCookie("csrf_token", csrfToken, 3600, "/", "", false, true)

	c.HTML(http.StatusUnauthorized, "password.html", gin.H{
		"message":   "密码错误，请重试",
		"csrfToken": csrfToken,
	})
}

func homeHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", nil)
}

// galleryHandler 处理画廊页面的请求
func galleryHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "gallery.html", nil)
}

// ImageInfo 存储图片信息的结构体
type ImageInfo struct {
	URL          string `json:"url"`          // 图片URL
	ThumbnailURL string `json:"thumbnailUrl"` // 缩略图URL (这里使用同一URL)
	OriginalName string `json:"originalName"` // 原始文件名
	UploadDate   string `json:"uploadDate"`   // 上传日期
	Directory    string `json:"directory"`    // 目录路径
}

// DirectoryInfo 存储目录信息的结构体
type DirectoryInfo struct {
	Path   string      `json:"path"`   // 目录路径
	Name   string      `json:"name"`   // 目录名称
	Images []ImageInfo `json:"images"` // 目录下的图片
}

// listImagesHandler 返回所有图片的列表
func listImagesHandler(c *gin.Context) {
	// 读取查询参数，如果有的话
	dir := c.Query("dir") // 如果指定了目录，则只列出该目录下的图片

	// 根目录是配置中的 WebP 目录
	baseDir := config.WebpDir

	// 如果指定了目录，则使用该目录
	if dir != "" {
		// 处理相对路径，防止目录遍历攻击
		cleanDir := filepath.Clean(dir)
		if cleanDir == ".." || strings.HasPrefix(cleanDir, "../") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效的目录路径"})
			return
		}
		baseDir = filepath.Join(baseDir, cleanDir)
	}

	// 检查目录是否存在
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "目录不存在"})
		return
	}

	// 存储目录和图片信息
	var directories []DirectoryInfo
	currentDir := DirectoryInfo{
		Path:   dir,
		Name:   filepath.Base(dir),
		Images: []ImageInfo{},
	}

	// 遍历目录
	files, err := os.ReadDir(baseDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取目录"})
		return
	}

	// 先处理子目录
	for _, file := range files {
		if file.IsDir() {
			// 这是一个子目录，添加到目录列表
			subDirPath := filepath.Join(dir, file.Name())
			directories = append(directories, DirectoryInfo{
				Path: subDirPath,
				Name: file.Name(),
				// 不预先加载子目录中的图片，等用户点击目录时再加载
				Images: []ImageInfo{},
			})
		}
	}

	// 再处理图片文件
	for _, file := range files {
		if !file.IsDir() {
			// 检查是否是WebP图片
			ext := strings.ToLower(filepath.Ext(file.Name()))
			if ext == ".webp" {
				// 这是一个WebP图片，添加到当前目录的图片列表
				imagePath := file.Name()
				if dir != "" {
					imagePath = filepath.Join(dir, file.Name())
				}

				// 获取图片信息
				imgURL := fmt.Sprintf("/img/%s", imagePath)

				// 创建一个图片信息对象
				imgInfo := ImageInfo{
					URL:          imgURL,
					ThumbnailURL: imgURL, // 使用同一个URL作为缩略图
					OriginalName: file.Name(),
					// 从文件名中提取上传日期（假设文件名格式为 timestamp.webp）
					UploadDate: formatTimestampFromFilename(file.Name()),
					Directory:  dir,
				}

				// 添加到当前目录的图片列表
				currentDir.Images = append(currentDir.Images, imgInfo)
			}
		}
	}

	// 如果请求的是根目录，则返回所有子目录和根目录下的图片
	if dir == "" {
		c.JSON(http.StatusOK, gin.H{
			"directories": directories,
			"images":      currentDir.Images,
		})
	} else {
		// 否则，返回当前目录的图片和子目录
		c.JSON(http.StatusOK, gin.H{
			"directory":   currentDir,
			"directories": directories,
			"images":      currentDir.Images,
		})
	}
}

// formatTimestampFromFilename 从文件名中提取并格式化时间戳
func formatTimestampFromFilename(filename string) string {
	// 去掉扩展名
	basename := strings.TrimSuffix(filename, filepath.Ext(filename))

	// 从文件名中提取时间戳部分（假设格式为 unixtime-milliseconds）
	parts := strings.Split(basename, "-")
	if len(parts) < 1 {
		return "" // 文件名格式不正确
	}

	// 尝试将时间戳转换为 time.Time
	timestamp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "" // 时间戳转换失败
	}

	// 将 Unix 时间戳转换为 time.Time
	t := time.Unix(timestamp, 0)

	// 格式化为易读的日期时间
	return t.Format("2006-01-02 15:04:05")
}

func uploadHandler(c *gin.Context) {
	// 从表单获取文件
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "获取文件错误",
		})
		return
	}
	defer file.Close()

	// 验证文件类型
	contentType := header.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "文件不是图片",
		})
		return
	}

	// 获取文件扩展名
	fileExt := filepath.Ext(header.Filename)
	if fileExt == "" {
		// 如果文件名没有扩展名，根据内容类型推断
		switch contentType {
		case "image/jpeg":
			fileExt = ".jpg"
		case "image/png":
			fileExt = ".png"
		case "image/gif":
			fileExt = ".gif"
		default:
			fileExt = ".jpg" // 默认扩展名
		}
	}

	// 生成文件路径：按YY/MM/DD目录结构，使用时间戳命名
	originalPath, webpPath, relativePath, err := generatePaths(fileExt)
	if err != nil {
		log.Printf("生成文件路径失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "生成文件路径失败",
		})
		return
	}

	// 保存原始文件
	dst, err := os.Create(originalPath)
	if err != nil {
		log.Printf("创建目标文件失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "创建目标文件失败",
		})
		return
	}
	defer dst.Close()

	// 复制文件内容
	if _, err = io.Copy(dst, file); err != nil {
		log.Printf("保存文件失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "保存文件失败",
		})
		return
	}

	// 获取原始文件大小
	originalInfo, err := os.Stat(originalPath)
	if err != nil {
		log.Printf("获取原始文件信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "获取文件信息失败",
		})
		return
	}
	originalSize := originalInfo.Size()

	// 转换为WebP并保存
	if err := convertToWebP(originalPath, webpPath); err != nil {
		log.Printf("转换为WebP失败: %v", err)
		// 即使WebP转换失败，我们也会继续处理
	}

	// 获取WebP文件大小
	webpSize := int64(0)
	webpInfo, err := os.Stat(webpPath)
	if err == nil {
		webpSize = webpInfo.Size()
	}

	// 计算压缩比例（如果有WebP文件）
	var compressionRatio float64 = 0
	if webpSize > 0 && originalSize > 0 {
		// 修改计算方式：显示节省了多少百分比，而不是大小比例
		// 原来的计算: compressionRatio = float64(webpSize) / float64(originalSize) * 100
		// 新计算: 100 - (webp大小占原图的比例)
		compressionRatio = 100 - (float64(webpSize) / float64(originalSize) * 100)
	}

	// 计算各种URL
	// 1. 传统的/img/路径 (向后兼容)
	imgURL := fmt.Sprintf("/img/%s", relativePath)

	// 格式化文件大小为KB或MB
	formatFileSize := func(sizeInBytes int64) string {
		if sizeInBytes < 1024 {
			return fmt.Sprintf("%d B", sizeInBytes)
		} else if sizeInBytes < 1024*1024 {
			return fmt.Sprintf("%.2f KB", float64(sizeInBytes)/1024)
		} else {
			return fmt.Sprintf("%.2f MB", float64(sizeInBytes)/(1024*1024))
		}
	}

	// 返回所有URL和Markdown格式给客户端
	c.JSON(http.StatusOK, gin.H{
		"status":             "success",
		"url":                imgURL,                       // 传统URL (向后兼容)
		"original_size":      originalSize,                 // 原始图片大小（字节）
		"original_size_text": formatFileSize(originalSize), // 原始图片大小（人类可读格式）
		"webp_size":          webpSize,                     // WebP图片大小（字节）
		"webp_size_text":     formatFileSize(webpSize),     // WebP图片大小（人类可读格式）
		"compression_ratio":  compressionRatio,             // 压缩比例（百分比）
		"message":            "图片已成功上传并转换",
	})
}

func imageHandler(c *gin.Context) {
	// 从URL参数中提取文件路径
	filePath := c.Param("filename")
	if filePath == "" {
		c.Status(http.StatusNotFound)
		return
	}

	// 提取文件扩展名
	ext := strings.ToLower(filepath.Ext(filePath))

	// 确定原始文件和WebP文件的路径
	// 路径格式为 YY/MM/DD/timestamp.ext
	originalPath := filepath.Join(config.PicsDir, filePath)

	// 计算WebP文件的相对路径和绝对路径
	dir := filepath.Dir(filePath)
	baseNameWithoutExt := strings.TrimSuffix(filepath.Base(filePath), ext)
	webpRelPath := filepath.Join(dir, baseNameWithoutExt+".webp")
	webpPath := filepath.Join(config.WebpDir, webpRelPath)

	log.Printf("请求路径: %s, 原始文件路径: %s, WebP文件路径: %s", filePath, originalPath, webpPath)

	// 检查WebP是否存在
	webpExists := false
	if _, err := os.Stat(webpPath); err == nil {
		webpExists = true
	}

	// 检查原始文件是否为GIF且是否为动画
	isAnimatedGif := false
	if ext == ".gif" {
		if _, err := os.Stat(originalPath); err == nil {
			// 检查是否为动画GIF
			if gifData, err := os.ReadFile(originalPath); err == nil {
				if gifImg, err := gif.DecodeAll(bytes.NewReader(gifData)); err == nil {
					isAnimatedGif = len(gifImg.Image) > 1
				}
			}
		}
	}

	// 如果WebP不存在但原始文件存在，则即时生成WebP
	if !webpExists {
		if _, err := os.Stat(originalPath); err == nil {
			// 确保WebP目录存在
			if err := os.MkdirAll(filepath.Dir(webpPath), 0755); err != nil {
				log.Printf("创建WebP目录失败: %v", err)
			} else {
				// 原始文件存在，生成WebP版本
				log.Printf("未找到 %s 的WebP版本，正在即时生成", filePath)
				if err := convertToWebP(originalPath, webpPath); err != nil {
					log.Printf("即时生成WebP失败: %v", err)
					// 即使WebP转换失败，我们也会继续提供原始图片
				} else {
					webpExists = true // WebP已成功创建
				}
			}
		}
	}

	// 对于尚未转换为动画WebP的动画GIF，提供原始文件以确保动画效果正常工作
	if isAnimatedGif && !webpExists {
		log.Printf("提供动画GIF: %s (WebP版本不可用)", originalPath)
		c.Header("Content-Type", "image/gif")
		c.File(originalPath)
		return
	}

	// 如果WebP存在（无论是预先存在的还是刚刚创建的）则提供WebP
	if webpExists {
		// 检查WebP是否实际上是一个复制的GIF文件（为了向后兼容）
		webpData, err := os.ReadFile(webpPath)
		if err == nil && len(webpData) >= 3 && string(webpData[0:3]) == "GIF" {
			// 这实际上是一个带有.webp扩展名的GIF文件
			log.Printf("检测到带有.webp扩展名的GIF文件，以GIF格式提供")
			c.Header("Content-Type", "image/gif")
			c.File(webpPath)
			return
		}

		// 这是一个真正的WebP文件
		log.Printf("提供WebP图片: %s", webpPath)
		c.Header("Content-Type", "image/webp")
		c.File(webpPath)
		return
	}

	// 如果原始文件存在则回退到原始文件
	if _, err := os.Stat(originalPath); err == nil {
		log.Printf("提供原始图片: %s", originalPath)
		// 根据扩展名确定内容类型
		contentType := "image/jpeg" // 默认
		switch ext {
		case ".png":
			contentType = "image/png"
		case ".gif":
			contentType = "image/gif"
		case ".svg":
			contentType = "image/svg+xml"
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		}
		c.Header("Content-Type", contentType)
		c.File(originalPath)
		return
	}

	// 文件都不存在
	log.Printf("文件不存在: %s", originalPath)
	c.Status(http.StatusNotFound)
}

// convertToWebP 将任何类型的图片转换为WebP格式
func convertToWebP(srcPath, dstPath string) error {
	// 检查源文件是否已经是WebP格式
	ext := strings.ToLower(filepath.Ext(srcPath))
	if ext == ".webp" {
		log.Printf("源文件已经是WebP格式，直接复制: %s", srcPath)
		return copyFile(srcPath, dstPath)
	}

	// 检测图片类型
	imgType, _, err := detectImageType(srcPath)
	if err != nil {
		return fmt.Errorf("检测图片类型失败: %w", err)
	}

	// 根据图片类型选择合适的转换方法
	if imgType == "gif" {
		// 动画GIF需要特殊处理
		return convertAnimatedGif(srcPath, dstPath)
	} else {
		// 所有其他图片(包括静态GIF)使用cwebp
		return convertWithCwebp(srcPath, dstPath)
	}
}

// detectImageType 检测图片类型和是否为动画
func detectImageType(filePath string) (imgType string, isAnimated bool, err error) {
	// 默认根据扩展名判断类型
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == "" {
		return "", false, fmt.Errorf("文件没有扩展名")
	}

	// 去掉扩展名前面的点
	imgType = ext[1:]

	// 针对GIF做特殊处理检查是否为动画
	if imgType == "gif" {
		file, err := os.Open(filePath)
		if err != nil {
			return imgType, false, nil // 无法打开文件，假设是静态图片
		}
		defer file.Close()

		gifImg, err := gif.DecodeAll(file)
		if err != nil {
			return imgType, false, nil // 解码失败，假设是静态图片
		}

		isAnimated = len(gifImg.Image) > 1
		log.Printf("检测到GIF图片: %s, 是否动画: %v", filePath, isAnimated)
	}

	return imgType, isAnimated, nil
}

// convertAnimatedGif 转换动画GIF为WebP格式
func convertAnimatedGif(srcPath, dstPath string) error {
	log.Printf("处理动画GIF: %s", srcPath)

	// 检查gif2webp是否可用
	if _, err := exec.LookPath("gif2webp"); err == nil {
		// 使用gif2webp转换，质量从配置获取
		// 注意: gif2webp需要参数和值分开传递
		cmd := exec.Command("gif2webp", "-q", fmt.Sprintf("%d", config.WebPQuality), "-m", "6", srcPath, "-mt", "-min_size", "-o", dstPath)
		output, err := cmd.CombinedOutput()
		if err == nil {
			log.Printf("成功将动画GIF转换为WebP: %s", dstPath)
			return nil
		}
		log.Printf("使用gif2webp转换失败: %v, 输出: %s", err, output)
	} else {
		log.Printf("未找到gif2webp工具，将使用文件复制作为备用方案")
	}

	// 如果转换失败则复制原文件
	return copyFile(srcPath, dstPath)
}

// convertWithCwebp 使用cwebp转换各种图片格式为WebP
func convertWithCwebp(srcPath, dstPath string) error {
	log.Printf("使用cwebp转换图片: %s", srcPath)

	// 获取原始文件大小
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("获取源文件信息失败: %w", err)
	}
	srcSize := srcInfo.Size()

	// 获取文件扩展名（现在直接用于日志，不单独存储变量）
	ext := strings.ToLower(filepath.Ext(srcPath))
	log.Printf("检测到文件类型: %s", ext)

	// 检查cwebp是否可用
	_, err = exec.LookPath("cwebp")
	if err != nil {
		log.Printf("cwebp工具不可用: %v, 将使用文件复制作为备用方案", err)
		return copyFile(srcPath, dstPath)
	}

	// 根据图片类型和大小设置不同的转换参数
	var cmd *exec.Cmd

	cmd = exec.Command("cwebp", "-q", fmt.Sprintf("%d", config.WebPQuality), "-z", "9", srcPath, "-o", dstPath)

	// 执行转换
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("cwebp转换失败: %v, 输出: %s", err, output)
		return copyFile(srcPath, dstPath)
	}

	// 检查转换后的文件大小
	dstInfo, err := os.Stat(dstPath)
	if err != nil {
		return fmt.Errorf("获取目标文件信息失败: %w", err)
	}
	dstSize := dstInfo.Size()

	// 如果WebP文件比原始文件大，使用原始文件替代
	if dstSize > srcSize {
		log.Printf("WebP转换后文件变大 (%d -> %d 字节)，保留原始格式", srcSize, dstSize)
		// 删除较大的WebP文件
		os.Remove(dstPath)
		// 复制原始文件到目标位置
		return copyFile(srcPath, dstPath)
	}

	compressionRatio := float64(dstSize) / float64(srcSize) * 100
	log.Printf("成功转换为WebP格式: %s (原始: %d字节, WebP: %d字节, 压缩率: %.1f%%)",
		dstPath, srcSize, dstSize, compressionRatio)
	return nil
}

// copyFile 在转换失败时复制原始文件
func copyFile(src, dst string) error {
	log.Printf("复制文件: %s -> %s", src, dst)

	inputFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer inputFile.Close()

	outputFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer outputFile.Close()

	if _, err := io.Copy(outputFile, inputFile); err != nil {
		return fmt.Errorf("复制文件失败: %w", err)
	}

	log.Printf("文件复制成功")
	return nil
}

// getDateFolderPath 获取按年/月/日组织的目录路径
func getDateFolderPath(baseDir string) (string, error) {
	now := time.Now()
	// 按照 YY/MM/DD 格式创建目录结构
	datePath := filepath.Join(
		baseDir,
		fmt.Sprintf("%02d", now.Year()%100), // 年份用两位数字
		fmt.Sprintf("%02d", now.Month()),    // 月份用两位数字
		fmt.Sprintf("%02d", now.Day()),      // 日期用两位数字
	)

	// 确保目录存在
	if err := os.MkdirAll(datePath, 0755); err != nil {
		return "", fmt.Errorf("创建日期目录失败: %w", err)
	}

	return datePath, nil
}

// generateTimestampFileName 生成基于时间戳的文件名
func generateTimestampFileName(ext string) string {
	now := time.Now()
	// 使用时间戳作为文件名: unixtime-milliseconds
	timestamp := fmt.Sprintf("%d-%03d", now.Unix(), now.Nanosecond()/1000000)
	return timestamp + ext
}

// generatePaths 为原始图片和WebP图片生成存储路径
func generatePaths(originalExt string) (originalPath, webpPath, relativePath string, err error) {
	// 获取原始图片的目录路径
	picsDirPath, err := getDateFolderPath(config.PicsDir)
	if err != nil {
		return "", "", "", err
	}

	// 获取WebP图片的目录路径
	webpDirPath, err := getDateFolderPath(config.WebpDir)
	if err != nil {
		return "", "", "", err
	}

	// 生成基于时间戳的文件名
	filename := generateTimestampFileName(originalExt)
	webpFilename := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".webp"

	// 构建完整的文件路径
	originalPath = filepath.Join(picsDirPath, filename)
	webpPath = filepath.Join(webpDirPath, webpFilename)

	// 计算相对路径，用于URL（形如 YY/MM/DD/filename.ext）
	currentYear := fmt.Sprintf("%02d", time.Now().Year()%100)
	currentMonth := fmt.Sprintf("%02d", time.Now().Month())
	currentDay := fmt.Sprintf("%02d", time.Now().Day())
	relativePath = filepath.Join(currentYear, currentMonth, currentDay, filename)

	return originalPath, webpPath, relativePath, nil
}

// downloadWebpHandler 提供WebP图片下载
func downloadWebpHandler(c *gin.Context) {
	// 获取文件路径
	filePath := c.Param("filename")
	if filePath == "" {
		c.Status(http.StatusNotFound)
		return
	}

	// 提取文件扩展名
	ext := strings.ToLower(filepath.Ext(filePath))

	// 构建WebP文件路径
	dir := filepath.Dir(filePath)
	baseNameWithoutExt := strings.TrimSuffix(filepath.Base(filePath), ext)
	webpRelPath := filepath.Join(dir, baseNameWithoutExt+".webp")
	webpPath := filepath.Join(config.WebpDir, webpRelPath)

	// 检查WebP文件是否存在
	if _, err := os.Stat(webpPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "WebP图片不存在"})
		return
	}

	// 设置Content-Disposition头，使浏览器下载文件而不是在浏览器中打开
	fileName := baseNameWithoutExt + ".webp"
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	c.Header("Content-Type", "image/webp")
	c.File(webpPath)
}

// downloadOriginalHandler 提供原始图片下载（需要权限校验）
func downloadOriginalHandler(c *gin.Context) {
	// 获取文件路径
	filePath := c.Param("filename")
	if filePath == "" {
		c.Status(http.StatusNotFound)
		return
	}

	// 构建原始文件路径
	originalPath := filepath.Join(config.PicsDir, filePath)

	// 检查原始文件是否存在
	if _, err := os.Stat(originalPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "原图不存在"})
		return
	}

	// 提取文件扩展名
	ext := strings.ToLower(filepath.Ext(filePath))

	// 设置Content-Disposition头，使浏览器下载文件而不是在浏览器中打开
	fileName := filepath.Base(filePath)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))

	// 根据扩展名设置Content-Type
	contentType := "image/jpeg" // 默认
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".gif":
		contentType = "image/gif"
	case ".svg":
		contentType = "image/svg+xml"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	}
	c.Header("Content-Type", contentType)
	c.File(originalPath)
}

// convertExistingImages 扫描所有原始图片目录并转换缺少对应WebP版本的图片
func convertExistingImages() {
	log.Println("开始扫描并转换现有图片...")

	// 记录开始时间，用于计算总耗时
	startTime := time.Now()

	// 统计计数
	var totalImages, convertedImages, errorImages int

	// 递归遍历原始图片目录
	err := filepath.Walk(config.PicsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("访问路径出错 %s: %v", path, err)
			return filepath.SkipDir
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		// 仅处理图片文件
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" && ext != ".webp" {
			return nil
		}

		totalImages++

		// 计算相对路径，用于生成WebP文件路径
		relPath, err := filepath.Rel(config.PicsDir, path)
		if err != nil {
			log.Printf("计算相对路径失败 %s: %v", path, err)
			errorImages++
			return nil
		}

		// 构建对应的WebP路径
		webpPath := filepath.Join(config.WebpDir, strings.TrimSuffix(relPath, ext)+".webp")

		// 检查WebP文件是否已存在
		if _, err := os.Stat(webpPath); os.IsNotExist(err) {
			// WebP文件不存在，需要转换
			log.Printf("转换图片: %s -> %s", path, webpPath)

			// 确保WebP目标目录存在
			webpDir := filepath.Dir(webpPath)
			if err := os.MkdirAll(webpDir, 0755); err != nil {
				log.Printf("创建WebP目录失败 %s: %v", webpDir, err)
				errorImages++
				return nil
			}

			// 调用转换函数
			if err := convertToWebP(path, webpPath); err != nil {
				log.Printf("转换失败 %s: %v", path, err)
				errorImages++
			} else {
				convertedImages++
			}
		}

		return nil
	})

	if err != nil {
		log.Printf("遍历图片目录失败: %v", err)
	}

	// 计算并显示统计信息
	duration := time.Since(startTime)
	log.Printf("批量转换完成: 总计 %d 张图片, 转换 %d 张, 失败 %d 张, 用时 %.2f 秒",
		totalImages, convertedImages, errorImages, duration.Seconds())
}
