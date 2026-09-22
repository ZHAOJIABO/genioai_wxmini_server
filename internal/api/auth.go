package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/auth"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

const (
	EventGuestLogin         constants.ServiceEventType = "GuestLogin"
	EventGuestLoginComplete constants.ServiceEventType = EventGuestLogin + constants.ServiceEventComplete

	EventSendVerifyCode         constants.ServiceEventType = "SendVerifyCode"
	EventSendVerifyCodeComplete constants.ServiceEventType = EventSendVerifyCode + constants.ServiceEventComplete

	EventRefreshToken         constants.ServiceEventType = "RefreshToken"
	EventRefreshTokenComplete constants.ServiceEventType = EventRefreshToken + constants.ServiceEventComplete

	EventLogout         constants.ServiceEventType = "Logout"
	EventLogoutComplete constants.ServiceEventType = EventLogout + constants.ServiceEventComplete
)

type AuthServer struct {
	authService         *auth.AuthService
	userService         *service.UserService
	userPersonalService *service.UserPersonalInfoService
	smsVerifyService    *service.SmsVerifyService
	configService       *service.ConfigService
	subscribeService    *service.SubscribeService

	vai.UnimplementedAuthServiceServer
}

func NewAuthServer(
	authService *auth.AuthService,
	userService *service.UserService,
	userpersonalService *service.UserPersonalInfoService,
	smsVerifyService *service.SmsVerifyService,
	configService *service.ConfigService,
	subscribeService *service.SubscribeService,
) *AuthServer {
	return &AuthServer{
		authService:         authService,
		userService:         userService,
		userPersonalService: userpersonalService,
		smsVerifyService:    smsVerifyService,
		configService:       configService,
		subscribeService:    subscribeService,
	}
}

func (s *AuthServer) GuestLogin(ctx context.Context, req *vai.GuestLoginRequest) (*vai.AuthResponse, error) {
	reqHeader := req.GetRequestHeader()
	var err error
	var rsp *vai.AuthResponse

	lang := constants.LanguageMap(reqHeader.GetDevice().GetLanguage())
	os := constants.MappingOS(reqHeader.GetDevice().GetOs())
	entryLogFields := []zap.Field{
		zap.Any(constants.ServiceEvent, "GuestLogin"),
	}
	zlog.LogWithContext(ctx).Info("GuestLogin", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, "GuestLoginComplete"),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("GuestLogin", exitLogFields...)
		}
	}()

	loginResult, err := s.authService.Login(ctx, req)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GuestLogin: authService.Login failed", zap.Error(err))
		rsp, _ = BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgInternalServer)
		return rsp, nil
	}

	user := loginResult.User
	userInfo := s.userService.BuildUserInfoByUserRecord(user)

	// 直接使用 user_record 表的字段
	userInfo.Nickname = user.UserName
	userInfo.Avatar = user.AvatarUrl
	userInfo.RegisterTime = time.Unix(user.CreateTime, 0).Format(common.TimestampFormat)

	err = s.BuildUserInfo(ctx, lang, os, userInfo, loginResult.CreditGranted, loginResult.CreditAmount)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GuestLogin: BuildUserInfo failed", zap.Error(err))
		rsp, _ = BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgInternalServer)
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.AuthResponse{
		AccessToken:  user.AccessToken,
		RefreshToken: user.RefreshToken,
		ExpireTime:   time.Unix(user.RefreshTokenExpired, 0).Format(common.TimestampFormat),
		UserInfo:     userInfo,
	})

	return rsp, nil
}

