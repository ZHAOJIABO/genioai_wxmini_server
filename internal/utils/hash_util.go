package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

// CalculateSHA256 计算给定字节数组的SHA256哈希值，并返回十六进制字符串表示
// 参数:
//   - data: 需要计算哈希值的字节数组
//
// 返回:
//   - 十六进制表示的SHA256哈希字符串
func CalculateSHA256(data []byte) string {
	hasher := sha256.New()
	hasher.Write(data)
	return hex.EncodeToString(hasher.Sum(nil))
}
