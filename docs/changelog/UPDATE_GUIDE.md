# 线上更新指南

## 更新流程概览

本文档详细说明如何更新线上的三个项目：genioAI_server、ai-brain、geinoAI_web

---

## 方法一：手动更新（推荐用于生产环境）

### 1. 标准更新流程

```bash
#!/bin/bash
# 完整更新脚本

cd /opt/genio

# 1. 备份当前数据
echo "=== 备份数据库 ==="
DATE=$(date +%Y%m%d_%H%M%S)
docker exec genio-mysql mysqldump -u root -p${MYSQL_ROOT_PASSWORD} \
  --all-databases > backups/mysql_before_update_$DATE.sql
gzip backups/mysql_before_update_$DATE.sql

# 2. 拉取最新代码
echo "=== 拉取最新代码 ==="
cd genioAI_server && git pull origin main && cd ..
cd ai-brain && git pull origin main && cd ..
cd geinoAI_web && git pull origin main && cd ..

# 3. 查看变更
echo "=== 代码变更记录 ==="
cd genioAI_server && git log -5 --oneline && cd ..

# 4. 重新构建镜像
echo "=== 构建 Docker 镜像 ==="
docker-compose -f docker-compose.prod.yml build

# 5. 滚动更新服务（零停机）
echo "=== 更新服务 ==="
docker-compose -f docker-compose.prod.yml up -d --no-deps --build backend
sleep 5
docker-compose -f docker-compose.prod.yml up -d --no-deps --build ai-brain
sleep 5
docker-compose -f docker-compose.prod.yml up -d --no-deps --build frontend

# 6. 检查服务状态
echo "=== 检查服务状态 ==="
docker-compose -f docker-compose.prod.yml ps

# 7. 查看日志
echo "=== 查看启动日志 ==="
docker-compose -f docker-compose.prod.yml logs --tail=50 backend
docker-compose -f docker-compose.prod.yml logs --tail=50 ai-brain
docker-compose -f docker-compose.prod.yml logs --tail=50 frontend

echo "=== 更新完成 ==="
```

### 2. 分步更新流程（更安全）

#### 2.1 更新单个服务（以 backend 为例）

```bash
cd /opt/genio

# 1. 拉取最新代码
cd genioAI_server
git fetch origin
git log HEAD..origin/main  # 查看即将更新的提交
git pull origin main
cd ..

# 2. 构建新镜像
docker-compose -f docker-compose.prod.yml build backend

# 3. 创建备份标签
docker tag genio-backend:latest genio-backend:backup-$(date +%Y%m%d-%H%M%S)

# 4. 停止旧容器并启动新容器
docker-compose -f docker-compose.prod.yml up -d --no-deps backend

# 5. 等待服务启动
sleep 10

# 6. 检查服务健康状态
docker-compose -f docker-compose.prod.yml ps backend
docker-compose -f docker-compose.prod.yml logs --tail=100 backend

# 7. 测试服务
curl -I http://localhost:8200/health  # 根据实际健康检查接口调整
```

#### 2.2 更新 AI Brain 服务

```bash
cd /opt/genio/ai-brain
git pull origin main
cd /opt/genio

# 构建并更新
docker-compose -f docker-compose.prod.yml build ai-brain
docker-compose -f docker-compose.prod.yml up -d --no-deps ai-brain

# 检查 gRPC 服务
docker-compose -f docker-compose.prod.yml logs --tail=100 ai-brain
# 测试 gRPC 连接
grpcurl -plaintext localhost:8080 list
```

#### 2.3 更新前端服务

```bash
cd /opt/genio/geinoAI_web
git pull origin main
cd /opt/genio

# 构建并更新
docker-compose -f docker-compose.prod.yml build frontend
docker-compose -f docker-compose.prod.yml up -d --no-deps frontend

# 检查前端服务
curl -I http://localhost:3000
docker-compose -f docker-compose.prod.yml logs --tail=100 frontend
```

