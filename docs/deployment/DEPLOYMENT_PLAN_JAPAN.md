# 日本服务器部署方案

## 项目概述

本方案将以下三个项目部署到日本服务器：

1. **genioAI_server** - Go后端服务（业务API服务）
2. **ai-brain** - Go AI中台（AI生图核心服务）
3. **geinoAI_web** - Nuxt.js前端Web服务

## 架构设计

```
Internet
    ↓
[Nginx (443/80)]
    ↓
    ├─→ Frontend (Nuxt.js) :3000
    ├─→ Backend API (genioAI_server) :8200
    └─→ AI Brain gRPC :8080
        └─→ MySQL :3306
        └─→ Redis :6379
```

## 服务依赖关系

```
geinoAI_web (前端)
    ↓ HTTP API
genioAI_server (后端)
    ↓ gRPC
ai-brain (AI中台)
    ↓
MySQL + Redis
```

## 部署方案选择

### 方案一：Docker Compose 部署（推荐）

**优点：**
- 环境隔离，易于管理
- 一键部署，快速回滚
- 资源限制和监控
- 易于扩展和迁移

**适用场景：** 中小规模应用，快速上线

### 方案二：Kubernetes 部署

**优点：**
- 高可用和自动扩缩容
- 服务发现和负载均衡
- 滚动更新零停机
- 完善的监控和日志

**适用场景：** 大规模应用，需要高可用

### 方案三：传统部署（Systemd）

**优点：**
- 资源占用少
- 直接部署，性能最优
- 易于调试

**适用场景：** 资源受限环境，简单应用

---

## 方案一：Docker Compose 部署（详细步骤）

### 1. 服务器环境准备

```bash
# 更新系统
sudo apt update && sudo apt upgrade -y

# 安装 Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# 安装 Docker Compose
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose

# 安装 Nginx
sudo apt install nginx -y

# 安装 Git
sudo apt install git -y

# 创建部署目录
sudo mkdir -p /opt/genio
sudo chown $USER:$USER /opt/genio
```

### 2. 项目目录结构

```
/opt/genio/
├── ai-brain/              # AI中台
│   ├── Dockerfile
│   ├── docker-compose.yml
│   └── configs/
├── genioAI_server/        # 后端服务
│   ├── Dockerfile
│   └── configs/
├── geinoAI_web/           # 前端服务
│   ├── Dockerfile
│   └── .env
├── nginx/                 # Nginx配置
│   └── conf.d/
└── docker-compose.prod.yml # 总编排文件
```

### 3. 创建统一的 Docker Compose 配置

创建 `/opt/genio/docker-compose.prod.yml`：

