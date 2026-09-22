# GenioAI Open API 方案设计文档

## 1. 背景与目标

将 GenioAI 的 AIGC 能力以 Open API 形式对外提供，允许第三方开发者通过 API Key 调用服务。

**核心目标：**
- 提供 API Key 的生成与管理能力
- 在现有 interceptor 认证链路中增加 API Key 验证分支
- 不影响现有客户端（App / Web）的认证流程

---

## 2. 现有架构分析

### 2.1 当前认证流程

```
HTTP Request
  → grpc-gateway 转换为 gRPC
    → UnaryInterceptor.Process()          // internal/rpc/init.go:259
      → ValidationHandler.ValidateAndExtract()  // internal/rpc/init.go:115
        → extractReqHeader(req)           // 通过反射从 protobuf message 中提取 RequestHeader
        → paramsCheck()                   // internal/rpc/interceptor.go:41
          → checkReqHeader()              // internal/rpc/interceptor.go:456
            → UserDao.GetUserByIdOrPhone() // 根据 user_id 查库验证用户
      → ContextBuilder.BuildContext()     // 将 user_id, project_id 等写入 context
    → handler(ctx, req)                   // 执行业务逻辑
```

**关键点：**
- 认证依赖 `RequestHeader` 中的 `user_id` 字段
- 白名单机制：`LoginRequest`、`ListImageGenerationModelsRequest` 等免认证
- `checkReqHeader` 校验失败时当前返回 `nil`（不阻断请求），这是为了兼容移动端部分场景

### 2.2 任务提交流程

`SubmitPictureForgeTask`（`internal/api/picture_forge.go:136`）：

```
提取 userID / projectID ← 从 context 中获取（interceptor 注入）
  → validateSubmitTaskRequest()
  → 判断模式（workflow / 模型直连）
  → CalculateFinalCredits() 计算积分
  → 事务 {
      DeductCreditsWithTx()   // 扣积分
      SubmitTaskWithTx()      // 创建任务
    }
  → 返回 task_id
```

---

## 3. 数据库设计

### 3.1 新增表：`va_api_key`

```sql
CREATE TABLE `va_api_key` (
    `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `key_id`          VARCHAR(64)     NOT NULL COMMENT 'API Key 的唯一标识（前缀部分，如 sk-xxx 的前8位），用于日志和展示',
    `key_hash`        VARCHAR(255)    NOT NULL COMMENT 'API Key 的 SHA256 哈希值，用于验证',
    `user_id`         VARCHAR(100)    NOT NULL COMMENT '关联的用户ID',
    `project_id`      VARCHAR(100)    NOT NULL DEFAULT 'com.domob.visionai' COMMENT '关联的项目ID',
    `name`            VARCHAR(100)    NOT NULL DEFAULT '' COMMENT 'Key 的名称/备注，方便用户管理多个 Key',
    `status`          TINYINT         NOT NULL DEFAULT 1 COMMENT '状态：0=禁用, 1=启用',
    `permissions`     JSON            NULL     COMMENT '权限配置，如允许的模型列表、workflow 列表等',
    `rate_limit`      INT             NOT NULL DEFAULT 60 COMMENT '每分钟请求上限',
    `daily_quota`     INT             NOT NULL DEFAULT 1000 COMMENT '每日请求上限',
    `expires_at`      TIMESTAMP       NULL     COMMENT '过期时间，NULL 表示永不过期',
    `last_used_at`    TIMESTAMP       NULL     COMMENT '最后使用时间',
    `created_at`      TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `updated_at`      TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at`      TIMESTAMP       NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_key_hash` (`key_hash`),
    KEY `idx_user_id` (`user_id`),
    KEY `idx_key_id` (`key_id`),
    KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Open API Key 管理表';