### 3. 零停机更新（蓝绿部署）

适用于需要完全零停机的场景：

```bash
#!/bin/bash
# 蓝绿部署脚本

cd /opt/genio

# 1. 启动新版本服务（使用不同端口）
cat > docker-compose.green.yml <<EOF
version: '3.8'
services:
  backend-green:
    build: ./genioAI_server
    container_name: genio-backend-green
    ports:
      - "8210:8200"
    environment:
      - MODE=PROD
    networks:
      - genio-network

networks:
  genio-network:
    external: true
EOF

# 2. 启动新版本
docker-compose -f docker-compose.green.yml up -d

# 3. 等待服务就绪
sleep 20
curl -f http://localhost:8210/health || exit 1

# 4. 更新 Nginx 配置切换流量
# 修改 nginx/conf.d/genio.conf
# upstream backend {
#     server backend:8200;  # 旧版本
#     server backend-green:8200 backup;  # 新版本
# }

# 5. 重载 Nginx
docker-compose -f docker-compose.prod.yml exec nginx nginx -s reload

# 6. 监控一段时间后，停止旧版本
sleep 300
docker-compose -f docker-compose.prod.yml stop backend

# 7. 切换为主版本
docker-compose -f docker-compose.prod.yml rm -f backend
docker rename genio-backend-green genio-backend
```

---

## 方法二：使用更新脚本（快速更新）

创建 `/opt/genio/update.sh`：

```bash
#!/bin/bash

set -e

SERVICE=$1  # 要更新的服务：backend, ai-brain, frontend, all

if [ -z "$SERVICE" ]; then
    echo "用法: ./update.sh [backend|ai-brain|frontend|all]"
    exit 1
fi

cd /opt/genio

# 更新函数
update_service() {
    local service=$1
    local dir=$2

    echo "=== 更新 $service ==="

    # 1. 拉取代码
    echo "1. 拉取最新代码..."
    cd $dir
    git fetch origin
    git log HEAD..origin/main --oneline
    git pull origin main
    cd /opt/genio

    # 2. 构建镜像
    echo "2. 构建镜像..."
    docker-compose -f docker-compose.prod.yml build $service

    # 3. 备份当前镜像
    echo "3. 备份当前镜像..."
    docker tag genio-$service:latest genio-$service:backup-$(date +%Y%m%d-%H%M%S)

    # 4. 更新服务
    echo "4. 更新服务..."
    docker-compose -f docker-compose.prod.yml up -d --no-deps $service

    # 5. 等待启动
    echo "5. 等待服务启动..."
    sleep 10

    # 6. 检查状态
    echo "6. 检查服务状态..."
    docker-compose -f docker-compose.prod.yml ps $service
    docker-compose -f docker-compose.prod.yml logs --tail=50 $service

    echo "=== $service 更新完成 ==="
}

# 根据参数更新对应服务
case $SERVICE in
    backend)
        update_service "backend" "genioAI_server"
        ;;
    ai-brain)
        update_service "ai-brain" "ai-brain"
        ;;
    frontend)
        update_service "frontend" "geinoAI_web"
        ;;
    all)
        echo "=== 开始更新所有服务 ==="

        # 先备份数据库
        echo "备份数据库..."
        DATE=$(date +%Y%m%d_%H%M%S)
        docker exec genio-mysql mysqldump -u root -p${MYSQL_ROOT_PASSWORD} \
          --all-databases > backups/mysql_before_update_$DATE.sql
        gzip backups/mysql_before_update_$DATE.sql

        # 依次更新
        update_service "ai-brain" "ai-brain"
        update_service "backend" "genioAI_server"
        update_service "frontend" "geinoAI_web"

        echo "=== 所有服务更新完成 ==="
        ;;
    *)
        echo "未知服务: $SERVICE"
        echo "可选: backend, ai-brain, frontend, all"
        exit 1
        ;;
esac
```

使用方法：

