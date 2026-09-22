package prometheus

import (
	"testing"
)

func TestRequestDurationMetric_Observe(t *testing.T) {
	// 测试正常情况
	metric := &RequestDurationMetric{
		Method:         "/VisionAiService/Chat",
		OS:             "ios",
		DurationE2E:    150.5,
		DurationServer: 80.2,
	}
	metric.Observe()

	// 测试缺少 Method
	metric2 := &RequestDurationMetric{
		Method:         "",
		OS:             "ios",
		DurationE2E:    150.5,
		DurationServer: 80.2,
	}
	metric2.Observe() // 应该不会 panic，但也不会记录

	// 测试缺少 OS
	metric3 := &RequestDurationMetric{
		Method:         "/VisionAiService/Chat",
		OS:             "",
		DurationE2E:    150.5,
		DurationServer: 80.2,
	}
	metric3.Observe() // 应该不会 panic，但也不会记录

	// 测试只有服务端耗时
	metric4 := &RequestDurationMetric{
		Method:         "/VisionAiService/Chat",
		OS:             "android",
		DurationE2E:    0,
		DurationServer: 60.5,
	}
	metric4.Observe()
}
