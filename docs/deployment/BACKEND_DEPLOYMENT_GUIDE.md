# GenioAI Server 独立部署指南

## 项目概述

本指南用于在日本服务器上独立部署 **genioAI_server** 后端服务。

## 架构说明

```
genioAI_server (独立部署)
├── HTTP API Server :8200
├── gRPC Server :8181
├── MySQL :3307 (独立数据库)
├── Redis :6380 (独立实例)
└── 连接到 AI Brain :8080 (gRPC)
```

**重要说明**：
- genioAI_server 使用**独立的 MySQL 和 Redis**
- ai-brain 也使用**独立的 MySQL 和 Redis**
- 它们不共享数据库和缓存
- genioAI_server 通过 gRPC 调用 ai-brain 服务

---

## 部署准备

### 1. 服务器环境

```bash
# 更新系统
sudo apt update && sudo apt upgrade -y

# 安装 Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo systemctl start docker
sudo systemctl enable docker

# 安装 Docker Compose
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose

# 安装其他工具
sudo apt install git nginx certbot -y

# 创建部署目录
sudo mkdir -p /opt/genio-backend
sudo chown $USER:$USER /opt/genio-backend
```

### 2. 克隆代码

```bash
cd /opt/genio-backend
git clone https://github.com/ZHAOJIABO/genioAI_server.git genioAI_server
cd genioAI_server
```

---

## 配置文件

### 1. Dockerfile

创建 `/opt/genio-backend/genioAI_server/Dockerfile`：

```dockerfile
# 构建阶段
FROM golang:1.24-alpine AS builder

WORKDIR /build

# 安装构建依赖
RUN apk add --no-cache git make

# 复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源码
COPY . .

# 编译
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-w -s" -o server ./cmd

# 运行阶段
FROM alpine:latest

# 安装运行时依赖
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# 设置时区为东京
ENV TZ=Asia/Tokyo

# 从构建阶段复制二进制文件
COPY --from=builder /build/server .

# 复制配置文件和资源
COPY --from=builder /build/conf ./conf
COPY --from=builder /build/assets ./assets

# 暴露端口
EXPOSE 8200 8181 9188

# 启动命令
CMD ["./server", "-config", "conf/server.prod.yaml"]
```

### 2. Docker Compose 配置

创建 `/opt/genio-backend/docker-compose.yml`：

```yaml
version: '3.8'

networks:
  genio-backend-network:
    driver: bridge
  # 共享网络，用于连接 ai-brain（如果已部署）
  genio-shared:
    external: true
    name: genio-shared

volumes:
  backend_mysql_data:
  backend_redis_data:

services:
  # MySQL 数据库（genioAI_server 独立使用）
  backend-mysql:
    image: mysql:8.0
    container_name: genio-backend-mysql
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: ${BACKEND_MYSQL_ROOT_PASSWORD}
      MYSQL_DATABASE: ${BACKEND_MYSQL_DATABASE}
      MYSQL_USER: ${BACKEND_MYSQL_USER}
      MYSQL_PASSWORD: ${BACKEND_MYSQL_PASSWORD}
    ports:
      - "3307:3306"  # 外部端口 3307，避免与 ai-brain 的 MySQL 冲突
    volumes:
      - backend_mysql_data:/var/lib/mysql
    command: >
      --character-set-server=utf8mb4
      --collation-server=utf8mb4_unicode_ci
      --default-authentication-plugin=mysql_native_password
    networks:
      - genio-backend-network
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost", "-u", "root", "-p${BACKEND_MYSQL_ROOT_PASSWORD}"]
      interval: 10s
      timeout: 5s
      retries: 5

  # Redis（genioAI_server 独立使用）
  backend-redis:
    image: redis:7-alpine
    container_name: genio-backend-redis
    restart: unless-stopped
    ports:
      - "6380:6379"  # 外部端口 6380，避免与 ai-brain 的 Redis 冲突
    volumes:
      - backend_redis_data:/data
    command: redis-server --appendonly yes --requirepass ${BACKEND_REDIS_PASSWORD}
    networks:
      - genio-backend-network
    healthcheck:
      test: ["CMD", "redis-cli", "--no-auth-warning", "-a", "${BACKEND_REDIS_PASSWORD}", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  # GenioAI Backend 服务
  backend:
    build:
      context: ./genioAI_server
      dockerfile: Dockerfile
    container_name: genio-backend
    restart: unless-stopped
    ports:
      - "8200:8200"  # HTTP API
      - "8181:8181"  # gRPC
      - "9188:9188"  # Metrics
    environment:
      - CONFIG_FILE=/app/conf/server.prod.yaml
    volumes:
      - ./genioAI_server/conf:/app/conf:ro
      - ./logs:/app/log
    networks:
      - genio-backend-network
      - genio-shared  # 连接共享网络以访问 ai-brain
    depends_on:
      backend-mysql:
        condition: service_healthy
      backend-redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8200/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
```