```bash
# 给脚本执行权限
chmod +x /opt/genio/update.sh

# 更新单个服务
./update.sh backend      # 更新后端
./update.sh ai-brain     # 更新 AI 中台
./update.sh frontend     # 更新前端

# 更新所有服务
./update.sh all
```

---

## 方法三：Git Hook 自动更新（高级）

### 3.1 服务器端设置 Git Hook

在服务器上设置裸仓库：

```bash
# 创建裸仓库目录
mkdir -p /opt/git-repos
cd /opt/git-repos

# 为每个项目创建裸仓库
git init --bare genioAI_server.git
git init --bare ai-brain.git
git init --bare geinoAI_web.git

# 创建 post-receive hook
cat > genioAI_server.git/hooks/post-receive <<'EOF'
#!/bin/bash

TARGET="/opt/genio/genioAI_server"
GIT_DIR="/opt/git-repos/genioAI_server.git"

while read oldrev newrev ref
do
    if [[ $ref =~ .*/main$ ]]; then
        echo "检测到 main 分支推送，开始部署..."

        # 检出代码
        git --work-tree=$TARGET --git-dir=$GIT_DIR checkout -f main

        # 执行更新
        cd /opt/genio
        docker-compose -f docker-compose.prod.yml build backend
        docker-compose -f docker-compose.prod.yml up -d --no-deps backend

        echo "Backend 部署完成"
    fi
done
EOF

chmod +x genioAI_server.git/hooks/post-receive

# 对其他两个项目做类似配置
```

### 3.2 本地添加远程仓库

```bash
# 在本地开发机上添加生产服务器为远程仓库
cd /path/to/local/genioAI_server
git remote add production ssh://user@your-server-ip/opt/git-repos/genioAI_server.git

# 推送即自动部署
git push production main
```

---

## 方法四：CI/CD 自动化部署（推荐生产环境）

### 4.1 使用 GitHub Actions

创建 `.github/workflows/deploy.yml`：

```yaml
name: Deploy to Production

on:
  push:
    branches: [ main ]
    paths:
      - 'cmd/**'
      - 'internal/**'
      - 'pkg/**'
      - 'go.mod'
      - 'go.sum'
      - 'Dockerfile'

jobs:
  deploy:
    runs-on: ubuntu-latest

    steps:
    - name: Checkout code
      uses: actions/checkout@v3

    - name: Set up SSH
      uses: webfactory/ssh-agent@v0.7.0
      with:
        ssh-private-key: ${{ secrets.SSH_PRIVATE_KEY }}

    - name: Deploy to server
      run: |
        ssh -o StrictHostKeyChecking=no user@your-server-ip << 'ENDSSH'
          cd /opt/genio/genioAI_server
          git pull origin main
          cd /opt/genio
          docker-compose -f docker-compose.prod.yml build backend
          docker-compose -f docker-compose.prod.yml up -d --no-deps backend
          docker-compose -f docker-compose.prod.yml logs --tail=50 backend
        ENDSSH

    - name: Health check
      run: |
        sleep 10
        curl -f https://api.yourdomain.com/health || exit 1

    - name: Notify
      if: always()
      run: |
        # 发送通知（钉钉/企业微信/Slack等）
        echo "Deployment completed"
```

### 4.2 使用 GitLab CI/CD

创建 `.gitlab-ci.yml`：

```yaml
stages:
  - build
  - deploy

variables:
  DOCKER_HOST: ssh://user@your-server-ip

build:
  stage: build
  only:
    - main
  script:
    - echo "Building application..."
    - go build -o server ./cmd

deploy:
  stage: deploy
  only:
    - main
  before_script:
    - 'which ssh-agent || ( apt-get update -y && apt-get install openssh-client -y )'
    - eval $(ssh-agent -s)
    - echo "$SSH_PRIVATE_KEY" | tr -d '\r' | ssh-add -
    - mkdir -p ~/.ssh
    - chmod 700 ~/.ssh
  script:
    - ssh -o StrictHostKeyChecking=no user@your-server-ip "
        cd /opt/genio &&
        ./update.sh backend
      "
  after_script:
    - echo "Deployment completed"
```

