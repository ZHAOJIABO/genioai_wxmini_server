# AI-Brain 中台部署指南

## 概述

本指南说明如何在同一台服务器上部署三个项目，并通过 gRPC 进行通信：

1. **前端 (geinoAI_web)** - Nuxt.js Web 应用
2. **后端 (genioAI_server)** - Go 业务 API 服务
3. **AI 中台 (ai-brain)** - Go AI 生图核心服务

## 架构图

```
Internet
    ↓
[Nginx (443/80)]
    ↓
    ├─→ Frontend (Nuxt.js) :3000
    └─→ Backend API (genioAI_server)
         ├─→ HTTP API :8200
         ├─→ gRPC :8181
         └─→ Metrics :9188
              ↓ gRPC 调用
         AI-Brain (ai-brain)
              ├─→ gRPC :8080
              ├─→ Metrics :9090
              ├─→ MySQL :3306 (ai-brain 独立)
              └─→ Redis :6379 (ai-brain 独立)

Backend 使用的资源:
    ├─→ MySQL :3307 (backend 独立)
    └─→ Redis :6380 (backend 独立)
```

## 服务依赖关系

```
用户请求
    ↓
前端 (geinoAI_web)
    ↓ HTTP API 调用
后端 (genioAI_server)
    ↓ gRPC 调用 (AIBrainServer.Addr)
AI 中台 (ai-brain)
    ↓
MySQL + Redis (ai-brain 独立实例)
```

## 关键配置说明

### 1. 后端连接 AI-Brain 配置

在 `genioAI_server/conf/server.yaml`:

```yaml
AIBrainServer:
  Addr: 127.0.0.1:8080  # 本地部署时使用
  # Addr: ai-brain:8080  # Docker 容器网络内使用
```

### 2. AI-Brain 配置

在 `ai-brain/.env`:

```bash
# gRPC 服务端口
GRPC_PORT=8080

# Metrics 端口
METRICS_PORT=9090

# 数据库配置（ai-brain 独立使用）
MYSQL_DSN=aibrain_user:123456@tcp(localhost:3306)/ai_brain?charset=utf8mb4&parseTime=True&loc=Local

# Redis 配置（ai-brain 独立使用）
REDIS_ADDR=localhost:6379
```

## 部署方案

### 方案一：Docker Compose 部署（推荐）

此方案将所有服务容器化，使用 Docker 网络进行内部通信。

#### 优点
- 环境隔离，避免端口冲突
- 服务发现简单（通过容器名访问）
- 易于管理和扩展
- 一键启动/停止所有服务

#### 部署步骤

##### 1. 目录结构

```
/opt/genio/
├── ai-brain/              # AI 中台项目
│   ├── .env
│   ├── docker-compose.yml
│   └── ...
├── genioAI_server/        # 后端项目
│   ├── conf/
│   │   └── server.prod.yaml
│   └── ...
├── geinoAI_web/          # 前端项目（如已部署）
│   └── ...
└── docker-compose.unified.yml  # 统一编排文件（可选）
```

##### 2. 修改 ai-brain 的 docker-compose.yml

编辑 `/opt/genio/ai-brain/docker-compose.yml`，确保服务加入共享网络：

```yaml
services:
  mysql:
    container_name: ai-brain-mysql
    ports:
      - "3306:3306"
    networks:
      - ai-brain-network
      - genio-shared  # 加入共享网络

  redis:
    container_name: ai-brain-redis
    ports:
      - "6379:6379"
    networks:
      - ai-brain-network
      - genio-shared

  ai-brain:
    container_name: ai-brain-core-app
    ports:
      - "8080:8080"  # gRPC 端口
      - "9090:9090"  # Metrics 端口
    networks:
      - ai-brain-network
      - genio-shared

networks:
  ai-brain-network:
    driver: bridge
  genio-shared:
    external: true
    name: genio-shared  # 与后端共享的网络
```

##### 3. 创建共享网络

```bash
# 创建共享网络（如果不存在）
docker network create genio-shared
```

##### 4. 修改后端配置

编辑 `genioAI_server/conf/server.prod.yaml`（生产环境配置）:

