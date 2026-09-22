# 三项目分别部署方案

## 方案概述

将三个项目分别部署到不同的服务器或环境，可以实现更好的资源隔离、独立扩展和故障隔离。

---

## 架构设计

### 方案 A：三台服务器分别部署（推荐）

```
Internet
    ↓
[CDN/Cloudflare]
    ↓
[负载均衡器]
    ├─→ Server 1: Frontend (Nuxt.js)     - 日本服务器 A
    ├─→ Server 2: Backend API            - 日本服务器 B
    │             + MySQL
    │             + Redis
    └─→ Server 3: AI Brain (gRPC)        - 日本服务器 C
                  + MySQL
                  + Redis
```

### 方案 B：前端 CDN + 后端服务器（性价比高）

```
Internet
    ↓
[CDN - Vercel/Netlify] ← 前端静态资源
    ↓
[日本服务器 A]
    ├─ Backend API :8200
    ├─ MySQL
    └─ Redis

[日本服务器 B]
    ├─ AI Brain :8080
    ├─ MySQL
    └─ Redis
```

### 方案 C：共享数据库（推荐中小型项目）

```
[日本服务器 A - 前端]
    └─ Nuxt.js :3000

[日本服务器 B - 后端]
    └─ Backend API :8200

[日本服务器 C - AI 中台]
    └─ AI Brain :8080

[日本服务器 D - 数据层]
    ├─ MySQL :3306
    └─ Redis :6379
```

---

## 详细部署方案

## 方案 A：三台服务器完全独立部署

### 服务器规划

| 服务器 | 用途 | 配置建议 | 端口 |
|--------|------|----------|------|
| Server A | Frontend | 1-2核 2GB | 3000, 80, 443 |
| Server B | Backend + MySQL + Redis | 4核 8GB | 8200, 3306, 6379 |
| Server C | AI Brain + MySQL + Redis | 4核 8GB | 8080, 9090, 3306, 6379 |

### 1. Server A - 前端服务器部署

#### 1.1 目录结构
```
/opt/frontend/
├── geinoAI_web/
├── docker-compose.yml
└── nginx/
    └── conf.d/
```

#### 1.2 Docker Compose 配置

创建 `/opt/frontend/docker-compose.yml`：

```yaml
version: '3.8'

services:
  frontend:
    build:
      context: ./geinoAI_web
      dockerfile: Dockerfile
    container_name: genio-frontend
    restart: unless-stopped
    ports:
      - "3000:3000"
    environment:
      - NODE_ENV=production
      - NUXT_PUBLIC_API_BASE_URL=https://api.yourdomain.com
    networks:
      - frontend-network

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
    networks:
      - frontend-network
    depends_on:
      - frontend

networks:
  frontend-network:
    driver: bridge
```

#### 1.3 环境配置

`/opt/frontend/geinoAI_web/.env`：

```bash
# API 地址指向 Server B
NUXT_PUBLIC_API_BASE_URL=https://api.yourdomain.com
```

#### 1.4 Nginx 配置

```nginx
server {
    listen 80;
    server_name yourdomain.com www.yourdomain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name yourdomain.com www.yourdomain.com;

    ssl_certificate /etc/nginx/ssl/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/privkey.pem;

    location / {
        proxy_pass http://frontend:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }
}
```

#### 1.5 部署命令

```bash
cd /opt/frontend
git clone <geinoAI_web-repo> geinoAI_web
docker-compose up -d
```

---

### 2. Server B - 后端 API 服务器部署

#### 2.1 目录结构
```
/opt/backend/
├── genioAI_server/
├── docker-compose.yml
└── nginx/
```

#### 2.2 Docker Compose 配置

创建 `/opt/backend/docker-compose.yml`：

