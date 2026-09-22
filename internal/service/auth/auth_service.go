package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/geoip"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/auth/types"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// LoginResult 登录结果，包含用户信息和积分发放信息
type LoginResult struct {
	User          *model.UserRecord
	CreditGranted bool // 是否发放了积分
	CreditAmount  int  // 发放的积分数量
	IsNewUser     bool
}

type AuthService struct {
	providerManager         *ProviderManager
	userDao                 *dao.UserDao
	redisClient             *redis.Client
	deviceService           *service.UserDeviceService
	dailyFreeCreditsService *service.DailyFreeCreditsService
}

func NewAuthService(userDao *dao.UserDao, providerManager *ProviderManager, redisClient *redis.Client, userDeviceService *service.UserDeviceService, dailyFreeCreditsService *service.DailyFreeCreditsService) *AuthService {
	return &AuthService{
		providerManager:         providerManager,
		userDao:                 userDao,
		redisClient:             redisClient,
		deviceService:           userDeviceService,
		dailyFreeCreditsService: dailyFreeCreditsService,
	}
}

// GetUserDao 返回 UserDao 实例，供外部访问
func (s *AuthService) GetUserDao() *dao.UserDao {
	return s.userDao
}

func (s *AuthService) extractBaseParams(ctx context.Context, header *vai.RequestHeader) *types.BaseLoginParams {
	return &types.BaseLoginParams{
		ProjectID: common.GetProjectID(ctx),
		DeviceInfo: &types.DeviceInfo{
			OS:         constants.MappingOS(header.GetDevice().GetOs()),
			DeviceID:   header.GetDevice().GetAndroidId(),
			AppVersion: header.GetApp().GetAppVersion(),
			UserType:   header.GetUserType().String(),
		},
		ClientIP: common.GetClientIP(ctx),
	}
}

