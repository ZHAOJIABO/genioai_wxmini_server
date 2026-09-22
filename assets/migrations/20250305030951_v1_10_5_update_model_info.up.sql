TRUNCATE TABLE `va_model`;
ALTER TABLE `va_model` ADD `is_vision` VARCHAR(255)  NULL  DEFAULT NULL  AFTER `deployment_name`;
ALTER TABLE `va_model` ADD `icon` VARCHAR(255)  NULL  DEFAULT NULL  AFTER `is_vision`;
ALTER TABLE `va_model` ADD `sort` TINYINT(4)  NULL  DEFAULT NULL  AFTER `icon`;
ALTER TABLE `va_model` CHANGE `status` `display` TINYINT(4)  NOT NULL  DEFAULT 1;

INSERT INTO `va_model` (`created_at`, `updated_at`, `deleted_at`, `model_id`, `model_name`, `deployment_name`, `is_vision`, `display`, `icon`, `sort`)
VALUES
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_GPT4O', '专业模式', 'gpt-4o', '1', '1', 'https://visionai-ugc.domob.cn/com.domob.visionai/static/model_icon/pro.png', '9'),
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_DOUBAO', 'MODEL_DOUBAO', 'ep-20241015150440-wzl76', '0', '0', '', '0'),
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_GPT4', 'MODEL_GPT4', 'gpt-4', '0', '0', '', '0'),
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_GPT4O_MINI', 'MODEL_GPT4O_MINI', 'gpt-4o-mini', '1', '0', '', '0'),
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_GPTO1_PREVIEW', 'MODEL_GPTO1_PREVIEW', 'gpt-o1-preview', '0', '0', '', '0'),
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_GPTO1_MINI', 'MODEL_GPTO1_MINI', 'gpt-o1-mini', '0', '0', '', '0'),
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_DOUBAO_PRO', 'MODEL_DOUBAO_PRO', 'ep-20250214171140-jhq27', '0', '0', '', '0'),
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_DOUBAO_VISION_PRO', '高效识图', 'ep-20250214171032-hbc67', '1', '1', 'https://visionai-ugc.domob.cn/com.domob.visionai/static/model_icon/quick.png', '10'),
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_HUOSHAN_DEEPSEEK_R1', '深度思考', 'ep-20250208111901-95vm2', '0', '1', 'https://visionai-ugc.domob.cn/com.domob.visionai/static/model_icon/think.png', '8'),
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_DEEPSEEK_V3', 'MODEL_DEEPSEEK_V3', 'deepseek-chat', '0', '0', '', '0'),
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_DEEPSEEK_R1', 'MODEL_DEEPSEEK_R1', 'deepseek-reasoner', '0', '0', '', '0'),
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_TENCENT_HUNYUAN_VISION', 'MODEL_TENCENT_HUNYUAN_VISION', 'hunyuan-vision', '1', '0', '', '0'),
	('2025-02-07 18:04:08.000', '2025-02-07 18:04:08.000', NULL, 'MODEL_TENCENT_HUNYUAN_TURBOS', 'MODEL_TENCENT_HUNYUAN_TURBOS', 'hunyuan-turbos-20250226', '1', '0', '', '0');