```yaml
version: '3.8'

networks:
  backend-network:
    driver: bridge

volumes:
  mysql_data:
  redis_data:

services:
  mysql:
    image: mysql:8.0
    container_name: backend-mysql
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
    networks:
      - backend-network
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: backend-redis
    restart: unless-stopped
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    command: redis-server --appendonly yes
    networks:
      - backend-network
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  backend:
    build:
      context: ./genioAI_server
      dockerfile: Dockerfile
    container_name: genio-backend
    restart: unless-stopped
    ports:
      - "8200:8200"
    environment:
      - MODE=PROD
      - HTTP_PORT=8200
      # 指向 Server C 的 AI Brain
      - AIBRAIN_ADDR=ai-brain.yourdomain.com:8080
      # 或使用 IP 地址
      # - AIBRAIN_ADDR=<Server-C-IP>:8080
      - MYSQL_ADDR=mysql
      - MYSQL_PORT=3306
      - MYSQL_USER=${MYSQL_USER}
      - MYSQL_PASSWORD=${MYSQL_PASSWORD}
      - MYSQL_DB=${MYSQL_DATABASE}
      - REDIS_ADDR=redis:6379
    networks:
      - backend-network
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy

  nginx:
    image: nginx:alpine
    container_name: backend-nginx
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/conf.d:/etc/nginx/conf.d:ro
      - ./nginx/ssl:/etc/nginx/ssl:ro
    networks:
      - backend-network
    depends_on:
      - backend
```

#### 2.3 环境配置

`/opt/backend/.env`：

```bash
# MySQL
MYSQL_ROOT_PASSWORD=your_strong_password
MYSQL_DATABASE=genio_db
MYSQL_USER=genio_user
MYSQL_PASSWORD=your_db_password

# AI Brain 地址（Server C）
AIBRAIN_HOST=ai-brain.yourdomain.com
AIBRAIN_PORT=8080
```

#### 2.4 Nginx 配置

```nginx
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

    client_max_body_size 50M;

    location / {
        proxy_pass http://backend:8200;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

        # 超时设置
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 300s;
    }
}
```

#### 2.5 防火墙配置

```bash
# 只允许 Server A (前端) 访问
sudo ufw allow from <Server-A-IP> to any port 8200
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

---

### 3. Server C - AI Brain 服务器部署

#### 3.1 目录结构
```
/opt/aibrain/
├── ai-brain/
├── docker-compose.yml
└── nginx/
```

#### 3.2 Docker Compose 配置

创建 `/opt/aibrain/docker-compose.yml`：

```yaml
version: '3.8'

networks:
  aibrain-network:
    driver: bridge

volumes:
  mysql_data:
  redis_data:

services:
  mysql:
    image: mysql:8.0
    container_name: aibrain-mysql
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
    networks:
      - aibrain-network
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: aibrain-redis
    restart: unless-stopped
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    command: redis-server --appendonly yes
    networks:
      - aibrain-network
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  ai-brain:
    build:
      context: ./ai-brain
      dockerfile: Dockerfile
    container_name: genio-ai-brain
    restart: unless-stopped
    ports:
      - "8080:8080"   # gRPC
      - "9090:9090"   # Metrics
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
      - aibrain-network
    depends_on:
      mysql:
        condition: service_healthy
      redis:
        condition: service_healthy

  # 可选：Nginx for gRPC
  nginx:
    image: nginx:alpine
    container_name: aibrain-nginx
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/conf.d:/etc/nginx/conf.d:ro
      - ./nginx/ssl:/etc/nginx/ssl:ro
    networks:
      - aibrain-network
    depends_on:
      - ai-brain
```

#### 3.3 环境配置

`/opt/aibrain/.env`：

```bash
# MySQL
MYSQL_ROOT_PASSWORD=your_strong_password
MYSQL_DATABASE=ai_brain_db
MYSQL_USER=aibrain_user
MYSQL_PASSWORD=your_db_password

# AI Providers
MINIMAX_APP_ID=your_app_id
MINIMAX_SECRET=your_secret
KLING_API_KEY=your_key
SILICONFLOW_API_KEY=your_key
AZURE_OPENAI_API_KEY=your_key
AZURE_OPENAI_ENDPOINT=your_endpoint
GEMINI_CREDENTIAL_JSON=your_credentials
```

#### 3.4 Nginx 配置（gRPC 代理）

```nginx
upstream grpc_backend {
    server ai-brain:8080;
}

server {
    listen 80 http2;
    server_name ai-brain.yourdomain.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name ai-brain.yourdomain.com;

    ssl_certificate /etc/nginx/ssl/fullchain.pem;
    ssl_certificate_key /etc/nginx/ssl/privkey.pem;

    # gRPC 配置
    location / {
        grpc_pass grpc://grpc_backend;
        grpc_set_header Host $host;
        grpc_set_header X-Real-IP $remote_addr;

        # 超时设置（AI 生图可能需要较长时间）
        grpc_read_timeout 600s;
        grpc_send_timeout 600s;
    }

    # Health check
    location /health {
        proxy_pass http://ai-brain:9090/health;
    }
}
```

#### 3.5 防火墙配置

```bash
# 只允许 Server B (后端) 访问 gRPC
sudo ufw allow from <Server-B-IP> to any port 8080

