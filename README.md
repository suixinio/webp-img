# WebP 图片转换服务

一个基于 Go 语言的高效 WebP 图片转换和管理服务，支持图片上传、自动 WebP 转换、在线画廊浏览等功能。

超过 **90%** 代码由 **AI** 自主生成，最新版本已用在 **个人生产环境中** 。

## ✨ 主要特性

- 🚀 **快速转换**：自动将 JPG、PNG、GIF 等格式转换为 WebP 格式
- 📁 **智能存储**：按年/月/日自动组织文件结构
- 🔒 **安全认证**：JWT 令牌认证，支持登录限流和 CSRF 保护
- 🎯 **动画支持**：完整支持动画 GIF 转换为动画 WebP
- 📱 **响应式界面**：现代化的 Web 界面，支持拖拽上传
- 🖼️ **在线画廊**：浏览和管理已上传的图片
- 📊 **压缩统计**：实时显示文件大小和压缩比例
- 🔄 **即时转换**：访问时自动生成缺失的 WebP 版本
- 📦 **批量上传**：支持多文件同时上传和处理

## 🏗️ 技术架构

- **后端**：Go + Gin 框架
- **认证**：JWT + CSRF 令牌双重保护
- **图片处理**：cwebp、gif2webp 命令行工具
- **存储**：本地文件系统，按日期分层存储
- **前端**：原生 HTML/CSS/JavaScript，Bootstrap Icons

## 🚀 快速开始

### 使用 Docker Compose（推荐）

1. 克隆项目并启动服务：
```bash
git clone https://github.com/suixinio/webp-img.git
cd webp-img
docker-compose up -d
```

2. 访问服务：
   - 浏览器打开 `http://localhost:8080`
   - 默认密码：`webpimg`

## ⚙️ 配置说明

所有配置通过环境变量进行设置：

### 服务器配置
| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `WEBP_SERVER_PORT` | `8080` | 服务器端口 |
| `WEBP_TEMPLATE_DIR` | `./templates` | HTML 模板目录 |

### 存储配置
| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `WEBP_UPLOAD_DIR` | `./uploads` | 上传根目录（向后兼容） |
| `WEBP_PICS_DIR` | `./uploads/pics` | 原始图片存储目录 |
| `WEBP_WEBP_DIR` | `./uploads/webp` | WebP 图片存储目录 |

### 图片处理配置
| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `WEBP_QUALITY` | `80` | WebP 压缩质量 (1-100) |
| `WEBP_CONVERT_EXISTING` | `false` | 启动时转换现有图片 |
| `WEBP_FORCE_REGENERATE` | `false` | 强制重新生成 WebP 文件 |

### 安全配置
| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `WEBP_ACCESS_PASSWORD` | `webpimg` | 页面访问密码 |
| `WEBP_JWT_SECRET` | `webpimg-secure-jwt-secret-key` | JWT 签名密钥 |
| `WEBP_JWT_EXPIRATION_HOURS` | `24` | JWT 过期时间（小时） |
| `WEBP_MAX_LOGIN_ATTEMPTS` | `5` | 最大登录尝试次数 |
| `WEBP_LOCKOUT_MINUTES` | `15` | 登录锁定时间（分钟） |

## 📂 目录结构

```
webp-img/
├── main.go                 # 主程序入口
├── config/
│   └── config.go          # 配置管理
├── security/
│   └── auth.go           # 认证和安全中间件
├── templates/            # HTML 模板
│   ├── index.html       # 上传页面
│   ├── gallery.html     # 画廊页面
│   ├── password.html    # 登录页面
│   └── css/            # 样式文件
├── uploads/             # 文件存储目录
│   ├── pics/           # 原始图片
│   │   └── YY/MM/DD/   # 按日期分层
│   └── webp/           # WebP 图片
│       └── YY/MM/DD/   # 按日期分层
├── Dockerfile           # Docker 镜像构建
└── docker-compose.yml   # Docker Compose 配置
```

## 🎯 核心功能

### 图片上传与转换

- **支持格式**：JPG、PNG、GIF、WebP
- **文件大小**：默认最大 10MB
- **转换质量**：可配置的 WebP 压缩质量
- **智能处理**：动画 GIF 保持动画效果
- **批量上传**：支持多文件同时处理

### 存储管理

- **分层存储**：按 `YY/MM/DD` 格式自动分类
- **文件命名**：使用时间戳确保唯一性
- **双重存储**：保留原始文件和 WebP 版本
- **即时转换**：访问时自动生成缺失的 WebP

### API 接口

| 路径 | 方法 | 说明 | 认证 |
|------|------|------|------|
| `/login` | GET/POST | 登录页面和认证 | ❌ |
| `/` | GET | 上传页面 | ✅ |
| `/gallery` | GET | 图片画廊 | ✅ |
| `/upload` | POST | 图片上传 | ✅ |
| `/api/images` | GET | 图片列表 API | ✅ |
| `/img/*filepath` | GET | 图片访问（优先 WebP） | ❌ |
| `/download/webp/*filepath` | GET | WebP 下载 | ❌ |

### 安全特性

- **JWT 认证**：基于令牌的身份认证
- **CSRF 保护**：防止跨站请求伪造
- **登录限流**：防止暴力破解攻击
- **路径验证**：防止目录遍历攻击

## 📊 性能优化

- **压缩效果**：通常可减少 25-80% 的文件大小
- **智能回退**：如果 WebP 更大则使用原格式
- **动画优化**：动画 GIF 使用专门的转换算法
- **缓存友好**：支持 HTTP 缓存头

## 🤝 贡献指南

1. Fork 项目
2. 创建功能分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开 Pull Request

## 📄 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。

## 🔗 相关资源

- [WebP 官方文档](https://developers.google.com/speed/webp/docs/using?hl=zh-cn)

## 📧 支持

如果您有任何问题或建议，请：

1. 查看 [Issues](https://github.com/suixinio/webp-img/issues)
2. 创建新的 Issue
3. 联系项目维护者

---

**享受高效的 WebP 图片转换体验！** 🎉