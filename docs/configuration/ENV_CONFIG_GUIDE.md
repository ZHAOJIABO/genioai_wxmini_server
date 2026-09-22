# 环境变量配置指南

## 快速配置步骤

### Step 1: 创建 .env 文件

```bash
cd /opt/genio-backend

# 从示例文件复制
cp genioAI_server/.env.example .env

# 编辑配置
vi .env
```

### Step 2: 必需配置项

以下配置**必须修改**，否则服务无法启动：

#### 1. 数据库配置（必需）

```bash
# 修改为强密码！生产环境不要使用默认值
BACKEND_MYSQL_ROOT_PASSWORD=your_very_strong_password_here_123!
BACKEND_MYSQL_DATABASE=genio_backend_db
BACKEND_MYSQL_USER=genio_backend_user
BACKEND_MYSQL_PASSWORD=your_strong_db_password_456!

# Redis 密码（必需）
BACKEND_REDIS_PASSWORD=your_redis_password_789!
```

**密码强度要求**：
- 至少 16 个字符
- 包含大小写字母、数字、特殊字符
- 不要使用常见单词

#### 2. AI 服务配置（根据使用情况选择）

你的项目已经配置了以下 AI 服务，可以直接使用：

**Azure OpenAI**（如果使用 GPT-4）：
```bash
AZURE_OPENAI_ENDPOINT=https://dmdsp-openai-us2.openai.azure.com/
AZURE_OPENAI_API_KEY=<your-azure-openai-api-key>
```

**豆包 Doubao**（如果使用豆包）：
```bash
DOUBAO_API_KEY=<your-doubao-api-key>
DOUBAO_ENDPOINT=https://ark.cn-beijing.volces.com/api/v3
DOUBAO_REGION=cn-beijing
DOUBAO_PRO_MODEL=ep-20250214171140-jhq27
DOUBAO_PRO_VISION_MODEL=ep-20250214171032-hbc67
```

**DeepSeek**（如果使用 DeepSeek）：
```bash
DEEPSEEK_API_KEY=<your-deepseek-api-key>
```

#### 3. 对象存储配置（必需）

你已经配置了 Cloudflare R2，可以直接使用：

```bash
R2_ENDPOINT=https://08e5d61be623f63d785df35604e48655.r2.cloudflarestorage.com
R2_ACCESS_KEY_ID=<your-r2-access-key-id>
R2_ACCESS_KEY_SECRET=<your-r2-secret-access-key>
R2_BUCKET=genio-ai
R2_PUBLIC_URL=https://cdn.appbobo.com
```

---

## 配置方式选择

你有**两种方式**配置这些值：

### 方式 A：在 .env 文件中配置（推荐）

在 `/opt/genio-backend/.env` 中配置所有环境变量，然后在 `server.prod.yaml` 中引用：

```yaml
# server.prod.yaml
Mysql:
  Addr: backend-mysql
  User: ${BACKEND_MYSQL_USER}
  Password: ${BACKEND_MYSQL_PASSWORD}
  Db: ${BACKEND_MYSQL_DATABASE}
  Port: 3306

LlmConfig:
  AzureOpenAI:
    Endpoint: "${AZURE_OPENAI_ENDPOINT}"
    APIKey: "${AZURE_OPENAI_API_KEY}"
```

**优点**：
- 敏感信息集中管理
- 不同环境切换方便
- 不会意外提交敏感信息到 Git

### 方式 B：直接在 server.prod.yaml 中配置

直接复制 `conf/server.yaml` 的配置到 `conf/server.prod.yaml`：

```yaml
# server.prod.yaml
LlmConfig:
  AzureOpenAI:
    Endpoint: "https://dmdsp-openai-us2.openai.azure.com/"
    APIKey: "<your-azure-openai-api-key>"
```

**优点**：
- 配置简单直接
- 不需要额外的环境变量文件

**缺点**：
- 需要注意不要把 server.prod.yaml 提交到公开仓库

---

## 完整配置示例（推荐配置）

创建 `/opt/genio-backend/.env`：

```bash
# ==================== 核心配置（必需修改）====================
# 数据库配置 - 请修改为强密码！
BACKEND_MYSQL_ROOT_PASSWORD=YourVeryStrongRootPassword123!@#
BACKEND_MYSQL_DATABASE=genio_backend_db
BACKEND_MYSQL_USER=genio_backend_user
BACKEND_MYSQL_PASSWORD=YourStrongDBPassword456!@#
BACKEND_REDIS_PASSWORD=YourStrongRedisPassword789!@#

# ==================== AI 服务配置（从 server.yaml 复制）====================
# Azure OpenAI
AZURE_OPENAI_ENDPOINT=https://dmdsp-openai-us2.openai.azure.com/
AZURE_OPENAI_API_KEY=<your-azure-openai-api-key>

# 豆包
DOUBAO_API_KEY=<your-doubao-api-key>
DOUBAO_ENDPOINT=https://ark.cn-beijing.volces.com/api/v3
DOUBAO_REGION=cn-beijing
DOUBAO_PRO_MODEL=ep-20250214171140-jhq27
DOUBAO_PRO_VISION_MODEL=ep-20250214171032-hbc67

# DeepSeek
DEEPSEEK_API_KEY=<your-deepseek-api-key>

# ==================== 存储配置（从 server.yaml 复制）====================
R2_ENDPOINT=https://08e5d61be623f63d785df35604e48655.r2.cloudflarestorage.com
R2_ACCESS_KEY_ID=<your-r2-access-key-id>
R2_ACCESS_KEY_SECRET=<your-r2-secret-access-key>
R2_BUCKET=genio-ai
R2_PUBLIC_URL=https://cdn.appbobo.com

# ==================== 邮件配置（可选）====================
SMTP_HOST=smtp.qq.com
SMTP_PORT=587
SMTP_USERNAME=624345999@qq.com
SMTP_PASSWORD=<your-smtp-app-password>
SMTP_FROM=624345999@qq.com

# ==================== Apple 登录配置（可选）====================
APPLE_TEAM_ID=J392M2H9CQ
APPLE_KID=3FV4897G7Z
```