### 3. 生产环境配置文件

创建 `/opt/genio-backend/genioAI_server/conf/server.prod.yaml`：

```yaml
servername: "genio-backend-prod"

LogConfig:
  LogPath: log/genio-backend.log
  LogLevel: info
  MaxAgeDays: 7
  MaxSize: 100
  MaxBackups: 10
  Compress: true

VisionAiServerConfig:
  Port: 8181          # gRPC 端口
  HTTPPort: 8200      # HTTP API 端口
  DialTimeout: 10s
  Mode: PROD          # 生产模式
  Region: jp          # 日本区域

# genioAI_server 独立的 Redis
Redis:
  Addr: "backend-redis:6379"
  Db: 0
  AuthInfo: "${BACKEND_REDIS_PASSWORD}"

# genioAI_server 独立的 MySQL
Mysql:
  Addr: backend-mysql
  User: ${BACKEND_MYSQL_USER}
  Password: ${BACKEND_MYSQL_PASSWORD}
  Db: ${BACKEND_MYSQL_DATABASE}
  Port: 3306
  MigrationsDir: /app/assets/migrations

# MongoDB（如果需要）
MongoDB:
  Addr: "mongodb://backend-mongo:27017"
  Db: genio_backend
  Collection: msg_

# Metrics
metrics:
  addr: ":9188"
  path: "/metrics"
  namespace: "genio_backend"

# GeoIP
geoip:
  enabled: true
  database_path: "/app/assets/GeoLite2-Country.mmdb"

# AI Brain gRPC 地址（重要！）
AIBrainServer:
  # 如果 ai-brain 已部署在同一服务器：
  Addr: genio-ai-brain:8080  # Docker 容器名
  # 或使用 localhost（如果 ai-brain 在主机上）：
  # Addr: host.docker.internal:8080

# LLM 配置
LlmConfig:
  AzureOpenAI:
    Endpoint: "${AZURE_OPENAI_ENDPOINT}"
    APIKey: "${AZURE_OPENAI_API_KEY}"
    GPT4O: "gpt-4o"

  Doubao:
    ApiKey: "${DOUBAO_API_KEY}"
    Endpoint: "${DOUBAO_ENDPOINT}"
    Region: "${DOUBAO_REGION}"
    DoubaoPro: "${DOUBAO_PRO_MODEL}"
    DoubaoProVision: "${DOUBAO_PRO_VISION_MODEL}"

  DeepSeek:
    Endpoint: "https://api.deepseek.com"
    APIKey: "${DEEPSEEK_API_KEY}"
    DeepSeekR1: "deepseek-reasoner"
    DeepSeekV3: "deepseek-chat"

  Gemini:
    AuthFile: "/app/conf/gemini_auth.json"

# 存储配置（根据实际使用的存储）
R2Config:
  Endpoint: "${R2_ENDPOINT}"
  AccessKeyId: "${R2_ACCESS_KEY_ID}"
  AccessKeySecret: "${R2_ACCESS_KEY_SECRET}"
  Bucket: "${R2_BUCKET}"
  UgcAddr: "${R2_PUBLIC_URL}"

# 邮件配置
Email:
  smtp_host: "${SMTP_HOST}"
  smtp_port: ${SMTP_PORT}
  username: "${SMTP_USERNAME}"
  password: "${SMTP_PASSWORD}"
  from_address: "${SMTP_FROM}"
  expire_minutes: 5

# 并发控制
Reconciler:
  enabled: true
  interval: 1m
  redis_scan_count: 1000
  max_tasks_per_round: 10000
  worker_pool_size: 16
  key_ttl_seconds: 3600
  queue:
    enabled: true
    interval: 1m
    redis_scan_count: 1000
    max_tasks_per_round: 10000
    worker_pool_size: 16
  concurrent:
    enabled: true
    interval: 1m
    redis_scan_count: 1000
    max_tasks_per_round: 10000
    worker_pool_size: 16
    awaiting_timeout: 10m

# 事件上报（可选）
EventSink:
  enabled: false
  addr: ""
  queue_size: 10000
  batch_size: 100
  flush_interval: 5s
  shutdown_timeout: 10s
  source: "genio_backend"
  server_node: ""

# 推送网关（可选）
PushGateway:
  enabled: false
  addr: ""
  timeout: 10s
  env: "production"
```