func (s *AuthService) Login(ctx context.Context, req interface{}) (*LoginResult, error) {

	var loginParams types.LoginParams
	var baseParams *types.BaseLoginParams
	var deviceInfo *vai.Device
	var appInfo *vai.App
	var providerType string
	var header *vai.RequestHeader
	switch typedReq := req.(type) {
	case *vai.GuestLoginRequest:
		header = typedReq.GetRequestHeader()
		baseParams = s.extractBaseParams(ctx, header)
		deviceInfo, appInfo = s.extractDeviceAndAppInfo(header)
		loginParams = &types.TempUserLoginParams{
			BaseLoginParams: baseParams,
			TempUserIden:    s.extractTempUserIden(ctx, header),
		}
		providerType = "temp"
	case *vai.PhoneAuthRequest:
		header = typedReq.GetRequestHeader()
		baseParams = s.extractBaseParams(ctx, header)
		deviceInfo, appInfo = s.extractDeviceAndAppInfo(header)
		loginParams = &types.PhoneVerifyLoginParams{
			BaseLoginParams: baseParams,
			PhoneNumber:     typedReq.GetPhoneNumber(),
			VerifyCode:      typedReq.GetVerifyCode(),
		}
		providerType = "default"
	case *vai.AppleAuthRequest:
		header = typedReq.GetRequestHeader()
		baseParams = s.extractBaseParams(ctx, header)
		deviceInfo, appInfo = s.extractDeviceAndAppInfo(header)
		loginParams = &types.AppleLoginParams{
			BaseLoginParams:   baseParams,
			IdToken:           typedReq.GetAppleIdToken(),
			AuthorizationCode: typedReq.GetAppleAuthorizationCode(),
		}
		providerType = "apple"
	case *vai.GoogleAuthRequest:
		header = typedReq.GetRequestHeader()
		baseParams = s.extractBaseParams(ctx, header)
		deviceInfo, appInfo = s.extractDeviceAndAppInfo(header)
		loginParams = &types.GoogleLoginParams{
			BaseLoginParams: baseParams,
			IdToken:         typedReq.GetGoogleIdToken(),
		}
		providerType = "google"
	case *vai.WeChatAuthRequest:
		header = typedReq.GetRequestHeader()
		baseParams = s.extractBaseParams(ctx, header)
		deviceInfo, appInfo = s.extractDeviceAndAppInfo(header)
		loginParams = &types.WeChatLoginParams{
			BaseLoginParams: baseParams,
			Code:            typedReq.GetCode(),
			AuthType:        typedReq.GetAuthType(),
		}
		providerType = "wechat_miniprogram"
	case *vai.EmailAuthRequest:
		header = typedReq.GetRequestHeader()
		baseParams = s.extractBaseParams(ctx, header)
		deviceInfo, appInfo = s.extractDeviceAndAppInfo(header)
		loginParams = &types.EmailVerifyLoginParams{
			BaseLoginParams: baseParams,
			Email:           typedReq.GetEmail(),
			VerifyCode:      typedReq.GetVerifyCode(),
		}
		providerType = "email"
	case *vai.EmailPasswordAuthRequest:
		header = typedReq.GetRequestHeader()
		baseParams = s.extractBaseParams(ctx, header)
		deviceInfo, appInfo = s.extractDeviceAndAppInfo(header)
		loginParams = &types.EmailPasswordLoginParams{
			BaseLoginParams: baseParams,
			Email:           typedReq.GetEmail(),
			Password:        typedReq.GetPassword(),
		}
		providerType = "email_password"
	default:
		zlog.LogWithContext(ctx).Error("unsupported request type", zap.Any("request", req))
		return nil, fmt.Errorf("unsupported request type: %T", req)
	}
	os := constants.MappingOS(deviceInfo.GetOs())
	clientVersion := appInfo.GetAppVersion()
	if clientVersion == "" && header.GetWebClient() != nil {
		clientVersion = header.GetWebClient().GetClientVersion()
	}
	if err := loginParams.Validate(); err != nil {
		return nil, err
	}

	provider, err := s.providerManager.GetProvider(baseParams.ProjectID, providerType)
	if err != nil {
		return nil, err
	}

	loginInfo, err := provider.Login(ctx, loginParams)
	if err != nil {
		zlog.LogWithContext(ctx).Error("登录失败",
			zap.String("login_type", providerType),
			zap.Error(err))
		return nil, err
	}

	userID, err := s.GenerateUserIDFromProviderID(ctx, loginInfo.ProviderUserID, providerType)
	if err != nil {
		zlog.LogWithContext(ctx).Error("生成用户ID失败",
			zap.String("login_type", providerType),
			zap.Error(err))
		return nil, err
	}

	deviceTimezone := deviceInfo.GetTimezone()
	user, isNewUser, err := s.createOrUpdateUser(ctx, userID, loginInfo, providerType, os, deviceTimezone, clientVersion)
	if err != nil {
		return nil, err
	}
	_, err = s.deviceService.CreateOrUpdate(header, user.UserId, "")
	if err != nil {
		zlog.LogWithContext(ctx).Error("Update User Device Info Error", zap.Error(err))
	}

	if isNewUser {
		zlog.LogWithContext(ctx).Info("Add New User",
			zap.String(constants.CtxIdfv, deviceInfo.GetIdfv()),
			zap.String(constants.CtxAndroidId, deviceInfo.GetAndroidId()),
			zap.String(constants.CtxPackageName, appInfo.GetPackageName()),
			zap.String(constants.CtxUserID, userID),
			zap.Any(constants.ServiceEvent, constants.EventNewUser),
		)
	}

	// 同步发放每日免费积分（已禁用）
	// granted, amount, err := s.syncGrantDailyCredits(ctx, user.ProjectID, user.UserId)
	// if err != nil {
	// 	zlog.LogWithContext(ctx).Warn("grant daily credits on login failed",
	// 		zap.Error(err), zap.String("project_id", user.ProjectID), zap.String("user_id", user.UserId))
	// }
	granted := false
	amount := 0

	// 新用户注册奖励（游客除外）
	if isNewUser && providerType != "temp" {
		s.grantRegisterBonus(ctx, user.ProjectID, user.UserId)
	}

	// 返回登录结果，包含积分发放信息
	return &LoginResult{
		User:          user,
		CreditGranted: granted,
		CreditAmount:  amount,
		IsNewUser:     isNewUser,
	}, nil
}

