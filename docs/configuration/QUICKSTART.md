# 快速部署指南

## 📦 已创建的文件

你的项目中已经包含以下部署文件：

```
genioAI_server/
├── docker-compose.yml           ✅ Docker 编排配置
├── Dockerfile                   ✅ 容器构建文件
├── deploy.sh                    ✅ 一键部署脚本
├── .env.example                 ✅ 环境变量示例
├── conf/
│   └── server.prod.yaml        ✅ 生产环境配置
└── ENV_CONFIG_GUIDE.md          ✅ 配置指南
```

---

## 🚀 快速开始（3步部署）

### Step 1: 创建环境变量文件

```bash
# 在项目根目录创建 .env 文件
cd /Users/zhaojiabo/Documents/trae_projects/genioAI_server

# 复制示例文件
cp .env.example .env

# 编辑配置（重要：必须修改数据库密码！）
vi .env
```

**最小必需配置**（修改这些值）：

```bash
# 数据库密码 - 必须修改！
BACKEND_MYSQL_ROOT_PASSWORD=改成你的强密码123!
BACKEND_MYSQL_DATABASE=genio_backend_db
BACKEND_MYSQL_USER=genio_backend_user
BACKEND_MYSQL_PASSWORD=改成你的强密码456!
BACKEND_REDIS_PASSWORD=改成你的强密码789!
```

其他配置（Azure、豆包、R2 等）已经从你的 `conf/server.yaml` 中提取，可以直接使用。

### Step 2: 给部署脚本执行权限

```bash
chmod +x deploy.sh
```

### Step 3: 运行部署脚本

```bash
./deploy.sh
```

部署脚本会自动：
1. ✅ 检查配置文件
2. ✅ 创建 Docker 网络
3. ✅ 启动 MySQL
4. ✅ 等待 MySQL 就绪
5. ✅ 启动 Redis
6. ✅ 启动后端服务

---

## ✅ 验证部署

### 1. 查看服务状态

```bash
docker-compose ps
```

应该看到 3 个服务都是 `Up` 状态：
```
NAME                    STATUS
genio-backend           Up (healthy)
genio-backend-mysql     Up (healthy)
genio-backend-redis     Up (healthy)
```

### 2. 查看日志

```bash
# 查看所有日志
docker-compose logs -f

# 只查看后端日志
docker-compose logs -f backend

# 最近 100 行日志
docker-compose logs --tail=100 backend
```

### 3. 测试 API

```bash
# 健康检查
curl http://localhost:8200/health

# Metrics
curl http://localhost:9188/metrics
```

### 4. 测试数据库连接

```bash
# MySQL
docker exec -it genio-backend-mysql mysql -u genio_backend_user -p

# Redis
docker exec -it genio-backend-redis redis-cli
```

---

## 🔧 常用命令

### 启动服务

```bash
docker-compose up -d
```

### 停止服务

```bash
docker-compose down
```

### 重启服务

```bash
# 重启所有
docker-compose restart

# 只重启后端
docker-compose restart backend
```

### 查看日志

```bash
# 实时日志
docker-compose logs -f backend

# 最近 N 行
docker-compose logs --tail=50 backend
```

### 进入容器

```bash
# 进入后端容器
docker exec -it genio-backend sh

# 进入 MySQL
docker exec -it genio-backend-mysql mysql -u root -p

# 进入 Redis
docker exec -it genio-backend-redis redis-cli
```

---

## 📊 服务端口

| 服务 | 端口 | 说明 |
|------|------|------|
| HTTP API | 8200 | RESTful API |
| gRPC | 8181 | gRPC 服务 |
| Metrics | 9188 | Prometheus 指标 |
| MySQL | 3307 | 数据库（外部访问） |
| Redis | 6380 | 缓存（外部访问） |

---

## ⚠️ 注意事项

### 1. 数据库密码

**必须修改** `.env` 中的默认密码：
- `BACKEND_MYSQL_ROOT_PASSWORD`
- `BACKEND_MYSQL_PASSWORD`
- `BACKEND_REDIS_PASSWORD`

### 2. AI Brain 连接

目前 `AIBrainServer.Addr` 为空，因为 ai-brain 还未部署。

**部署 ai-brain 后**，修改 `conf/server.prod.yaml`：

```yaml
AIBrainServer:
  Addr: genio-ai-brain:8080  # ai-brain 容器名
```

然后重启服务：

```bash
docker-compose restart backend
```

### 3. 文件权限

```bash
# 限制 .env 文件权限
chmod 600 .env

# 确认权限
ls -la .env
# 应显示: -rw------- 1 user user
```

### 4. 不要提交敏感文件

以下文件已添加到 `.gitignore`：
- ✅ `.env`
- ✅ `conf/server.prod.yaml`
- ✅ `docker-compose.yml`
- ✅ `Dockerfile`

---

## 🔄 更新代码

```bash
# 1. 拉取最新代码
git pull origin main

# 2. 重新构建
docker-compose build backend

# 3. 重启服务
docker-compose up -d --no-deps backend

# 4. 查看日志
docker-compose logs -f backend
```

---

## 🐛 故障排查

### 服务无法启动

```bash
# 查看详细日志
docker-compose logs backend

# 检查配置
docker-compose config

# 查看容器状态
docker ps -a
```

### 数据库连接失败

```bash
# 检查 MySQL 状态
docker-compose ps backend-mysql

# 测试连接
docker exec genio-backend-mysql mysqladmin ping -u root -p
```

### 端口被占用

```bash
# 查看端口占用
sudo lsof -i :8200
sudo lsof -i :3307

# 修改端口映射（docker-compose.yml）
ports:
  - "8201:8200"  # 改为其他端口
```

---

## 📚 更多文档

- 📖 **详细部署指南**: `BACKEND_DEPLOYMENT_GUIDE.md`
- 🔧 **环境配置指南**: `ENV_CONFIG_GUIDE.md`
- 🔄 **更新指南**: `UPDATE_GUIDE.md`

---

## 🆘 需要帮助？

如果遇到问题：

1. 查看日志：`docker-compose logs -f backend`
2. 检查配置：`docker-compose config`
3. 参考故障排查章节
4. 查看详细文档

---

## ✨ 下一步

部署成功后，可以继续：

1. ✅ **genioAI_server** - 已部署
2. ⏳ **ai-brain** - AI 中台（下一步）
3. ⏳ **geinoAI_web** - 前端（最后）

每个服务独立部署，互不影响！
