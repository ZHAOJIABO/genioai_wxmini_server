-- ============================================
-- Web端邮箱注册/登录功能 - 数据库迁移脚本
-- ============================================
-- 功能: 为用户表添加密码哈希字段，支持邮箱密码登录
-- 日期: 2025-01-22
-- 版本: 1.0.0
-- ============================================

-- 1. 添加密码哈希字段
-- 用于存储使用bcrypt加密的用户密码
ALTER TABLE user_record
ADD COLUMN password_hash VARCHAR(255) DEFAULT ''
COMMENT '密码哈希（使用bcrypt加密，成本因子10）';

-- 2. 添加索引以提升邮箱查询性能
-- 由于邮箱登录需要根据邮箱查询用户，添加索引可以提升性能
CREATE INDEX idx_email ON user_record(email);

-- 3. 为project_id和email组合添加唯一索引
-- 确保同一项目中邮箱唯一（可选，根据业务需求决定）
-- 注意：如果已有重复邮箱数据，需要先清理
-- ALTER TABLE user_record
-- ADD UNIQUE INDEX uk_project_email (project_id, email);

-- ============================================
-- 验证迁移结果
-- ============================================

-- 检查字段是否添加成功
SHOW COLUMNS FROM user_record LIKE 'password_hash';

-- 检查索引是否创建成功
SHOW INDEX FROM user_record WHERE Key_name = 'idx_email';

-- 统计现有用户数量
SELECT
    COUNT(*) as total_users,
    COUNT(DISTINCT email) as users_with_email,
    COUNT(password_hash) as users_with_password,
    COUNT(CASE WHEN password_hash != '' THEN 1 END) as users_with_set_password
FROM user_record;

-- ============================================
-- 回滚脚本（如需回滚，执行以下语句）
-- ============================================

-- -- 删除密码哈希字段
-- ALTER TABLE user_record DROP COLUMN password_hash;
--
-- -- 删除邮箱索引
-- DROP INDEX idx_email ON user_record;
--
-- -- 删除唯一索引（如果创建了）
-- -- ALTER TABLE user_record DROP INDEX uk_project_email;

-- ============================================
-- 数据清理脚本（可选）
-- ============================================

-- 如果需要清理测试数据中的密码
-- UPDATE user_record SET password_hash = '' WHERE password_hash IS NOT NULL;

-- 查询设置了密码的用户
-- SELECT user_id, email, login_type, create_time
-- FROM user_record
-- WHERE password_hash != ''
-- ORDER BY create_time DESC;

-- ============================================
-- 注意事项：
-- ============================================
-- 1. 执行前请备份数据库
-- 2. 建议在测试环境先验证
-- 3. 生产环境执行时选择低峰期
-- 4. password_hash字段使用bcrypt加密，不可逆
-- 5. 如果表数据量大，添加索引可能需要较长时间
-- 6. 唯一索引创建前需确保数据中无重复邮箱
