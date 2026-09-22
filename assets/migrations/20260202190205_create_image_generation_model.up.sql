-- 创建生图模型表
CREATE TABLE IF NOT EXISTS `va_image_generation_model` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `model_name` varchar(64) NOT NULL COMMENT '模型名称标识（如 flux-dev, sd3, wanx）',
  `display_name` varchar(128) NOT NULL COMMENT '显示名称',
  `description` varchar(500) DEFAULT NULL COMMENT '模型描述',
  `icon` varchar(255) DEFAULT NULL COMMENT '模型图标URL',
  `provider` varchar(64) DEFAULT NULL COMMENT '执行器/提供商（如 comfyui, kling, gpt4o）',
  `support_t2i` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否支持文生图',
  `support_i2i` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否支持图生图',
  `credit_points` int(11) NOT NULL DEFAULT '0' COMMENT '积分消耗',
  `default_width` int(11) NOT NULL DEFAULT '1024' COMMENT '默认宽度',
  `default_height` int(11) NOT NULL DEFAULT '1024' COMMENT '默认高度',
  `support_qualities` varchar(255) DEFAULT NULL COMMENT '支持的质量选项（JSON数组字符串）',
  `max_images` int(11) NOT NULL DEFAULT '1' COMMENT '最大生成图片数',
  `enabled` tinyint(1) NOT NULL DEFAULT '1' COMMENT '是否启用',
  `sort` int(11) NOT NULL DEFAULT '0' COMMENT '排序（值越大越靠前）',
  `project_id` varchar(64) NOT NULL DEFAULT 'visionai' COMMENT '项目ID',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_model_name` (`model_name`,`project_id`),
  KEY `idx_deleted_at` (`deleted_at`),
  KEY `idx_enabled` (`enabled`),
  KEY `idx_project_id` (`project_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='生图模型配置表';

-- 插入默认的生图模型数据
INSERT INTO `va_image_generation_model`
(`created_at`, `updated_at`, `model_name`, `display_name`, `description`, `icon`, `provider`, `support_t2i`, `support_i2i`, `credit_points`, `default_width`, `default_height`, `support_qualities`, `max_images`, `enabled`, `sort`, `project_id`)
VALUES
('2026-02-02 19:00:00', '2026-02-02 19:00:00', 'flux-dev', 'Flux Dev', 'Flux Dev 模型，适合高质量图片生成', '', 'comfyui', 1, 1, 10, 1024, 1024, '["standard","hd"]', 4, 1, 100, 'visionai'),
('2026-02-02 19:00:00', '2026-02-02 19:00:00', 'flux-schnell', 'Flux Schnell', 'Flux Schnell 模型，快速生成', '', 'comfyui', 1, 1, 5, 1024, 1024, '["standard"]', 4, 1, 90, 'visionai'),
('2026-02-02 19:00:00', '2026-02-02 19:00:00', 'sd3', 'Stable Diffusion 3', 'Stable Diffusion 3 模型', '', 'comfyui', 1, 1, 8, 1024, 1024, '["standard","hd"]', 4, 1, 80, 'visionai'),
('2026-02-02 19:00:00', '2026-02-02 19:00:00', 'wanx', '通义万相', '阿里通义万相模型', '', 'comfyui', 1, 0, 6, 1024, 1024, '["standard"]', 1, 1, 70, 'visionai');