### 4. 环境变量配置

创建 `/opt/genio-backend/.env`：

```bash
# ==================== 数据库配置 ====================
# Backend MySQL（独立数据库）
BACKEND_MYSQL_ROOT_PASSWORD=<your-db-password>
BACKEND_MYSQL_DATABASE=genio_backend
BACKEND_MYSQL_USER=root
BACKEND_MYSQL_PASSWORD=<your-db-password>

# Backend Redis（独立实例）
BACKEND_REDIS_PASSWORD=<your-db-password>

# ==================== LLM 服务配置 ====================
# Azure OpenAI
AZURE_OPENAI_ENDPOINT=https://your-resource.openai.azure.com/
AZURE_OPENAI_API_KEY=your_azure_openai_key

# 豆包（字节跳动）
DOUBAO_API_KEY=your_doubao_api_key
DOUBAO_ENDPOINT=https://ark.cn-beijing.volces.com/api/v3
DOUBAO_REGION=cn-beijing
DOUBAO_PRO_MODEL=your_model_endpoint
DOUBAO_PRO_VISION_MODEL=your_vision_model_endpoint

# DeepSeek
DEEPSEEK_API_KEY=your_deepseek_api_key

# ==================== 对象存储配置 ====================
# Cloudflare R2
R2_ENDPOINT=https://08e5d61be623f63d785df35604e48655.r2.cloudflarestorage.com
R2_ACCESS_KEY_ID=<your-r2-access-key-id>
R2_ACCESS_KEY_SECRET=<your-r2-secret-access-key>
R2_BUCKET=genio-ai
R2_PUBLIC_URL=https://cdn.appbobo.com

# ==================== 邮件配置 ====================
SMTP_HOST=smtp.qq.com
SMTP_PORT=587
SMTP_USERNAME=624345999@qq.com
SMTP_PASSWORD=<your-smtp-app-password>  # Outlook应用专用密码
SMTP_FROM=624345999@qq.com

# ==================== 其他配置 ====================
# Apple Sign In（如果需要）
APPLE_TEAM_ID=your_team_id
APPLE_KID=your_kid
APPLE_PRIVATE_KEY=your_private_key_base64
```

---

## 部署步骤

### Step 1: 创建共享网络（用于服务间通信）

```bash
# 创建共享 Docker 网络，供 genioAI_server 和 ai-brain 通信
docker network create genio-shared
```

### Step 2: 配置文件准备

```bash
cd /opt/genio-backend/genioAI_server

# 1. 创建生产配置文件
cp conf/server.yaml.example conf/server.prod.yaml

# 2. 编辑生产配置
vi conf/server.prod.yaml
# 按照上面的模板修改配置

# 3. 配置环境变量
cd /opt/genio-backend
vi .env
# 填写所有必需的环境变量

# 4. 准备 Gemini 认证文件（如果使用 Gemini）
# 将 gemini_auth.json 放到 conf/ 目录
```

### Step 3: 数据库初始化

```bash
cd /opt/genio-backend

# 启动 MySQL
docker-compose up -d backend-mysql

# 等待 MySQL 启动
sleep 10

# 导入初始 SQL（如果有）
# docker exec -i genio-backend-mysql mysql -u root -p${BACKEND_MYSQL_ROOT_PASSWORD} ${BACKEND_MYSQL_DATABASE} < init.sql

# 或运行迁移
# docker-compose run --rm backend ./server -migrate
```

