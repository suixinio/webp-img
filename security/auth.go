package security

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/suixinio/webp-img/config"
	"golang.org/x/crypto/bcrypt"
)

var (
	// 登录尝试记录
	loginAttempts     = make(map[string]int)
	loginAttemptTimes = make(map[string]time.Time)
	attemptsMutex     = &sync.Mutex{}
)

// GenerateToken 生成JWT令牌
func GenerateToken(cfg *config.Config) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"authorized": true,
		"exp":        time.Now().Add(cfg.JWTExpirationTime).Unix(),
	})

	tokenString, err := token.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ValidateToken 验证JWT令牌
func ValidateToken(tokenString string, cfg *config.Config) (bool, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(cfg.JWTSecret), nil
	})

	if err != nil {
		return false, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		exp, ok := claims["exp"].(float64)
		if !ok {
			return false, errors.New("invalid token expiration")
		}

		if int64(exp) < time.Now().Unix() {
			return false, errors.New("token expired")
		}

		return true, nil
	}

	return false, errors.New("invalid token")
}

// HashPassword 使用bcrypt对密码进行哈希处理
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

// CheckPasswordHash 检查密码与哈希值是否匹配
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// AuthMiddleware 验证JWT令牌的中间件
func AuthMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从Cookie中获取令牌
		tokenCookie, err := c.Cookie("auth_token")
		if err != nil {
			c.Redirect(http.StatusSeeOther, "/login")
			c.Abort()
			return
		}

		// 验证令牌
		valid, err := ValidateToken(tokenCookie, cfg)
		if err != nil || !valid {
			// 清除无效的令牌
			c.SetCookie("auth_token", "", -1, "/", "", false, true)
			c.Redirect(http.StatusSeeOther, "/login")
			c.Abort()
			return
		}

		c.Next()
	}
}

// CSRFMiddleware 添加CSRF保护中间件
func CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 只对POST、PUT、DELETE等修改操作检查CSRF
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
			token := c.GetHeader("X-CSRF-Token")
			csrfCookie, err := c.Cookie("csrf_token")

			if err != nil || token == "" || token != csrfCookie {
				c.JSON(http.StatusForbidden, gin.H{
					"error": "CSRF 验证失败",
				})
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// GenerateCSRFToken 生成CSRF令牌
func GenerateCSRFToken() string {
	// 生成随机令牌
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"nonce": time.Now().UnixNano(),
	})

	// 对令牌进行签名，使用随机密钥
	tokenString, _ := token.SignedString([]byte(time.Now().String()))
	return tokenString
}

// CheckLoginAttempts 检查登录尝试次数，实现登录限速
func CheckLoginAttempts(ip string, cfg *config.Config) bool {
	attemptsMutex.Lock()
	defer attemptsMutex.Unlock()

	// 检查IP是否被锁定
	if lockedUntil, exists := loginAttemptTimes[ip]; exists {
		if time.Now().Before(lockedUntil) {
			return false // IP已被锁定
		}
		// 锁定时间已过，重置计数
		delete(loginAttempts, ip)
		delete(loginAttemptTimes, ip)
	}

	// 检查是否允许登录尝试
	attempts, exists := loginAttempts[ip]
	return !exists || attempts < cfg.MaxLoginAttempts
}

// RecordLoginAttempt 记录登录尝试
func RecordLoginAttempt(ip string, success bool, cfg *config.Config) {
	attemptsMutex.Lock()
	defer attemptsMutex.Unlock()

	if success {
		// 登录成功，重置计数
		delete(loginAttempts, ip)
		delete(loginAttemptTimes, ip)
		return
	}

	// 登录失败，增加计数
	loginAttempts[ip]++

	// 如果失败次数达到阈值，锁定账户
	if loginAttempts[ip] >= cfg.MaxLoginAttempts {
		loginAttemptTimes[ip] = time.Now().Add(cfg.LockoutDuration)
	}
}