func (s *AuthServer) PhoneAuth(ctx context.Context, req *vai.PhoneAuthRequest) (*vai.AuthResponse, error) {
	reqHeader := req.GetRequestHeader()
	phoneNumber := req.GetPhoneNumber()
	verifyCode := req.GetVerifyCode()
	lang := constants.LanguageMap(reqHeader.GetDevice().GetLanguage())
	os := constants.MappingOS(reqHeader.GetDevice().GetOs())
	var err error
	var rsp *vai.AuthResponse

	entryLogFields := []zap.Field{
		zap.String("PhoneNumber", phoneNumber),
		zap.String("VerifyCode", verifyCode),
		zap.Any(constants.ServiceEvent, constants.EventLogin),
	}
	zlog.LogWithContext(ctx).Info("PhoneAuth", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, constants.EventLoginComplete),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("PhoneAuth", exitLogFields...)
		}
	}()

	loginResult, err := s.authService.Login(ctx, req)
	if err != nil {
		rsp = &vai.AuthResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, constants.ErrMsgInvalidRequest),
		}
		return rsp, nil
	}

	user := loginResult.User
	userInfo := s.userService.BuildUserInfoByUserRecord(user)

	// 直接使用 user_record 表的字段
	userInfo.Nickname = user.UserName
	userInfo.Avatar = user.AvatarUrl
	userInfo.RegisterTime = time.Unix(user.CreateTime, 0).Format(common.TimestampFormat)

	err = s.BuildUserInfo(ctx, lang, os, userInfo, loginResult.CreditGranted, loginResult.CreditAmount)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_INVALID_REQUEST, err, constants.ErrMsgInvalidRequest)
		return rsp, nil
	}

	rsp = &vai.AuthResponse{
		ResponseHeader: common.BuildResponseHeader(vai.StatusCode_SUCCESS, "success"),
		AccessToken:    user.AccessToken,
		ExpireTime:     time.Unix(user.AccessTokenExpired, 0).Format(common.TimestampFormat),
		RefreshToken:   user.RefreshToken,
		UserInfo:       userInfo,
	}

	return rsp, nil
}

func (s *AuthServer) AppleAuth(ctx context.Context, req *vai.AppleAuthRequest) (*vai.AuthResponse, error) {
	reqHeader := req.GetRequestHeader()
	lang := constants.LanguageMap(reqHeader.GetDevice().GetLanguage())
	os := constants.MappingOS(reqHeader.GetDevice().GetOs())
	appleIdToken := req.GetAppleIdToken()
	var err error
	var rsp *vai.AuthResponse

	entryLogFields := []zap.Field{
		zap.String("AppleIdToken", appleIdToken),
		zap.Any(constants.ServiceEvent, constants.EventLogin),
	}
	zlog.LogWithContext(ctx).Info("AppleAuth", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, constants.EventLoginComplete),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("AppleAuth", exitLogFields...)
		}
	}()
	loginResult, err := s.authService.Login(ctx, req)
	if err != nil {
		rsp = &vai.AuthResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, constants.ErrMsgInvalidRequest),
		}
		return rsp, nil
	}
	user := loginResult.User
	userInfo := s.userService.BuildUserInfoByUserRecord(user)

	// 直接使用 user_record 表的字段
	userInfo.Nickname = user.UserName
	userInfo.Avatar = user.AvatarUrl
	userInfo.RegisterTime = time.Unix(user.CreateTime, 0).Format(common.TimestampFormat)

	err = s.BuildUserInfo(ctx, lang, os, userInfo, loginResult.CreditGranted, loginResult.CreditAmount)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_INVALID_REQUEST, err, constants.ErrMsgInvalidRequest)
		return rsp, nil
	}
	rsp, err = BuildSuccessResponse(&vai.AuthResponse{
		AccessToken:  user.AccessToken,
		RefreshToken: user.RefreshToken,
		ExpireTime:   time.Unix(user.RefreshTokenExpired, 0).Format(common.TimestampFormat),
		UserInfo:     userInfo,
	})

	return rsp, nil
}

func (s *AuthServer) GoogleAuth(ctx context.Context, req *vai.GoogleAuthRequest) (*vai.AuthResponse, error) {
	reqHeader := req.GetRequestHeader()
	lang := constants.LanguageMap(reqHeader.GetDevice().GetLanguage())
	os := constants.MappingOS(reqHeader.GetDevice().GetOs())
	googleIdToken := req.GetGoogleIdToken()
	var err error
	var rsp *vai.AuthResponse

	entryLogFields := []zap.Field{
		zap.String("GoogleIdToken", googleIdToken),
		zap.Any(constants.ServiceEvent, constants.EventLogin),
	}
	zlog.LogWithContext(ctx).Info("GoogleAuth", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, constants.EventLoginComplete),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("GoogleAuth", exitLogFields...)
		}
	}()

	loginResult, err := s.authService.Login(ctx, req)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_INVALID_REQUEST, err, constants.ErrMsgInvalidRequest)
		return rsp, nil
	}

	user := loginResult.User
	userInfo := s.userService.BuildUserInfoByUserRecord(user)

	// 直接使用 user_record 表的字段
	userInfo.Nickname = user.UserName
	userInfo.Avatar = user.AvatarUrl
	userInfo.RegisterTime = time.Unix(user.CreateTime, 0).Format(common.TimestampFormat)

	err = s.BuildUserInfo(ctx, lang, os, userInfo, loginResult.CreditGranted, loginResult.CreditAmount)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_INVALID_REQUEST, err, constants.ErrMsgInvalidRequest)
		return rsp, nil
	}
	rsp, err = BuildSuccessResponse(&vai.AuthResponse{
		AccessToken:  user.AccessToken,
		RefreshToken: user.RefreshToken,
		ExpireTime:   time.Unix(user.RefreshTokenExpired, 0).Format(common.TimestampFormat),
		UserInfo:     userInfo,
	})

	return rsp, nil
}

