package utils

import (
	"errors"
	"time"
)

// ParseTimeString 解析时间字符串，支持time.DateOnly和time.DateTime格式
// 参数:
//   - timeStr: 时间字符串，格式可能是"2006-01-02"或"2006-01-02 15:04:05"
//
// 返回:
//   - 解析后的时间对象
//   - 如果解析失败，返回错误信息
func ParseTimeString(timeStr string) (time.Time, error) {
	// 尝试按照DateOnly格式解析
	t, err := time.Parse(time.DateOnly, timeStr)
	if err == nil {
		return t, nil
	}

	// 尝试按照DateTime格式解析
	t, err = time.Parse(time.DateTime, timeStr)
	if err == nil {
		return t, nil
	}

	// 如果都不是，返回错误
	return time.Time{}, errors.New("time string format is not DateOnly(2006-01-02) or DateTime(2006-01-02 15:04:05)")
}