```yaml
version: '3.8'

networks:
  genio-network:
    driver: bridge

volumes:
  mysql_data:
  redis_data:

services:
  # MySQL 数据库
  mysql:
    image: mysql:8.0
    container_name: genio-mysql
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_ROOT_PASSWORD}
      MYSQL_DATABASE: ${MYSQL_DATABASE}
      MYSQL_USER: ${MYSQL_USER}
      MYSQL_PASSWORD: ${MYSQL_PASSWORD}
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql
    command: >
      --character-set-server=utf8mb4
      --collation-server=utf8mb4_unicode_ci
      --default-authentication-plugin=mysql_native_password
    networks:
      - genio-network
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5

  # Redis 缓存
  redis:
    image: redis:7-alpine
    container_name: genio-redis
    restart: unless-stopped
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    command: redis-server --appendonly yes
    networks:
      - genio-network
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  # AI Brain 服务
  ai-brain:
    build:
      context: ./ai-brain
      dockerfile: Dockerfile
    container_name: genio-ai-brain
    restart: unless-stopped
    ports:
      - "8080:8080"  # gRPC
      - "9090:9090"  # Metrics
    environment:
      - ENV=PROD
      - SERVER_NAME=ai-brain-prod
      - LOG_LEVEL=info
      - GRPC_PORT=8080
      - METRICS_PORT=9090
      - MYSQL_DSN=${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(mysql:3306)/${MYSQL_DATABASE}?charset=utf8mb4&parseTime=True&loc=Local
      - REDIS_ADDR=redis:6379
      - REDIS_DB=0
      # AI Provider Keys
      - MINIMAX_APP_ID=${MINIMAX_APP_ID}
      - MINIMAX_SECRET=${MINIMAX_SECRET}
      - KLING_API_KEY=${KLING_API_KEY}
      - SILICONFLOW_API_KEY=${SILICONFLOW_API_KEY}
      - AZURE_OPENAI_API_KEY=${AZURE_OPENAI_API_KEY}
      - AZURE_OPENAI_ENDPOINT=${AZURE_OPENAI_ENDPOINT}
      - GEMINI_CREDENTIAL_JSON=${GEMINI_CREDENTIAL_JSON}
    networks:
      - genio-network
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy

  # Backend API 服务
  backend:
    build:
      context: ./genioAI_server
      dockerfile: Dockerfile
    container_name: genio-backend
    restart: unless-stopped
    ports:
      - "8200:8200"  # HTTP API
      - "8201:8201"  # gRPC
    environment:
      - MODE=PROD
      - HTTP_PORT=8200
      - GRPC_PORT=8201
      - AIBRAIN_ADDR=ai-brain:8080
      - MYSQL_ADDR=mysql
      - MYSQL_PORT=3306
      - MYSQL_USER=${MYSQL_USER}
      - MYSQL_PASSWORD=${MYSQL_PASSWORD}
      - MYSQL_DB=${MYSQL_DATABASE}
      - REDIS_ADDR=redis:6379
    volumes:
      - ./genioAI_server/configs:/app/configs:ro
    networks:
      - genio-network
    depends_on:
      - mysql
      - redis
      - ai-brain

  # Frontend Web 服务
  frontend:
    build:
      context: ./geinoAI_web
      dockerfile: Dockerfile
      args:
        - NODE_ENV=production
    container_name: genio-frontend
    restart: unless-stopped
    ports:
      - "3000:3000"
    environment:
      - NUXT_PUBLIC_API_BASE_URL=https://api.yourdomain.com
    networks:
      - genio-network
    depends_on:
      - backend

  # Nginx 反向代理
  nginx:
    image: nginx:alpine
    container_name: genio-nginx
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/conf.d:/etc/nginx/conf.d:ro
      - ./nginx/ssl:/etc/nginx/ssl:ro
      - ./nginx/logs:/var/log/nginx
    networks:
      - genio-network
    depends_on:
      - frontend
      - backend
```

### 4. 创建 Dockerfile

#### 4.1 ai-brain Dockerfile

创建 `/opt/genio/ai-brain/Dockerfile`：

```dockerfile
FROM golang:1.24-alpine AS builder

WORKDIR /build

# 复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源码
COPY . .

# 编译
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o server ./cmd/server

# 运行镜像
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# 设置时区为东京
ENV TZ=Asia/Tokyo

COPY --from=builder /build/server .
COPY --from=builder /build/configs ./configs

EXPOSE 8080 9090

CMD ["./server"]
```

#### 4.2 genioAI_server Dockerfile

创建 `/opt/genio/genioAI_server/Dockerfile`：

```dockerfile
FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

ENV TZ=Asia/Tokyo

COPY --from=builder /build/server .
COPY --from=builder /build/conf ./conf

EXPOSE 8200 8201

CMD ["./server", "-config", "conf/config.prod.yaml"]
```

#### 4.3 geinoAI_web Dockerfile

创建 `/opt/genio/geinoAI_web/Dockerfile`：

```dockerfile
FROM node:20-alpine AS builder

WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM node:20-alpine

WORKDIR /app

ENV NODE_ENV=production
ENV TZ=Asia/Tokyo

COPY --from=builder /app/.output ./.output
COPY --from=builder /app/package*.json ./

EXPOSE 3000

CMD ["node", ".output/server/index.mjs"]
```

