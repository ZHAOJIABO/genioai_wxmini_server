package biz

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"va_visionai_server/internal/zlog"
)

// callbackProcessor 专门处理分片 token 的业务逻辑，包含剔除 ###answer、截断 ###follow_question 以及保留边界 suffix。
type callbackProcessor struct {
	suffix             string
	processed          map[string]bool
	stop               bool
	followQuestionText string
}

const (
	answerMarker         = "###answer"
	removeMarker1        = "просто"
	followQuestionMarker = "**Guess what you want to ask**"
)

// newCallbackProcessor 初始化一个 callbackProcessor
func newCallbackProcessor() *callbackProcessor {
	return &callbackProcessor{
		suffix: "",
		processed: map[string]bool{
			answerMarker:         false,
			followQuestionMarker: false,
		},
		stop:               false,
		followQuestionText: "",
	}
}

// process 接收一个新的 token，并返回本次需要立即发送的文本 toSend（为空表示无需发送）
func (p *callbackProcessor) process(ctx context.Context, token string) (toSend string, stopNow bool) {
	zlog.LogWithContext(ctx).Debug("[(p *callbackProcessor) process] Token", zap.String("token", token))
	// —— 新增：如果已经进入 followQuestion 阶段，只累积，不再输出 ——
	if p.processed[followQuestionMarker] {
		p.followQuestionText += token
		return "", false
	}

	if p.stop {
		return "", true
	}
	data := p.suffix + token

	if !p.processed[answerMarker] && strings.Contains(data, answerMarker) {
		data = strings.ReplaceAll(data, answerMarker, "")
		p.processed[answerMarker] = true
	}

	if strings.Contains(data, removeMarker1) {
		data = strings.ReplaceAll(data, removeMarker1, "")
	}

	if !p.processed[followQuestionMarker] && strings.Contains(data, followQuestionMarker) {
		marker := followQuestionMarker
		pos := strings.Index(data, marker)
		if pos > 0 {
			toSend = data[:pos]
		}
		// 记录标记之后的初始片段
		p.followQuestionText = data[pos+len(marker):]
		p.processed[followQuestionMarker] = true
		// 接下来不再走 suffix/flush 逻辑
		p.suffix = ""
		// 注意：这里不再设置 p.stop，继续接收后续所有 token
		return toSend, false
	}

	markers := []string{followQuestionMarker, answerMarker}
	maxLen := 0
	for _, m := range markers {
		if l := len(m); l > maxLen {
			maxLen = l
		}
	}
	keep := maxLen - 1

	runes := []rune(data)
	n := len(runes)
	if n <= keep {
		p.suffix = data
		return "", false
	}
	toSend = string(runes[:n-keep])
	p.suffix = string(runes[n-keep:])
	return toSend, false
}
