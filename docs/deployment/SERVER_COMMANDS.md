# 服务器监控命令参考

常用的服务器管理和监控命令速查手册。

## 📦 Docker 容器管理

```bash
# 查看运行中的容器
docker ps

# 查看所有容器（包括已停止）
docker ps -a

# 查看容器资源使用（实时）
docker stats

# 查看 Docker Compose 服务状态
docker-compose ps

# 重启服务
docker-compose restart backend

# 查看服务配置
docker-compose config
```

## 📋 日志查看

```bash
# 查看容器日志（最后50行）
docker logs --tail=50 genio-backend

# 实时查看日志
docker logs -f genio-backend

# Docker Compose 查看日志
docker-compose logs -f backend

# 过滤错误日志
docker logs genio-backend 2>&1 | grep -i error

# 查看应用日志文件
tail -f logs/server.log

# 查看最近的错误
tail -100 logs/server.log | grep -i error
```

## 💻 系统资源监控

```bash
# 查看系统资源
top
# 或使用更友好的工具
htop

# 查看内存使用
free -h

# 查看磁盘使用
df -h

# 查看当前目录大小
du -sh .

# 查看 Docker 磁盘占用
docker system df

# 查看系统负载
uptime
```

## 🌐 网络和端口

```bash
# 查看监听的端口
netstat -tlnp
# 或使用 ss（更快）
ss -tlnp

# 查看特定端口
netstat -tlnp | grep :8200

# 测试端口连通性
nc -zv localhost 8200

# 测试 HTTP 服务
curl http://localhost:8200

# 测试 Metrics 端点
curl http://localhost:9188/metrics

# 查看 Docker 网络
docker network ls
```

## 🗄️ 数据库管理

### MySQL

```bash
# 进入 MySQL 容器
docker exec -it genio-backend-mysql bash

# 直接执行 MySQL 命令
docker exec -it genio-backend-mysql mysql -uroot -p

# 查看数据库列表
docker exec genio-backend-mysql mysql -uroot -p${BACKEND_MYSQL_ROOT_PASSWORD} -e "SHOW DATABASES;"

# 查看当前连接数
docker exec genio-backend-mysql mysql -uroot -p${BACKEND_MYSQL_ROOT_PASSWORD} -e "SHOW PROCESSLIST;"
```

### Redis

```bash
# 连接 Redis CLI
docker exec -it genio-backend-redis redis-cli -a ${BACKEND_REDIS_PASSWORD}

# 查看 Redis 信息
docker exec genio-backend-redis redis-cli -a ${BACKEND_REDIS_PASSWORD} --no-auth-warning INFO

# 查看内存使用
docker exec genio-backend-redis redis-cli -a ${BACKEND_REDIS_PASSWORD} --no-auth-warning INFO memory

# 查看键的数量
docker exec genio-backend-redis redis-cli -a ${BACKEND_REDIS_PASSWORD} --no-auth-warning DBSIZE
```

## 🔍 服务健康检查

```bash
# 检查 HTTP 服务
curl -v http://localhost:8200

# 检查 Metrics
curl http://localhost:9188/metrics

# 检查容器健康状态
docker inspect --format='{{.State.Health.Status}}' genio-backend

# 持续监控（每5秒检查一次）
watch -n 5 'curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8200'
```

## 🎯 快速诊断脚本

创建一个健康检查脚本：

```bash
#!/bin/bash
# health-check.sh - 一键健康检查

echo "=== 容器状态 ==="
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

echo -e "\n=== 资源使用 ==="
docker stats --no-stream

echo -e "\n=== 端口监听 ==="
ss -tlnp | grep -E ':(8200|8181|9188|3307|6380)'

echo -e "\n=== 磁盘使用 ==="
df -h

echo -e "\n=== 内存使用 ==="
free -h

echo -e "\n=== HTTP 健康检查 ==="
curl -s -o /dev/null -w "HTTP API (8200): %{http_code}\n" http://localhost:8200
curl -s -o /dev/null -w "Metrics (9188): %{http_code}\n" http://localhost:9188/metrics

echo -e "\n=== 最近错误日志 ==="
docker logs --tail=20 genio-backend 2>&1 | grep -i error || echo "无错误"
```

