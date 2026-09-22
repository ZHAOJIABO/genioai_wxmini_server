# Gemini Provider "record not found" 错误修复指南

## 问题描述

错误信息：
```
stream chat processing failed: chat service processing failed: 创建Handler失败: record not found
```

## 根本原因

AI Brain (AIGC Core) 服务在处理 Gemini 任务时，无法在数据库中找到 Gemini provider 的配置记录。

## 解决步骤

### 1. 检查 AI Brain 是否正在运行

```bash
# 检查 AI Brain 服务状态
docker ps | grep ai-brain

# 查看 AI Brain 日志
docker logs ai-brain-core-app -f
```

### 2. 连接到 AI Brain 数据库

```bash
# 连接到 AI Brain MySQL 数据库
docker exec -it ai-brain-mysql mysql -u aibrain_user -p123456 ai_brain
```

### 3. 检查 provider 配置表

在 MySQL 中执行：

```sql
-- 查看所有 provider 配置
SELECT * FROM providers;

-- 查看模型配置
SELECT * FROM models WHERE provider = 'gemini';

-- 查看 API 配置
SELECT * FROM api_configs WHERE provider = 'gemini';
```

### 4. 插入 Gemini Provider 配置（如果缺失）

根据 AI Brain 的数据库结构，需要插入相应的配置记录。具体的 SQL 语句需要根据 AI Brain 项目的数据库架构来确定。

**示例（需要根据实际表结构调整）：**

```sql
-- 插入 provider 配置
INSERT INTO providers (name, type, enabled, created_at, updated_at)
VALUES ('gemini', 'image_generation', true, NOW(), NOW());

-- 插入模型配置
INSERT INTO models (provider, model_name, enabled, created_at, updated_at)
VALUES ('gemini', 'gemini-2.5-flash-image-preview', true, NOW(), NOW());

-- 插入 API 配置（包含认证信息）
INSERT INTO api_configs (provider, api_key, endpoint, config_json, created_at, updated_at)
VALUES ('gemini', 'YOUR_GEMINI_API_KEY', 'https://generativelanguage.googleapis.com', '{}', NOW(), NOW());
```

### 5. 检查 Gemini API 密钥配置

确保 AI Brain 有访问 Gemini API 的凭证：

#### 方法 A：通过环境变量

编辑 AI Brain 的 `.env` 文件：

```bash
# Gemini API 配置
GEMINI_API_KEY=your_gemini_api_key_here
GEMINI_ENDPOINT=https://generativelanguage.googleapis.com
```

#### 方法 B：通过配置文件

如果 AI Brain 使用配置文件，确保包含 Gemini 配置：

```yaml
providers:
  gemini:
    enabled: true
    api_key: your_gemini_api_key_here
    endpoint: https://generativelanguage.googleapis.com
    models:
      - gemini-2.5-flash-image-preview
```

### 6. 重启 AI Brain 服务

```bash
# 重启 AI Brain
docker-compose restart ai-brain

# 查看启动日志
docker logs ai-brain-core-app -f
```

### 7. 验证修复

在 vision-ai-server 中重新提交 Gemini 任务，检查是否还有错误。

## 临时解决方案

如果 Gemini provider 配置复杂或暂时无法修复，可以考虑：

1. **使用其他 provider**：临时切换到其他图像生成服务（如 DALL-E、Midjourney 等）
2. **禁用 Gemini 执行器**：在 vision-ai-server 中暂时禁用 Gemini executor

```go
// 在 internal/service/picture_generate/service.go 中
// 临时注释掉 Gemini executor 的注册
// service.RegisterExecutor(gemini_executor.NewGeminiExecutor(...))
```

## 联系 AI Brain 团队

如果以上步骤无法解决问题，需要联系 AI Brain 项目的维护者：

1. 确认 AI Brain 是否支持 Gemini provider
2. 获取正确的数据库初始化脚本
3. 获取 provider 配置的正确格式

## 相关文件

- AI Brain 配置：`ai-brain/.env`
- Vision AI Server 配置：`conf/server.yaml` (AIBrainServer.Addr)
- Gemini Executor：`internal/service/picture_generate/gemini_executor.go`

## 检查清单

- [ ] AI Brain 服务正在运行
- [ ] AI Brain 数据库连接正常
- [ ] providers 表中存在 gemini 记录
- [ ] models 表中存在 gemini 相关模型
- [ ] api_configs 表中存在 gemini API 配置
- [ ] Gemini API 密钥有效且权限正确
- [ ] 重启 AI Brain 后配置生效
- [ ] 测试任务提交成功
