package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type UserService struct {
	// redisCfg    conf.Redis
	redisClient   *redis.Client
	userDao       dao.UserDao
	userAmountDao *dao.UserAmountDao
	deviceService *UserDeviceService
}

func NewUserService(userDao *dao.UserDao, userAmountDao *dao.UserAmountDao, deviceService *UserDeviceService) (*UserService, error) {
	vs := &UserService{
		// redisCfg: conf.GlobalConfig.Redis,
		userDao:       *userDao,
		userAmountDao: userAmountDao,
		deviceService: deviceService,
	}
	return vs, vs.init()
}

func (s *UserService) init() error {
	s.redisClient = db.GetRedis()
	return s.redisClient.Ping().Err()
}

func (s *UserService) Login(ctx context.Context, req *vai.LoginRequest) (*model.UserRecord, error) {
	projectID := common.GetProjectID(ctx)
	phoneNumber := req.GetPhoneNumber()
	iden := phoneNumber
	iden = strings.ReplaceAll(iden, "+", "")
	os := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())
	userType := req.GetRequestHeader().GetUserType()
	androidId := req.GetRequestHeader().GetDevice().GetAndroidId()

	switch userType {
	case vai.UserType_USER_TYPE_REGISTER:
		if !utils.IsValidPhoneNumber(phoneNumber) {
			return nil, errors.New("invalid phone number")
		}
	case vai.UserType_USER_TYPE_TEMP:
		iden = s.extractTempUserIden(ctx, req)
	}

	user, existed, err := s.getOrCreate(ctx, iden, phoneNumber, androidId, userType)
	if err != nil {
		return nil, errors.New("create or get user failed " + err.Error())
	}
	if user.Status == 1 {
		return nil, errors.New("user has blocked")
	}
	if !existed {
		err = s.handleNewUser(ctx, req, user, userType, projectID)
		if err != nil {
			zlog.LogWithContext(ctx).Error("Handle new user error", zap.Error(err))
		}
	}

	_, err = s.deviceService.CreateOrUpdate(req.GetRequestHeader(), user.UserId, req.GetOpenId())
	if err != nil {
		zlog.LogWithContext(ctx).Error("Update User Device Info Error", zap.Error(err))
	}

	userRecord, err := s.userDao.ResetTokens(projectID, user.UserId)
	if err != nil {
		return nil, errors.New("reset user token failed " + err.Error())
	}
	if err = s.refreshSession(userRecord, os); err != nil {
		return nil, errors.New("refresh user token failed " + err.Error())
	}
	rawIp, _ := s.getIpFromCtx(ctx)
	err = s.userDao.ResetIP(projectID, user.UserId, rawIp)
	if err != nil {
		return nil, errors.New("reset user last login ip failed " + err.Error())
	}
	return s.userDao.GetUserByIdOrPhone(projectID, user.UserId)
}

