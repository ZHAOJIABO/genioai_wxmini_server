package common

import (
	"time"

	"va_visionai_server/internal/constants"
	vai "va_visionai_server/internal/va_interface"
)

// BuildInterceptorErrorResponse 构建拦截器错误响应
func BuildInterceptorErrorResponse(reqID, userID string, err error) *vai.InterceptorErrorResponse {
	code := constants.ErrMsgMapCode[err.Error()]
	return &vai.InterceptorErrorResponse{
		ResponseHeader: &vai.ResponseHeader{
			ReqId:              reqID,
			Code:               vai.StatusCode(code),
			ResponseStatusCode: vai.ResponseStatusCode(code),
			Msg:                err.Error(),
			UserId:             userID,
			ServerTime:         time.Now().Format("2006-01-02 15:04:05"),
		},
	}
}