### Step 4: 构建和启动服务

```bash
cd /opt/genio-backend

# 构建镜像
docker-compose build backend

# 启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f backend
```

### Step 5: 验证部署

```bash
# 检查服务状态
docker-compose ps

# 测试 HTTP API
curl http://localhost:8200/health

# 测试 gRPC（如果有 grpcurl）
grpcurl -plaintext localhost:8181 list

# 查看 Metrics
curl http://localhost:9188/metrics

# 查看日志
docker-compose logs backend
```

---

## Nginx 反向代理配置

创建 `/etc/nginx/sites-available/genio-backend`：

```nginx
upstream backend_http {
    server localhost:8200;
}

# HTTP -> HTTPS 重定向
server {
    listen 80;
    server_name api.yourdomain.com;
    return 301 https://$server_name$request_uri;
}

# HTTPS 配置
server {
    listen 443 ssl http2;
    server_name api.yourdomain.com;

    # SSL 证书配置
    ssl_certificate /etc/letsencrypt/live/api.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/api.yourdomain.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    # 日志
    access_log /var/log/nginx/backend-access.log;
    error_log /var/log/nginx/backend-error.log;

    # 请求体大小限制（用于图片上传）
    client_max_body_size 50M;

    # API 代理
    location / {
        proxy_pass http://backend_http;
        proxy_http_version 1.1;

        # Headers
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 超时设置
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 300s;
    }

    # Metrics 端点（可选，仅内部访问）
    location /metrics {
        proxy_pass http://localhost:9188/metrics;
        allow 127.0.0.1;
        deny all;
    }
}
```

启用配置：

```bash
# 创建软链接
sudo ln -s /etc/nginx/sites-available/genio-backend /etc/nginx/sites-enabled/

# 测试配置
sudo nginx -t

# 获取 SSL 证书
sudo certbot --nginx -d api.yourdomain.com

# 重载 Nginx
sudo systemctl reload nginx
```

---

## 连接 AI Brain

genioAI_server 需要通过 gRPC 调用 ai-brain 服务。确保配置正确：

### 方案 A：AI Brain 也使用 Docker 部署

在 `server.prod.yaml` 中：

```yaml
AIBrainServer:
  Addr: genio-ai-brain:8080  # ai-brain 的容器名
```

确保两个服务在同一网络：

```yaml
# backend 的 docker-compose.yml
networks:
  - genio-backend-network
  - genio-shared  # 共享网络
```

### 方案 B：AI Brain 直接在主机运行

在 `server.prod.yaml` 中：

```yaml
AIBrainServer:
  Addr: host.docker.internal:8080
```

并在 docker-compose.yml 中添加：

```yaml
services:
  backend:
    extra_hosts:
      - "host.docker.internal:host-gateway"
```

### 方案 C：使用服务器 IP

```yaml
AIBrainServer:
  Addr: 10.0.1.x:8080  # ai-brain 服务器的内网 IP
```

---

## 运维管理

### 1. 查看日志

```bash
# 实时日志
docker-compose logs -f backend

# 查看最近日志
docker-compose logs --tail=100 backend

# 查看特定时间范围
docker logs genio-backend --since 1h

# 查看应用日志文件
tail -f logs/genio-backend.log
```

### 2. 重启服务

```bash
cd /opt/genio-backend

# 重启 backend 服务
docker-compose restart backend

# 重启所有服务
docker-compose restart

# 完全停止并重新启动
docker-compose down
docker-compose up -d
```

### 3. 更新代码

```bash
cd /opt/genio-backend/genioAI_server

# 拉取最新代码
git fetch origin
git log HEAD..origin/main --oneline  # 查看变更
git pull origin main

# 重新构建并部署
cd /opt/genio-backend
docker-compose build backend
docker-compose up -d --no-deps backend

# 验证
docker-compose logs --tail=50 backend
curl http://localhost:8200/health
```

### 4. 数据库备份

创建备份脚本 `/opt/genio-backend/backup.sh`：