func (s *UserService) InitMappingUser(ctx context.Context) error {
	userMapping, err := s.userDao.ListUserMapping()
	if err != nil {
		return err
	}
	rdb := db.GetRedis()
	for _, item := range userMapping {
		err := rdb.HSet(constants.RedisKeyUserIDMapping+item.ProjectID, item.UserID, item.MappingUserID).Err()
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *UserService) getOrCreate(ctx context.Context, iden, phoneNumber, androidId string, userType vai.UserType) (*model.UserRecord, bool, error) {
	projectID := common.GetProjectID(ctx)
	user, err := s.userDao.GetUserByIdOrPhone(projectID, iden)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	if err == nil {
		return s.handleExistingUser(ctx, user)
	}

	// androidIdUser, err := s.CheckUserByAndroidId(ctx, androidId)
	// if err == nil && androidIdUser.PhoneNumber == "" {
	// 	androidIdUser.PhoneNumber = phoneNumber
	// 	err = udao.UpdateUser(androidIdUser.ProjectID, androidIdUser.UserId, androidIdUser)
	// 	if err != nil {
	// 		zlog.LogWithContext(ctx).Error("Bind Phone Number To Existing User Error", zap.Error(err))
	// 		return nil, false, err
	// 	}
	// 	return androidIdUser, false, nil
	// }

	return s.createNewUser(ctx, iden, phoneNumber, userType, projectID)
}

func (s *UserService) handleExistingUser(ctx context.Context, user *model.UserRecord) (*model.UserRecord, bool, error) {
	var err error
	if err != nil {
		zlog.LogWithContext(ctx).Error("Update User Error", zap.Error(err))
	}
	user = s.getUserWithMapping(ctx, user)
	return user, true, nil
}

func (s *UserService) getUserWithMapping(ctx context.Context, user *model.UserRecord) *model.UserRecord {
	rdb := db.GetRedis()
	userMapping, err := rdb.HGet(constants.RedisKeyUserIDMapping, user.UserId).Result()
	if err != nil {
		zlog.LogWithContext(ctx).Warn("GetUserIDMapping HGet Error", zap.Error(err))
	}
	if userMapping != "" {
		user.UserId = userMapping
	}
	return user
}

func (s *UserService) createNewUser(ctx context.Context, iden, phoneNumber string, userType vai.UserType, projectID string) (*model.UserRecord, bool, error) {
	now := time.Now().Unix()
	user := &model.UserRecord{
		ProjectID:  projectID,
		UserId:     iden,
		CreateTime: now,
		UpdateTime: now,
	}

	s.setUserTypeSpecificFields(user, userType, phoneNumber)

	rawIp, _ := s.getIpFromCtx(ctx)
	user.RegisterIP = rawIp
	user.UUID = strings.ReplaceAll(uuid.New().String(), "-", "")
	err := s.userDao.Create(*user)
	if err != nil {
		return nil, false, err
	}

	return user, false, nil
}

func (s *UserService) setUserTypeSpecificFields(user *model.UserRecord, userType vai.UserType, phoneNumber string) {
	switch userType {
	case vai.UserType_USER_TYPE_REGISTER:
		user.PhoneNumber = phoneNumber
	case vai.UserType_USER_TYPE_TEMP:
	}
	user.UserType = uint(userType)
}

func (s *UserService) CheckUserByAndroidId(ctx context.Context, androidId string) (*model.UserRecord, error) {
	return s.userDao.GetUserByAndroidId(androidId)
}

func (s *UserService) extractTempUserIden(ctx context.Context, req *vai.LoginRequest) string {
	iden := ""
	result := strings.Builder{}
	result.WriteString("86")
	device := req.GetRequestHeader().GetDevice()
	deviceOS := constants.MappingOS(device.GetOs())
	appStore := req.GetRequestHeader().GetAppStore()

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

	switch appStore {
	case vai.AppStore_DOUYIN_MINI_PROGRAM:
		fallthrough
	case vai.AppStore_WECHAT_MINI_PROGRAM:
		iden = req.GetOpenId()
	}

	if iden == "" {
		uuidStr := uuid.New().String()
		iden = strings.ReplaceAll(uuidStr, "-", "")
	}
	hasher := sha256.New()
	hasher.Write([]byte(iden))
	idenHash := hasher.Sum(nil)
	hashInt := new(big.Int)
	hashInt.SetBytes(idenHash)
	result.WriteString(hashInt.String()[:10])
	zlog.LogWithContext(ctx).Info("Generate Temp UserID",
		zap.String("TempUserID", result.String()),
		zap.String(constants.CtxOSName, deviceOS),
		zap.String("Iden", iden),
	)
	return result.String()
}

func (s *UserService) refreshSession(user model.UserRecord, os string) error {
	rdb := db.GetRedis()
	// Lua 脚本
	script := `
    redis.call('SET', KEYS[1], ARGV[1])
    redis.call('EXPIRE', KEYS[1], ARGV[2])
    return 1
    `
	sessionKey := fmt.Sprintf("%s:%s:%s", constants.RedisKeyUserSession, user.UserId, os)
	_, err := rdb.Eval(script, []string{sessionKey}, user.AccessToken, 3600*24).Result()
	if err != nil {
		return err
	}
	return nil
}

func isAlphanumeric(s string) bool {
	// 正则表达式：匹配只包含字母和数字的字符串
	re := regexp.MustCompile("^[a-zA-Z0-9]+$")
	return re.MatchString(s)
}

func (s *UserService) VerifyAccessToken(ctx context.Context, userId, accessToken, os string) (vai.StatusCode, error) {
	if userId == "" {
		return vai.StatusCode_INVALID_USER, constants.ERR_INVALID_USER_ID
	}
	projectID := common.GetProjectID(ctx)
	user, err := s.userDao.GetUserByIdOrPhone(projectID, userId)
	if err != nil {
		return vai.StatusCode_INVALID_USER, constants.ERR_USER_INVALID
	}
	if user.UserType == uint(vai.UserType_USER_TYPE_TEMP) || user.LoginType == "temp" {
		return vai.StatusCode_SUCCESS, nil
	}
	rdb := db.GetRedis()
	sessionKey := s.GetUserSessionKey(*user, os)
	session, err := rdb.Get(sessionKey).Result()
	if errors.Is(err, redis.Nil) {
		return vai.StatusCode_EXPIRED_ACCESS_TOKEN, constants.ERR_TOKEN_EXPIRED
	}
	if session != accessToken {
		return vai.StatusCode_INVALID_ACCESS_TOKEN, constants.ERR_INVALID_TOKEN
	}
	if user.Status == 1 {
		return vai.StatusCode_INVALID_USER, constants.ERR_USER_BLOCKED
	}
	return vai.StatusCode_SUCCESS, nil
}

func (s *UserService) GetUserSessionKey(user model.UserRecord, os string) string {
	return fmt.Sprintf("%s:%s:%s", constants.RedisKeyUserSession, user.UserId, os)
}

func (s *UserService) RefreshAccessToken(ctx context.Context, userId, refreshToken, os string) (*model.UserRecord, error) {
	projectID := common.GetProjectID(ctx)
	user, err := s.userDao.RefreshAccessToken(projectID, refreshToken, userId)
	if err != nil {
		return nil, err
	}
	if err := s.refreshSession(*user, os); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) GetUserRecord(ctx context.Context, userId string) (*model.UserRecord, error) {
	projectID := common.GetProjectID(ctx)
	return s.userDao.GetUserByIdOrPhone(projectID, userId)
}

func (s *UserService) GetUserInfo(ctx context.Context, userId string) (*vai.UserInfo, error) {
	projectID := common.GetProjectID(ctx)
	user, err := s.userDao.GetUserByIdOrPhone(projectID, userId)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Get User Info Error", zap.Error(err))
		return nil, err
	}
	// 刷新 refresh token 过期时间
	s.RefreshTokenExpired(ctx, userId)
	userInfo := &vai.UserInfo{
		UserId:        user.UserId,
		UserName:      user.UserName,
		PhoneNumber:   user.PhoneNumber,
		Avatar:        user.AvatarUrl,
		Gender:        0,
		Birthday:      "",
		Nickname:      user.UserName, // 使用 username 作为 Nickname
		RegisterTime:  time.Unix(user.CreateTime, 0).Format(common.TimestampFormat),
		LastLoginTime: time.Unix(user.UpdateTime, 0).Format(common.TimestampFormat),
	}
	return userInfo, nil
}

