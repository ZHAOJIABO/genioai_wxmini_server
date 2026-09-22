package utils

import (
	"testing"
)

func TestVerifyRandomParams(t *testing.T) {
	// 测试不同的种子
	seeds := []int64{1, 42, 20240322, 999999}

	for _, seed := range seeds {
		ok, msg := VerifyRandomParams(seed)
		if !ok {
			t.Errorf("Seed %d failed: %s", seed, msg)
		}

		// 验证多次调用结果一致
		for i := 0; i < 10; i++ {
			a1, b1, c1, d1 := GetLogarithmicParams(seed)
			a2, b2, c2, d2 := GetLogarithmicParams(seed)

			if a1 != a2 || b1 != b2 || c1 != c2 || d1 != d2 {
				t.Errorf("Parameters not fixed for seed %d on iteration %d", seed, i)
			}

			// 打印参数用于检查
			t.Logf("Seed %d: a=%d, b=%.2f, c=%d, d=%d", seed, a1, b1, c1, d1)
		}
	}
}

func TestLogarithmicCalculationWithSeed(t *testing.T) {
	seed := int64(20240322)
	inputs := []int{0, 1, 10, 100, 1000}

	// 验证同一个种子多次计算结果一致
	for _, x := range inputs {
		result1 := LogarithmicCalculationWithSeed(x, seed)
		result2 := LogarithmicCalculationWithSeed(x, seed)

		if result1 != result2 {
			t.Errorf("Results not consistent for x=%d: got %d and %d", x, result1, result2)
		}

		t.Logf("x=%d: result=%d", x, result1)
	}
}
