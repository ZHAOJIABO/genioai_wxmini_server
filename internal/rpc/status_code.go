package rpc

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// StatusCodeMapper 用于将 StatusCode 映射为 ResponseStatusCode
type StatusCodeMapper struct {
	codeMap map[vai.StatusCode]vai.ResponseStatusCode
}

// NewStatusCodeMapper 创建一个新的状态码映射器
func NewStatusCodeMapper() *StatusCodeMapper {
	mapper := &StatusCodeMapper{
		codeMap: make(map[vai.StatusCode]vai.ResponseStatusCode),
	}

	// 初始化映射关系
	mapper.codeMap = map[vai.StatusCode]vai.ResponseStatusCode{
		vai.StatusCode_SUCCESS:                        vai.ResponseStatusCode_RESPONSE_STATUS_CODE_SUCCESS,
		vai.StatusCode_INVALID_PARAM:                  vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_PARAM,
		vai.StatusCode_INVALID_REQUEST:                vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_REQUEST,
		vai.StatusCode_REQUEST_FAILED:                 vai.ResponseStatusCode_RESPONSE_STATUS_CODE_REQUEST_FAILED,
		vai.StatusCode_INVALID_ACCESS_TOKEN:           vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_ACCESS_TOKEN,
		vai.StatusCode_EXPIRED_ACCESS_TOKEN:           vai.ResponseStatusCode_RESPONSE_STATUS_CODE_EXPIRED_ACCESS_TOKEN,
		vai.StatusCode_INVALID_REFRESH_TOKEN:          vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_REFRESH_TOKEN,
		vai.StatusCode_INVALID_USER:                   vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_USER,
		vai.StatusCode_INVALID_APP:                    vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_APP,
		vai.StatusCode_INVALID_DEVICE:                 vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_DEVICE,
		vai.StatusCode_INVALID_PHONE_NUMBER:           vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_PHONE_NUMBER,
		vai.StatusCode_INVALID_VERIFY_CODE:            vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_VERIFY_CODE,
		vai.StatusCode_AMOUNT_EXHAUSTED:               vai.ResponseStatusCode_RESPONSE_STATUS_CODE_AMOUNT_EXHAUSTED,
		vai.StatusCode_SEND_MESSAGE_TOO_OFTEN:         vai.ResponseStatusCode_RESPONSE_STATUS_CODE_SEND_MESSAGE_TOO_OFTEN,
		vai.StatusCode_FILE_TOO_LARGE:                 vai.ResponseStatusCode_RESPONSE_STATUS_CODE_FILE_TOO_LARGE,
		vai.StatusCode_DAILY_AMOUNT_LIMIT_EXCEEDED:    vai.ResponseStatusCode_RESPONSE_STATUS_CODE_DAILY_AMOUNT_LIMIT_EXCEEDED,
		vai.StatusCode_FAILED_BY_ALREADY_SIGNIN:       vai.ResponseStatusCode_RESPONSE_STATUS_CODE_FAILED_BY_ALREADY_SIGNIN,
		vai.StatusCode_FAILED_BY_AD_AWARD_LIMIT:       vai.ResponseStatusCode_RESPONSE_STATUS_CODE_FAILED_BY_AD_AWARD_LIMIT,
		vai.StatusCode_DAILY_TOKEN_LIMIT_EXCEEDED:     vai.ResponseStatusCode_RESPONSE_STATUS_CODE_DAILY_TOKEN_LIMIT_EXCEEDED,
		vai.StatusCode_USER_NAME_EXISTS:               vai.ResponseStatusCode_RESPONSE_STATUS_CODE_USER_NAME_EXISTS,
		vai.StatusCode_UPLOAD_CONTENT_SENSITIVE:       vai.ResponseStatusCode_RESPONSE_STATUS_CODE_UPLOAD_CONTENT_SENSITIVE,
		vai.StatusCode_TASK_PROCESSING_LIMIT_EXCEEDED: vai.ResponseStatusCode_RESPONSE_STATUS_CODE_TASK_PROCESSING_LIMIT_EXCEEDED,
		vai.StatusCode_CREDIT_POINT_NOT_ENOUGH:        vai.ResponseStatusCode_RESPONSE_STATUS_CODE_CREDIT_POINT_NOT_ENOUGH,
		vai.StatusCode_SUBSCRIBE_EXPIRED:              vai.ResponseStatusCode_RESPONSE_STATUS_CODE_SUBSCRIBE_EXPIRED,
		vai.StatusCode_SUBSCRIBE_NOT_ACTIVATE:         vai.ResponseStatusCode_RESPONSE_STATUS_CODE_SUBSCRIBE_NOT_ACTIVATE,
		// 邮箱登录注册相关
		vai.StatusCode_INVALID_EMAIL_FORMAT:      vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_EMAIL_FORMAT,
		vai.StatusCode_INVALID_EMAIL_OR_PASSWORD: vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_EMAIL_OR_PASSWORD,
		vai.StatusCode_EMAIL_ALREADY_EXISTS:      vai.ResponseStatusCode_RESPONSE_STATUS_CODE_EMAIL_ALREADY_EXISTS,
		vai.StatusCode_EMAIL_NOT_REGISTERED:      vai.ResponseStatusCode_RESPONSE_STATUS_CODE_EMAIL_NOT_REGISTERED,
		vai.StatusCode_INVALID_PASSWORD_FORMAT:   vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_PASSWORD_FORMAT,
		vai.StatusCode_INVALID_EMAIL_VERIFY_CODE: vai.ResponseStatusCode_RESPONSE_STATUS_CODE_INVALID_EMAIL_VERIFY_CODE,
		vai.StatusCode_EMAIL_VERIFY_CODE_EXPIRED: vai.ResponseStatusCode_RESPONSE_STATUS_CODE_EMAIL_VERIFY_CODE_EXPIRED,
		vai.StatusCode_PASSWORD_NOT_SET:          vai.ResponseStatusCode_RESPONSE_STATUS_CODE_PASSWORD_NOT_SET,
	}

	return mapper
}

