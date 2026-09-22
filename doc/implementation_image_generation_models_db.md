# 生图模型列表接口 - 数据库实现方案

## 变更概述

将生图模型列表接口的数据来源从 `ModelConfigManager`（内存配置）改为**数据库表查询**，实现动态配置和集中管理。

---

## 📦 新增文件

### 1. 数据库迁移文件
- **位置**：`assets/migrations/20260202190205_create_image_generation_model.up.sql`
- **功能**：创建 `va_image_generation_model` 表并插入默认数据
- **回滚**：`assets/migrations/20260202190205_create_image_generation_model.down.sql`

### 2. Model 层
- **文件**：`internal/model/image_generation_model.go`
- **结构体**：`ImageGenerationModel`
- **方法**：`ToProto()` - 转换为 API 响应格式

### 3. DAO 层
- **文件**：`internal/dao/image_generation_model.go`
- **方法**：
  - `ListEnabledModels()` - 查询启用的模型
  - `GetModelByName()` - 根据名称查询
  - `CreateModel()` / `UpdateModel()` / `DeleteModel()` - CRUD 操作

---

## 🔄 修改文件

### 1. API 层
**文件**：`internal/api/picture_forge.go`

**修改点**：
```go
// 修改前：从内存配置获取
availableModels := s.modelConfigManager.ListAvailableModels()

// 修改后：从数据库查询
modelDao := dao.NewImageGenerationModelDao(db.GetDB())
models, err := modelDao.ListEnabledModels(ctx, projectID)
```

**新增导入**：
```go
"va_visionai_server/internal/dao"
"va_visionai_server/internal/db"
```

### 2. 认证拦截器
**文件**：`internal/rpc/interceptor.go`

**修改点**：将 `ListImageGenerationModels` 接口添加到免认证白名单
```go
// 免认证接口白名单
_, isLoginReq := msg.(*vai.LoginRequest)
_, isListImageModelsReq := msg.(*vai.ListImageGenerationModelsRequest)

// 如果不在白名单中，则需要进行用户认证
if !isLoginReq && !isListImageModelsReq {
    if err := checkReqHeader(ctx, reqHeader); err != nil {
        return nil
    }
}
```

**说明**：
- ✅ 该接口**不需要用户认证**，可在未登录状态下调用
- ✅ 适用于前端首页展示模型列表等场景

### 3. API 文档
**文件**：`doc/api_list_image_generation_models.md`

**更新内容**：
- 标注接口为公开接口（无需认证）
- 更新请求示例（移除必需的 user_id 和 access_token）
- 添加数据库表结构说明
- 添加新增/禁用模型的 SQL 示例
- 更新注意事项（数据库相关）
- 添加数据迁移指南

---

## 🗄️ 数据库表结构

### 表名
`va_image_generation_model`

### 核心字段
| 字段 | 类型 | 说明 |
|------|------|------|
| `model_name` | varchar(64) | 模型唯一标识 |
| `display_name` | varchar(128) | 显示名称 |
| `description` | varchar(500) | 模型描述 |
| `support_t2i` | tinyint(1) | 是否支持文生图 |
| `support_i2i` | tinyint(1) | 是否支持图生图 |
| `credit_points` | int | 积分消耗 |
| `enabled` | tinyint(1) | 是否启用 |
| `sort` | int | 排序（值越大越靠前） |
| `project_id` | varchar(64) | 项目ID |

### 默认数据
系统自动插入 4 个模型：
1. **Flux Dev** (10 积分)
2. **Flux Schnell** (5 积分)
3. **Stable Diffusion 3** (8 积分)
4. **通义万相** (6 积分)

---

## 🚀 使用方式

### 1. 运行数据库迁移
```bash
migrate -path ./assets/migrations -database "mysql://user:pass@tcp(host:port)/test_zhao" up
```

### 2. 添加新模型
```sql
INSERT INTO `va_image_generation_model`
(`model_name`, `display_name`, `description`, `credit_points`,
 `support_t2i`, `support_i2i`, `enabled`, `sort`, `project_id`)
VALUES
('new-model', '新模型名称', '模型描述', 15, 1, 1, 1, 120, 'visionai');
```

### 3. 禁用模型
```sql
UPDATE `va_image_generation_model`
SET `enabled` = 0
WHERE `model_name` = 'flux-dev';
```