func (s *UserService) BuildUserInfoByUserRecord(user *model.UserRecord) *vai.UserInfo {
	userInfo := &vai.UserInfo{
		UserId:      user.UserId,
		UserName:    user.UserName,
		PhoneNumber: user.PhoneNumber,
		Avatar:      user.AvatarUrl,
		Gender:      0,
		Birthday:    "",
	}
	return userInfo
}

func (s *UserService) Logout(ctx context.Context, userId string) error {
	projectID := common.GetProjectID(ctx)
	return s.userDao.Logout(projectID, userId)
}
func (s *UserService) getIpFromCtx(ctx context.Context) (string, error) {
	rawIP := ""
	ok := false
	if rawIP, ok = ctx.Value(constants.CtxIP).(string); !ok {
		return "", errors.New("ip is empty")
	}
	parsedAddr, err := net.ResolveTCPAddr("tcp", rawIP)
	if err != nil {
		fmt.Println("解析地址失败:", err)
		return rawIP, err
	}
	if parsedAddr.IP.IsLoopback() {
		rawIP = "127.0.0.1"
	} else if parsedAddr.IP.To4() != nil {
		rawIP = parsedAddr.IP.String()
	} else {
		rawIP = parsedAddr.IP.String()
	}
	return rawIP, nil
}