# 允许 HTTPS 访问（用于健康检查）
sudo ufw allow 443/tcp

# Metrics 端口（可选，仅监控系统访问）
sudo ufw allow from <Monitoring-Server-IP> to any port 9090

sudo ufw enable
```

---

## 网络通信配置

### 1. 服务间通信方式

#### 方式 A：通过域名（推荐）

```bash
# Server B 的 Backend 连接 Server C 的 AI Brain
AIBRAIN_ADDR=ai-brain.yourdomain.com:8080

# Server A 的 Frontend 连接 Server B 的 Backend
NUXT_PUBLIC_API_BASE_URL=https://api.yourdomain.com
```

**优点**：
- 易于迁移
- 支持负载均衡
- SSL 加密通信

#### 方式 B：通过内网 IP（性能最优）

如果服务器在同一内网：

```bash
# Server B Backend 配置
AIBRAIN_ADDR=10.0.1.3:8080  # Server C 内网 IP

# Server A Frontend 配置
NUXT_PUBLIC_API_BASE_URL=http://10.0.1.2:8200  # Server B 内网 IP
```

**优点**：
- 性能最优
- 无需域名解析
- 节省流量费用

#### 方式 C：VPN 组网（最安全）

使用 WireGuard 或 Tailscale 建立虚拟专用网络：

```bash
# 安装 Tailscale
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up

# 各服务器配置
AIBRAIN_ADDR=100.x.x.x:8080  # Tailscale 分配的 IP
```

**优点**：
- 端到端加密
- 穿透 NAT
- 跨云服务商

---

### 2. DNS 配置

在域名服务商配置 A 记录：

```
yourdomain.com              A    <Server-A-IP>  # 前端
www.yourdomain.com          A    <Server-A-IP>
api.yourdomain.com          A    <Server-B-IP>  # 后端 API
ai-brain.yourdomain.com     A    <Server-C-IP>  # AI Brain
```

---

## 完整部署流程

### Step 1: 准备三台服务器

```bash
# 在每台服务器上执行
sudo apt update && sudo apt upgrade -y
sudo apt install docker.io docker-compose nginx git ufw -y
sudo systemctl start docker
sudo systemctl enable docker
```

### Step 2: 部署 Server C (AI Brain)

```bash
# SSH 到 Server C
ssh user@<Server-C-IP>

cd /opt
sudo mkdir aibrain && sudo chown $USER:$USER aibrain
cd aibrain

# 克隆代码
git clone <ai-brain-repo> ai-brain

# 配置环境变量
vi .env  # 填写配置

# 创建 docker-compose.yml（使用上面的配置）
vi docker-compose.yml

# 配置 SSL 证书
sudo certbot certonly --standalone -d ai-brain.yourdomain.com
mkdir -p nginx/ssl
sudo cp /etc/letsencrypt/live/ai-brain.yourdomain.com/fullchain.pem nginx/ssl/
sudo cp /etc/letsencrypt/live/ai-brain.yourdomain.com/privkey.pem nginx/ssl/

# 部署
docker-compose up -d

# 验证
docker-compose ps
docker-compose logs -f ai-brain
```

### Step 3: 部署 Server B (Backend)

```bash
# SSH 到 Server B
ssh user@<Server-B-IP>

cd /opt
sudo mkdir backend && sudo chown $USER:$USER backend
cd backend

# 克隆代码
git clone <genioAI_server-repo> genioAI_server

# 配置环境变量（注意 AIBRAIN_ADDR）
vi .env
# AIBRAIN_ADDR=ai-brain.yourdomain.com:8080

# 创建 docker-compose.yml
vi docker-compose.yml

# 配置 SSL
sudo certbot certonly --standalone -d api.yourdomain.com
mkdir -p nginx/ssl
sudo cp /etc/letsencrypt/live/api.yourdomain.com/fullchain.pem nginx/ssl/
sudo cp /etc/letsencrypt/live/api.yourdomain.com/privkey.pem nginx/ssl/

# 部署
docker-compose up -d