```bash
#!/bin/bash

BACKUP_DIR="/opt/genio-backend/backups"
DATE=$(date +%Y%m%d_%H%M%S)

mkdir -p $BACKUP_DIR

# 备份 Backend MySQL
docker exec genio-backend-mysql mysqldump \
  -u root -p${BACKEND_MYSQL_ROOT_PASSWORD} \
  --all-databases \
  --single-transaction \
  --quick \
  --lock-tables=false \
  > $BACKUP_DIR/backend_mysql_$DATE.sql

# 压缩
gzip $BACKUP_DIR/backend_mysql_$DATE.sql

# 删除 7 天前的备份
find $BACKUP_DIR -name "backend_mysql_*.sql.gz" -mtime +7 -delete

echo "备份完成: $BACKUP_DIR/backend_mysql_$DATE.sql.gz"
```

设置定时备份：

```bash
chmod +x /opt/genio-backend/backup.sh

# 每天凌晨 2 点备份
echo "0 2 * * * /opt/genio-backend/backup.sh >> /var/log/genio-backup.log 2>&1" | crontab -
```

### 5. 数据库恢复

```bash
# 恢复备份
gunzip -c backups/backend_mysql_20240203_020000.sql.gz | \
  docker exec -i genio-backend-mysql mysql -u root -p${BACKEND_MYSQL_ROOT_PASSWORD}
```

### 6. 监控和健康检查

```bash
# 检查容器状态
docker-compose ps

# 检查服务健康
curl http://localhost:8200/health

# 检查数据库连接
docker exec genio-backend-mysql mysqladmin ping -u root -p${BACKEND_MYSQL_ROOT_PASSWORD}

# 检查 Redis
docker exec genio-backend-redis redis-cli -a ${BACKEND_REDIS_PASSWORD} ping

# 查看 Metrics
curl http://localhost:9188/metrics
```

---

## 故障排查

### 1. 服务无法启动

```bash
# 查看详细日志
docker-compose logs backend

# 检查配置文件
docker-compose config

# 进入容器调试
docker exec -it genio-backend sh
```

### 2. 无法连接数据库

```bash
# 检查 MySQL 状态
docker-compose ps backend-mysql

# 测试数据库连接
docker exec -it genio-backend-mysql mysql -u root -p${BACKEND_MYSQL_ROOT_PASSWORD}

# 检查网络连通性
docker exec genio-backend ping backend-mysql
```

### 3. 无法连接 AI Brain

```bash
# 检查 AI Brain 服务状态
docker ps | grep ai-brain

# 测试 gRPC 连接
grpcurl -plaintext localhost:8080 list

# 检查网络连通性
docker exec genio-backend ping genio-ai-brain

# 或测试主机连接
docker exec genio-backend ping host.docker.internal
```

### 4. 端口冲突

```bash
# 检查端口占用
sudo netstat -tlnp | grep 8200
sudo netstat -tlnp | grep 3307
sudo netstat -tlnp | grep 6380

# 修改 docker-compose.yml 中的端口映射
```

---

## 安全建议

### 1. 防火墙配置

```bash
# 只允许必要的端口
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 22/tcp

# 如果需要外部访问 MySQL（不推荐）
# sudo ufw allow from <trusted-ip> to any port 3307

sudo ufw enable
```

### 2. 强化 Docker 安全

```yaml
# 在 docker-compose.yml 中添加资源限制
services:
  backend:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 4G
        reservations:
          cpus: '1'
          memory: 2G
```

### 3. 定期更新

```bash
# 更新系统
sudo apt update && sudo apt upgrade -y

# 更新 Docker 镜像
docker-compose pull
docker-compose up -d
```

---

## 下一步

部署完 genioAI_server 后，可以继续部署：

1. **AI Brain** - AI 生图中台服务
2. **Frontend** - Nuxt.js 前端应用

每个服务使用各自独立的数据库和 Redis，通过共享 Docker 网络进行通信。

---

## 快速命令参考

```bash
# 启动服务
docker-compose up -d

# 停止服务
docker-compose down

# 查看日志
docker-compose logs -f backend

# 重启服务
docker-compose restart backend

# 更新代码
cd genioAI_server && git pull && cd ..
docker-compose build backend && docker-compose up -d --no-deps backend

# 备份数据库
./backup.sh

# 查看状态
docker-compose ps
curl http://localhost:8200/health
```
