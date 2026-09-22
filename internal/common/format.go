package common

import (
	"context"
	"fmt"
	"net/mail"
	"reflect"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

func FormatResponse(header *vai.ResponseHeader) string {
	if header == nil {
		return "ResponseHeader nil"
	}
	return fmt.Sprintf("code: %d, msg: %s, rid: %s, rt: %d, st: %s, sv: %s, uid: %s",
		header.GetCode(), header.GetMsg(), header.GetReqId(), header.GetResponseTimeMs(), header.GetServerTime(), header.GetServerVersion(), header.GetUserId())
}

func FormatRequest(header *vai.RequestHeader) string {
	if header == nil {
		return "RequestHeader nil"
	}
	return fmt.Sprintf("rid: %s, at: %s, uid: %s",
		header.GetReqId(), header.GetAccessToken(), header.GetUserId())
}

const TimestampFormat = "2006-01-02 15:04:05"

func BuildHeader(code vai.StatusCode) *vai.ResponseHeader {
	return &vai.ResponseHeader{
		Code:               code,
		ResponseStatusCode: vai.ResponseStatusCode(code),
		Msg:                constants.CodeMsg(code),
	}
}

func BuildErrorResponse[T any](ctx context.Context, code vai.StatusCode, err error, logMsg string) (*T, error) {
	logger := zlog.LogWithContext(ctx)
	if err != nil {
		logger.Error(logMsg, zap.Error(err))
	}

	resp := new(T)
	v := reflect.ValueOf(resp).Elem()
	headerField := v.FieldByName("ResponseHeader")
	if headerField.IsValid() && headerField.CanSet() {
		header := BuildHeader(code)
		if err != nil {
			header.Msg = err.Error()
		}
		headerField.Set(reflect.ValueOf(header))
	}

	return resp, nil
}

// BuildSuccessResponse builds a success response
func BuildSuccessResponse[T any](resp *T) (*T, error) {
	v := reflect.ValueOf(resp).Elem()
	headerField := v.FieldByName("ResponseHeader")
	if headerField.IsValid() && headerField.CanSet() {
		header := BuildHeader(vai.StatusCode_SUCCESS)
		headerField.Set(reflect.ValueOf(header))
	}
	return resp, nil
}

func IsValidEmail(email string) bool {

	emailAddress, err := mail.ParseAddress(email)
	return err == nil && emailAddress.Address == email
}