func (s *AuthService) RefreshAccessToken(ctx context.Context, userID, refreshToken, os string) (*types.TokenInfo, error) {
	projectID := common.GetProjectID(ctx)
	logFields := []zap.Field{
		zap.String("project_id", projectID),
		zap.String("user_id", userID),
		zap.Any(constants.ServiceEvent, "token_refresh"),
	}
	zlog.LogWithContext(ctx).Info("Refreshing token", logFields...)

	user, err := s.userDao.GetUserByIdOrPhone(projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("User not found",
			append(logFields, zap.Error(err))...)
		return nil, fmt.Errorf("user not found: %w", err)
	}

	if user.RefreshToken != refreshToken {
		zlog.LogWithContext(ctx).Error("Invalid refresh token",
			logFields...)
		return nil, types.ErrInvalidToken
	}

	now := types.CurrentTimeInSeconds()
	if user.RefreshTokenExpired < now {
		zlog.LogWithContext(ctx).Error("Refresh token expired",
			logFields...)
		return nil, types.ErrTokenRefreshFailed
	}

	userRecord, err := s.userDao.RefreshAccessToken(projectID, refreshToken, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to reset tokens",
			append(logFields, zap.Error(err))...)
		return nil, fmt.Errorf("failed to reset tokens: %w", err)
	}
	sessionKey := s.GetUserSessionKey(*userRecord, os)
	_, err = s.redisClient.Set(sessionKey, userRecord.AccessToken, time.Duration(userRecord.AccessTokenExpired-now)*time.Second).Result()
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to set session",
			append(logFields, zap.Error(err))...)
		return nil, fmt.Errorf("failed to set session: %w", err)
	}

	tokenInfo := &types.TokenInfo{
		AccessToken: userRecord.AccessToken,
		ExpiresIn:   userRecord.AccessTokenExpired - now,
	}

	zlog.LogWithContext(ctx).Info("Token refreshed successfully",
		logFields...)

	return tokenInfo, nil
}
func (s *AuthService) GetUserSessionKey(user model.UserRecord, os string) string {
	return fmt.Sprintf("%s:%s:%s", constants.RedisKeyUserSession, user.UserId, os)
}

func (s *AuthService) Logout(ctx context.Context, userID string) error {
	projectID := common.GetProjectID(ctx)
	logFields := []zap.Field{
		zap.String("project_id", projectID),
		zap.String("user_id", userID),
		zap.Any(constants.ServiceEvent, "logout"),
	}
	zlog.LogWithContext(ctx).Info("Logging out", logFields...)

	err := s.userDao.ResetTokensWithEmptyValue(projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to reset tokens",
			append(logFields, zap.Error(err))...)
		return fmt.Errorf("failed to reset tokens: %w", err)
	}

	zlog.LogWithContext(ctx).Info("Logout successful", logFields...)

	return nil
}

func (s *AuthService) createOrUpdateUser(ctx context.Context, userID string, loginInfo *types.LoginInfo, loginType string, os string, deviceTimezone string, clientVersion string) (*model.UserRecord, bool, error) {
	projectID := common.GetProjectID(ctx)

	user, err := s.userDao.GetUserByIdOrPhone(projectID, userID)
	var isNewUser bool
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("查询用户失败: %w", err)
	}
	var refreshToken string
	if loginInfo.ExtraData != nil {
		if v, ok := loginInfo.ExtraData["refresh_token"].(string); ok && v != "" {
			refreshToken = v
		}
	}

	if err != nil {
		now := time.Now()
		isNewUser = true
		uuid, err := s.GenerateUniqueUUID(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("生成UUID失败: %w", err)
		}
		userName := loginInfo.Email
		if userName == "" {
			userName = uuid
		}
		exists, err := s.CheckUserNameExists(ctx, userName)
		if err != nil {
			zlog.LogWithContext(ctx).Error("检查用户名是否存在失败", zap.Error(err))
		}
		if exists {
			zlog.LogWithContext(ctx).Error("用户名已存在", zap.String("user_name", userName))
			userName = s.GenerateUniqueUsername(ctx, userName)
		}

		// 获取用户国家代码（从IP解析）
		clientIP := extractClientIP(ctx)
		country := geoip.GetCountryCode(clientIP)

		user = &model.UserRecord{
			ProjectID:      projectID,
			UserId:         userID,
			UUID:           uuid,
			UserName:       userName,
			PhoneNumber:    loginInfo.PhoneNumber,
			Email:          loginInfo.Email,
			AvatarUrl:      loginInfo.Avatar,
			CreateTime:     now.Unix(),
			UpdateTime:     now.Unix(),
			LoginType:      loginType,
			RegisterIP:     clientIP,
			LastLoginIP:    clientIP,
			ProviderUserID: loginInfo.ProviderUserID,
		}
		// 只在解析到有效地理信息时才设置（避免保存 "unknown" 和空字符串）
		if country != "" && country != "unknown" {
			user.Country = country
		}
		// 使用客户端上报的时区
		if deviceTimezone != "" {
			user.Timezone = deviceTimezone
		}
		if clientVersion != "" {
			user.AppVersion = clientVersion
		}
		if loginType == "apple" && refreshToken != "" {
			user.AppleRefreshToken = refreshToken
		}

		if err := s.userDao.Create(*user); err != nil {
			return nil, false, fmt.Errorf("创建用户失败: %w", err)
		}
	} else {
		// 获取用户国家代码（从IP解析）
		clientIP := extractClientIP(ctx)
		country := geoip.GetCountryCode(clientIP)

		user.LastLoginIP = clientIP
		// 更新用户国家代码（仅在解析到有效值时更新，允许覆盖原有的 "unknown" 值）
		if country != "" && country != "unknown" {
			user.Country = country
		}
		// 使用客户端上报的时区
		if deviceTimezone != "" {
			user.Timezone = deviceTimezone
		}

		if loginInfo.NickName != "" {
			user.UserName = loginInfo.NickName
		}
		if loginInfo.Email != "" {
			user.Email = loginInfo.Email
		}
		if loginInfo.Avatar != "" {
			user.AvatarUrl = loginInfo.Avatar
		}
		if loginInfo.PhoneNumber != "" {
			user.PhoneNumber = loginInfo.PhoneNumber
		}
		if user.UUID == "" {
			uuid, err := s.GenerateUniqueUUID(ctx)
			if err != nil {
				zlog.LogWithContext(ctx).Error("generate uuid failed for existing user without uuid", zap.Error(err))
			}
			user.UUID = uuid
		}
		if loginType == "apple" && refreshToken != "" {
			user.AppleRefreshToken = refreshToken
		}
		if err := s.userDao.Update(user); err != nil {
			return nil, false, fmt.Errorf("更新用户失败: %w", err)
		}
	}

	userRecord, err := s.userDao.ResetTokens(projectID, userID)
	if err != nil {
		return nil, false, fmt.Errorf("重置令牌失败: %w", err)
	}
	now := types.CurrentTimeInSeconds()
	_, err = s.redisClient.Set(s.GetUserSessionKey(userRecord, os), userRecord.AccessToken, time.Duration(userRecord.AccessTokenExpired-now)*time.Second).Result()
	if err != nil {
		return nil, false, fmt.Errorf("设置session失败: %w", err)
	}

	return &userRecord, isNewUser, nil
}

