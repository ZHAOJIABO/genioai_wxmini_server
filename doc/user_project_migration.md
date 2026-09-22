# 用户项目 ID 迁移指南

## 问题描述

当 Web 端使用新的项目 ID `com.web.genioai` 时，如果用户记录仍在旧项目 `com.domob.visionai` 下，会导致以下错误：

```
record not found
SELECT * FROM `va_user_record` WHERE project_id = 'com.domob.visionai' AND (user_id = '...' OR phone_number = '...')
```

## 项目 ID 说明

项目 ID 定义在 `internal/constants/constants.go`:

- `com.domob.visionai` - 旧的默认项目 ID
- `com.web.genioai` - Web 端新项目 ID ✅
- 其他项目 ID（piclib, picflow, visualai 等）

## 解决方案

### 方案1: 迁移用户到新项目（推荐）

为用户在新项目下创建记录，保留原有项目的数据：

```sql
-- 批量迁移所有用户到新项目
INSERT INTO va_user_record (
    user_id,
    project_id,
    user_type,
    status,
    phone_number,
    nickname,
    avatar_url,
    country,
    timezone,
    created_at,
    updated_at
)
SELECT
    user_id,
    'com.web.genioai' as project_id,
    user_type,
    status,
    phone_number,
    nickname,
    avatar_url,
    country,
    timezone,
    NOW() as created_at,
    NOW() as updated_at
FROM va_user_record
WHERE project_id = 'com.domob.visionai'
  AND user_id NOT IN (
      SELECT user_id FROM va_user_record WHERE project_id = 'com.web.genioai'
  );
```

### 方案2: 更新现有用户的项目 ID

直接将用户的项目 ID 更新为新值（慎用，会丢失旧项目关联）：

```sql
-- 仅在确认不需要保留旧项目数据时使用
UPDATE va_user_record
SET project_id = 'com.web.genioai',
    updated_at = NOW()
WHERE project_id = 'com.domob.visionai';
```

### 方案3: 为单个用户添加新项目

```sql
-- 为特定用户添加新项目记录
INSERT INTO va_user_record (
    user_id,
    project_id,
    user_type,
    status,
    created_at,
    updated_at
)
SELECT
    user_id,
    'com.web.genioai',
    user_type,
    status,
    NOW(),
    NOW()
FROM va_user_record
WHERE user_id = '6632b23de9e927fc10f4cca925ed0497'
  AND project_id = 'com.domob.visionai'
LIMIT 1;
```

## 验证迁移结果

```sql
-- 检查迁移后的用户数量
SELECT
    project_id,
    COUNT(*) as user_count
FROM va_user_record
GROUP BY project_id;

-- 检查特定用户的项目记录
SELECT
    user_id,
    project_id,
    user_type,
    status,
    created_at
FROM va_user_record
WHERE user_id = '6632b23de9e927fc10f4cca925ed0497'
ORDER BY project_id;
```

## 前端配置

确保前端发送正确的项目 ID：

```javascript
// Web 端请求头配置
request_header: {
    web_client: {
        package_name: "com.web.genioai",  // ✅ 正确
        client_version: "1.0.0"
    },
    // ... 其他字段
}
```

## 注意事项

1. **备份数据**：执行迁移前务必备份 `va_user_record` 表
2. **唯一约束**：表中有 `(user_id, project_id)` 的唯一约束，同一用户可以在多个项目下存在
3. **关联数据**：迁移后需要检查其他表（如积分、订阅等）是否也需要更新 project_id
4. **测试验证**：在生产环境执行前，先在测试环境验证迁移脚本

## 相关文件

- 项目 ID 常量定义：`internal/constants/constants.go`
- 项目 ID 提取逻辑：`internal/rpc/interceptor.go:216-233`
- 用户 DAO：`internal/dao/user.go`
- 修复脚本：`scripts/fix_user_project.sql`