func (s *AuthServer) SendPhoneVerifyCode(ctx context.Context, req *vai.PhoneVerifyCodeRequest) (*vai.PhoneVerifyCodeResponse, error) {
	phoneNumber := req.GetPhoneNumber()
	var err error
	var rsp *vai.PhoneVerifyCodeResponse

	entryLogFields := []zap.Field{
		zap.String("PhoneNumber", phoneNumber),
		zap.Any(constants.ServiceEvent, "SendVerifyCode"),
	}
	zlog.LogWithContext(ctx).Info("SendPhoneVerifyCode", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, "SendVerifyCodeComplete"),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("SendPhoneVerifyCode", exitLogFields...)
		}
	}()

	if !utils.IsValidPhoneNumber(phoneNumber) {
		rsp = &vai.PhoneVerifyCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, constants.ErrMsgInvalidRequest),
		}
		return rsp, nil
	}

	_, err = s.smsVerifyService.SendVerifyCode(ctx, phoneNumber)
	if err != nil {
		rsp = &vai.PhoneVerifyCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, constants.ErrMsgInternalServer),
		}
		return rsp, nil
	}

	rsp = &vai.PhoneVerifyCodeResponse{
		ResponseHeader: common.BuildResponseHeader(vai.StatusCode_SUCCESS, "success"),
	}

	return rsp, nil
}

func (s *AuthServer) RefreshToken(ctx context.Context, req *vai.TokenRefreshRequest) (*vai.TokenRefreshResponse, error) {
	reqHeader := req.GetRequestHeader()
	refreshToken := req.GetRefreshToken()
	os := constants.MappingOS(reqHeader.GetDevice().GetOs())
	var err error
	var rsp *vai.TokenRefreshResponse

	entryLogFields := []zap.Field{
		zap.Any(constants.ServiceEvent, "RefreshToken"),
	}
	zlog.LogWithContext(ctx).Info("RefreshToken", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, "RefreshTokenComplete"),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("RefreshToken", exitLogFields...)
		}
	}()

	userId := reqHeader.GetUserId()
	if userId == "" {
		rsp = &vai.TokenRefreshResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_USER, constants.ErrMsgInvalidUser),
		}
		return rsp, nil
	}

	tokenInfo, err := s.authService.RefreshAccessToken(ctx, userId, refreshToken, os)
	if err != nil {
		rsp = &vai.TokenRefreshResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REFRESH_TOKEN, constants.ErrMsgInvalidRefreshToken),
		}
		return rsp, nil
	}

	rsp = &vai.TokenRefreshResponse{
		ResponseHeader: common.BuildResponseHeader(vai.StatusCode_SUCCESS, "success"),
		AccessToken:    tokenInfo.AccessToken,
		ExpireTime:     time.Unix(tokenInfo.ExpiresIn/1000, 0).Format(common.TimestampFormat),
	}

	return rsp, nil
}

func (s *AuthServer) Logout(ctx context.Context, req *vai.AuthLogoutRequest) (*vai.AuthLogoutResponse, error) {
	reqHeader := req.GetRequestHeader()
	var err error
	var rsp *vai.AuthLogoutResponse

	entryLogFields := []zap.Field{
		zap.Any(constants.ServiceEvent, "Logout"),
	}
	zlog.LogWithContext(ctx).Info("Logout", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, "LogoutComplete"),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("Logout", exitLogFields...)
		}
	}()

	userId := reqHeader.GetUserId()
	if userId == "" {
		rsp = &vai.AuthLogoutResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_USER, constants.ErrMsgInvalidUser),
		}
		return rsp, nil
	}

	err = s.authService.Logout(ctx, userId)
	if err != nil {
		rsp = &vai.AuthLogoutResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, constants.ErrMsgInternalServer),
		}
		return rsp, nil
	}

	rsp = &vai.AuthLogoutResponse{
		ResponseHeader: common.BuildResponseHeader(vai.StatusCode_SUCCESS, "success"),
	}

	return rsp, nil
}