func extractClientIP(ctx context.Context) string {
	return common.GetClientIP(ctx)
}

func (s *AuthService) extractTempUserIden(ctx context.Context, reqHeader *vai.RequestHeader) string {
	iden := ""
	result := strings.Builder{}
	result.WriteString("86")
	device := reqHeader.GetDevice()
	deviceOS := constants.MappingOS(device.GetOs())

	switch deviceOS {
	case constants.ANDROID:
		aid := device.GetAndroidId()
		if aid != "" {
			iden = aid
		} else {
			iden = device.GetOaid()
		}
		result.WriteString("3")
	case constants.IOS:
		idfv := device.GetIdfv()
		if idfv != "" {
			iden = idfv
		} else {
			iden = device.GetIdfa()
		}
		result.WriteString("2")
	}

	if iden == "" {
		uuidStr := uuid.New().String()
		iden = strings.ReplaceAll(uuidStr, "-", "")
	}
	iden = s.GenerateUserIDFromString(ctx, iden)
	result.WriteString(iden[:10])
	zlog.LogWithContext(ctx).Info("Generate Temp UserID",
		zap.String("TempUserID", result.String()),
		zap.String(constants.CtxOSName, deviceOS),
		zap.String("Iden", iden),
	)
	return result.String()
}

func (s *AuthService) extractDeviceAndAppInfo(header *vai.RequestHeader) (*vai.Device, *vai.App) {
	return header.GetDevice(), header.GetApp()
}

func (s *AuthService) GenerateUserIDFromString(ctx context.Context, inputStr string) string {
	if inputStr == "" {
		zlog.LogWithContext(ctx).Warn("Empty input string for UserID generation, using UUID as fallback")
		uuidStr := uuid.New().String()
		inputStr = strings.ReplaceAll(uuidStr, "-", "")
	}
	hasher := sha256.New()
	hasher.Write([]byte(inputStr))
	hashBytes := hasher.Sum(nil)
	hashInt := new(big.Int)
	hashInt.SetBytes(hashBytes)
	userID := hashInt.String()
	zlog.LogWithContext(ctx).Info("Generated UserID from input",
		zap.String("input", inputStr),
		zap.String("user_id", userID),
	)

	return userID
}

func (s *AuthService) GenerateUniqueUUID(ctx context.Context) (string, error) {
	for {
		uuidStr := uuid.New().String()
		exists, err := s.CheckUUIDExists(ctx, uuidStr)
		if err != nil {
			return "", err
		}
		if !exists {
			return strings.ReplaceAll(uuidStr, "-", ""), nil
		}
	}
}