---

## 数据库迁移更新

### 1. 有数据库变更时的更新流程

```bash
#!/bin/bash
# 带数据库迁移的更新流程

cd /opt/genio

# 1. 完整备份
echo "=== 备份数据库 ==="
DATE=$(date +%Y%m%d_%H%M%S)
docker exec genio-mysql mysqldump -u root -p${MYSQL_ROOT_PASSWORD} \
  --all-databases --single-transaction --quick --lock-tables=false \
  > backups/mysql_full_$DATE.sql
gzip backups/mysql_full_$DATE.sql

# 2. 拉取代码
cd genioAI_server
git pull origin main

# 3. 检查是否有新的迁移文件
if [ -d "migrations" ]; then
    echo "=== 发现数据库迁移文件 ==="
    ls -la migrations/
fi

# 4. 停止应用服务（保留数据库）
docker-compose -f docker-compose.prod.yml stop backend

# 5. 执行数据库迁移
# 方法 A: 使用 migrate 工具
docker run --rm -v $(pwd)/migrations:/migrations \
  --network genio-network \
  migrate/migrate:latest \
  -path=/migrations \
  -database "mysql://user:pass@mysql:3306/dbname" \
  up

# 方法 B: 使用应用内置的迁移
docker-compose -f docker-compose.prod.yml run --rm backend \
  ./server -migrate

# 6. 构建新镜像
cd /opt/genio
docker-compose -f docker-compose.prod.yml build backend

# 7. 启动新版本
docker-compose -f docker-compose.prod.yml up -d backend

# 8. 验证
sleep 10
docker-compose -f docker-compose.prod.yml logs --tail=100 backend
```

---

## 回滚流程

### 1. 快速回滚到备份镜像

```bash
#!/bin/bash
# 回滚脚本

SERVICE=$1  # backend, ai-brain, frontend

if [ -z "$SERVICE" ]; then
    echo "用法: ./rollback.sh [backend|ai-brain|frontend]"
    exit 1
fi

cd /opt/genio

# 1. 查看可用的备份镜像
echo "=== 可用的备份镜像 ==="
docker images | grep "genio-$SERVICE.*backup"

# 2. 选择要回滚的版本
read -p "输入要回滚的镜像标签 (例如: backup-20240203-143000): " BACKUP_TAG

if [ -z "$BACKUP_TAG" ]; then
    echo "未选择备份版本，退出"
    exit 1
fi

# 3. 停止当前服务
echo "=== 停止当前服务 ==="
docker-compose -f docker-compose.prod.yml stop $SERVICE

# 4. 标记当前版本为故障版本
docker tag genio-$SERVICE:latest genio-$SERVICE:failed-$(date +%Y%m%d-%H%M%S)

# 5. 回滚到备份版本
echo "=== 回滚到 $BACKUP_TAG ==="
docker tag genio-$SERVICE:$BACKUP_TAG genio-$SERVICE:latest

# 6. 启动服务
docker-compose -f docker-compose.prod.yml up -d $SERVICE

# 7. 检查状态
sleep 10
docker-compose -f docker-compose.prod.yml ps $SERVICE
docker-compose -f docker-compose.prod.yml logs --tail=50 $SERVICE

echo "=== 回滚完成 ==="
```

### 2. 回滚代码和数据库