func (s *AuthServer) BuildUserInfo(ctx context.Context, lang, os string, userInfo *vai.UserInfo, creditGranted bool, creditAmount int) error {
	var popAdsConfig struct {
		ShowOpenAds  bool `json:"show_open_ads"`
		ShowInnerAds bool `json:"show_inner_ads"`
	}
	err := s.configService.GetPopAdsConfig(&popAdsConfig)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetPopAdsConfig Error", zap.Error(err))
	}
	var popSubscribeConfig struct {
		PopSubscribePageCount int `json:"pop_subscribe_page_count"`
		PopSubscribeInterval  int `json:"pop_subscribe_interval"`
	}
	err = s.configService.GetPopSubscribeConfig(&popSubscribeConfig)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetPopSubscribeConfig Error", zap.Error(err))
	}
	shareURL, err := s.configService.GetConfValue("android_share_url")
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetConfValue Error", zap.Error(err))
	}

	// 获取用户积分信息 - 直接查询 va_user_amount 表
	projectID := common.GetProjectID(ctx)
	totalAmount, memberAmount, purchaseAmount, systemGrantAmount, err := s.subscribeService.GetUserCreditAmount(ctx, projectID, userInfo.GetUserId())
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetUserCreditAmount Error", zap.Error(err))
	} else {
		// 填充 GenioUserAmount 数据
		userInfo.GenioAmount = &vai.GenioUserAmount{
			TotalAmount:       int32(totalAmount),
			MemberAmount:      int32(memberAmount),
			PurchaseAmount:    int32(purchaseAmount),
			SystemGrantAmount: int32(systemGrantAmount),
		}
	}

	// 先获取订阅信息
	subscribeInfo, err := s.subscribeService.GetSubscribeInfo(ctx, userInfo.GetUserId(), os, lang, "")
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetSubscribeInfo Error", zap.Error(err))
	}
	userInfo.SubscribeInfo = subscribeInfo

	// 获取用户配置（包含首页弹窗信息），传入 subscribeInfo
	userConfig := s.configService.GetUserConfig(ctx, projectID, userInfo.GetUserId(), subscribeInfo)

	// 设置其他配置项
	userConfig.ShareUrl = shareURL
	userConfig.PopSubscribe = popSubscribeConfig.PopSubscribePageCount > 0
	userInfo.UserConfig = userConfig

	shouldPopSubscribe, err := s.configService.ShouldPopSubscribe(ctx, userInfo.GetUserId())
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetShouldPopSubscribe Error", zap.Error(err))
	}
	userInfo.UserConfig.PopSubscribe = shouldPopSubscribe
	if subscribeInfo != nil && subscribeInfo.GetStatus() == vai.SubscribeStatus_SubscribeActivate {
		userInfo.UserConfig.PopSubscribe = false
	}

	if creditGranted && creditAmount > 0 {
		if err := s.setDailyCreditPopupCache(ctx, projectID, userInfo.GetUserId()); err != nil {
			zlog.LogWithContext(ctx).Warn("Failed to set daily credit popup cache", zap.Error(err))
		}
		userInfo.UserConfig.PopFreeDailyScore = true
		userInfo.UserConfig.FreeDailyCredit = int32(creditAmount)
		zlog.LogWithContext(ctx).Info("Setting daily credit popup",
			zap.Bool("creditGranted", creditGranted),
			zap.Int("creditAmount", creditAmount),
			zap.String("userId", userInfo.GetUserId()))
	}

	return nil
}

