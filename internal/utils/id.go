package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
)

// GenerateMsgID 生成新的消息ID
func GenerateMsgID() string {
	newUUID := uuid.New()
	prefix := "om"
	return fmt.Sprintf("%s-%s", prefix, newUUID.String())
}

// GeneratePromptHistoryID 生成新的prompt历史消息ID
func GeneratePromptHistoryID() string {
	newUUID := uuid.New()
	prefix := "ph"
	return fmt.Sprintf("%s-%s", prefix, newUUID.String())
}

// GeneratePromptHash 生成 Prompt 内容的 SHA256 哈希值，用于快速去重比对
// 参数:
//   - prompt: Prompt 文本内容
//
// 返回:
//   - 64位16进制字符串形式的哈希值
func GeneratePromptHash(prompt string) string {
	hash := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(hash[:])
}
