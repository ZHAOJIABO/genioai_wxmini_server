package rpc

import (
	"time"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	vai "va_visionai_server/internal/va_interface"
)

func BuildChatStreamMessage(code vai.StatusCode, msgToken, errMsg string, isEnd bool) (resp *vai.ChatMessageStreamResponse) {
	if errMsg == "" {
		errMsg = constants.CodeMsg(code)
	}
	resp = &vai.ChatMessageStreamResponse{
		ResponseHeader: &vai.ResponseHeader{
			Code:           code,
			Msg:            errMsg,
			ResponseTimeMs: time.Now().UnixMilli(),
			ServerTime:     time.Now().Format(common.TimestampFormat),
		},
		IsEnd:        isEnd,
		MessageToken: msgToken,
	}
	return
}