// setDailyCreditPopupCache 设置每日积分弹窗缓存
func (s *AuthServer) setDailyCreditPopupCache(ctx context.Context, projectID, userID string) error {
	// Redis key: daily_credit_popup:{project_id}:{user_id}:{YYYY-MM-DD}
	today := time.Now().Format("2006-01-02")
	key := fmt.Sprintf("daily_credit_popup:%s:%s:%s", projectID, userID, today)

	// 设置缓存，过期时间到当天结束
	now := time.Now()
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
	duration := endOfDay.Sub(now)

	// 这里需要 Redis 客户端，暂时用日志记录
	zlog.LogWithContext(ctx).Info("Setting daily credit popup cache",
		zap.String("key", key),
		zap.Duration("duration", duration))

	// TODO: 实际的 Redis 设置逻辑
	// return s.redisClient.Set(key, "1", duration).Err()
	return nil
}

// checkDailyCreditPopupCache 检查每日积分弹窗缓存
func (s *AuthServer) checkDailyCreditPopupCache(ctx context.Context, projectID, userID string) (bool, error) {
	today := time.Now().Format("2006-01-02")
	key := fmt.Sprintf("daily_credit_popup:%s:%s:%s", projectID, userID, today)

	// TODO: 实际的 Redis 检查逻辑
	// exists, err := s.redisClient.Exists(key).Result()
	// return exists > 0, err

	zlog.LogWithContext(ctx).Debug("Checking daily credit popup cache",
		zap.String("key", key))

	return false, nil // 暂时返回 false，表示未设置过缓存
}

// SendEmailVerifyCode 发送邮箱验证码
func (s *AuthServer) SendEmailVerifyCode(ctx context.Context, req *vai.EmailVerifyCodeRequest) (*vai.EmailVerifyCodeResponse, error) {
	reqHeader := req.GetRequestHeader()
	email := req.GetEmail()
	var err error
	var rsp *vai.EmailVerifyCodeResponse

	entryLogFields := []zap.Field{
		zap.String("Email", email),
		zap.Any(constants.ServiceEvent, EventSendVerifyCode),
	}
	zlog.LogWithContext(ctx).Info("SendEmailVerifyCode", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, EventSendVerifyCodeComplete),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, reqHeader.GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("SendEmailVerifyCode", exitLogFields...)
		}
	}()

	if !utils.IsValidEmail(email) {
		rsp = &vai.EmailVerifyCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_EMAIL_FORMAT, "Invalid email format"),
		}
		return rsp, nil
	}

	emailVerifyService, err := service.NewEmailVerifyService()
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to create email verify service", zap.Error(err))
		rsp = &vai.EmailVerifyCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, constants.ErrMsgInternalServer),
		}
		return rsp, nil
	}

	_, err = emailVerifyService.SendVerifyCode(ctx, email)
	if err != nil {
		rsp = &vai.EmailVerifyCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, "Failed to send verification code"),
		}
		return rsp, nil
	}

	rsp = &vai.EmailVerifyCodeResponse{
		ResponseHeader: common.BuildResponseHeader(vai.StatusCode_SUCCESS, "success"),
	}

	return rsp, nil
}

// EmailAuth 邮箱验证码登录
func (s *AuthServer) EmailAuth(ctx context.Context, req *vai.EmailAuthRequest) (*vai.AuthResponse, error) {
	reqHeader := req.GetRequestHeader()
	lang := constants.LanguageMap(reqHeader.GetDevice().GetLanguage())
	os := constants.MappingOS(reqHeader.GetDevice().GetOs())
	email := req.GetEmail()
	var err error
	var rsp *vai.AuthResponse

	entryLogFields := []zap.Field{
		zap.String("Email", email),
		zap.Any(constants.ServiceEvent, constants.EventLogin),
	}
	zlog.LogWithContext(ctx).Info("EmailAuth", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, constants.EventLoginComplete),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("EmailAuth", exitLogFields...)
		}
	}()

	loginResult, err := s.authService.Login(ctx, req)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "invalid verification code") || strings.Contains(errMsg, "invalid verify code"):
			rsp = &vai.AuthResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_EMAIL_VERIFY_CODE, "Invalid verification code"),
			}
		case strings.Contains(errMsg, "expired") || strings.Contains(errMsg, "code expired"):
			rsp = &vai.AuthResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_EMAIL_VERIFY_CODE_EXPIRED, "Verification code expired"),
			}
		case strings.Contains(errMsg, "email not registered") || strings.Contains(errMsg, "user not found"):
			rsp = &vai.AuthResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_EMAIL_NOT_REGISTERED, "Email not registered"),
			}
		default:
			rsp = &vai.AuthResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, errMsg),
			}
		}
		return rsp, nil
	}

	user := loginResult.User
	userInfo := s.userService.BuildUserInfoByUserRecord(user)

	// 直接使用 user_record 表的字段
	userInfo.Nickname = user.UserName
	userInfo.Avatar = user.AvatarUrl
	userInfo.RegisterTime = time.Unix(user.CreateTime, 0).Format(common.TimestampFormat)

	err = s.BuildUserInfo(ctx, lang, os, userInfo, loginResult.CreditGranted, loginResult.CreditAmount)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_INVALID_REQUEST, err, constants.ErrMsgInvalidRequest)
		return rsp, nil
	}

	rsp = &vai.AuthResponse{
		ResponseHeader: common.BuildResponseHeader(vai.StatusCode_SUCCESS, "success"),
		AccessToken:    user.AccessToken,
		ExpireTime:     time.Unix(user.AccessTokenExpired, 0).Format(common.TimestampFormat),
		RefreshToken:   user.RefreshToken,
		UserInfo:       userInfo,
		IsFirstLogin:   loginResult.CreditGranted,
		AuthType:       vai.AuthType_AUTH_TYPE_EMAIL,
	}

	return rsp, nil
}

