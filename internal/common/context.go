package common

import (
	"context"

	"va_visionai_server/internal/constants"
)

func CtxGetStrValue(ctx context.Context, key string) string {
	if ctx == nil {
		return ""
	}
	if ctx.Value(key) == nil {
		return ""
	}
	if value, ok := ctx.Value(key).(string); ok {
		return value
	}
	return ""
}

func CtxSetStrValue(ctx context.Context, key string, value string) context.Context {
	return context.WithValue(ctx, key, value)
}

func CtxSetValue(ctx context.Context, key string, value interface{}) context.Context {
	return context.WithValue(ctx, key, value)
}

func CtxGetValue(ctx context.Context, key string) interface{} {
	return ctx.Value(key)
}

func GetProjectID(ctx context.Context) string {
	projectID := CtxGetStrValue(ctx, constants.CtxProjectID)
	if projectID == "" {
		projectID = "com.domob.visionai"
	}
	return projectID
}

func GetUserID(ctx context.Context) string {
	userID := CtxGetStrValue(ctx, constants.CtxUserID)
	if userID == "" {
		return ""
	}
	return userID
}

func GetModelDeploymentName(ctx context.Context) string {
	modelDeploymentName := CtxGetStrValue(ctx, constants.CtxModelDeploymentName)
	if modelDeploymentName == "" {
		modelDeploymentName = "gpt-4o"
	}
	return modelDeploymentName
}

func GetClientIP(ctx context.Context) string {
	ip := CtxGetStrValue(ctx, constants.CtxIP)
	if ip == "" {
		return ""
	}
	return ip
}

func GetLang(ctx context.Context) string {
	lang := CtxGetStrValue(ctx, constants.CtxLang)
	if lang == "" {
		return "en"
	}
	return lang
}

func GetAppVersion(ctx context.Context) string {
	appVersion := CtxGetStrValue(ctx, constants.CtxAppVersiopn)
	if appVersion == "" {
		return "0.0.1"
	}
	return appVersion
}

func GetOSName(ctx context.Context) string {
	osName := CtxGetStrValue(ctx, constants.CtxOSName)
	if osName == "" {
		return constants.IOS
	}
	return osName
}
