package main

import (
	"bytes"
	"fmt"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 全局配置
var config *Config

func main() {
	// 加载配置
	config = LoadConfig()

	// 设置Gin路由器
	router := gin.Default()

	// 加载HTML模板
	router.LoadHTMLGlob(filepath.Join(config.TemplateDir, "*"))

	// 定义路由
	router.GET("/", homeHandler)
	router.POST("/upload", uploadHandler)
	router.GET("/img/*filename", imageHandler) // 使用通配符匹配包含路径的文件名

	// 设置静态文件目录，用于向后兼容
	router.Static("/uploads", config.UploadDir)

	// 设置新的静态文件目录
	router.Static("/pics", config.PicsDir)
	router.Static("/webp", config.WebpDir)

	// 启动服务器
	log.Printf("服务器在端口 %s 上启动...\n", config.ServerPort)
	router.Run(":" + config.ServerPort)
}

func homeHandler(c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", nil)
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

	// 转换为WebP并保存
	if err := convertToWebP(originalPath, webpPath); err != nil {
		log.Printf("转换为WebP失败: %v", err)
		// 即使WebP转换失败，我们也会继续处理
	}

	// 返回URL给客户端
	imageURL := fmt.Sprintf("/img/%s", relativePath)
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"url":     imageURL,
		"message": "图片已成功上传并转换",
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
	// 检测图片类型
	imgType, isAnimated, err := detectImageType(srcPath)
	if err != nil {
		return fmt.Errorf("检测图片类型失败: %w", err)
	}

	// 根据图片类型选择合适的转换方法
	if isAnimated && imgType == "gif" {
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
		cmd := exec.Command("gif2webp", "-q", fmt.Sprintf("%d", config.WebPQuality), "-m", "6", "-mixed", srcPath, "-o", dstPath)
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

	// 检查cwebp是否可用
	_, err := exec.LookPath("cwebp")
	if err != nil {
		log.Printf("cwebp工具不可用: %v, 将使用文件复制作为备用方案", err)
		return copyFile(srcPath, dstPath)
	}

	// 使用cwebp转换，质量从配置获取
	// 注意: cwebp需要参数和值分开传递
	cmd := exec.Command("cwebp", "-q", fmt.Sprintf("%d", config.WebPQuality), srcPath, "-o", dstPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("cwebp转换失败: %v, 输出: %s", err, output)
		// 如果转换失败，复制原文件
		return copyFile(srcPath, dstPath)
	}

	log.Printf("成功转换为WebP格式: %s", dstPath)
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
