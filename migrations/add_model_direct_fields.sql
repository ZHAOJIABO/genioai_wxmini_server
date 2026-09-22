-- 添加模型直连模式支持字段
-- Migration: add_model_direct_fields
-- Created: 2026-02-03

-- 为 picture_task 表添加模型直连模式相关字段
ALTER TABLE `picture_task`
ADD COLUMN `task_mode` TINYINT(1) DEFAULT 0 COMMENT '任务模式: 0-workflow模式 1-模型直连模式' AFTER `workflow_type`,
ADD COLUMN `model_name` VARCHAR(64) DEFAULT '' COMMENT '模型名称(模型直连模式)' AFTER `task_mode`,
ADD COLUMN `user_prompt` TEXT COMMENT '用户输入prompt(模型直连模式)' AFTER `model_name`,
ADD COLUMN `negative_prompt` TEXT COMMENT '负面prompt(模型直连模式)' AFTER `user_prompt`,
ADD INDEX `idx_task_mode` (`task_mode`);

-- 说明：
-- 1. task_mode: 0 = workflow模式（默认，兼容历史数据），1 = 模型直连模式
-- 2. model_name: 模型直连模式下使用的模型名称（如 flux-dev, sd3, wanx, kling）
-- 3. user_prompt: 用户输入的生图prompt（模型直连模式）
-- 4. negative_prompt: 负面prompt，用于指定不想要的内容（可选）
-- 5. idx_task_mode 索引：用于按任务模式筛选查询