func (s *UserService) RefreshTokenExpired(ctx context.Context, userId string) error {
	projectID := common.GetProjectID(ctx)
	return s.userDao.RefreshTokenExpired(projectID, userId)
}

func (s *UserService) handleNewUser(ctx context.Context, req *vai.LoginRequest, user *model.UserRecord, userType vai.UserType, projectID string) error {
	var eventType string
	if userType == vai.UserType_USER_TYPE_REGISTER {
		eventType = constants.RegisterByPhoneNum
	} else if userType == vai.UserType_USER_TYPE_TEMP {
		eventType = constants.AppFirstOpen
	}

	loginAttributionService := NewattributionService()
	//记录初始基本数据
	req.RequestHeader.UserId = user.UserId
	_, err := loginAttributionService.RecordEventInfo(req.GetRequestHeader(), eventType)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Record EventInfo Error", zap.Error(err))
		return err
	}

	zlog.LogWithContext(ctx).Info("Add New User",
		zap.String(constants.CtxIdfv, req.GetRequestHeader().GetDevice().GetIdfv()),
		zap.String(constants.CtxAndroidId, req.GetRequestHeader().GetDevice().GetAndroidId()),
		zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
		zap.String(constants.CtxUserID, req.GetRequestHeader().GetUserId()),
		zap.Any(constants.ServiceEvent, constants.EventNewUser),
	)

	return s.userAmountDao.SetUserAmount(ctx, projectID, user.UserId, false)
}

// EditUserName 修改用户名
// 校验用户名格式，检查用户名是否已经存在，更新用户名
func (s *UserService) EditUserName(ctx context.Context, userID string, userName string) error {
	// 输入验证
	if userID == "" {
		return constants.ERR_INVALID_USER_ID
	}

	// 检查用户名格式
	if err := s.validateUserName(ctx, userName); err != nil {
		return err
	}

	projectID := common.GetProjectID(ctx)

	// 检查用户名是否已存在
	exists, err := s.userDao.CheckUserNameExists(projectID, userName)
	if err != nil {
		return err
	}
	if exists {
		return constants.ERR_USER_NAME_EXISTS
	}

	// 更新用户名
	return s.userDao.UpdateUserName(projectID, userID, userName)
}

// validateUserName 验证用户名格式
// 1. 长度在7-15个字符之间
// 2. 只包含大小写字母、数字、下划线和连字符
func (s *UserService) validateUserName(ctx context.Context, userName string) error {
	// 检查长度
	if len(userName) < 7 || len(userName) > 15 {
		zlog.LogWithContext(ctx).Error("用户名长度不符合要求", zap.String("user_name", userName))
		return constants.ERR_INVALID_PARAM
	}

	// 检查字符
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validPattern.MatchString(userName) {
		zlog.LogWithContext(ctx).Error("用户名包含非法字符", zap.String("user_name", userName))
		return constants.ERR_INVALID_PARAM
	}

	return nil
}

// DeleteUser 注销账号
func (s *UserService) DeleteUser(ctx context.Context, userID string) error {
	projectID := common.GetProjectID(ctx)
	return s.userDao.DeleteUser(projectID, userID)
}

// EditUserInfo 修改用户个人信息（用户名和/或头像）
func (s *UserService) EditUserInfo(ctx context.Context, userID string, userName string, avatar string) error {
	if userID == "" {
		return constants.ERR_INVALID_USER_ID
	}

	projectID := common.GetProjectID(ctx)
	updates := make(map[string]interface{})

	// 如果传了 user_name，做格式校验和重复校验
	if userName != "" {
		if err := s.validateUserName(ctx, userName); err != nil {
			return err
		}
		exists, err := s.userDao.CheckUserNameExists(projectID, userName)
		if err != nil {
			return err
		}
		if exists {
			return constants.ERR_USER_NAME_EXISTS
		}
		updates["user_name"] = userName
	}

	// 如果传了 avatar，加入更新
	if avatar != "" {
		updates["avatar_url"] = avatar
	}

	// 如果什么都没传，返回参数错误
	if len(updates) == 0 {
		return constants.ERR_INVALID_PARAM
	}

	return s.userDao.UpdateUserInfo(projectID, userID, updates)
}