使用方法：
```bash
chmod +x health-check.sh
./health-check.sh
```

## 🔧 Docker 维护

```bash
# 清理未使用的镜像
docker image prune

# 清理未使用的容器
docker container prune

# 一键清理所有未使用的资源
docker system prune

# 查看可以清理的空间
docker system df

# 备份镜像
docker save genio-backend:latest | gzip > genio-backend-backup.tar.gz

# 恢复镜像
docker load < genio-backend-backup.tar.gz
```

## 🆘 常见问题排查

### 服务无法访问

```bash
# 1. 检查容器是否运行
docker ps | grep genio-backend

# 2. 检查容器日志
docker logs --tail=50 genio-backend

# 3. 检查端口监听
netstat -tlnp | grep 8200

# 4. 测试本地连接
curl http://localhost:8200
```

### 性能问题

```bash
# 1. 检查资源使用
docker stats

# 2. 检查系统负载
uptime

# 3. 查看慢日志
grep "slow\|timeout" logs/server.log

# 4. 检查磁盘空间
df -h
```

### 数据库连接问题

```bash
# 1. 检查 MySQL 容器状态
docker ps | grep mysql

# 2. 检查 MySQL 连接
docker exec genio-backend-mysql mysql -uroot -p${BACKEND_MYSQL_ROOT_PASSWORD} -e "SELECT 1;"

# 3. 查看数据库日志
docker logs genio-backend-mysql

# 4. 检查连接数
docker exec genio-backend-mysql mysql -uroot -p${BACKEND_MYSQL_ROOT_PASSWORD} -e "SHOW PROCESSLIST;"
```

### Redis 连接问题

```bash
# 1. 检查 Redis 容器
docker ps | grep redis

# 2. 测试 Redis 连接
docker exec genio-backend-redis redis-cli -a ${BACKEND_REDIS_PASSWORD} --no-auth-warning PING

# 3. 查看 Redis 日志
docker logs genio-backend-redis

# 4. 检查 Redis 内存
docker exec genio-backend-redis redis-cli -a ${BACKEND_REDIS_PASSWORD} --no-auth-warning INFO memory
```

## 📊 性能监控

### 实时监控

```bash
# 容器资源监控
docker stats

# 系统资源监控
htop

# 网络连接监控
watch -n 2 'netstat -ant | grep ESTABLISHED | wc -l'

# 日志错误监控
tail -f logs/server.log | grep --color=always -i error
```

### 历史统计

```bash
# 统计请求数
grep "HTTP" logs/server.log | wc -l

# 统计错误数
grep "ERROR" logs/server.log | wc -l

# 按状态码统计
grep "HTTP" logs/server.log | awk '{print $9}' | sort | uniq -c

# 统计最慢的请求
grep "duration" logs/server.log | sort -k10 -rn | head -20
```

## 📝 配置检查

```bash
# 查看环境变量
cat .env

# 查看配置文件
cat conf/server.yaml

# 验证 Docker Compose 配置
docker-compose config

# 查看容器环境变量
docker inspect genio-backend | grep -A 20 Env
```

## 🔐 安全检查

```bash
# 查看防火墙状态
sudo ufw status

# 查看开放的端口
sudo netstat -tlnp

# 查看登录用户
who

# 查看登录历史
last

# 查看失败的登录尝试
sudo lastb
```

---

## 💡 使用技巧

1. **使用别名简化命令**：
```bash
# 添加到 ~/.bashrc
alias dps='docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"'
alias dlogs='docker logs -f genio-backend'
alias dstats='docker stats --no-stream'
```

2. **组合命令快速排查**：
```bash
# 查看服务状态 + 最近日志 + 资源使用
docker ps && echo "---" && docker logs --tail=10 genio-backend && echo "---" && docker stats --no-stream
```

3. **使用 watch 实时监控**：
```bash
# 每2秒刷新一次
watch -n 2 'docker stats --no-stream'
```

## 📚 相关文档

- [快速开始指南](../configuration/QUICKSTART.md)
- [后端部署指南](BACKEND_DEPLOYMENT_GUIDE.md)
- [零停机部署指南](ZERO_DOWNTIME_DEPLOYMENT_GUIDE.md)
- [环境配置指南](../configuration/ENV_CONFIG_GUIDE.md)
