ALTER TABLE `va_image_generation_model` ADD COLUMN `max_input_images` int(11) NOT NULL DEFAULT '0' COMMENT '最大输入图片数,0表示不支持' AFTER `max_images`;
UPDATE `va_image_generation_model` SET `max_input_images` = 4 WHERE `provider` = 'gemini';
UPDATE `va_image_generation_model` SET `max_input_images` = 1 WHERE `support_i2i` = 1 AND `max_input_images` = 0;
