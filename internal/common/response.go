package common

import (
	vai "va_visionai_server/internal/va_interface"
)

// BuildResponseHeader 构建响应头
func BuildResponseHeader(code vai.StatusCode, msg string) *vai.ResponseHeader {
	return &vai.ResponseHeader{
		Code: code,
		Msg:  msg,
	}
}