// EmailPasswordAuth 邮箱密码登录
func (s *AuthServer) EmailPasswordAuth(ctx context.Context, req *vai.EmailPasswordAuthRequest) (*vai.AuthResponse, error) {
	reqHeader := req.GetRequestHeader()
	lang := constants.LanguageMap(reqHeader.GetDevice().GetLanguage())
	os := constants.MappingOS(reqHeader.GetDevice().GetOs())
	email := req.GetEmail()
	var err error
	var rsp *vai.AuthResponse

	entryLogFields := []zap.Field{
		zap.String("Email", email),
		zap.Any(constants.ServiceEvent, constants.EventLogin),
	}
	zlog.LogWithContext(ctx).Info("EmailPasswordAuth", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, constants.EventLoginComplete),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("EmailPasswordAuth", exitLogFields...)
		}
	}()

	loginResult, err := s.authService.Login(ctx, req)
	if err != nil {
		errMsg := err.Error()
		switch {
		case errMsg == "user not found":
			rsp = &vai.AuthResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_EMAIL_NOT_REGISTERED, "Email not registered"),
			}
		case errMsg == "invalid email or password":
			rsp = &vai.AuthResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_EMAIL_OR_PASSWORD, "Invalid email or password"),
			}
		case errMsg == "password not set for this account":
			rsp = &vai.AuthResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_PASSWORD_NOT_SET, "Please use email verification code to login"),
			}
		default:
			rsp = &vai.AuthResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, errMsg),
			}
		}
		return rsp, nil
	}

	user := loginResult.User
	userInfo := s.userService.BuildUserInfoByUserRecord(user)

	// 直接使用 user_record 表的字段
	userInfo.Nickname = user.UserName
	userInfo.Avatar = user.AvatarUrl
	userInfo.RegisterTime = time.Unix(user.CreateTime, 0).Format(common.TimestampFormat)

	err = s.BuildUserInfo(ctx, lang, os, userInfo, loginResult.CreditGranted, loginResult.CreditAmount)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_INVALID_REQUEST, err, constants.ErrMsgInvalidRequest)
		return rsp, nil
	}

	rsp = &vai.AuthResponse{
		ResponseHeader: common.BuildResponseHeader(vai.StatusCode_SUCCESS, "success"),
		AccessToken:    user.AccessToken,
		ExpireTime:     time.Unix(user.AccessTokenExpired, 0).Format(common.TimestampFormat),
		RefreshToken:   user.RefreshToken,
		UserInfo:       userInfo,
		IsFirstLogin:   false,
		AuthType:       vai.AuthType_AUTH_TYPE_EMAIL,
	}

	return rsp, nil
}

