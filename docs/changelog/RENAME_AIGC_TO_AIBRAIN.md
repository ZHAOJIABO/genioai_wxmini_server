# AIGCServer 重命名为 AIBrainServer - 修改总结

## 修改概述

已将配置项 `AIGCServer` 重命名为 `AIBrainServer`，使命名更准确地反映其用途（连接 AI Brain 服务）。

---

## ✅ 已修改的文件

### 1. 核心代码文件

#### `conf/conf.go`
```go
// 修改前
AIGCServer struct {
    Addr string
}

// 修改后
AIBrainServer struct {
    Addr string
}
```

以及 Config 结构体：
```go
// 修改前
AIGCServer           AIGCServer

// 修改后
AIBrainServer        AIBrainServer
```

#### `internal/bootstrap/service_provider.go`
```go
// 修改前
Addr: conf.GlobalConfig.AIGCServer.Addr,

// 修改后
Addr: conf.GlobalConfig.AIBrainServer.Addr,
```

#### `internal/service/ai_clients/aigc_core/client.go`
```go
// 修改前
Addr: conf.GlobalConfig.AIGCServer.Addr,

// 修改后
Addr: conf.GlobalConfig.AIBrainServer.Addr,
```

### 2. 配置文件

#### `conf/server.yaml`
```yaml
# 修改前
AIGCServer:
  Addr: 127.0.0.1:8080

# 修改后
AIBrainServer:
  Addr: 127.0.0.1:8080
```

#### `conf/server.yaml.example`
```yaml
# 修改前
AIGCServer:
  Addr: 127.0.0.1:8080

# 修改后
AIBrainServer:
  Addr: 127.0.0.1:8080
```

#### `conf/server.prod.yaml`
```yaml
# 修改前
AIGCServer:
  Addr: ""

# 修改后
AIBrainServer:
  Addr: ""
```

### 3. 文档文件

#### `BACKEND_DEPLOYMENT_GUIDE.md`
- 更新了配置示例中的 `AIGCServer` → `AIBrainServer`
- 更新了三种连接方案的配置说明

#### `QUICKSTART.md`
- 更新了 AI Brain 连接配置说明

---

## 🔧 迁移指南

如果你已经在使用旧的配置，需要做以下修改：

### 1. 更新配置文件

在你的 `conf/server.prod.yaml` 或 `conf/server.yaml` 中：

```yaml
# 将这个
AIGCServer:
  Addr: your-ai-brain-address:8080

# 改为这个
AIBrainServer:
  Addr: your-ai-brain-address:8080
```

### 2. 重新编译（如果需要）

```bash
# 清理旧的构建缓存
go clean -cache

# 重新编译
go build -o server ./cmd
```

### 3. 重新部署

```bash
# Docker 部署
docker-compose build backend
docker-compose up -d backend

# 查看日志
docker-compose logs -f backend
```

---

## ✅ 验证修改

### 1. 检查配置加载

启动服务后，查看日志确认配置正确加载：

```bash
docker-compose logs backend | grep -i "brain"
```

### 2. 测试连接

如果 AI Brain 已部署，测试 gRPC 连接：

```bash
# 使用 grpcurl 测试
grpcurl -plaintext localhost:8080 list
```

### 3. 检查代码引用

确认所有引用都已更新：

```bash
# 搜索旧的配置名
grep -r "AIGCServer" --include="*.go" --include="*.yaml"

# 应该只在文档或测试文件中出现，核心代码中不应该有
```

---

## 📝 注意事项

1. **向后兼容性**：此修改不向后兼容，旧的配置文件需要更新
2. **配置文件**：确保所有环境（开发、测试、生产）的配置文件都已更新
3. **文档同步**：相关文档已同步更新
4. **Git 提交**：建议单独提交此重命名修改，便于追踪

---

## 🎯 配置示例

### 开发环境
```yaml
AIBrainServer:
  Addr: 127.0.0.1:8080
```

### Docker 部署
```yaml
AIBrainServer:
  Addr: genio-ai-brain:8080  # 容器名
```

### 跨服务器部署
```yaml
AIBrainServer:
  Addr: 192.168.1.100:8080  # AI Brain 服务器 IP
```

---

## 📚 相关文档

- [后端部署指南](BACKEND_DEPLOYMENT_GUIDE.md)
- [快速开始](QUICKSTART.md)
- [环境配置指南](ENV_CONFIG_GUIDE.md)

---

## 🔄 回滚方案

如果需要回滚到旧版本：

```bash
# 1. 回滚代码
git revert <commit-hash>

# 2. 恢复配置文件中的 AIGCServer 命名

# 3. 重新编译部署
```

---

修改完成时间：2025-02-04