---

## 配置优先级建议

### 第一优先级（立即配置）：

1. ✅ **数据库密码** - 修改为强密码
2. ✅ **Redis 密码** - 修改为强密码
3. ✅ **存储配置** - 使用现有的 R2 配置

### 第二优先级（根据业务需要）：

4. **Azure OpenAI** - 如果使用 GPT-4
5. **豆包 Doubao** - 如果使用豆包
6. **DeepSeek** - 如果使用 DeepSeek

### 第三优先级（可选功能）：

7. **邮件配置** - 如果需要发送验证码邮件
8. **Apple 登录** - 如果支持 Apple 登录

---

## 简化版配置（最小化启动）

如果只是先启动服务测试，只需配置这些：

```bash
# 最小化配置
BACKEND_MYSQL_ROOT_PASSWORD=TestPassword123!
BACKEND_MYSQL_DATABASE=genio_backend_db
BACKEND_MYSQL_USER=genio_user
BACKEND_MYSQL_PASSWORD=TestPassword456!
BACKEND_REDIS_PASSWORD=TestRedis789!

# 存储（必需）
R2_ENDPOINT=https://08e5d61be623f63d785df35604e48655.r2.cloudflarestorage.com
R2_ACCESS_KEY_ID=<your-r2-access-key-id>
R2_ACCESS_KEY_SECRET=<your-r2-secret-access-key>
R2_BUCKET=genio-ai
R2_PUBLIC_URL=https://cdn.appbobo.com
```

其他 AI 服务的 API Key 可以先不配置，等需要使用时再添加。

---

## 验证配置

### 1. 检查 .env 文件格式

```bash
# 查看配置（隐藏密码）
cat .env | grep -v PASSWORD | grep -v SECRET

# 检查是否有语法错误
docker-compose config
```

### 2. 测试数据库连接

```bash
# 启动数据库
docker-compose up -d backend-mysql

# 测试连接
docker exec -it genio-backend-mysql mysql \
  -u ${BACKEND_MYSQL_USER} \
  -p${BACKEND_MYSQL_PASSWORD} \
  -e "SELECT 'Database connection successful';"
```

### 3. 测试 Redis 连接

```bash
# 启动 Redis
docker-compose up -d backend-redis

# 测试连接
docker exec -it genio-backend-redis redis-cli \
  -a ${BACKEND_REDIS_PASSWORD} \
  ping
```

---

## 安全建议

### 1. 文件权限

```bash
# 限制 .env 文件权限
chmod 600 /opt/genio-backend/.env

# 确保只有当前用户可读
ls -la .env
# 应显示：-rw------- 1 user user
```

### 2. Git 忽略

确保 `.env` 在 `.gitignore` 中：

```bash
# 检查 .gitignore
cat genioAI_server/.gitignore | grep .env

# 如果没有，添加
echo ".env" >> genioAI_server/.gitignore
echo ".env.*" >> genioAI_server/.gitignore
echo "!.env.example" >> genioAI_server/.gitignore
```

### 3. 密码管理

**生产环境强密码生成**：

```bash
# 生成强密码
openssl rand -base64 32

# 或使用
head -c 32 /dev/urandom | base64
```

### 4. 定期更换

- 数据库密码：每 3-6 个月
- API Key：根据服务商建议
- Redis 密码：每 6 个月

---

## 常见问题

### Q1: 配置后服务无法启动？

```bash
# 检查环境变量是否正确加载
docker-compose config

# 查看启动日志
docker-compose logs backend
```

### Q2: 如何修改已运行服务的配置？

```bash
# 1. 修改 .env
vi .env

# 2. 重启服务（会重新读取环境变量）
docker-compose down
docker-compose up -d
```

### Q3: 配置文件中的变量没有生效？

检查格式：
```bash
# 正确格式
KEY=value

# 错误格式（不要有空格）
KEY = value  # ❌
KEY= value   # ❌
KEY =value   # ❌
```

### Q4: 是否需要配置所有 AI 服务？

不需要！只配置你实际使用的服务。比如：
- 只用 Azure OpenAI → 只配置 Azure 相关变量
- 只用豆包 → 只配置 Doubao 相关变量

---

## 下一步

配置完成后：

```bash
# 1. 验证配置
docker-compose config

# 2. 启动服务
docker-compose up -d

# 3. 查看日志
docker-compose logs -f backend

# 4. 测试 API
curl http://localhost:8200/health
```

配置有问题随时问我！
