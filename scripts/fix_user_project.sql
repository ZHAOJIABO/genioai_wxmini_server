-- 修复用户项目 ID 不匹配问题
-- 执行前请先备份数据！

-- 1. 检查当前用户的 project_id
SELECT
    user_id,
    project_id,
    phone_number,
    user_type,
    status,
    created_at
FROM va_user_record
WHERE user_id = '6632b23de9e927fc10f4cca925ed0497'
   OR phone_number = '6632b23de9e927fc10f4cca925ed0497';

-- 2. 如果用户存在但 project_id 是 'com.domob.visionai'，更新为 'com.web.genioai'
-- 取消注释下面的 SQL 来执行更新
/*
UPDATE va_user_record
SET project_id = 'com.web.genioai',
    updated_at = NOW()
WHERE user_id = '6632b23de9e927fc10f4cca925ed0497'
  AND project_id = 'com.domob.visionai';
*/

-- 3. 或者为用户添加新项目的记录（保留原有项目）
-- 取消注释下面的 SQL 来添加新项目记录
/*
INSERT INTO va_user_record (
    user_id,
    project_id,
    user_type,
    status,
    phone_number,
    nickname,
    avatar_url,
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
    NOW() as created_at,
    NOW() as updated_at
FROM va_user_record
WHERE user_id = '6632b23de9e927fc10f4cca925ed0497'
  AND project_id = 'com.domob.visionai'
LIMIT 1;
*/

-- 4. 如果用户完全不存在，创建新用户
-- 取消注释下面的 SQL 来创建新用户
/*
INSERT INTO va_user_record (
    user_id,
    project_id,
    user_type,
    status,
    created_at,
    updated_at
) VALUES (
    '6632b23de9e927fc10f4cca925ed0497',
    'com.web.genioai',
    0,  -- USER_TYPE_NORMAL
    0,  -- 正常状态
    NOW(),
    NOW()
);
*/

-- 5. 验证修复结果
SELECT
    user_id,
    project_id,
    user_type,
    status
FROM va_user_record
WHERE user_id = '6632b23de9e927fc10f4cca925ed0497';
