package utils

// GetStandardAspectRatio 将输入的宽高转换为标准比例文本（兼容性函数）
// 注意：建议使用 WANXConfigService.GetStandardAspectRatio() 以支持动态配置
func GetStandardAspectRatio(width, height int) string {
	if width <= 0 || height <= 0 {
		return "1:1" // 默认值
	}

	ratio := float64(height) / float64(width)

	// 直接硬编码标准比例判断，简单有效
	switch {
	case ratio >= 0.95 && ratio <= 1.05: // 1:1
		return "1:1"
	case ratio >= 1.25 && ratio <= 1.4: // 3:4
		return "3:4"
	case ratio >= 0.7 && ratio <= 0.8: // 4:3
		return "4:3"
	case ratio >= 1.7 && ratio <= 1.85: // 9:16
		return "9:16"
	case ratio >= 0.54 && ratio <= 0.6: // 16:9
		return "16:9"
	default:
		// 兜底：找最接近的
		if ratio > 1.0 {
			return "3:4" // 竖屏默认
		} else {
			return "4:3" // 横屏默认
		}
	}
}
