FROM golang:1.24-alpine AS builder

# 安装构建依赖
RUN apk add --no-cache git

# 设置工作目录
WORKDIR /app

# 复制 go.mod 和 go.sum
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -o webp-img .

# 使用更小的基础镜像
FROM alpine:latest

# 安装 WebP 工具
RUN apk add --no-cache libwebp-tools

# 为应用创建非root用户
# RUN adduser -D -H -h /app appuser

# 设置工作目录
WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=builder /app/webp-img .

# 复制模板文件
COPY --from=builder /app/templates ./templates

# 创建上传目录结构并设置权限
# RUN mkdir -p /app/uploads/pics /app/uploads/webp && \
#     chown -R appuser:appuser /app

# 切换到非root用户
# USER appuser

# 设置环境变量
ENV WEBP_SERVER_PORT=8080
ENV WEBP_UPLOAD_DIR=/app/uploads
ENV WEBP_TEMPLATE_DIR=/app/templates
ENV WEBP_PICS_DIR=/app/uploads/pics
ENV WEBP_WEBP_DIR=/app/uploads/webp
ENV WEBP_QUALITY=80
ENV WEBP_ACCESS_PASSWORD=webpimg
ENV WEBP_JWT_SECRET=webpimg-secure-jwt-secret-key
ENV WEBP_JWT_EXPIRATION_HOURS=24
ENV WEBP_MAX_LOGIN_ATTEMPTS=5
ENV WEBP_LOCKOUT_MINUTES=15

# 开放端口
EXPOSE 8080

# 设置健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/login || exit 1

# 运行应用
CMD ["./webp-img"]