### 5. Nginx 配置

创建 `/opt/genio/nginx/conf.d/genio.conf`：

```nginx
# 前端服务
server {
    listen 80;
    server_name yourdomain.com www.yourdomain.com;

    # 重定向到 HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name yourdomain.com www.yourdomain.com;

    # SSL 证书配置
    ssl_certificate /etc/nginx/ssl/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    # 日志
    access_log /var/log/nginx/genio-access.log;
    error_log /var/log/nginx/genio-error.log;

    # 前端静态资源和页面
    location / {
        proxy_pass http://frontend:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }
}

# API 服务
server {
    listen 80;
    server_name api.yourdomain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name api.yourdomain.com;

    ssl_certificate /etc/nginx/ssl/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;

    access_log /var/log/nginx/api-access.log;
    error_log /var/log/nginx/api-error.log;

    # 请求体大小限制（用于图片上传）
    client_max_body_size 50M;

    location / {
        proxy_pass http://backend:8200;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # 超时设置
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 300s;
    }
}
```

### 6. 环境变量配置

创建 `/opt/genio/.env`：

```bash
# MySQL
MYSQL_ROOT_PASSWORD=your_strong_root_password
MYSQL_DATABASE=genio_db
MYSQL_USER=genio_user
MYSQL_PASSWORD=your_strong_db_password

# AI Providers
MINIMAX_APP_ID=your_minimax_app_id
MINIMAX_SECRET=your_minimax_secret
KLING_API_KEY=your_kling_api_key
SILICONFLOW_API_KEY=your_siliconflow_api_key
AZURE_OPENAI_API_KEY=your_azure_key
AZURE_OPENAI_ENDPOINT=your_azure_endpoint
GEMINI_CREDENTIAL_JSON=your_gemini_credentials

# Domain
DOMAIN=yourdomain.com
```

### 7. 部署脚本

创建 `/opt/genio/deploy.sh`：

```bash
#!/bin/bash

set -e

echo "=== GenioAI 生产环境部署 ==="

# 1. 拉取最新代码
echo "1. 拉取最新代码..."
cd /opt/genio

for project in ai-brain genioAI_server geinoAI_web; do
    if [ -d "$project" ]; then
        echo "更新 $project..."
        cd $project
        git pull origin main
        cd ..
    else
        echo "错误: $project 目录不存在"
        exit 1
    fi
done

# 2. 构建镜像
echo "2. 构建 Docker 镜像..."
docker-compose -f docker-compose.prod.yml build

# 3. 停止旧服务
echo "3. 停止旧服务..."
docker-compose -f docker-compose.prod.yml down

# 4. 启动新服务
echo "4. 启动新服务..."
docker-compose -f docker-compose.prod.yml up -d

# 5. 等待服务启动
echo "5. 等待服务启动..."
sleep 10

# 6. 检查服务状态
echo "6. 检查服务状态..."
docker-compose -f docker-compose.prod.yml ps

# 7. 查看日志
echo "7. 最近的日志..."
docker-compose -f docker-compose.prod.yml logs --tail=50

echo "=== 部署完成 ==="
echo "访问: https://yourdomain.com"
```

### 8. 部署步骤

```bash
# 1. 克隆代码到服务器
cd /opt/genio
git clone <genioAI_server-repo> genioAI_server
git clone <ai-brain-repo> ai-brain
git clone <geinoAI_web-repo> geinoAI_web

# 2. 配置环境变量
vi /opt/genio/.env
# 填写所有必需的环境变量

# 3. 创建 Nginx 配置目录
mkdir -p nginx/conf.d nginx/ssl nginx/logs

# 4. 配置 SSL 证书（Let's Encrypt）
sudo apt install certbot python3-certbot-nginx -y
sudo certbot certonly --nginx -d yourdomain.com -d www.yourdomain.com -d api.yourdomain.com
sudo cp /etc/letsencrypt/live/yourdomain.com/fullchain.pem nginx/ssl/
sudo cp /etc/letsencrypt/live/yourdomain.com/privkey.pem nginx/ssl/

# 5. 执行部署
chmod +x deploy.sh
./deploy.sh

# 6. 配置自动续期证书
echo "0 0 1 * * certbot renew --quiet" | sudo crontab -
```