```

### 3.2 新增表：`va_api_key_usage_log`（可选，建议后续实现）

```sql
CREATE TABLE `va_api_key_usage_log` (
    `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `key_id`      VARCHAR(64)     NOT NULL COMMENT 'API Key 标识',
    `user_id`     VARCHAR(100)    NOT NULL,
    `endpoint`    VARCHAR(255)    NOT NULL COMMENT '调用的接口名称',
    `task_id`     VARCHAR(64)     NULL     COMMENT '关联的任务ID',
    `status`      VARCHAR(20)     NOT NULL COMMENT '调用结果：success / failed',
    `error_msg`   VARCHAR(500)    NULL,
    `ip`          VARCHAR(50)     NULL,
    `credit_cost` INT             NOT NULL DEFAULT 0 COMMENT '本次调用消耗积分',
    `created_at`  TIMESTAMP       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    KEY `idx_key_id` (`key_id`),
    KEY `idx_user_id_created` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='API Key 使用日志';
```

---

## 4. Model 层设计

### 4.1 ApiKey Model

文件：`internal/model/api_key.go`

```go
package model

import (
    "database/sql"
    "gorm.io/gorm"
)

type ApiKey struct {
    ID          uint           `gorm:"primarykey"`
    KeyID       string         `gorm:"column:key_id;type:varchar(64);uniqueIndex"`
    KeyHash     string         `gorm:"column:key_hash;type:varchar(255);uniqueIndex:uk_key_hash"`
    UserID      string         `gorm:"column:user_id;type:varchar(100);index"`
    ProjectID   string         `gorm:"column:project_id;type:varchar(100);default:com.domob.visionai"`
    Name        string         `gorm:"column:name;type:varchar(100)"`
    Status      int8           `gorm:"column:status;type:tinyint;default:1"` // 0=禁用, 1=启用
    Permissions string         `gorm:"column:permissions;type:json"`
    RateLimit   int            `gorm:"column:rate_limit;default:60"`
    DailyQuota  int            `gorm:"column:daily_quota;default:1000"`
    ExpiresAt   sql.NullTime   `gorm:"column:expires_at"`
    LastUsedAt  sql.NullTime   `gorm:"column:last_used_at"`
    CreatedAt   sql.NullTime
    UpdatedAt   sql.NullTime
    DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (ApiKey) TableName() string {
    return "api_key" // GORM 会加上 va_ 前缀
}
```

---

## 5. 接口设计

### 5.1 生成 API Key

**请求方式：** POST
**路径：** `/v1/open/api-keys`
**认证：** 需要已登录用户（通过现有 RequestHeader 认证）

**请求体：**
```json
{
    "request_header": { "user_id": "xxx", ... },
    "name": "My Production Key",
    "permissions": {
        "allowed_models": ["flux-schnell", "kling-v1"],
        "allowed_workflows": []
    }
}
```

**响应体：**
```json
{
    "response_header": { "status_code": 0 },
    "api_key": "sk-genio-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
    "key_id": "sk-genio-xx",
    "name": "My Production Key",
    "created_at": "2026-03-18T10:00:00Z"
}
```

> **重要：** `api_key` 明文仅在创建时返回一次，后续无法查看。服务端只存储哈希值。

### 5.2 列出 API Key

**请求方式：** POST
**路径：** `/v1/open/api-keys/list`

**响应体：**
```json
{
    "response_header": { "status_code": 0 },
    "keys": [
        {
            "key_id": "sk-genio-xx",
            "name": "My Production Key",
            "status": 1,
            "last_used_at": "2026-03-18T09:30:00Z",
            "created_at": "2026-03-18T10:00:00Z"
        }
    ]
}
```

### 5.3 吊销/禁用 API Key

**请求方式：** POST
**路径：** `/v1/open/api-keys/revoke`

**请求体：**
```json
{
    "request_header": { "user_id": "xxx", ... },
    "key_id": "sk-genio-xx"
}
```

### 5.4 通过 API Key 提交任务

**请求方式：** POST
**路径：** `/v1/open/tasks/submit`（或复用现有的 `SubmitPictureForgeTask`）
**认证：** `Authorization: Bearer sk-genio-xxxxxxxx`

**请求体（模型直连模式示例）：**
```json
{
    "model_name": "flux-schnell",
    "user_prompt": "a cat sitting on a rainbow",
    "negative_prompt": "",
    "user_images": [],
    "strength": 0.7,
    "callback_url": "https://your-server.com/webhook/task-complete"
}
```

> 注意：通过 API Key 调用时，不需要传 `request_header`，身份信息从 API Key 中解析。

### 5.5 查询任务状态

**请求方式：** POST
**路径：** `/v1/open/tasks/query`
**认证：** `Authorization: Bearer sk-genio-xxxxxxxx`

**请求体：**
```json
{
    "task_id": "task_xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
}
```

**响应体：**
```json
{
    "task_id": "task_xxx",
    "status": "completed",
    "progress": 100,
    "result": {
        "images": ["https://cdn.example.com/result.png"]
    },
    "credit_cost": 10,
    "created_at": "2026-03-18T10:00:00Z",
    "completed_at": "2026-03-18T10:00:30Z"
}
```

---

## 6. 认证流程改造

### 6.1 改造点：Interceptor 层

在 `internal/rpc/init.go` 的 `ValidationHandler.ValidateAndExtract()` 中增加 API Key 分支：

```
ValidateAndExtract(ctx, req)
  ├── 尝试从 gRPC metadata / HTTP Header 中读取 Authorization
  │
  ├── [有 API Key] → apiKeyAuth(ctx, apiKey)
  │     → SHA256(apiKey) 查 va_api_key 表（优先查 Redis 缓存）
  │     → 校验 status、expires_at
  │     → 限流检查（Redis INCR + EXPIRE）
  │     → 构造 ValidationResult（填充 user_id、project_id）
  │     → 在 context 中标记 is_api_key_request = true
  │
  └── [无 API Key] → 走现有的 extractReqHeader + checkReqHeader 逻辑（不变）
```

### 6.2 Context 扩展

在 `internal/constants/context.go` 中新增：

```go
const (
    CtxIsApiKeyRequest = "IsApiKeyRequest"
    CtxApiKeyID        = "ApiKeyID"
)
```

### 6.3 API Key 验证函数

文件：`internal/rpc/apikey_auth.go`（新增）

```go
func apiKeyAuth(ctx context.Context, rawKey string) (*ValidationResult, error) {
    // 1. 计算哈希
    keyHash := sha256Hex(rawKey)

    // 2. 优先查 Redis 缓存
    cached := redis.Get("apikey:" + keyHash)
    if cached == nil {
        // 查数据库
        apiKey := apiKeyDao.GetByKeyHash(keyHash)
        if apiKey == nil {
            return nil, errors.New("invalid api key")
        }
        // 写入 Redis 缓存（TTL 5 分钟）
        redis.Set("apikey:" + keyHash, apiKey, 5*time.Minute)
        cached = apiKey
    }

    // 3. 状态检查
    if cached.Status != 1 {
        return nil, errors.New("api key disabled")
    }
    if cached.ExpiresAt.Valid && cached.ExpiresAt.Time.Before(time.Now()) {
        return nil, errors.New("api key expired")
    }

    // 4. 限流检查（Redis 滑动窗口）
    if !checkRateLimit(cached.KeyID, cached.RateLimit) {
        return nil, errors.New("rate limit exceeded")
    }

    // 5. 构造结果
    result := &ValidationResult{
        RequestHeader: buildApiKeyRequestHeader(cached),
        MappedUserID:  cached.UserID,
    }
    return result, nil
}
```

### 6.4 限流方案

使用 Redis 实现滑动窗口限流：

```
Key:  visionai:apikey:ratelimit:{key_id}:{minute_window}
操作: INCR + EXPIRE 60s
判断: 当前值 > rate_limit → 拒绝
```

每日配额：

```
Key:  visionai:apikey:daily:{key_id}:{date}
操作: INCR + EXPIRE 86400s
判断: 当前值 > daily_quota → 拒绝
```

---

## 7. API Key 生成策略

### 7.1 格式

```
sk-genio-{32位随机字符}
```

- 前缀 `sk-genio-` 方便识别来源
- 随机部分使用 `crypto/rand` 生成，Base62 编码
- 总长度约 40 字符

### 7.2 存储

- 生成时返回明文给用户（仅一次）
- 服务端存储 `SHA256(api_key)` 哈希值
- `key_id` 存储前缀（如 `sk-genio-ab`），用于展示和日志

### 7.3 生成流程

```go
func GenerateApiKey() (plainKey string, keyID string, keyHash string) {
    randomBytes := make([]byte, 24)
    crypto_rand.Read(randomBytes)
    randomPart := base62.Encode(randomBytes)

    plainKey = "sk-genio-" + randomPart
    keyID = plainKey[:11]  // "sk-genio-xx"
    keyHash = sha256Hex(plainKey)
    return
}
```

---

## 8. 需要改动的文件清单

| 文件 | 改动内容 |
|------|----------|
| `internal/model/api_key.go` | **新增** ApiKey Model |
| `internal/dao/api_key.go` | **新增** ApiKey DAO（增删改查） |
| `internal/service/apikey_service.go` | **新增** API Key 生成、验证、管理的 Service |
| `internal/api/open_api.go` | **新增** Open API 接口实现（生成 Key、提交任务、查询任务） |
| `internal/rpc/apikey_auth.go` | **新增** API Key 认证逻辑 |
| `internal/rpc/init.go` | **修改** `ValidateAndExtract()` 增加 API Key 认证分支 |
| `internal/constants/context.go` | **修改** 新增 `CtxIsApiKeyRequest`、`CtxApiKeyID` |
| `internal/constants/constants.go` | **修改** 新增 API Key 相关错误码 |
| `pkg/va_interface/open_api.proto` | **新增** Open API 的 protobuf 定义 |
| `cmd/main.go` | **修改** 注册 Open API Service |
| `assets/migrations/xxx_create_api_key.up.sql` | **新增** 建表 SQL |
| `internal/bootstrap/` | **修改** 注入 ApiKey 相关依赖 |

---

## 9. 安全考虑

| 风险 | 应对措施 |
|------|----------|
| API Key 泄露 | 支持即时吊销；哈希存储不可逆向 |
| 暴力枚举 Key | Key 空间足够大（24 字节随机 ≈ 192 bit）；限流保护 |
| 重放攻击 | HTTPS 强制；可选请求签名（timestamp + nonce） |
| 越权访问 | API Key 绑定 user_id，权限字段控制可用范围 |
| 积分滥用 | 复用现有积分系统，API Key 用户同样扣积分 |
| DDoS | Redis 限流（分钟级 + 日级），可接入 WAF |

---

## 10. 实施步骤

1. **数据库迁移** — 创建 `va_api_key` 表
2. **Model + DAO** — 实现 ApiKey 的数据访问层
3. **Service** — 实现 API Key 生成、验证、管理逻辑
4. **Interceptor 改造** — 在认证链路中增加 API Key 分支
5. **Open API 接口** — 实现生成 Key、提交任务、查询任务接口
6. **Protobuf 定义** — 新增 Open API 相关 message 和 service
7. **限流** — 实现 Redis 滑动窗口限流
8. **测试** — 单元测试 + 集成测试
9. **（可选）Webhook 回调** — 任务完成后通知第三方
10. **（可选）使用日志** — 实现 `va_api_key_usage_log` 记录调用明细
