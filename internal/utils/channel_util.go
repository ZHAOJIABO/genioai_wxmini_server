package utils

import (
	"va_visionai_server/internal/common"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/zlog"
)

func SafeCloseChan(ch interface{}) {
	// 使用 defer + recover 防止重复关闭引发 panic
	defer func() {
		if r := recover(); r != nil {
			zlog.Logger.Warn("Recover: Chan Already Closed")
		}
	}()
	// 尝试将 ch 转换为通道类型
	switch v := ch.(type) {
	case chan []byte:
		close(v)
	case chan []string:
		close(v)
	case chan struct{}:
		close(v)
	case chan<- []byte:
		close(v)
	case chan interface{}:
		close(v)
	case model.Output:
		close(v)
	case common.EventChannel:
		close(v)
	case model.CancelCh:
		close(v)
	default:
		zlog.Logger.Warn("Invalid channel type")
	}
}
