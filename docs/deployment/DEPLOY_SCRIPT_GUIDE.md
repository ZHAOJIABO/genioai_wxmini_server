# 一键部署脚本使用指南

## 功能特性

这个部署脚本提供以下功能：

- ✅ 自动检查和拉取最新代码
- ✅ 自动备份当前运行的镜像（保留最近 3 个备份）
- ✅ 构建新的 Docker 镜像
- ✅ 零停机时间部署（保留数据库和 Redis 数据）
- ✅ 健康检查和服务测试
- ✅ 一键回滚到上一个版本
- ✅ 彩色日志输出，清晰易读

## 使用方法

### 1. 首次部署或代码更新后的部署

在服务器上的项目目录执行：

```bash
cd /opt/genio-backend
./deploy.sh
```

脚本会自动执行以下步骤：
1. 检查 Git 仓库状态和未提交的更改
2. 拉取最新代码（如果有更新）
3. 询问是否继续部署
4. 备份当前镜像
5. 构建新镜像
6. 停止旧容器
7. 确保数据库和 Redis 运行
8. 启动新容器
9. 等待健康检查
10. 测试服务连接
11. 显示部署结果

### 2. 查看服务状态

```bash
./deploy.sh --status
```

显示：
- 所有容器的运行状态
- 最近的日志输出

### 3. 查看实时日志

```bash
./deploy.sh --logs
```

实时跟踪后端服务的日志输出（Ctrl+C 退出）。

### 4. 回滚到上一个版本

如果新版本有问题，可以快速回滚：

```bash
./deploy.sh --rollback
```

脚本会：
1. 找到最新的备份镜像
2. 加载备份镜像
3. 重启容器

### 5. 查看帮助

```bash
./deploy.sh --help
```

## 典型使用场景

### 场景 1：日常代码更新

```bash
# 在本地提交并推送代码
git add .
git commit -m "feat: 新功能"
git push origin main

# 在服务器上部署
cd /opt/genio-backend
./deploy.sh
```

### 场景 2：紧急回滚

```bash
# 发现新版本有问题，立即回滚
./deploy.sh --rollback

# 查看日志确认
./deploy.sh --logs
```

### 场景 3：检查服务状态

```bash
# 查看服务是否正常
./deploy.sh --status

# 查看实时日志
./deploy.sh --logs
```

## 注意事项

### 数据安全

- ✅ **数据库和 Redis 数据不会被删除**，使用 Docker volumes 持久化
- ✅ 脚本会自动备份镜像，保留最近 3 个版本
- ✅ 部署失败时提示是否回滚

### 部署前检查

脚本会自动检查：
- 是否在正确的项目目录
- `.env` 文件是否存在
- Git 仓库状态（如果是 Git 仓库）
- 是否有未提交的更改

### 健康检查

脚本会等待服务通过健康检查（最多 60 秒），确保：
- MySQL 已就绪
- Redis 已就绪
- Backend 服务健康

### 端口测试

部署成功后自动测试：
- Metrics 端点 (9188)
- HTTP API (8200)
- gRPC 服务 (8181)

## 备份管理

备份文件存储在 `/opt/genio-backend-backups/` 目录：

```bash
# 查看所有备份
ls -lh /opt/genio-backend-backups/

# 手动加载备份
docker load < /opt/genio-backend-backups/genio-backend_20260204_140530.tar.gz
```

脚本自动保留最近 3 个备份，旧备份会被自动删除。

## 故障排查

### 部署失败

如果部署失败，脚本会：
1. 显示最近 50 行日志
2. 询问是否回滚
3. 保持当前状态以便调试

手动查看完整日志：

```bash
docker-compose logs backend
```

### 健康检查超时

如果健康检查一直失败：

```bash
# 查看容器状态
docker ps -a

# 查看详细日志
docker-compose logs backend

# 检查配置文件
cat genioAI_server/conf/server.prod.yaml
```

### 网络问题

如果容器无法连接到 MySQL 或 Redis：

```bash
# 检查网络
docker network ls

# 重新创建网络
docker network rm genio-shared
docker network create genio-shared

# 重新部署
./deploy.sh
```

## 高级用法

### 不使用 Git

如果不是 Git 仓库，脚本会跳过代码拉取步骤，直接构建和部署。

### 修改配置

编辑 [deploy.sh](deploy.sh) 顶部的配置变量：

```bash
PROJECT_DIR="/opt/genio-backend"      # 项目目录
GIT_BRANCH="main"                     # Git 分支
BACKUP_DIR="/opt/genio-backend-backups"  # 备份目录
```

### 自动化部署

配合 cron 或 webhook 实现自动部署：

```bash
# 示例：每天凌晨 2 点自动部署
0 2 * * * cd /opt/genio-backend && ./deploy.sh < /dev/null
```

## 访问地址

部署成功后的访问地址：

- **API (外部)**: https://genioai.appbobo.com
- **API (本地)**: http://localhost:8200
- **gRPC**: localhost:8181
- **Metrics**: http://localhost:9188/metrics

## 相关文档

- [后端部署指南](BACKEND_DEPLOYMENT_GUIDE.md)
- [环境变量配置](ENV_CONFIG_GUIDE.md)
- [快速开始](QUICKSTART.md)
