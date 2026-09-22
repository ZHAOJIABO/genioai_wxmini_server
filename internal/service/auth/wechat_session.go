package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/auth/providers/wechat"
	"va_visionai_server/internal/service/auth/types"
	vai "va_visionai_server/internal/va_interface"
)

// validWeChatCaller deliberately disallows guest shortcuts and stale DB tokens.
func validWeChatCaller(user *model.UserRecord, projectID, userID, token string, now int64) bool {
	return user != nil && projectID != "" && userID != "" && token != "" &&
		user.ProjectID == projectID && user.UserId == userID &&
		user.LoginType == wechat.ProviderType && user.Status == 0 &&
		user.AccessTokenExpired > now && subtle.ConstantTimeCompare([]byte(user.AccessToken), []byte(token)) == 1
}

func (s *AuthService) WeChatSession(ctx context.Context, header *vai.RequestHeader, reset bool) (bool, error) {
	projectID := common.GetProjectID(ctx)
	userID, token := header.GetUserId(), header.GetAccessToken()
	if projectID == "" || userID == "" || token == "" {
		return false, types.ErrInvalidToken
	}
	user, err := s.userDao.GetUserByIdOrPhone(projectID, userID)
	if err != nil {
		return false, types.ErrInvalidToken
	}
	if !validWeChatCaller(user, projectID, userID, token, time.Now().Unix()) {
		return false, types.ErrInvalidToken
	}
	session, err := s.redisClient.WithContext(ctx).Get(s.GetUserSessionKey(*user, constants.MappingOS(header.GetDevice().GetOs()))).Result()
	if err != nil || subtle.ConstantTimeCompare([]byte(session), []byte(token)) != 1 {
		return false, types.ErrInvalidToken
	}
	provider, err := s.providerManager.GetProvider(projectID, wechat.ProviderType)
	if err != nil {
		return false, err
	}
	wx, ok := provider.(*wechat.Provider)
	if !ok {
		return false, errors.New("wechat provider unavailable")
	}
	return wx.Session(ctx, projectID, user.ProviderUserID, reset)
}
