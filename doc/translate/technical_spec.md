# 后端语言本地化技术方案

## 1. 概述

本文档详细描述后端语言本地化系统的技术实现方案，包括架构设计、数据模型、接口设计和实现步骤。

### 1.1 目标

- 支持系统内置消息（错误信息、状态消息等）的多语言翻译
- 支持数据库业务数据（产品名称、提示词标题等）的多语言翻译
- 提供高性能的翻译引擎，最小化对请求延迟的影响
- 支持语言：zh（中文）、en（英文）、ru（俄语）、vi（越南语）

### 1.2 设计原则

- **统一存储**：系统消息和业务数据都存储在数据库，单一数据源
- **默认语言**：英文 (en)，翻译缺失时回退到英文
- **缓存优先**：使用 Redis 缓存减少数据库查询
- **启动预热**：系统消息在应用启动时预热到缓存
- **最小侵入**：尽量减少对现有业务代码的修改

---

## 2. 架构设计

### 2.1 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                         gRPC 请求                                │
│                    (Header: device.language)                     │
└─────────────────────────────────┬───────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Interceptor 拦截器                          │
│                  (提取语言，注入 Context)                         │
└─────────────────────────────────┬───────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────┐
│                       翻译引擎 (Translator)                       │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │                      Redis 缓存                             │  │
│  │                                                            │  │
│  │  系统消息: trans:system:error::zh → {field: value}         │  │
│  │  业务数据: trans:entity:product:101:zh → {field: value}    │  │
│  │                                                            │  │
│  │  TTL: 系统消息 24h / 业务数据 1h                            │  │
│  └────────────────────────────────┬───────────────────────────┘  │
│                                   │ cache miss                   │
│                                   ▼                              │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │                    MySQL 数据库                             │  │
│  │                   (va_translation)                          │  │
│  │                                                            │  │
│  │  存储内容:                                                  │  │
│  │  - 系统消息 (category=system): 错误码、状态、模板名          │  │
│  │  - 业务数据 (category=entity): Product、Prompt、Workflow    │  │
│  └────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘

                              启动时
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                        缓存预热 (Warmup)                         │
│                                                                  │
│  从 MySQL 加载所有系统消息 → 写入 Redis (TTL: 24h)               │
└─────────────────────────────────────────────────────────────────┘
```

### 2.2 组件职责

| 组件 | 职责 | 位置 |
|------|------|------|
| Assembler | Model→Proto 转换 + 翻译统一处理 | `internal/assembler/*.go` |
| Translator | 翻译引擎统一入口 | `internal/i18n/translator.go` |
| TranslationDao | 翻译数据访问层 | `internal/dao/translation.go` |
| TranslationCache | Redis 缓存封装 | `internal/i18n/cache.go` |

### 2.3 Assembler 层设计

**为什么需要 Assembler 层？**

| 对比项 | 原方案（API 手动翻译） | Assembler 方案 |
|--------|---------------------|---------------|
| 代码重复 | 每个 API 都要写翻译逻辑 | 统一在 Assembler 中 |
| 容易遗漏 | 新 API 可能忘记翻译 | Assembler 强制处理 |
| 测试 | 需要测试每个 API 的翻译 | 只测试 Assembler |
| 职责分离 | Handler 混合业务+翻译 | Handler 只关心业务 |
| 扩展性 | 修改需要改多处 | 只改 Assembler |

**Assembler 层职责：**

1. **Model → Proto 转换**：将数据库模型转换为 gRPC 响应结构
2. **批量翻译字段**：内部调用 Translator 批量获取翻译
3. **字段默认值处理**：翻译缺失时保留原值

**文件结构：**

```
internal/
├── assembler/                    # Assembler 层
│   ├── base.go                   # 基础接口和工具函数
│   ├── product_assembler.go      # 产品转换器
│   ├── workflow_assembler.go     # 工作流转换器
│   └── prompt_assembler.go       # 提示词转换器
├── i18n/                         # 翻译引擎
│   ├── translator.go
│   ├── cache.go
│   └── warmup.go
└── api/
    └── product.go                # Handler 调用 Assembler
```

---

## 3. 数据模型

### 3.1 数据库表设计

**表名**: `va_translation`

```sql
CREATE TABLE va_translation (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,

    -- Key 组成（分离字段，便于查询和索引）
    category VARCHAR(20) NOT NULL COMMENT '分类: system(系统消息) / entity(业务数据)',
    module VARCHAR(50) NOT NULL COMMENT '模块: error, status, template, product, workflow',
    ref_id VARCHAR(100) NOT NULL DEFAULT '' COMMENT '实体ID，系统消息为空字符串',
    field VARCHAR(50) NOT NULL COMMENT '字段名: invalid_param, name, title, description',

    -- 语言和值
    lang VARCHAR(10) NOT NULL COMMENT '语言代码: zh, en, ru, vi',
    value TEXT NOT NULL COMMENT '翻译后的文本值',

    -- 元数据
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    -- 索引
    UNIQUE KEY uk_translation (category, module, ref_id, field, lang),
    INDEX idx_category_module_lang (category, module, lang)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='多语言翻译数据表';
```

### 3.2 数据分类说明

| category | 说明 | ref_id | 示例 |
|----------|------|--------|------|
| system | 系统消息（错误码、状态、模板名） | 空字符串 | error.invalid_param |
| entity | 业务数据（产品、提示词等） | 实体主键ID | product.101.name |

### 3.3 数据示例

**系统消息**（category=system, ref_id=''）：

| category | module | ref_id | field | lang | value |
|----------|--------|--------|-------|------|-------|
| system | error | | invalid_param | zh | 请求参数错误 |
| system | error | | invalid_param | en | Invalid parameter |
| system | error | | user_blocked | zh | 用户被锁定 |
| system | error | | user_blocked | en | User is blocked |
| system | status | | pending | zh | 待支付 |
| system | status | | pending | en | Pending |
| system | template | | diving | zh | 跳水 |
| system | template | | diving | en | Diving |

**业务数据**（category=entity, ref_id=实体ID）：

| category | module | ref_id | field | lang | value |
|----------|--------|--------|-------|------|-------|
| entity | product | 101 | name | zh | 年度会员 |
| entity | product | 101 | name | en | Annual Membership |
| entity | product | 101 | description | zh | 365天VIP权益 |
| entity | product | 101 | description | en | 365 days VIP benefits |
| entity | workflow | 201 | title | zh | 人物肖像 |
| entity | workflow | 201 | title | en | Portrait |

### 3.4 Model 定义

**文件**: `internal/model/translation.go`

```go
package model

import "time"

// Translation 翻译数据模型
type Translation struct {
    ID        uint64    `gorm:"primaryKey;autoIncrement"`
    Category  string    `gorm:"column:category;type:varchar(20);not null"`
    Module    string    `gorm:"column:module;type:varchar(50);not null"`
    RefID     string    `gorm:"column:ref_id;type:varchar(100);not null;default:''"`
    Field     string    `gorm:"column:field;type:varchar(50);not null"`
    Lang      string    `gorm:"column:lang;type:varchar(10);not null"`
    Value     string    `gorm:"column:value;type:text;not null"`
    CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
    UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (Translation) TableName() string {
    return "va_translation"
}

// 分类常量
const (
    CategorySystem = "system" // 系统消息
    CategoryEntity = "entity" // 业务数据
)

// 系统消息模块常量
const (
    ModuleError    = "error"    // 错误消息
    ModuleStatus   = "status"   // 状态消息
    ModuleTemplate = "template" // 模板名称
)
```

---

## 4. 接口设计

### 4.1 Assembler 基础接口

**文件**: `internal/assembler/base.go`

```go
package assembler

import "va_visionai_server/internal/i18n"

// TranslatableAssembler 可翻译的组装器基础接口
type TranslatableAssembler interface {
    SetTranslator(translator *i18n.Translator)
}

// BaseAssembler 基础组装器
type BaseAssembler struct {
    translator *i18n.Translator
}

// NewBaseAssembler 创建基础组装器
func NewBaseAssembler(translator *i18n.Translator) BaseAssembler {
    return BaseAssembler{translator: translator}
}

// SetTranslator 设置翻译器
func (a *BaseAssembler) SetTranslator(translator *i18n.Translator) {
    a.translator = translator
}

// applyTranslation 应用翻译的通用辅助函数
// 如果翻译存在且非空，则覆盖目标值
func applyTranslation(target *string, trans map[string]string, field string) {
    if trans == nil {
        return
    }
    if v, ok := trans[field]; ok && v != "" {
        *target = v
    }
}

// extractIDs 提取 ID 列表的辅助函数
func extractIDs[T any](items []T, getID func(T) string) []string {
    ids := make([]string, len(items))
    for i, item := range items {
        ids[i] = getID(item)
    }
    return ids
}
```

### 4.2 ProductAssembler 实现

**文件**: `internal/assembler/product_assembler.go`

```go
package assembler

import (
    "context"
    "fmt"

    "va_visionai_server/internal/i18n"
    "va_visionai_server/internal/model"
    "va_visionai_server/proto/vai"
)

// ProductAssembler 产品组装器
type ProductAssembler struct {
    BaseAssembler
}

// NewProductAssembler 创建产品组装器
func NewProductAssembler(translator *i18n.Translator) *ProductAssembler {
    return &ProductAssembler{
        BaseAssembler: NewBaseAssembler(translator),
    }
}

// ToProtoList 批量转换产品（带翻译）
func (a *ProductAssembler) ToProtoList(ctx context.Context, models []*model.Product) []*vai.Product {
    if len(models) == 0 {
        return nil
    }

    // 1. 提取 ID
    ids := extractIDs(models, func(m *model.Product) string {
        return fmt.Sprintf("%d", m.ID)
    })

    // 2. 批量获取翻译
    translations := a.translator.TDBatch(ctx, "product", ids, []string{"name", "description"})

    // 3. 转换
    result := make([]*vai.Product, len(models))
    for i, m := range models {
        refID := fmt.Sprintf("%d", m.ID)
        result[i] = a.ToProto(m, translations[refID])
    }

    return result
}

// ToProto 单个转换产品（带翻译）
func (a *ProductAssembler) ToProto(m *model.Product, trans map[string]string) *vai.Product {
    p := &vai.Product{
        Id:          m.ID,
        Name:        m.Name,        // 默认值（数据库原值）
        Description: m.Description, // 默认值
        Price:       m.Price,
        // ... 其他字段
    }

    // 应用翻译覆盖
    applyTranslation(&p.Name, trans, "name")
    applyTranslation(&p.Description, trans, "description")

    return p
}

// ToProtoSingle 单个转换产品（自动获取翻译）
// 用于只需要转换单个产品的场景
func (a *ProductAssembler) ToProtoSingle(ctx context.Context, m *model.Product) *vai.Product {
    refID := fmt.Sprintf("%d", m.ID)
    trans := a.translator.TDFields(ctx, "product", refID, []string{"name", "description"})
    return a.ToProto(m, trans)
}
```

### 4.3 WorkflowAssembler 实现

**文件**: `internal/assembler/workflow_assembler.go`

```go
package assembler

import (
    "context"
    "fmt"

    "va_visionai_server/internal/i18n"
    "va_visionai_server/internal/model"
    "va_visionai_server/proto/vai"
)

// WorkflowAssembler 工作流组装器
type WorkflowAssembler struct {
    BaseAssembler
}

// NewWorkflowAssembler 创建工作流组装器
func NewWorkflowAssembler(translator *i18n.Translator) *WorkflowAssembler {
    return &WorkflowAssembler{
        BaseAssembler: NewBaseAssembler(translator),
    }
}

// ToProtoList 批量转换工作流（带翻译）
func (a *WorkflowAssembler) ToProtoList(ctx context.Context, models []*model.Workflow) []*vai.Workflow {
    if len(models) == 0 {
        return nil
    }

    // 1. 提取 ID
    ids := extractIDs(models, func(m *model.Workflow) string {
        return fmt.Sprintf("%d", m.ID)
    })

    // 2. 批量获取翻译
    translations := a.translator.TDBatch(ctx, "workflow", ids, []string{"title", "description"})

    // 3. 转换
    result := make([]*vai.Workflow, len(models))
    for i, m := range models {
        refID := fmt.Sprintf("%d", m.ID)
        result[i] = a.ToProto(m, translations[refID])
    }

    return result
}

// ToProto 单个转换工作流（带翻译）
func (a *WorkflowAssembler) ToProto(m *model.Workflow, trans map[string]string) *vai.Workflow {
    w := &vai.Workflow{
        Id:          m.ID,
        Title:       m.Title,       // 默认值
        Description: m.Description, // 默认值
        // ... 其他字段
    }

    // 应用翻译覆盖
    applyTranslation(&w.Title, trans, "title")
    applyTranslation(&w.Description, trans, "description")

    return w
}
```

### 4.4 Translator 接口

**文件**: `internal/i18n/translator.go`

```go
package i18n

import (
    "context"
    "fmt"

    "va_visionai_server/internal/constants"
)

// Translator 翻译器
type Translator struct {
    dao   *TranslationDao
    cache *TranslationCache
}

// NewTranslator 创建翻译器
func NewTranslator(dao *TranslationDao, cache *TranslationCache) *Translator {
    return &Translator{
        dao:   dao,
        cache: cache,
    }
}

// T 翻译系统消息
// 用法: translator.T(ctx, "error", "invalid_param") → "请求参数错误"
func (t *Translator) T(ctx context.Context, module, field string) string {
    lang := t.GetLang(ctx)
    return t.get(ctx, CategorySystem, module, "", field, lang)
}

// TD 翻译业务数据单字段
// 用法: translator.TD(ctx, "product", "101", "name") → "年度会员"
func (t *Translator) TD(ctx context.Context, module, refID, field string) string {
    lang := t.GetLang(ctx)
    return t.get(ctx, CategoryEntity, module, refID, field, lang)
}

// TDFields 翻译业务数据多字段
// 用法: translator.TDFields(ctx, "product", "101", []string{"name", "description"})
func (t *Translator) TDFields(ctx context.Context, module, refID string, fields []string) map[string]string {
    lang := t.GetLang(ctx)
    return t.getFields(ctx, CategoryEntity, module, refID, fields, lang)
}

// TDBatch 批量翻译业务数据
// 用法: translator.TDBatch(ctx, "product", []string{"101", "102"}, []string{"name", "description"})
func (t *Translator) TDBatch(ctx context.Context, module string, refIDs, fields []string) map[string]map[string]string {
    lang := t.GetLang(ctx)
    result := make(map[string]map[string]string)

    for _, refID := range refIDs {
        translations := t.getFields(ctx, CategoryEntity, module, refID, fields, lang)
        if len(translations) > 0 {
            result[refID] = translations
        }
    }

    return result
}

// GetLang 从 Context 获取当前语言
func (t *Translator) GetLang(ctx context.Context) string {
    if lang, ok := ctx.Value(constants.CtxLang).(string); ok && lang != "" {
        return lang
    }
    return "en" // 默认英文
}

// get 获取单个翻译
func (t *Translator) get(ctx context.Context, category, module, refID, field, lang string) string {
    fields := t.getFields(ctx, category, module, refID, []string{field}, lang)
    if val, ok := fields[field]; ok {
        return val
    }
    return ""
}

// getFields 获取多个字段的翻译
func (t *Translator) getFields(ctx context.Context, category, module, refID string, fields []string, lang string) map[string]string {
    // 1. 查缓存
    cacheKey := t.cache.BuildKey(category, module, refID, lang)
    if cached := t.cache.Get(ctx, cacheKey); cached != nil {
        return filterFields(cached, fields)
    }

    // 2. 查数据库
    translations, err := t.dao.GetByKey(ctx, category, module, refID, lang)
    if err != nil {
        return nil
    }

    // 3. 写入缓存
    if len(translations) > 0 {
        ttl := t.cache.GetTTL(category)
        t.cache.Set(ctx, cacheKey, translations, ttl)
    }

    result := filterFields(translations, fields)

    // 4. 回退到英文
    if len(result) < len(fields) && lang != "en" {
        enFields := t.getFields(ctx, category, module, refID, fields, "en")
        for _, f := range fields {
            if _, ok := result[f]; !ok {
                if val, ok := enFields[f]; ok {
                    result[f] = val
                }
            }
        }
    }

    return result
}

// filterFields 过滤需要的字段
func filterFields(all map[string]string, fields []string) map[string]string {
    if len(fields) == 0 {
        return all
    }
    result := make(map[string]string)
    for _, f := range fields {
        if val, ok := all[f]; ok {
            result[f] = val
        }
    }
    return result
}
```

### 4.2 TranslationDao

**文件**: `internal/dao/translation.go`

```go
package dao

import (
    "context"

    "gorm.io/gorm"
    "va_visionai_server/internal/model"
)

// TranslationDao 翻译数据访问层
type TranslationDao struct {
    db *gorm.DB
}

// NewTranslationDao 创建 DAO
func NewTranslationDao(db *gorm.DB) *TranslationDao {
    return &TranslationDao{db: db}
}

// GetByKey 根据 Key 获取翻译（返回该 Key 下所有字段）
func (d *TranslationDao) GetByKey(ctx context.Context, category, module, refID, lang string) (map[string]string, error) {
    var translations []model.Translation

    err := d.db.WithContext(ctx).
        Where("category = ? AND module = ? AND ref_id = ? AND lang = ?", category, module, refID, lang).
        Find(&translations).Error

    if err != nil {
        return nil, err
    }

    result := make(map[string]string)
    for _, t := range translations {
        result[t.Field] = t.Value
    }

    return result, nil
}

// ListByCategory 列出某分类某模块的所有翻译
func (d *TranslationDao) ListByCategory(ctx context.Context, category, module, lang string) ([]model.Translation, error) {
    var translations []model.Translation

    err := d.db.WithContext(ctx).
        Where("category = ? AND module = ? AND lang = ?", category, module, lang).
        Find(&translations).Error

    return translations, err
}

// Upsert 插入或更新翻译
func (d *TranslationDao) Upsert(ctx context.Context, t *model.Translation) error {
    return d.db.WithContext(ctx).
        Where("category = ? AND module = ? AND ref_id = ? AND field = ? AND lang = ?",
            t.Category, t.Module, t.RefID, t.Field, t.Lang).
        Assign(model.Translation{Value: t.Value}).
        FirstOrCreate(t).Error
}

// Delete 删除翻译
func (d *TranslationDao) Delete(ctx context.Context, category, module, refID string) error {
    return d.db.WithContext(ctx).
        Where("category = ? AND module = ? AND ref_id = ?", category, module, refID).
        Delete(&model.Translation{}).Error
}
```

### 4.3 TranslationCache

**文件**: `internal/i18n/cache.go`

```go
package i18n

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/go-redis/redis"
)

const (
    cacheKeyPrefix    = "trans:"
    systemCacheTTL    = 24 * time.Hour // 系统消息 24 小时
    entityCacheTTL    = time.Hour      // 业务数据 1 小时
)

// TranslationCache 翻译缓存
type TranslationCache struct {
    client *redis.Client
}

// NewTranslationCache 创建缓存
func NewTranslationCache(client *redis.Client) *TranslationCache {
    return &TranslationCache{client: client}
}

// BuildKey 构建缓存键
// 格式: trans:{category}:{module}:{ref_id}:{lang}
func (c *TranslationCache) BuildKey(category, module, refID, lang string) string {
    return fmt.Sprintf("%s%s:%s:%s:%s", cacheKeyPrefix, category, module, refID, lang)
}

// Get 获取缓存
func (c *TranslationCache) Get(ctx context.Context, key string) map[string]string {
    data, err := c.client.Get(key).Bytes()
    if err != nil {
        return nil
    }

    var result map[string]string
    if json.Unmarshal(data, &result) != nil {
        return nil
    }

    return result
}

// Set 设置缓存
func (c *TranslationCache) Set(ctx context.Context, key string, value map[string]string, ttl time.Duration) error {
    data, err := json.Marshal(value)
    if err != nil {
        return err
    }

    return c.client.Set(key, data, ttl).Err()
}

// GetTTL 根据分类获取 TTL
func (c *TranslationCache) GetTTL(category string) time.Duration {
    if category == CategorySystem {
        return systemCacheTTL
    }
    return entityCacheTTL
}

// Invalidate 清除缓存
func (c *TranslationCache) Invalidate(ctx context.Context, category, module, refID string) error {
    pattern := fmt.Sprintf("%s%s:%s:%s:*", cacheKeyPrefix, category, module, refID)
    keys, err := c.client.Keys(pattern).Result()
    if err != nil {
        return err
    }
    if len(keys) > 0 {
        return c.client.Del(keys...).Err()
    }
    return nil
}

// InvalidateAll 清除所有翻译缓存
func (c *TranslationCache) InvalidateAll(ctx context.Context) error {
    pattern := cacheKeyPrefix + "*"
    keys, err := c.client.Keys(pattern).Result()
    if err != nil {
        return err
    }
    if len(keys) > 0 {
        return c.client.Del(keys...).Err()
    }
    return nil
}
```

### 4.4 缓存预热

**文件**: `internal/i18n/warmup.go`

```go
package i18n

import (
    "context"

    "go.uber.org/zap"
    "va_visionai_server/internal/model"
    "va_visionai_server/internal/zlog"
)

// Warmup 预热系统消息缓存
// 在应用启动时调用
func (t *Translator) Warmup(ctx context.Context) error {
    logger := zlog.LogWithContext(ctx)
    logger.Info("Starting translation cache warmup...")

    langs := []string{"zh", "en", "ru", "vi"}
    modules := []string{model.ModuleError, model.ModuleStatus, model.ModuleTemplate}

    for _, lang := range langs {
        for _, module := range modules {
            // 从数据库加载
            translations, err := t.dao.ListByCategory(ctx, model.CategorySystem, module, lang)
            if err != nil {
                logger.Error("Failed to load translations",
                    zap.String("module", module),
                    zap.String("lang", lang),
                    zap.Error(err))
                continue
            }

            if len(translations) == 0 {
                continue
            }

            // 组装为 map
            data := make(map[string]string)
            for _, tr := range translations {
                data[tr.Field] = tr.Value
            }

            // 写入缓存
            cacheKey := t.cache.BuildKey(model.CategorySystem, module, "", lang)
            if err := t.cache.Set(ctx, cacheKey, data, systemCacheTTL); err != nil {
                logger.Error("Failed to cache translations",
                    zap.String("module", module),
                    zap.String("lang", lang),
                    zap.Error(err))
                continue
            }

            logger.Info("Cached translations",
                zap.String("module", module),
                zap.String("lang", lang),
                zap.Int("count", len(data)))
        }
    }

    logger.Info("Translation cache warmup completed")
    return nil
}
```

---

## 5. 集成方案

### 5.1 错误消息集成

**修改文件**: `internal/constants/constants.go`

```go
// 错误消息 Key 映射（用于从 StatusCode 查找翻译 key）
var ErrMsgFieldMap = map[vai.StatusCode]string{
    vai.StatusCode_SUCCESS:                  "success",
    vai.StatusCode_INVALID_PARAM:            "invalid_param",
    vai.StatusCode_INVALID_REQUEST:          "invalid_request",
    vai.StatusCode_REQUEST_FAILED:           "request_failed",
    vai.StatusCode_INVALID_ACCESS_TOKEN:     "invalid_access_token",
    vai.StatusCode_EXPIRED_ACCESS_TOKEN:     "expired_access_token",
    vai.StatusCode_INVALID_USER:             "invalid_user",
    vai.StatusCode_USER_BLOCKED:             "user_blocked",
    vai.StatusCode_AMOUNT_EXHAUSTED:         "amount_exhausted",
    vai.StatusCode_SEND_MESSAGE_TOO_OFTEN:   "send_message_too_often",
    vai.StatusCode_FILE_TOO_LARGE:           "file_too_large",
    vai.StatusCode_CONTENT_SAFE_POLICY:      "content_safe_policy",
    vai.StatusCode_USERNAME_EXISTS:          "username_exists",
    // ... 其他状态码
}

// GetErrorMessage 获取翻译后的错误消息
func GetErrorMessage(ctx context.Context, code vai.StatusCode, translator *i18n.Translator) string {
    if field, ok := ErrMsgFieldMap[code]; ok {
        return translator.T(ctx, "error", field)
    }
    return translator.T(ctx, "error", "request_failed")
}
```

### 5.2 响应头构建集成

**修改文件**: `internal/common/response.go`

```go
// BuildHeader 构建响应头（带翻译）
func BuildHeader(ctx context.Context, code vai.StatusCode, translator *i18n.Translator) *vai.ResponseHeader {
    return &vai.ResponseHeader{
        Code: code,
        Msg:  constants.GetErrorMessage(ctx, code, translator),
    }
}
```

### 5.3 业务数据翻译集成（Assembler 方式）

**示例**: `internal/api/product.go`

```go
type ProductServer struct {
    productService   *service.ProductService
    productAssembler *assembler.ProductAssembler  // 注入 Assembler
}

func NewProductServer(
    productService *service.ProductService,
    productAssembler *assembler.ProductAssembler,
) *ProductServer {
    return &ProductServer{
        productService:   productService,
        productAssembler: productAssembler,
    }
}

func (s *ProductServer) ListProducts(ctx context.Context, req *vai.ListProductsRequest) (*vai.ListProductsResponse, error) {
    // 1. 业务逻辑
    products, err := s.productService.List(ctx)
    if err != nil {
        return BuildErrorResponse[vai.ListProductsResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "failed to list products")
    }

    // 2. 使用 Assembler 转换（内部自动处理翻译）
    protoProducts := s.productAssembler.ToProtoList(ctx, products)

    // 3. 构建响应
    return BuildSuccessResponse(&vai.ListProductsResponse{
        Products: protoProducts,
    })
}

func (s *ProductServer) GetProduct(ctx context.Context, req *vai.GetProductRequest) (*vai.GetProductResponse, error) {
    // 1. 业务逻辑
    product, err := s.productService.GetByID(ctx, req.ProductId)
    if err != nil {
        return BuildErrorResponse[vai.GetProductResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "product not found")
    }

    // 2. 使用 Assembler 转换单个
    protoProduct := s.productAssembler.ToProtoSingle(ctx, product)

    // 3. 构建响应
    return BuildSuccessResponse(&vai.GetProductResponse{
        Product: protoProduct,
    })
}
```

**对比：Assembler 方式 vs 手动翻译方式**

```go
// ❌ 手动翻译方式（每个 API 都要写翻译逻辑）
func (s *ProductServer) ListProducts(ctx context.Context, req *vai.Request) (*vai.Response, error) {
    products := s.productService.List(ctx)

    // 每个 API 都要写这些重复代码
    productIDs := make([]string, len(products))
    for i, p := range products {
        productIDs[i] = fmt.Sprintf("%d", p.ID)
    }
    translations := s.translator.TDBatch(ctx, "product", productIDs, []string{"name", "description"})

    var result []*vai.Product
    for _, p := range products {
        item := &vai.Product{Id: p.ID, Name: p.Name, Description: p.Description}
        refID := fmt.Sprintf("%d", p.ID)
        if t, ok := translations[refID]; ok {
            if name, ok := t["name"]; ok && name != "" { item.Name = name }
            if desc, ok := t["description"]; ok && desc != "" { item.Description = desc }
        }
        result = append(result, item)
    }

    return BuildSuccessResponse(&vai.ListProductsResponse{Products: result})
}

// ✅ Assembler 方式（简洁、统一）
func (s *ProductServer) ListProducts(ctx context.Context, req *vai.Request) (*vai.Response, error) {
    products := s.productService.List(ctx)

    // 一行代码完成转换 + 翻译
    protoProducts := s.productAssembler.ToProtoList(ctx, products)

    return BuildSuccessResponse(&vai.ListProductsResponse{Products: protoProducts})
}
```

### 5.4 应用启动集成

**修改文件**: `cmd/main.go`

```go
func main() {
    // ... 初始化数据库、Redis 等

    // 创建翻译器
    translationDao := dao.NewTranslationDao(db)
    translationCache := i18n.NewTranslationCache(redisClient)
    translator := i18n.NewTranslator(translationDao, translationCache)

    // 预热系统消息缓存
    if err := translator.Warmup(context.Background()); err != nil {
        log.Printf("Translation warmup failed: %v", err)
    }

    // ... 启动服务
}
```

---

## 6. 数据迁移

### 6.1 创建翻译表

**文件**: `assets/migrations/YYYYMMDDHHMMSS_add_translation_table.up.sql`

```sql
CREATE TABLE IF NOT EXISTS va_translation (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    category VARCHAR(20) NOT NULL COMMENT '分类: system / entity',
    module VARCHAR(50) NOT NULL COMMENT '模块名',
    ref_id VARCHAR(100) NOT NULL DEFAULT '' COMMENT '实体ID',
    field VARCHAR(50) NOT NULL COMMENT '字段名',
    lang VARCHAR(10) NOT NULL COMMENT '语言代码',
    value TEXT NOT NULL COMMENT '翻译值',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY uk_translation (category, module, ref_id, field, lang),
    INDEX idx_category_module_lang (category, module, lang)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='多语言翻译数据表';
```

### 6.2 初始化系统消息

**文件**: `assets/migrations/YYYYMMDDHHMMSS_init_system_translations.up.sql`

```sql
-- 错误消息 (中文)
INSERT INTO va_translation (category, module, ref_id, field, lang, value) VALUES
('system', 'error', '', 'success', 'zh', '成功'),
('system', 'error', '', 'invalid_param', 'zh', '请求参数错误'),
('system', 'error', '', 'invalid_request', 'zh', '非法请求'),
('system', 'error', '', 'request_failed', 'zh', '请求失败'),
('system', 'error', '', 'invalid_access_token', 'zh', 'AccessToken错误'),
('system', 'error', '', 'expired_access_token', 'zh', 'AccessToken过期'),
('system', 'error', '', 'invalid_user', 'zh', '无效的用户'),
('system', 'error', '', 'user_blocked', 'zh', '用户被锁定'),
('system', 'error', '', 'amount_exhausted', 'zh', '积分消耗完毕'),
('system', 'error', '', 'send_message_too_often', 'zh', '发送消息过于频繁'),
('system', 'error', '', 'file_too_large', 'zh', '文件过大'),
('system', 'error', '', 'content_safe_policy', 'zh', '内容敏感'),
('system', 'error', '', 'username_exists', 'zh', '用户名已存在')
ON DUPLICATE KEY UPDATE value = VALUES(value);

-- 错误消息 (英文)
INSERT INTO va_translation (category, module, ref_id, field, lang, value) VALUES
('system', 'error', '', 'success', 'en', 'Success'),
('system', 'error', '', 'invalid_param', 'en', 'Invalid parameter'),
('system', 'error', '', 'invalid_request', 'en', 'Invalid request'),
('system', 'error', '', 'request_failed', 'en', 'Request failed'),
('system', 'error', '', 'invalid_access_token', 'en', 'Invalid access token'),
('system', 'error', '', 'expired_access_token', 'en', 'Access token expired'),
('system', 'error', '', 'invalid_user', 'en', 'Invalid user'),
('system', 'error', '', 'user_blocked', 'en', 'User is blocked'),
('system', 'error', '', 'amount_exhausted', 'en', 'Credits exhausted'),
('system', 'error', '', 'send_message_too_often', 'en', 'Sending messages too frequently'),
('system', 'error', '', 'file_too_large', 'en', 'File too large'),
('system', 'error', '', 'content_safe_policy', 'en', 'Content contains sensitive information'),
('system', 'error', '', 'username_exists', 'en', 'Username already exists')
ON DUPLICATE KEY UPDATE value = VALUES(value);

-- 订单状态 (中文)
INSERT INTO va_translation (category, module, ref_id, field, lang, value) VALUES
('system', 'status', '', 'pending', 'zh', '待支付'),
('system', 'status', '', 'processing', 'zh', '支付中'),
('system', 'status', '', 'paid', 'zh', '支付成功'),
('system', 'status', '', 'payment_failed', 'zh', '支付失败'),
('system', 'status', '', 'cancelled', 'zh', '已取消'),
('system', 'status', '', 'activated', 'zh', '订阅激活'),
('system', 'status', '', 'expired', 'zh', '订阅到期'),
('system', 'status', '', 'refunded', 'zh', '已退款')
ON DUPLICATE KEY UPDATE value = VALUES(value);

-- 订单状态 (英文)
INSERT INTO va_translation (category, module, ref_id, field, lang, value) VALUES
('system', 'status', '', 'pending', 'en', 'Pending'),
('system', 'status', '', 'processing', 'en', 'Processing'),
('system', 'status', '', 'paid', 'en', 'Paid'),
('system', 'status', '', 'payment_failed', 'en', 'Payment Failed'),
('system', 'status', '', 'cancelled', 'en', 'Cancelled'),
('system', 'status', '', 'activated', 'en', 'Activated'),
('system', 'status', '', 'expired', 'en', 'Expired'),
('system', 'status', '', 'refunded', 'en', 'Refunded')
ON DUPLICATE KEY UPDATE value = VALUES(value);

-- 模板名称 (中文)
INSERT INTO va_translation (category, module, ref_id, field, lang, value) VALUES
('system', 'template', '', 'diving', 'zh', '跳水'),
('system', 'template', '', 'rings', 'zh', '吊环'),
('system', 'template', '', 'pubg', 'zh', '绝地求生'),
('system', 'template', '', 'labubu', 'zh', '万物皆可labubu')
ON DUPLICATE KEY UPDATE value = VALUES(value);

-- 模板名称 (英文)
INSERT INTO va_translation (category, module, ref_id, field, lang, value) VALUES
('system', 'template', '', 'diving', 'en', 'Diving'),
('system', 'template', '', 'rings', 'en', 'Rings'),
('system', 'template', '', 'pubg', 'en', 'PUBG'),
('system', 'template', '', 'labubu', 'en', 'Labubu')
ON DUPLICATE KEY UPDATE value = VALUES(value);
```

### 6.3 迁移业务数据

**文件**: `assets/migrations/YYYYMMDDHHMMSS_migrate_entity_translations.up.sql`

```sql
-- 迁移产品数据的中文翻译
INSERT INTO va_translation (category, module, ref_id, field, lang, value)
SELECT 'entity', 'product', CAST(id AS CHAR), 'name', 'zh', name
FROM va_product
WHERE name IS NOT NULL AND name != ''
ON DUPLICATE KEY UPDATE value = VALUES(value);

INSERT INTO va_translation (category, module, ref_id, field, lang, value)
SELECT 'entity', 'product', CAST(id AS CHAR), 'description', 'zh', description
FROM va_product
WHERE description IS NOT NULL AND description != ''
ON DUPLICATE KEY UPDATE value = VALUES(value);

-- 英文翻译需要单独添加
```

---

## 7. 实现步骤

### Phase 1: 基础设施

1. 创建 `internal/i18n/` 包目录结构
2. 实现 `internal/model/translation.go` - 数据模型
3. 实现 `internal/dao/translation.go` - 数据访问层
4. 实现 `internal/i18n/cache.go` - 缓存封装
5. 实现 `internal/i18n/translator.go` - 翻译器
6. 实现 `internal/i18n/warmup.go` - 缓存预热
7. 创建数据库迁移脚本

### Phase 2: Assembler 层

1. 创建 `internal/assembler/` 包目录结构
2. 实现 `internal/assembler/base.go` - 基础接口和工具函数
3. 实现 `internal/assembler/product_assembler.go` - 产品组装器
4. 实现 `internal/assembler/workflow_assembler.go` - 工作流组装器
5. 实现 `internal/assembler/prompt_assembler.go` - 提示词组装器

### Phase 3: 集成翻译

1. 修改 `constants/constants.go` - 添加错误消息 Key 映射
2. 修改 `common/response.go` - 响应头构建集成翻译器
3. 修改 `bootstrap/service_provider.go` - 注入 Translator 和 Assembler 依赖
4. 修改 `cmd/main.go` - 启动时预热缓存

### Phase 4: 数据迁移

1. 执行建表迁移脚本
2. 执行系统消息初始化脚本
3. 执行业务数据迁移脚本
4. 添加英文翻译数据

### Phase 5: API 集成

1. 修改 Product 相关 API - 注入 ProductAssembler，使用 Assembler 转换
2. 修改 Workflow 相关 API - 注入 WorkflowAssembler，使用 Assembler 转换
3. 修改 Prompt 相关 API - 注入 PromptAssembler，使用 Assembler 转换

### Phase 6: 测试验证

1. 编写单元测试（Translator、Assembler）
2. 编写集成测试
3. 端到端验证
4. 性能测试

---

## 8. 文件清单

### 新建文件

| 文件路径 | 说明 |
|---------|------|
| `internal/assembler/base.go` | Assembler 基础接口和工具函数 |
| `internal/assembler/product_assembler.go` | 产品组装器（转换 + 翻译） |
| `internal/assembler/workflow_assembler.go` | 工作流组装器（转换 + 翻译） |
| `internal/assembler/prompt_assembler.go` | 提示词组装器（转换 + 翻译） |
| `internal/i18n/translator.go` | 翻译器实现 |
| `internal/i18n/cache.go` | 缓存封装 |
| `internal/i18n/warmup.go` | 缓存预热 |
| `internal/model/translation.go` | 数据模型 |
| `internal/dao/translation.go` | 数据访问层 |
| `assets/migrations/xxx_add_translation_table.up.sql` | 建表迁移 |
| `assets/migrations/xxx_init_system_translations.up.sql` | 系统消息初始化 |
| `assets/migrations/xxx_migrate_entity_translations.up.sql` | 业务数据迁移 |

### 修改文件

| 文件路径 | 修改内容 |
|---------|---------|
| `internal/constants/constants.go` | 添加错误消息 Key 映射 |
| `internal/common/response.go` | BuildHeader 集成翻译器 |
| `internal/bootstrap/service_provider.go` | 注入 Translator 和 Assembler 依赖 |
| `internal/db/mysql.go` | 注册 Translation 模型迁移 |
| `cmd/main.go` | 启动时预热缓存 |
| `internal/api/product.go` | 注入 ProductAssembler，使用 Assembler 转换 |
| `internal/api/workflow.go` | 注入 WorkflowAssembler，使用 Assembler 转换 |
| `internal/api/prompt.go` | 注入 PromptAssembler，使用 Assembler 转换 |

---

## 9. 风险与注意事项

### 9.1 启动依赖

- **风险**: 应用启动需要数据库和 Redis 可用
- **缓解**: 这对于大多数服务来说已经是必须的；预热失败不阻塞启动

### 9.2 缓存预热

- **风险**: 预热失败导致系统消息查询走数据库
- **缓解**: 预热失败记录日志但不阻塞启动；首次查询会自动缓存

### 9.3 缓存一致性

- **风险**: 更新翻译后缓存未及时清除
- **缓解**: 提供 InvalidateCache 方法；更新翻译时调用清除缓存

### 9.4 回退逻辑

- **风险**: 翻译缺失导致显示空白
- **缓解**: 多级回退：指定语言 → 英文 → 数据库原值

---

## 10. 附录

### 10.1 支持的语言列表

| 代码 | 语言 | 优先级 |
|------|------|--------|
| en | 英文 | 默认/回退 |
| zh | 中文 | 支持 |
| ru | 俄语 | 支持 |
| vi | 越南语 | 支持 |

### 10.2 需要翻译的模块统计

| category | module | 说明 | 预估数据量 |
|----------|--------|------|-----------|
| system | error | 错误消息 | ~54 条 × 4 语言 |
| system | status | 订单状态 | ~13 条 × 4 语言 |
| system | template | 模板名称 | ~15 条 × 4 语言 |
| entity | product | 产品 | ~20 条 × 4 语言 |
| entity | prompt | 提示词 | ~100 条 × 4 语言 |
| entity | workflow | 工作流 | ~15 条 × 4 语言 |

### 10.3 Redis Key 格式

```
trans:{category}:{module}:{ref_id}:{lang}

示例：
trans:system:error::zh           → {"invalid_param": "请求参数错误", ...}
trans:system:status::en          → {"pending": "Pending", ...}
trans:entity:product:101:zh      → {"name": "年度会员", "description": "365天VIP权益"}
trans:entity:workflow:201:en     → {"title": "Portrait"}
```
