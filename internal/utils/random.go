package utils

import "math/rand"

// GenerateRandomNumber 生成指定长度的随机整数
func GenerateRandomNumber(length int) int64 {
	min := int64(1)
	max := int64(9)
	for i := 1; i < length; i++ {
		min *= 10
		max = max*10 + 9
	}

	return min + rand.Int63n(max-min+1)
}
