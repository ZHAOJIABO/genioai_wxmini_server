package utils

import (
	_ "golang.org/x/image/webp"
)

// GetImageInfo 获取图片的基本信息
func GetGenderString(sex int) string {
	if sex == 2 {
		return "Female"
	} else if sex == 1 {
		return "Male"
	} else {
		return "Unknown"
	}
}

// CalculateBMIMetric 使用公制单位计算BMI
// weight: 体重（千克）
// height: 身高（米）
// 返回BMI值
func CalculateBMIMetric(weight, height float32) float32 {
	if height <= 0 || weight <= 0 {
		return 0
	}
	return weight / (height * height)
}

// CalculateBMIImperial 使用英制单位计算BMI
// weight: 体重（磅）
// height: 身高（英寸）
// 返回BMI值
func CalculateBMIImperial(weight, height float32) float32 {
	if height <= 0 || weight <= 0 {
		return 0
	}
	return (weight / (height * height)) * 703
}