```bash
#!/bin/bash
# 完整回滚（包括数据库）

BACKUP_DATE=$1  # 例如: 20240203_143000

if [ -z "$BACKUP_DATE" ]; then
    echo "用法: ./full_rollback.sh BACKUP_DATE"
    echo "例如: ./full_rollback.sh 20240203_143000"
    echo ""
    echo "可用的备份:"
    ls -lh /opt/genio/backups/
    exit 1
fi

cd /opt/genio

# 1. 停止所有应用服务
echo "=== 停止所有应用服务 ==="
docker-compose -f docker-compose.prod.yml stop backend ai-brain frontend

# 2. 恢复数据库
echo "=== 恢复数据库 ==="
BACKUP_FILE="backups/mysql_before_update_${BACKUP_DATE}.sql.gz"

if [ ! -f "$BACKUP_FILE" ]; then
    echo "备份文件不存在: $BACKUP_FILE"
    exit 1
fi

# 解压并恢复
gunzip -c $BACKUP_FILE | docker exec -i genio-mysql mysql -u root -p${MYSQL_ROOT_PASSWORD}

# 3. 回滚代码
echo "=== 回滚代码 ==="
cd genioAI_server
git reflog  # 查看历史
read -p "输入要回滚到的 commit hash: " COMMIT_HASH
git reset --hard $COMMIT_HASH
cd /opt/genio

# 4. 重新构建
docker-compose -f docker-compose.prod.yml build backend

# 5. 启动服务
docker-compose -f docker-compose.prod.yml up -d

echo "=== 完整回滚完成 ==="
```

---

## 监控更新状态

### 1. 健康检查脚本

创建 `/opt/genio/health_check.sh`：

```bash
#!/bin/bash

echo "=== 服务健康检查 ==="

# 检查容器状态
echo ""
echo "1. 容器状态:"
docker-compose -f /opt/genio/docker-compose.prod.yml ps

# 检查 Backend API
echo ""
echo "2. Backend API 健康检查:"
curl -s http://localhost:8200/health | jq '.' || echo "Backend API 异常"

# 检查 AI Brain
echo ""
echo "3. AI Brain 健康检查:"
curl -s http://localhost:9090/health || echo "AI Brain 异常"

# 检查 Frontend
echo ""
echo "4. Frontend 健康检查:"
curl -s -I http://localhost:3000 | head -n 1

# 检查数据库连接
echo ""
echo "5. 数据库连接:"
docker exec genio-mysql mysqladmin ping -u root -p${MYSQL_ROOT_PASSWORD} 2>/dev/null && echo "MySQL OK" || echo "MySQL 异常"

# 检查 Redis
echo ""
echo "6. Redis 连接:"
docker exec genio-redis redis-cli ping

# 检查磁盘空间
echo ""
echo "7. 磁盘空间:"
df -h /opt/genio

# 检查内存使用
echo ""
echo "8. 内存使用:"
free -h

echo ""
echo "=== 检查完成 ==="
```

### 2. 设置定时健康检查

```bash
# 每5分钟检查一次
echo "*/5 * * * * /opt/genio/health_check.sh >> /var/log/genio-health.log 2>&1" | crontab -
```

---

## 最佳实践

### 1. 更新前检查清单

- [ ] 已备份数据库
- [ ] 已查看代码变更记录
- [ ] 已测试新功能（如有）
- [ ] 已通知相关人员
- [ ] 选择低峰期更新
- [ ] 准备好回滚方案

### 2. 更新建议

1. **小步快跑**: 频繁小更新优于大版本更新
2. **测试环境**: 先在测试环境验证
3. **金丝雀发布**: 先更新一台服务器，观察无异常后再全量更新
4. **监控告警**: 更新后密切关注监控指标
5. **保留备份**: 至少保留最近3个版本的备份

### 3. 紧急回滚标准

出现以下情况应立即回滚：
- 服务无法启动
- 错误率突增（>5%）
- 响应时间显著增加（>2倍）
- 数据库连接失败
- 关键功能异常

---

## 总结

推荐的更新方案：

**日常小更新**: 使用 `update.sh` 脚本
```bash
./update.sh backend
```

**重大版本更新**: 使用完整的手动流程
```bash
# 1. 备份
# 2. 更新
# 3. 验证
# 4. 回滚准备
```

**自动化部署**: 配置 CI/CD 流程（GitHub Actions/GitLab CI）

**紧急回滚**: 使用 `rollback.sh` 脚本
```bash
./rollback.sh backend
```