// MapStatusCode 将 StatusCode 映射为 ResponseStatusCode
func (m *StatusCodeMapper) MapStatusCode(code vai.StatusCode) vai.ResponseStatusCode {
	if newCode, ok := m.codeMap[code]; ok {
		return newCode
	}
	zlog.Logger.Warn("Unknown status code mapping", zap.Int32("code", int32(code)))
	return vai.ResponseStatusCode_RESPONSE_STATUS_CODE_UNKNOWN_ERROR
}

// UnaryServerInterceptor 创建一个 unary 拦截器来处理状态码映射
func (m *StatusCodeMapper) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			return resp, err
		}

		if msg, ok := resp.(proto.Message); ok {
			if err := m.processResponse(ctx, msg); err != nil {
				zlog.LogWithContext(ctx).Error("Failed to process response", zap.Error(err))
			}
		}

		return resp, nil
	}
}

// StreamServerInterceptor 创建一个 stream 拦截器来处理状态码映射
func (m *StatusCodeMapper) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		wrapper := &statusCodeStreamWrapper{
			ServerStream: ss,
			ctx:          ss.Context(),
			mapper:       m,
		}
		return handler(srv, wrapper)
	}
}

// processResponse 处理响应消息中的状态码映射
func (m *StatusCodeMapper) processResponse(ctx context.Context, msg proto.Message) error {
	header := m.extractResponseHeader(msg)
	if header == nil {
		return nil
	}
	if header.GetResponseStatusCode() == 0 {
		header.ResponseStatusCode = m.MapStatusCode(header.GetCode())
	}
	return nil
}

// statusCodeStreamWrapper 包装 ServerStream 以处理流式响应
type statusCodeStreamWrapper struct {
	grpc.ServerStream
	ctx    context.Context
	mapper *StatusCodeMapper
}

func (w *statusCodeStreamWrapper) Context() context.Context {
	return w.ctx
}

func (w *statusCodeStreamWrapper) SendMsg(m interface{}) error {
	if msg, ok := m.(proto.Message); ok {
		if err := w.mapper.processResponse(w.ctx, msg); err != nil {
			zlog.LogWithContext(w.ctx).Error("Failed to process stream message", zap.Error(err))
			// 不要因为状态码映射失败而中断消息发送
		}
	}
	return w.ServerStream.SendMsg(m)
}