```yaml
AIBrainServer:
  Addr: ai-brain-core-app:8080  # 使用容器名访问
```

或者在 `docker-compose.yml` 中通过环境变量覆盖：

```yaml
services:
  backend:
    environment:
      - AIBRAIN_ADDR=ai-brain-core-app:8080
```

##### 5. 启动服务

```bash
# 1. 启动 AI-Brain（必须先启动）
cd /opt/genio/ai-brain
docker-compose up -d

# 等待 AI-Brain 启动完成（约 30-60 秒）
docker-compose logs -f ai-brain  # 查看日志

# 2. 启动后端服务
cd /opt/genio/genioAI_server
./deploy.sh  # 使用现有的部署脚本

# 3. 验证连接
docker exec genio-backend sh -c "nc -zv ai-brain-core-app 8080"
```

##### 6. 健康检查

```bash
# 检查 AI-Brain 健康状态
curl http://localhost:9090/health

# 检查后端是否能访问 AI-Brain
docker exec genio-backend sh -c "wget -O- http://ai-brain-core-app:9090/health"

# 查看 gRPC 连接日志
docker logs genio-backend | grep -i aibrain
```

---

### 方案二：直接部署（本地进程）

此方案将 ai-brain 直接运行在宿主机上，不使用 Docker。

#### 优点
- 资源占用更少
- 调试更方便
- 性能更优

#### 缺点
- 需要手动管理依赖（MySQL、Redis）
- 端口可能冲突
- 环境隔离性差

#### 部署步骤

##### 1. 安装依赖

```bash
# 安装 Go (如未安装)
wget https://go.dev/dl/go1.24.4.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.4.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 验证安装
go version
```

##### 2. 启动独立的 MySQL 和 Redis

由于后端已经占用了 3307 和 6380 端口，ai-brain 可以使用 Docker 的默认端口（3306 和 6379）：

```bash
cd /opt/genio/ai-brain

# 只启动数据库服务（不启动 ai-brain）
docker-compose up -d mysql redis

# 验证数据库服务
docker ps | grep ai-brain
```

##### 3. 配置 ai-brain

编辑 `/opt/genio/ai-brain/.env`:

```bash
# 连接到 Docker 容器的 MySQL 和 Redis
MYSQL_DSN=aibrain_user:123456@tcp(localhost:3306)/ai_brain?charset=utf8mb4&parseTime=True&loc=Local
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=

# gRPC 端口
GRPC_PORT=8080

# Metrics 端口
METRICS_PORT=9090

# AI Provider Keys（从后端复制）
MINIMAX_APP_ID=your_minimax_app_id
MINIMAX_SECRET=your_minimax_secret
KLING_API_KEY=your_kling_api_key
KLING_APP_ID=your_kling_app_id
KLING_SECRET=your_kling_secret
SILICONFLOW_API_KEY=your_siliconflow_api_key
AZURE_OPENAI_API_KEY=your_azure_key
AZURE_OPENAI_ENDPOINT=your_azure_endpoint
GEMINI_CREDENTIAL_JSON=your_gemini_credentials
```

##### 4. 编译和启动 ai-brain

```bash
cd /opt/genio/ai-brain

# 编译
go build -o bin/ai-brain cmd/server/main.go

# 启动（前台运行，用于测试）
./bin/ai-brain

# 或者使用 systemd 后台运行（见下文）
```

##### 5. 配置 systemd 服务

创建 `/etc/systemd/system/ai-brain.service`:

```ini
[Unit]
Description=AI Brain Service
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
User=your_user
WorkingDirectory=/opt/genio/ai-brain
Environment="PATH=/usr/local/go/bin:/usr/bin:/bin"
ExecStart=/opt/genio/ai-brain/bin/ai-brain
Restart=always
RestartSec=10

# 日志配置
StandardOutput=journal
StandardError=journal
SyslogIdentifier=ai-brain

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
# 重载 systemd 配置
sudo systemctl daemon-reload

# 启动服务
sudo systemctl start ai-brain

# 设置开机自启
sudo systemctl enable ai-brain

# 查看状态
sudo systemctl status ai-brain

# 查看日志
sudo journalctl -u ai-brain -f
```

