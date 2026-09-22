package api

import (
	"context"
	"errors"
	"time"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/service/auth/providers/wechat"
	"va_visionai_server/internal/service/auth/types"
	vai "va_visionai_server/internal/va_interface"
)

func (s *AuthServer) WeChatAuth(ctx context.Context, req *vai.WeChatAuthRequest) (*vai.AuthResponse, error) {
	result, err := s.authService.Login(ctx, req)
	if err != nil {
		if errors.Is(err, types.ErrInvalidCredentials) || errors.Is(err, types.ErrInvalidLoginContext) {
			return BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_INVALID_REQUEST, err, "微信登录凭证无效，请重新登录")
		}
		return BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgInternalServer)
	}
	user := result.User
	info := s.userService.BuildUserInfoByUserRecord(user)
	info.Nickname, info.Avatar = user.UserName, user.AvatarUrl
	info.RegisterTime = time.Unix(user.CreateTime, 0).Format(common.TimestampFormat)
	header := req.GetRequestHeader()
	if err := s.BuildUserInfo(ctx, constants.LanguageMap(header.GetDevice().GetLanguage()), constants.MappingOS(header.GetDevice().GetOs()), info, result.CreditGranted, result.CreditAmount); err != nil {
		return BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgInternalServer)
	}
	return BuildSuccessResponse(&vai.AuthResponse{
		AccessToken:  user.AccessToken,
		RefreshToken: user.RefreshToken,
		ExpireTime:   time.Unix(user.AccessTokenExpired, 0).Format(common.TimestampFormat),
		UserInfo:     info,
		IsFirstLogin: result.IsNewUser,
		AuthType:     vai.AuthType_AUTH_TYPE_WECHAT,
	})
}

func (s *AuthServer) WeChatCheckSession(ctx context.Context, req *vai.WeChatSessionRequest) (*vai.WeChatSessionResponse, error) {
	return s.weChatSession(ctx, req, false)
}

func (s *AuthServer) WeChatResetSession(ctx context.Context, req *vai.WeChatSessionRequest) (*vai.WeChatSessionResponse, error) {
	return s.weChatSession(ctx, req, true)
}

func (s *AuthServer) weChatSession(ctx context.Context, req *vai.WeChatSessionRequest, reset bool) (*vai.WeChatSessionResponse, error) {
	valid, err := s.authService.WeChatSession(ctx, req.GetRequestHeader(), reset)
	if err != nil {
		code := vai.StatusCode_REQUEST_FAILED
		if errors.Is(err, types.ErrInvalidToken) {
			code = vai.StatusCode_INVALID_ACCESS_TOKEN
		}
		if errors.Is(err, types.ErrInvalidLoginContext) || errors.Is(err, wechat.ErrSessionUnavailable) {
			code = vai.StatusCode_INVALID_REQUEST
		}
		return BuildErrorResponse[vai.WeChatSessionResponse](ctx, code, err, "微信登录态操作失败")
	}
	return BuildSuccessResponse(&vai.WeChatSessionResponse{Valid: valid})
}
