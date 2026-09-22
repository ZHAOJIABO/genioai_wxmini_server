package api

import (
	"context"

	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/auth"
	"va_visionai_server/internal/service/chat"
	"va_visionai_server/internal/service/profile"
	"va_visionai_server/internal/service/push_gateway"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type UserServer struct {
	userService             *service.UserService
	userPersonalInfoService *service.UserPersonalInfoService
	configService           *service.ConfigService
	subscribeService        *service.SubscribeService
	authService             *auth.AuthService
	chatService             *chat.ChatService
	profileService          *profile.ProfileService
	pushGatewayClient       *push_gateway.Client

	vai.UnimplementedUserServiceServer
}

func NewUserServer(
	userService *service.UserService,
	userPersonalInfoService *service.UserPersonalInfoService,
	configService *service.ConfigService,
	subscribeService *service.SubscribeService,
	authService *auth.AuthService,
	_ interface{},
	chatService *chat.ChatService,
	profileService *profile.ProfileService,
	pushGatewayClient *push_gateway.Client,
) *UserServer {
	return &UserServer{
		userService:             userService,
		userPersonalInfoService: userPersonalInfoService,
		configService:           configService,
		subscribeService:        subscribeService,
		authService:             authService,
		chatService:             chatService,
		profileService:          profileService,
		pushGatewayClient:       pushGatewayClient,
	}
}

// GetUserInfo 获取用户信息
func (s *UserServer) GetUserInfo(ctx context.Context, req *vai.UserInfoRequest) (*vai.UserInfoResponse, error) {
	reqHeader := req.GetRequestHeader()
	os := constants.MappingOS(reqHeader.GetDevice().GetOs())
	lang := constants.LanguageMap(reqHeader.GetDevice().GetLanguage())
	var err error
	var rsp *vai.UserInfoResponse

	defer func() {
		zlog.LogWithContext(ctx).Info("GetUserInfo",
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
			zap.Error(err))
	}()

	switch reqHeader.GetUserType() {
	case vai.UserType_USER_TYPE_REGISTER:
		code, err := isVerifyAccessToken(s.userService, reqHeader)
		if err != nil {
			rsp = &vai.UserInfoResponse{}
			rsp, err = BuildErrorResponse[vai.UserInfoResponse](ctx, code, err, constants.CodeMsg(code))
			return rsp, nil
		}
	case vai.UserType_USER_TYPE_TEMP:
	}

	userInfo, err := s.userService.GetUserInfo(ctx, reqHeader.GetUserId())
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetUserInfo Error",
			zap.String("UserId", reqHeader.GetUserId()),
			zap.Error(err))
		rsp = &vai.UserInfoResponse{}
		rsp, err = BuildErrorResponse[vai.UserInfoResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	// 直接使用 user_record 表的字段，不再查询 va_user_profile 表
	// Nickname 和 Avatar 已经在 GetUserInfo 中从 user_record 获取

	err = s.BuildUserInfo(ctx, lang, os, userInfo)
	if err != nil {
		rsp = &vai.UserInfoResponse{}
		rsp, err = BuildErrorResponse[vai.UserInfoResponse](ctx, vai.StatusCode_INVALID_REQUEST, err, constants.ErrMsgInvalidRequest)
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.UserInfoResponse{
		UserInfo: userInfo,
	})
	return rsp, nil
}

func (s *UserServer) BuildUserInfo(ctx context.Context, lang, os string, userInfo *vai.UserInfo) error {
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

	// 对于 GetUserInfo 接口，积分弹窗字段应该始终为 false
	// 因为积分弹窗只在登录时首次显示
	// 这里需要根据实际的 UserConfig 字段来设置
	// userInfo.UserConfig.PopDailyCredits = false

	return nil
}

// DeleteUser 注销账号
func (s *UserServer) DeleteUser(ctx context.Context, req *vai.DeleteUserRequest) (*vai.DeleteUserResponse, error) {
	reqHeader := req.GetRequestHeader()
	var err error
	var rsp *vai.DeleteUserResponse

	defer func() {
		zlog.LogWithContext(ctx).Info("DeleteUser",
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
			zap.Error(err),
		)
	}()

	// 已注册用户需验证 access token
	if reqHeader.GetUserType() == vai.UserType_USER_TYPE_REGISTER {
		if code, tokenErr := isVerifyAccessToken(s.userService, reqHeader); tokenErr != nil {
			rsp = &vai.DeleteUserResponse{}
			rsp, err = BuildErrorResponse[vai.DeleteUserResponse](ctx, code, tokenErr, constants.CodeMsg(code))
			return rsp, nil
		}
	}

	projectID := common.GetProjectID(ctx)
	userID := reqHeader.GetUserId()

	user, err := s.userService.GetUserRecord(ctx, userID)
	if err != nil {
		rsp = &vai.DeleteUserResponse{}
		rsp, err = BuildErrorResponse[vai.DeleteUserResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}
	os := constants.MappingOS(reqHeader.GetDevice().GetOs())
	if os == constants.IOS {
		err = s.authService.RevokeRefreshToken(ctx, user.AppleRefreshToken, projectID, "apple")
		if err != nil {
			zlog.LogWithContext(ctx).Error("RevokeRefreshToken Error", zap.Error(err))
		}
	}

	// 清理用户聊天数据
	if err := s.chatService.ArchiveAllChats(ctx, projectID, userID); err != nil {
		zlog.LogWithContext(ctx).Error("Failed to archive user chats",
			zap.String("userID", userID),
			zap.Error(err))
	}

	// 清理用户个人资料数据
	if err := s.profileService.DeleteUserProfile(ctx, projectID, userID); err != nil {
		zlog.LogWithContext(ctx).Error("Failed to delete user profile",
			zap.String("userID", userID),
			zap.Error(err))
	}

	// 调用 Service 软删除用户
	err = s.userService.DeleteUser(ctx, userID)
	if err != nil {
		rsp = &vai.DeleteUserResponse{}
		rsp, err = BuildErrorResponse[vai.DeleteUserResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.DeleteUserResponse{})
	return rsp, nil
}

// EditUserInfo 修改用户个人信息
func (s *UserServer) EditUserInfo(ctx context.Context, req *vai.EditUserInfoRequest) (*vai.EditUserInfoResponse, error) {
	reqHeader := req.GetRequestHeader()
	var err error
	var rsp *vai.EditUserInfoResponse

	defer func() {
		zlog.LogWithContext(ctx).Info("EditUserInfo",
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
			zap.Error(err),
		)
	}()

	// 仅注册用户需验证 access token
	if reqHeader.GetUserType() == vai.UserType_USER_TYPE_REGISTER {
		if code, tokenErr := isVerifyAccessToken(s.userService, reqHeader); tokenErr != nil {
			rsp = &vai.EditUserInfoResponse{}
			rsp, err = BuildErrorResponse[vai.EditUserInfoResponse](ctx, code, tokenErr, constants.CodeMsg(code))
			return rsp, nil
		}
	}

	err = s.userService.EditUserInfo(ctx, reqHeader.GetUserId(), req.GetUserName(), req.GetAvatar())
	if err != nil {
		zlog.LogWithContext(ctx).Error("EditUserInfo Error",
			zap.String("UserId", reqHeader.GetUserId()),
			zap.Error(err))
		rsp = &vai.EditUserInfoResponse{}
		rsp, err = BuildErrorResponse[vai.EditUserInfoResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.EditUserInfoResponse{})
	return rsp, nil
}

// UserNotifyToken 用户推送 token 上报
func (s *UserServer) UserNotifyToken(ctx context.Context, req *vai.UserNotifyTokenRequest) (*vai.UserNotifyTokenResponse, error) {
	reqHeader := req.GetRequestHeader()
	token := req.GetToken()
	var err error
	var rsp *vai.UserNotifyTokenResponse

	defer func() {
		zlog.LogWithContext(ctx).Info("UserNotifyToken",
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
			zap.Error(err))
	}()

	if token == "" {
		rsp, err = BuildErrorResponse[vai.UserNotifyTokenResponse](ctx, vai.StatusCode_INVALID_REQUEST, nil, constants.ErrMsgInvalidRequest)
		return rsp, nil
	}

	userID := reqHeader.GetUserId()
	osType := reqHeader.GetDevice().GetOs()
	appID := GetPackageName(reqHeader)

	platform := "android"
	if osType == 2 {
		platform = "ios"
	}

	if s.pushGatewayClient == nil {
		zlog.LogWithContext(ctx).Warn("pushGatewayClient 未初始化，跳过推送 token 注册",
			zap.String("user_id", userID))
		rsp, err = BuildSuccessResponse(&vai.UserNotifyTokenResponse{})
		return rsp, nil
	}

	registerErr := s.pushGatewayClient.RegisterDevice(ctx, &push_gateway.RegisterDeviceRequest{
		UserID:   userID,
		Platform: platform,
		Token:    token,
		AppID:    appID,
	})
	if registerErr != nil {
		zlog.LogWithContext(ctx).Error("注册推送 token 到 push-gateway 失败",
			zap.String("user_id", userID),
			zap.String("platform", platform),
			zap.Error(registerErr))
	}

	rsp, err = BuildSuccessResponse(&vai.UserNotifyTokenResponse{})
	return rsp, nil
}