##### 6. 配置后端连接

编辑 `genioAI_server/conf/server.yaml`:

```yaml
AIBrainServer:
  Addr: 127.0.0.1:8080  # 本地进程使用 localhost
```

##### 7. 重启后端服务

```bash
cd /opt/genio/genioAI_server
./deploy.sh --restart
```

---

## 部署脚本

### 方案一：Docker 一键部署脚本

创建 `/opt/genio/deploy-all.sh`:

```bash
#!/bin/bash

set -e

echo "=== 部署 GenioAI 全栈服务 ==="

# 1. 创建共享网络
if ! docker network ls | grep -q genio-shared; then
    echo "创建共享网络..."
    docker network create genio-shared
fi

# 2. 部署 AI-Brain
echo "部署 AI-Brain..."
cd /opt/genio/ai-brain
docker-compose up -d

# 等待 AI-Brain 启动
echo "等待 AI-Brain 启动..."
sleep 30

# 检查 AI-Brain 健康状态
until curl -f http://localhost:9090/health &>/dev/null; do
    echo "等待 AI-Brain 就绪..."
    sleep 5
done
echo "AI-Brain 已就绪"

# 3. 部署后端
echo "部署后端服务..."
cd /opt/genio/genioAI_server
./deploy.sh

echo "=== 部署完成 ==="
echo "AI-Brain gRPC: localhost:8080"
echo "AI-Brain Metrics: http://localhost:9090"
echo "Backend API: http://localhost:8200"
echo "Backend Metrics: http://localhost:9188"
```

### 方案二：本地进程一键部署脚本

创建 `/opt/genio/deploy-ai-brain.sh`:

```bash
#!/bin/bash

set -e

echo "=== 部署 AI-Brain (本地进程) ==="

cd /opt/genio/ai-brain

# 1. 启动数据库服务
echo "启动数据库服务..."
docker-compose up -d mysql redis

# 等待数据库就绪
sleep 10

# 2. 编译 ai-brain
echo "编译 ai-brain..."
go build -o bin/ai-brain cmd/server/main.go

# 3. 重启服务
echo "重启 ai-brain 服务..."
sudo systemctl restart ai-brain

# 4. 检查状态
sleep 5
sudo systemctl status ai-brain

# 5. 健康检查
echo "健康检查..."
sleep 5
curl http://localhost:9090/health

echo "=== AI-Brain 部署完成 ==="
```

---

## 验证部署

### 1. 检查 AI-Brain 服务

```bash
# 健康检查
curl http://localhost:9090/health

# 查看 Metrics
curl http://localhost:9090/metrics

# 查看容器状态（Docker 部署）
docker ps | grep ai-brain

# 查看日志（Docker 部署）
docker logs ai-brain-core-app -f

# 查看日志（systemd 部署）
sudo journalctl -u ai-brain -f
```

### 2. 检查后端连接

```bash
# 查看后端日志中的 gRPC 连接信息
docker logs genio-backend | grep -i "aibrain\|grpc"

# 从后端容器测试连接（Docker 部署）
docker exec genio-backend sh -c "nc -zv ai-brain-core-app 8080"

# 从后端容器测试连接（本地部署）
docker exec genio-backend sh -c "nc -zv host.docker.internal 8080"
```

### 3. 测试 gRPC 调用

可以使用 `grpcurl` 工具测试：

```bash
# 安装 grpcurl
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# 列出 AI-Brain 的 gRPC 服务
grpcurl -plaintext localhost:8080 list

# 健康检查
grpcurl -plaintext localhost:8080 aibrain.AIBrainService/HealthCheck
```

---

## 监控和维护

### 查看服务状态

```bash
# Docker 部署
docker ps | grep -E "ai-brain|genio-backend"
docker stats

# 本地部署
sudo systemctl status ai-brain
ps aux | grep ai-brain
```

### 查看日志

```bash
# AI-Brain 日志（Docker）
docker logs -f ai-brain-core-app

# AI-Brain 日志（systemd）
sudo journalctl -u ai-brain -f

# 后端日志
docker logs -f genio-backend
```

### 重启服务

