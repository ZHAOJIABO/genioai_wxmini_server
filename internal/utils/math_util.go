package utils

import (
	"math"
	"math/rand"
)

// LogarithmicCalculation 计算 f(x)=a⋅log_b(x+c)+d
// 其中参数根据种子生成固定的随机数:
// a: [10,200] 整数
// b: [1.00,2.00] 浮点数(2位小数)
// c: [3,200] 整数
// d: [3,200] 整数
func LogarithmicCalculation(x int) int {
	// 使用固定种子初始化随机数生成器
	seed := int64(20240322) // 使用一个固定的种子值
	r := rand.New(rand.NewSource(seed))

	// 生成固定的随机参数
	a := r.Intn(191) + 10  // [10,200]
	b := 1.0 + r.Float64() // [1.00,2.00]
	c := r.Intn(198) + 3   // [3,200]
	d := r.Intn(198) + 3   // [3,200]

	// 计算对数值
	// 使用换底公式: log_b(x) = ln(x) / ln(b)
	result := float64(a)*(math.Log(float64(x+c))/math.Log(b)) + float64(d)

	// 返回四舍五入后的整数结果
	return int(math.Round(result))
}

// GetLogarithmicParams 根据种子获取固定的 a,b,c,d 参数值
func GetLogarithmicParams(seed int64) (a int, b float64, c, d int) {
	r := rand.New(rand.NewSource(seed))

	a = r.Intn(191) + 10  // [10,200]
	b = 1.0 + r.Float64() // [1.00,2.00]
	c = r.Intn(198) + 3   // [3,200]
	d = r.Intn(198) + 3   // [3,200]

	return
}

// LogarithmicCalculationWithSeed 使用指定种子计算 f(x)=a⋅log_b(x+c)+d
func LogarithmicCalculationWithSeed(x int, seed int64) int {
	a, b, c, d := GetLogarithmicParams(seed)

	// 计算对数值
	result := float64(a)*(math.Log(float64(x+c))/math.Log(b)) + float64(d)

	return int(math.Round(result))
}

// VerifyRandomParams 验证随机参数的生成是否固定且在范围内
func VerifyRandomParams(seed int64) (bool, string) {
	// 第一次获取参数
	a1, b1, c1, d1 := GetLogarithmicParams(seed)

	// 第二次获取参数
	a2, b2, c2, d2 := GetLogarithmicParams(seed)

	// 验证参数是否固定
	if a1 != a2 || b1 != b2 || c1 != c2 || d1 != d2 {
		return false, "Parameters are not fixed for the same seed"
	}

	// 验证参数范围
	if a1 < 10 || a1 > 200 {
		return false, "Parameter 'a' is out of range [10,200]"
	}
	if b1 < 1.0 || b1 > 2.0 {
		return false, "Parameter 'b' is out of range [1.00,2.00]"
	}
	if c1 < 3 || c1 > 200 {
		return false, "Parameter 'c' is out of range [3,200]"
	}
	if d1 < 3 || d1 > 200 {
		return false, "Parameter 'd' is out of range [3,200]"
	}

	return true, "All parameters are fixed and within range"
}
