package utils

import (
	"encoding/base64"
)

// Encode 将字符串编码为 Base64
func Base64Encode(data string) string {
	return base64.StdEncoding.EncodeToString([]byte(data))
}

// Decode 将 Base64 编码的字符串解码为原始字符串
func Base64Decode(encodedData string) (string, error) {
	decodedBytes, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		return "", err
	}
	return string(decodedBytes), nil
}
