package api

import (
	"context"
	"reflect"
	"strings"

	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/service"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// BuildErrorResponse builds an error response with logging
func BuildErrorResponse[T any](ctx context.Context, code vai.StatusCode, err error, logMsg string) (*T, error) {
	logger := zlog.LogWithContext(ctx)
	if err != nil {
		logger.Error(logMsg, zap.Error(err))
	}

	resp := new(T)
	v := reflect.ValueOf(resp).Elem()
	errorCode := constants.ErrMsgMapCode[err.Error()]
	if errorCode == 0 {
		errorCode = code
	}

	headerField := v.FieldByName("ResponseHeader")
	if headerField.IsValid() && headerField.CanSet() {
		header := common.BuildHeader(errorCode)

		// 对于任务限制相关的错误，使用原始的详细错误消息
		if err != nil {
			errMsg := err.Error()
			if errMsg == constants.ErrMsgTaskQueueLimitExceeded ||
			   errMsg == constants.ErrMsgTaskConcurrentLimitExceeded {
				header.Msg = errMsg
			}
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
		header := common.BuildHeader(vai.StatusCode_SUCCESS)
		headerField.Set(reflect.ValueOf(header))
	}
	return resp, nil
}

// GetPackageName 统一获取包名/应用标识符
// 优先从 App 获取，如果不存在则从 WebClient 获取
// 用于获取 project_id
func GetPackageName(header *vai.RequestHeader) string {
	// 优先从 App 获取（移动端）
	if app := header.GetApp(); app != nil && app.GetPackageName() != "" {
		return app.GetPackageName()
	}

	// 从 WebClient 获取（Web端）
	if webClient := header.GetWebClient(); webClient != nil && webClient.GetPackageName() != "" {
		return webClient.GetPackageName()
	}

	// 如果都没有，返回空字符串
	return ""
}

func isVerifyAccessToken(userService *service.UserService, header *vai.RequestHeader) (vai.StatusCode, error) {
	os := constants.MappingOS(header.GetDevice().GetOs())
	ctx := context.TODO()
	projectID := GetPackageName(header)
	ctx = context.WithValue(ctx, constants.CtxProjectID, projectID)
	return userService.VerifyAccessToken(ctx, header.GetUserId(), header.GetAccessToken(), os)
}

func GetLowQualityImages(url string) string {
	if strings.HasSuffix(url, "-low.jpg") {
		return url
	}
	return strings.Replace(url, ".jpg", "-low.jpg", 1)
}