// EmailRegister 邮箱注册
func (s *AuthServer) EmailRegister(ctx context.Context, req *vai.EmailRegisterRequest) (*vai.EmailRegisterResponse, error) {
	reqHeader := req.GetRequestHeader()
	email := req.GetEmail()
	verifyCode := req.GetVerifyCode()
	password := req.GetPassword()
	confirmPassword := req.GetConfirmPassword()
	var err error
	var rsp *vai.EmailRegisterResponse

	entryLogFields := []zap.Field{
		zap.String("Email", email),
		zap.Any(constants.ServiceEvent, "EmailRegister"),
	}
	zlog.LogWithContext(ctx).Info("EmailRegister", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, "EmailRegisterComplete"),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, reqHeader.GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("EmailRegister", exitLogFields...)
		}
	}()

	if !utils.IsValidEmail(email) {
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_EMAIL_FORMAT, "Invalid email format"),
		}
		return rsp, nil
	}

	if password != confirmPassword {
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_PASSWORD_FORMAT, "Passwords do not match"),
		}
		return rsp, nil
	}

	if len(password) < 6 || len(password) > 20 {
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_PASSWORD_FORMAT, "Password length must be between 6-20 characters"),
		}
		return rsp, nil
	}

	emailVerifyService, err := service.NewEmailVerifyService()
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to create email verify service", zap.Error(err))
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, constants.ErrMsgInternalServer),
		}
		return rsp, nil
	}

	if err := emailVerifyService.Verify(ctx, email, verifyCode); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "expired") {
			rsp = &vai.EmailRegisterResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_EMAIL_VERIFY_CODE_EXPIRED, "Verification code expired. Please request a new one."),
			}
		} else {
			rsp = &vai.EmailRegisterResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_EMAIL_VERIFY_CODE, "Invalid verification code"),
			}
		}
		return rsp, nil
	}

	projectID := common.GetProjectID(ctx)
	existingUser, _ := s.authService.GetUserDao().GetUserByEmail(projectID, email)
	if existingUser != nil {
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_EMAIL_ALREADY_EXISTS, "Email already registered"),
		}
		return rsp, nil
	}

	emailAuthReq := &vai.EmailAuthRequest{
		RequestHeader: reqHeader,
		Email:         email,
		VerifyCode:    verifyCode,
	}

	loginResult, err := s.authService.Login(ctx, emailAuthReq)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to create user", zap.Error(err))
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, "Registration failed: "+err.Error()),
		}
		return rsp, nil
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to hash password", zap.Error(err))
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, "Failed to set password"),
		}
		return rsp, nil
	}

	user := loginResult.User
	user.PasswordHash = string(passwordHash)
	if err := s.authService.GetUserDao().Update(user); err != nil {
		zlog.LogWithContext(ctx).Error("Failed to update user password", zap.Error(err))
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, "Failed to set password"),
		}
		return rsp, nil
	}

	emailVerifyService.DeleteVerifyCode(ctx, email)

	zlog.LogWithContext(ctx).Info("Email registration successful",
		zap.String("email", email),
		zap.String("user_id", user.UserId))

	rsp = &vai.EmailRegisterResponse{
		ResponseHeader:  common.BuildResponseHeader(vai.StatusCode_SUCCESS, "Registration successful"),
		NeedSetPassword: false,
	}

	return rsp, nil
}

// SendPasswordResetCode 发送密码重置验证码
func (s *AuthServer) SendPasswordResetCode(ctx context.Context, req *vai.SendPasswordResetCodeRequest) (*vai.SendPasswordResetCodeResponse, error) {
	reqHeader := req.GetRequestHeader()
	email := req.GetEmail()
	var err error
	var rsp *vai.SendPasswordResetCodeResponse

	entryLogFields := []zap.Field{
		zap.String("Email", email),
		zap.Any(constants.ServiceEvent, "SendPasswordResetCode"),
	}
	zlog.LogWithContext(ctx).Info("SendPasswordResetCode", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, "SendPasswordResetCodeComplete"),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, reqHeader.GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("SendPasswordResetCode", exitLogFields...)
		}
	}()

	// 1. 校验邮箱格式
	if !utils.IsValidEmail(email) {
		rsp = &vai.SendPasswordResetCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_EMAIL_FORMAT, "Invalid email format"),
		}
		return rsp, nil
	}

	// 2. 校验邮箱已注册
	projectID := common.GetProjectID(ctx)
	existingUser, _ := s.authService.GetUserDao().GetUserByEmail(projectID, email)
	if existingUser == nil {
		rsp = &vai.SendPasswordResetCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_EMAIL_NOT_REGISTERED, "Email not registered"),
		}
		return rsp, nil
	}

	// 3. 校验用户已设置密码
	if existingUser.PasswordHash == "" {
		rsp = &vai.SendPasswordResetCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_PASSWORD_NOT_SET, "Password not set, please use verification code to login"),
		}
		return rsp, nil
	}

	// 4. 发送密码重置验证码
	emailVerifyService, err := service.NewEmailVerifyService()
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to create email verify service", zap.Error(err))
		rsp = &vai.SendPasswordResetCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, constants.ErrMsgInternalServer),
		}
		return rsp, nil
	}

	_, err = emailVerifyService.SendPasswordResetCode(ctx, email)
	if err != nil {
		rsp = &vai.SendPasswordResetCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, "Failed to send password reset code"),
		}
		return rsp, nil
	}

	rsp = &vai.SendPasswordResetCodeResponse{
		ResponseHeader: common.BuildResponseHeader(vai.StatusCode_SUCCESS, "success"),
	}
	return rsp, nil
}