# 验证
curl https://api.yourdomain.com/health
```

### Step 4: 部署 Server A (Frontend)

```bash
# SSH 到 Server A
ssh user@<Server-A-IP>

cd /opt
sudo mkdir frontend && sudo chown $USER:$USER frontend
cd frontend

# 克隆代码
git clone <geinoAI_web-repo> geinoAI_web

# 配置环境变量
vi geinoAI_web/.env
# NUXT_PUBLIC_API_BASE_URL=https://api.yourdomain.com

# 创建 docker-compose.yml
vi docker-compose.yml

# 配置 SSL
sudo certbot certonly --standalone -d yourdomain.com -d www.yourdomain.com
mkdir -p nginx/ssl
sudo cp /etc/letsencrypt/live/yourdomain.com/fullchain.pem nginx/ssl/
sudo cp /etc/letsencrypt/live/yourdomain.com/privkey.pem nginx/ssl/

# 部署
docker-compose up -d

# 验证
curl https://yourdomain.com
```

---

## 分别更新流程

### 更新前端（Server A）

```bash
ssh user@<Server-A-IP>
cd /opt/frontend/geinoAI_web
git pull origin main
cd /opt/frontend
docker-compose build frontend
docker-compose up -d --no-deps frontend
```

### 更新后端（Server B）

```bash
ssh user@<Server-B-IP>
cd /opt/backend/genioAI_server
git pull origin main
cd /opt/backend
docker-compose build backend
docker-compose up -d --no-deps backend
```

### 更新 AI Brain（Server C）

```bash
ssh user@<Server-C-IP>
cd /opt/aibrain/ai-brain
git pull origin main
cd /opt/aibrain
docker-compose build ai-brain
docker-compose up -d --no-deps ai-brain
```

---

## 方案对比

### 分别部署 vs 统一部署

| 对比项 | 分别部署 | 统一部署 |
|--------|----------|----------|
| **资源利用** | 灵活，按需分配 | 共享资源 |
| **扩展性** | 高，独立扩展 | 受限 |
| **故障隔离** | 优秀，单点故障影响小 | 差，单点故障影响全局 |
| **成本** | 较高（多台服务器） | 较低（单台服务器） |
| **运维复杂度** | 较高 | 较低 |
| **性能** | 优秀，独立资源 | 资源竞争 |
| **部署速度** | 较慢 | 快速 |

### 推荐场景

**选择分别部署**：
- ✅ 流量较大，需要独立扩展
- ✅ 对可用性要求高
- ✅ 预算充足
- ✅ 需要针对不同服务优化

**选择统一部署**：
- ✅ 初期项目，流量不大
- ✅ 预算有限
- ✅ 快速上线需求
- ✅ 运维资源有限

---

## 优化建议

### 1. 使用 CDN 加速前端

将前端部署到 Vercel/Netlify，只保留后端服务器：

```bash
# 在本地
cd geinoAI_web
vercel --prod

# 配置环境变量
vercel env add NUXT_PUBLIC_API_BASE_URL production
```

### 2. 数据库读写分离

```yaml
# Server B - Backend 使用主从数据库
environment:
  - MYSQL_MASTER_ADDR=<Server-B-IP>:3306
  - MYSQL_SLAVE_ADDR=<Server-B-Slave-IP>:3306
```

### 3. Redis 哨兵模式

部署 Redis Sentinel 实现高可用。

### 4. 负载均衡

使用 Nginx/HAProxy 实现多实例负载均衡。

---

## 监控和告警

### 统一监控方案

在独立监控服务器部署 Prometheus + Grafana：

```yaml
# 监控服务器 docker-compose.yml
services:
  prometheus:
    image: prom/prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml

  grafana:
    image: grafana/grafana
    ports:
      - "3000:3000"
```

配置抓取各服务器指标：

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'backend'
    static_configs:
      - targets: ['<Server-B-IP>:9090']

  - job_name: 'ai-brain'
    static_configs:
      - targets: ['<Server-C-IP>:9090']
```

---

## 总结

**推荐方案**：

1. **小型项目**：统一部署（1台服务器）
2. **中型项目**：Server B + Server C 分离（2台服务器）
3. **大型项目**：完全分离（3台服务器） + CDN

**关键配置**：
- 正确配置服务间通信地址
- 配置防火墙规则
- SSL 证书管理
- 定期备份各服务器数据

所有配置文件和脚本都已提供，可以根据实际需求选择部署方案！
