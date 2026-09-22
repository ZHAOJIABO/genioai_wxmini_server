package utils

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/pkoukk/tiktoken-go"
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/zlog"
)

// calculateTokens 计算一组消息的总 token 数量
func CalculateTokens(messages []azopenai.ChatRequestMessageClassification) (int, error) {
	// 开始计时
	startTime := time.Now()
	enc, err := tiktoken.EncodingForModel("gpt-4o")
	if err != nil {
		return 0, errors.New("failed to get encoding: " + err.Error())
	}

	var totalTokens int
	for _, msg := range messages {
		msgBytes, err := json.Marshal(msg)
		if err != nil {
			return 0, errors.New("failed to marshal message: " + err.Error())
		}
		tokenCount := len(enc.Encode(string(msgBytes), nil, nil))
		totalTokens += tokenCount
	}
	// 计算耗时
	elapsed := time.Since(startTime)
	zlog.Logger.Info("CalculateTokens", zap.Int("totalTokens", totalTokens), zap.Duration("elapsed", elapsed))
	return totalTokens, nil
}

func GetInOutTokenCount(ctx context.Context) (int, int) {
	var inToken, outToken int
	inTokenValue := ctx.Value(constants.LLMInputToken)
	if _, ok := inTokenValue.(int); ok {
		inToken = inTokenValue.(int)
	}
	outTokenValue := ctx.Value(constants.LLMOutputToken)
	if _, ok := outTokenValue.(int); ok {
		outToken = outTokenValue.(int)
	}
	return inToken, outToken
}