// ResetPassword 重置密码
func (s *AuthServer) ResetPassword(ctx context.Context, req *vai.ResetPasswordRequest) (*vai.ResetPasswordResponse, error) {
	reqHeader := req.GetRequestHeader()
	email := req.GetEmail()
	verifyCode := req.GetVerifyCode()
	newPassword := req.GetNewPassword()
	confirmPassword := req.GetConfirmPassword()
	var err error
	var rsp *vai.ResetPasswordResponse

	entryLogFields := []zap.Field{
		zap.String("Email", email),
		zap.Any(constants.ServiceEvent, "ResetPassword"),
	}
	zlog.LogWithContext(ctx).Info("ResetPassword", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, "ResetPasswordComplete"),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, reqHeader.GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("ResetPassword", exitLogFields...)
		}
	}()

	// 1. 校验邮箱格式
	if !utils.IsValidEmail(email) {
		rsp = &vai.ResetPasswordResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_EMAIL_FORMAT, "Invalid email format"),
		}
		return rsp, nil
	}

	// 2. 校验密码长度和一致性
	if len(newPassword) < 6 || len(newPassword) > 20 {
		rsp = &vai.ResetPasswordResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_PASSWORD_FORMAT, "Password length must be between 6-20 characters"),
		}
		return rsp, nil
	}

	if newPassword != confirmPassword {
		rsp = &vai.ResetPasswordResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_PASSWORD_FORMAT, "Passwords do not match"),
		}
		return rsp, nil
	}

	// 3. 校验邮箱已注册
	projectID := common.GetProjectID(ctx)
	existingUser, _ := s.authService.GetUserDao().GetUserByEmail(projectID, email)
	if existingUser == nil {
		rsp = &vai.ResetPasswordResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_EMAIL_NOT_REGISTERED, "Email not registered"),
		}
		return rsp, nil
	}

	// 4. 验证验证码
	emailVerifyService, err := service.NewEmailVerifyService()
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to create email verify service", zap.Error(err))
		rsp = &vai.ResetPasswordResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, constants.ErrMsgInternalServer),
		}
		return rsp, nil
	}

	if err := emailVerifyService.VerifyPasswordResetCode(ctx, email, verifyCode); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "过期") {
			rsp = &vai.ResetPasswordResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_EMAIL_VERIFY_CODE_EXPIRED, "Verification code expired. Please request a new one."),
			}
		} else {
			rsp = &vai.ResetPasswordResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_EMAIL_VERIFY_CODE, "Invalid verification code"),
			}
		}
		return rsp, nil
	}

	// 5. 哈希新密码
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to hash password", zap.Error(err))
		rsp = &vai.ResetPasswordResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, "Failed to reset password"),
		}
		return rsp, nil
	}

	// 6. 更新密码
	existingUser.PasswordHash = string(passwordHash)
	if err := s.authService.GetUserDao().Update(existingUser); err != nil {
		zlog.LogWithContext(ctx).Error("Failed to update user password", zap.Error(err))
		rsp = &vai.ResetPasswordResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, "Failed to reset password"),
		}
		return rsp, nil
	}

	// 7. 清理验证码
	emailVerifyService.DeletePasswordResetCode(ctx, email)

	zlog.LogWithContext(ctx).Info("Password reset successful",
		zap.String("email", email),
		zap.String("user_id", existingUser.UserId))

	rsp = &vai.ResetPasswordResponse{
		ResponseHeader: common.BuildResponseHeader(vai.StatusCode_SUCCESS, "Password reset successful"),
	}
	return rsp, nil
}