```bash
# 重启 AI-Brain（Docker）
cd /opt/genio/ai-brain
docker-compose restart ai-brain

# 重启 AI-Brain（systemd）
sudo systemctl restart ai-brain

# 重启后端
cd /opt/genio/genioAI_server
./deploy.sh --restart
```

### Prometheus 监控

AI-Brain 已配置 Prometheus 监控：

```bash
# 访问 Prometheus UI
http://localhost:9091

# 访问 Grafana
http://localhost:3000
# 默认账号：admin / admin123

# 查询 AI-Brain Metrics
curl http://localhost:9090/metrics
```

---

## 故障排查

### AI-Brain 无法启动

```bash
# 检查端口占用
sudo netstat -tlnp | grep -E "8080|9090"

# 检查数据库连接
docker exec ai-brain-mysql mysql -u aibrain_user -p123456 -e "SELECT 1"

# 检查 Redis 连接
docker exec ai-brain-redis redis-cli ping

# 查看详细日志
docker logs ai-brain-core-app --tail=100
```

### 后端无法连接 AI-Brain

```bash
# 检查网络连接（Docker 部署）
docker network inspect genio-shared

# 测试容器间通信
docker exec genio-backend ping ai-brain-core-app

# 检查防火墙
sudo ufw status
sudo ufw allow 8080/tcp

# 检查后端配置
docker exec genio-backend cat /app/conf/server.prod.yaml | grep AIBrainServer
```

### gRPC 调用失败

```bash
# 查看后端日志中的错误
docker logs genio-backend | grep -i error

# 使用 grpcurl 测试
grpcurl -plaintext localhost:8080 aibrain.AIBrainService/HealthCheck

# 检查 AI-Brain gRPC 端口
docker exec ai-brain-core-app netstat -tlnp | grep 8080
```

---

## 性能优化建议

### 1. 资源限制（Docker）

在 `docker-compose.yml` 中添加资源限制：

```yaml
services:
  ai-brain:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 4G
        reservations:
          cpus: '1'
          memory: 2G
```

### 2. 连接池配置

在 ai-brain `.env` 中调整：

```bash
# Redis 连接池
REDIS_POOL_SIZE=20
REDIS_MIN_IDLE_CONNS=5

# 任务处理器数量
TASK_WORKER_COUNT=5
```

### 3. MySQL 优化

```bash
# 进入 MySQL 容器
docker exec -it ai-brain-mysql mysql -u root -p123456

# 优化配置
SET GLOBAL max_connections = 200;
SET GLOBAL innodb_buffer_pool_size = 2147483648;  # 2GB
```

---

## 安全建议

1. **修改默认密码**：更改 MySQL、Redis 的默认密码
2. **配置防火墙**：只开放必要的端口（80、443）
3. **使用 HTTPS**：配置 SSL 证书
4. **定期备份**：配置数据库自动备份
5. **日志轮转**：配置日志自动清理
6. **资源监控**：设置 Prometheus 告警

---

## 总结

### 推荐部署方案

- **生产环境**：推荐使用 **方案一（Docker Compose）**，易于管理和扩展
- **开发环境**：可使用 **方案二（本地进程）**，便于调试

### 关键配置项

| 服务 | 配置项 | Docker 值 | 本地值 |
|-----|--------|----------|--------|
| AI-Brain gRPC | GRPC_PORT | 8080 | 8080 |
| AI-Brain Metrics | METRICS_PORT | 9090 | 9090 |
| Backend → AI-Brain | AIBrainServer.Addr | `ai-brain-core-app:8080` | `127.0.0.1:8080` |
| AI-Brain MySQL | 端口 | 3306 | 3306 |
| AI-Brain Redis | 端口 | 6379 | 6379 |

### 快速命令

```bash
# 部署所有服务（Docker）
cd /opt/genio && ./deploy-all.sh

# 查看所有服务状态
docker ps | grep -E "ai-brain|genio"

# 查看所有服务日志
docker logs -f ai-brain-core-app
docker logs -f genio-backend

# 重启所有服务
cd /opt/genio/ai-brain && docker-compose restart
cd /opt/genio/genioAI_server && ./deploy.sh --restart
```