### 4. 调整排序
```sql
UPDATE `va_image_generation_model`
SET `sort` = 150
WHERE `model_name` = 'flux-dev';
```

---

## ✅ 优势

### 与原方案对比

| 对比项 | 原方案（内存配置） | 新方案（数据库） |
|--------|-------------------|-----------------|
| **配置方式** | JSON 文件 / 硬编码 | 数据库表 |
| **修改方式** | 修改代码/配置文件 + 重启 | SQL 语句，无需重启 |
| **权限控制** | 不支持 | 支持项目隔离 |
| **动态管理** | 不支持 | 支持实时增删改 |
| **排序控制** | 代码逻辑 | 数据库 sort 字段 |
| **数据持久化** | 文件 | 数据库 |
| **集群一致性** | 依赖配置同步 | 自动一致 |

### 新方案优势
1. ✅ **实时生效**：修改数据库立即生效，无需重启服务
2. ✅ **集中管理**：所有配置存储在数据库，便于统一管理
3. ✅ **项目隔离**：不同项目可以配置不同的模型列表
4. ✅ **灵活排序**：通过 `sort` 字段动态调整展示顺序
5. ✅ **易于扩展**：新增模型只需插入数据，无需修改代码
6. ✅ **集群友好**：多实例自动同步，无需配置文件分发

---

## 📝 注意事项

1. **首次部署**：需要运行数据库迁移创建表
2. **数据备份**：建议定期备份 `va_image_generation_model` 表
3. **缓存策略**：当前未加缓存，如果查询频繁可以考虑加 Redis 缓存
4. **软删除**：表使用 GORM 的软删除机制，删除的记录仍保留
5. **唯一约束**：`(model_name, project_id)` 联合唯一，避免重复
6. **无需认证**：该接口已配置为公开接口，无需用户登录即可调用
   - 适用于前端首页展示模型列表
   - 方便用户在注册前浏览可用模型
   - 提升用户体验，降低使用门槛

---

## 🔧 相关文件清单

```
genioAI_server/
├── assets/migrations/
│   ├── 20260202190205_create_image_generation_model.up.sql   # 迁移文件
│   └── 20260202190205_create_image_generation_model.down.sql # 回滚文件
├── internal/
│   ├── model/
│   │   └── image_generation_model.go              # Model 层
│   ├── dao/
│   │   └── image_generation_model.go              # DAO 层
│   ├── api/
│   │   └── picture_forge.go                        # API 实现（已修改）
│   └── rpc/
│       └── interceptor.go                          # 认证拦截器（已修改）
└── doc/
    └── api_list_image_generation_models.md         # API 文档（已更新）
```

---

## 🧪 测试建议

### 1. 功能测试

#### 无需认证测试（推荐）
```bash
# 不传递用户信息，测试公开接口
curl -X POST http://localhost:8080/v1/pictureforge/list_image_generation_models \
  -H "Content-Type: application/json" \
  -d '{"request_header": {}}'

# 或使用格式化输出
curl -X POST http://localhost:8080/v1/pictureforge/list_image_generation_models \
  -H "Content-Type: application/json" \
  -d '{"request_header": {}}' | jq .
```

#### 带设备信息测试
```bash
# 传递设备信息（用于统计）
curl -X POST http://localhost:8080/v1/pictureforge/list_image_generation_models \
  -H "Content-Type: application/json" \
  -d '{
    "request_header": {
      "device": {
        "os": "web",
        "language": "zh-CN"
      }
    }
  }' | jq .
```

### 2. 数据库测试
```sql
-- 查看所有模型
SELECT * FROM va_image_generation_model WHERE enabled = 1;

-- 测试添加模型
INSERT INTO va_image_generation_model
  (model_name, display_name, enabled, project_id)
VALUES
  ('test-model', '测试模型', 1, 'visionai');

-- 验证接口返回（应该包含新模型）
```

---

## 📚 相关接口

- **接口文档**：[doc/api_list_image_generation_models.md](../doc/api_list_image_generation_models.md)
- **gRPC 方法**：`PictureForgeService.ListImageGenerationModels`
- **HTTP 地址**：`POST /v1/pictureforge/list_image_generation_models`

---

**变更日期**：2026-02-02
**影响范围**：生图模型列表查询
**兼容性**：向后兼容，API 接口不变