### 9. 监控和维护

#### 9.1 查看日志

```bash
# 查看所有服务日志
docker-compose -f docker-compose.prod.yml logs -f

# 查看特定服务日志
docker-compose -f docker-compose.prod.yml logs -f backend
docker-compose -f docker-compose.prod.yml logs -f ai-brain
docker-compose -f docker-compose.prod.yml logs -f frontend
```

#### 9.2 重启服务

```bash
# 重启所有服务
docker-compose -f docker-compose.prod.yml restart

# 重启单个服务
docker-compose -f docker-compose.prod.yml restart backend
```

#### 9.3 备份数据库

创建 `/opt/genio/backup.sh`：

```bash
#!/bin/bash

BACKUP_DIR="/opt/genio/backups"
DATE=$(date +%Y%m%d_%H%M%S)
MYSQL_CONTAINER="genio-mysql"

mkdir -p $BACKUP_DIR

# 备份 MySQL
docker exec $MYSQL_CONTAINER mysqldump -u root -p${MYSQL_ROOT_PASSWORD} --all-databases > $BACKUP_DIR/mysql_$DATE.sql

# 压缩备份
gzip $BACKUP_DIR/mysql_$DATE.sql

# 删除7天前的备份
find $BACKUP_DIR -name "mysql_*.sql.gz" -mtime +7 -delete

echo "备份完成: $BACKUP_DIR/mysql_$DATE.sql.gz"
```

#### 9.4 设置定时备份

```bash
# 每天凌晨2点备份
echo "0 2 * * * /opt/genio/backup.sh" | crontab -
```

### 10. 性能优化建议

1. **启用 HTTP/2 和 Gzip**
2. **配置 CDN**（如 Cloudflare）
3. **Redis 持久化**配置
4. **MySQL 参数调优**
5. **Docker 资源限制**

```yaml
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

### 11. 安全建议

1. **配置防火墙**（ufw）
2. **定期更新系统和 Docker**
3. **使用强密码**
4. **配置 fail2ban**
5. **限制 SSH 访问**
6. **定期备份**

---

## 方案二：Kubernetes 部署（简要）

适合大规模应用，需要配置：
- Kubernetes 集群（GKE、EKS、AKS 或自建）
- Helm Charts
- Ingress Controller
- Persistent Volumes
- ConfigMaps 和 Secrets

---

## 常见问题排查

### 服务无法启动

```bash
# 检查日志
docker-compose -f docker-compose.prod.yml logs backend

# 检查网络
docker network ls
docker network inspect genio-network

# 检查端口占用
sudo netstat -tlnp | grep 8200
```

### 数据库连接失败

```bash
# 进入容器测试连接
docker exec -it genio-backend sh
ping mysql
nc -zv mysql 3306
```

### AI Brain gRPC 连接失败

```bash
# 检查 AI Brain 健康状态
curl http://localhost:9090/health

# 测试 gRPC 连接
grpcurl -plaintext localhost:8080 list
```

---

## 总结

这个方案提供了完整的 Docker Compose 部署流程，包括：
- ✅ 所有服务容器化
- ✅ Nginx 反向代理和 SSL
- ✅ 数据库和缓存持久化
- ✅ 自动重启和健康检查
- ✅ 日志管理和备份策略
- ✅ 一键部署和回滚

根据实际需求，可以选择 Docker Compose（方案一）或 Kubernetes（方案二）。对于初期部署，推荐使用 Docker Compose 方案，简单高效。