func (s *AuthService) CheckUUIDExists(ctx context.Context, uuid string) (bool, error) {
	if uuid == "" {
		return false, constants.ERR_INVALID_PARAM
	}

	projectID := common.GetProjectID(ctx)
	if projectID == "" {
		return false, errors.New("Not found project id")
	}

	return s.userDao.CheckUUIDExists(projectID, uuid)
}

func (s *AuthService) CheckUserNameExists(ctx context.Context, userName string) (bool, error) {
	if userName == "" {
		return false, constants.ERR_INVALID_PARAM
	}

	projectID := common.GetProjectID(ctx)
	if projectID == "" {
		return false, errors.New("Not found project id")
	}

	exists, err := s.userDao.CheckUserNameExists(projectID, userName)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Check user name exists error",
			zap.String("project_id", projectID),
			zap.String("user_name", userName),
			zap.Error(err))
		return false, err
	}

	return exists, nil
}

func (s *AuthService) GenerateUniqueUsername(ctx context.Context, customUUID string) string {
	projectID := common.GetProjectID(ctx)
	if projectID == "" {
		return ""
	}
	baseUUID := customUUID
	baseUUID = strings.ReplaceAll(baseUUID, "-", "")

	for {
		uuidPrefix := baseUUID
		if len(uuidPrefix) > 10 {
			uuidPrefix = uuidPrefix[:10]
		}
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		randomNum := r.Intn(9000) + 1000
		username := fmt.Sprintf("%s#%d", uuidPrefix, randomNum)
		exists, err := s.CheckUserNameExists(ctx, username)
		if err != nil {
			zlog.LogWithContext(ctx).Error("Check username exists error",
				zap.String("project_id", projectID),
				zap.String("username", username),
				zap.Error(err))
			continue
		}

		if !exists {
			zlog.LogWithContext(ctx).Info("Generated unique username",
				zap.String("project_id", projectID),
				zap.String("username", username),
			)
			return username
		}
		zlog.LogWithContext(ctx).Info("Username already exists, trying again",
			zap.String("project_id", projectID),
			zap.String("username", username),
		)
	}
}

func (s *AuthService) GenerateUserIDFromProviderID(ctx context.Context, providerID, providerType string) (string, error) {
	if providerID == "" {
		zlog.LogWithContext(ctx).Error("提供的providerID为空")
		return "", errors.New("providerID不能为空")
	}
	if providerType == "default" || providerType == "temp" {
		return providerID, nil
	}
	hasher := sha256.New()
	hasher.Write([]byte(providerID))
	hashBytes := hasher.Sum(nil)
	userID := hex.EncodeToString(hashBytes)
	zlog.LogWithContext(ctx).Info("从providerID生成UserID",
		zap.String("provider_id", providerID),
		zap.String("user_id", userID),
	)

	return userID[:32], nil
}

func (s *AuthService) RevokeRefreshToken(ctx context.Context, refreshToken, projectID, providerID string) error {
	provider, err := s.providerManager.GetProvider(projectID, providerID)
	if err != nil {
		return err
	}
	return provider.RevokeRefreshToken(ctx, refreshToken)
}

// syncGrantDailyCredits 同步发放每日免费积分
// 返回值: 是否发放, 发放金额, 错误
func (s *AuthService) syncGrantDailyCredits(ctx context.Context, projectID, userID string) (bool, int, error) {
	if s.dailyFreeCreditsService == nil || projectID == "" || userID == "" {
		return false, 0, nil
	}

	// 设置超时控制
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	granted, amount, err := s.dailyFreeCreditsService.CheckAndGrantDailyCredits(timeoutCtx, projectID, userID)
	if err != nil {
		return false, 0, err
	}

	return granted, amount, nil
}

// grantRegisterBonus 为新注册正式用户发放注册奖励积分
func (s *AuthService) grantRegisterBonus(ctx context.Context, projectID, userID string) {
	if s.dailyFreeCreditsService == nil || projectID == "" || userID == "" {
		return
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	granted, amount, err := s.dailyFreeCreditsService.GrantRegisterBonus(timeoutCtx, projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("grant register bonus failed",
			zap.Error(err), zap.String("project_id", projectID), zap.String("user_id", userID))
		return
	}
	if granted {
		zlog.LogWithContext(ctx).Info("register bonus granted on login",
			zap.String("project_id", projectID), zap.String("user_id", userID), zap.Int("amount", amount))
	}
}
