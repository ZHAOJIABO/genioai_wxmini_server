package dao

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/zlog"
)

type UserDao struct {
	db *gorm.DB
}

func NewUserDao(db *gorm.DB) *UserDao {
	return &UserDao{db: db}
}

// Helper function to generate a secure token
func generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// GetUserByAppleIdentity 通过 Apple 唯一标识查询用户
func (u *UserDao) GetUserByAppleIdentity(projectID string, appleIdentity string) (*model.UserRecord, error) {
	var user model.UserRecord
	err := u.db.Model(&model.UserRecord{}).Where("project_id = ? AND apple_auth_identity_token = ?", projectID, appleIdentity).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (u *UserDao) Create(user model.UserRecord) error {
	if user.ProjectID == "" {
		return errors.New("project_id is required")
	}
	return u.db.Create(&user).Error
}

// GetUserByIdOrPhone retrieves a user by UserID or PhoneNumber
func (u *UserDao) GetUserByIdOrPhone(projectID string, identifier string) (*model.UserRecord, error) {
	var user model.UserRecord
	err := u.db.Model(&model.UserRecord{}).Where("project_id = ? AND (user_id = ? OR phone_number = ?)", projectID, identifier, identifier).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByEmail retrieves a user by Email
func (u *UserDao) GetUserByEmail(projectID string, email string) (*model.UserRecord, error) {
	var user model.UserRecord
	err := u.db.Model(&model.UserRecord{}).
		Where("project_id = ? AND email = ?", projectID, email).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// ResetTokens resets the user's RefreshToken and AccessToken
func (u *UserDao) ResetTokens(projectID string, userId string) (model.UserRecord, error) {
	var user model.UserRecord
	err := u.db.Model(&model.UserRecord{}).Where("project_id = ? AND user_id = ?", projectID, userId).First(&user).Error
	if err != nil {
		return user, errors.New("user not found")
	}

	user.RefreshToken, _ = generateToken()
	user.AccessToken, _ = generateToken()
	user.RefreshTokenExpired = time.Now().Add(365 * 24 * time.Hour).Unix() // Example expiration: 365 days
	user.AccessTokenExpired = time.Now().Add(30 * 24 * time.Hour).Unix()   // Example expiration: 30 day
	user.UpdateTime = time.Now().Unix()
	if user.UUID == "" {
		user.UUID = strings.ReplaceAll(uuid.New().String(), "-", "")
	}

	err = u.db.Model(&model.UserRecord{}).Where("id = ?", user.ID).Save(&user).Error
	if err != nil {
		return user, err
	}
	return user, nil
}

// RefreshAccessToken resets the user's AccessToken using the RefreshToken
func (u *UserDao) RefreshAccessToken(projectID string, refreshToken, userId string) (*model.UserRecord, error) {
	var user model.UserRecord

	err := u.db.Model(&model.UserRecord{}).Where("project_id = ? AND refresh_token = ?", projectID, refreshToken).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, constants.ERR_INVALID_REFRESH_TOKEN
	}
	if err != nil {
		return nil, err
	}
	if userId != user.UserId {
		zlog.Logger.Error("refreshToken not match userid", zap.String("refreshToken", refreshToken))
		return nil, constants.ERR_USER_INVALID
	}

	// Check if the refresh token is expired
	if user.RefreshTokenExpired < time.Now().Unix() {
		return nil, constants.ERR_REFRESH_TOKEN_EXPIRED
	}

	// Generate new access token
	user.AccessToken, _ = generateToken()
	user.AccessTokenExpired = time.Now().Add(24 * time.Hour).Unix() // Example expiration: 1 day
	user.UpdateTime = time.Now().Unix()

	err = u.db.Model(&model.UserRecord{}).Where("id = ?", user.ID).Save(&user).Error
	if err != nil {
		zlog.Logger.Error("Update User Refresh Token Error", zap.Error(err))
		return nil, constants.ERR_INTERNAL_SERVER
	}
	return &user, nil
}

// ValidateAccessToken validates if the access token is still valid for a given userId

// Logout logs the user out by invalidating the tokens
func (u *UserDao) Logout(projectID string, userId string) error {
	var user model.UserRecord
	err := u.db.Model(&model.UserRecord{}).Where("project_id = ? AND user_id = ?", projectID, userId).First(&user).Error
	if err != nil {
		return constants.ERR_USER_INVALID
	}

	// Invalidate tokens
	user.AccessToken = ""
	user.RefreshToken = ""
	user.AccessTokenExpired = 0
	user.RefreshTokenExpired = 0
	user.UpdateTime = time.Now().Unix()

	err = u.db.Model(&model.UserRecord{}).Where("id = ?", user.ID).Save(&user).Error
	if err != nil {
		zlog.Logger.Error("Update User Logout Error", zap.Error(err))
		return constants.ERR_INTERNAL_SERVER
	}
	return nil
}

func (u *UserDao) CreateUserMapping(userID, mappingUserID, reason string) error {
	userMapping := model.UserMapping{
		UserID:        userID,
		MappingUserID: mappingUserID,
		Reason:        reason,
	}
	return u.db.Create(&userMapping).Error
}

func (u *UserDao) ListUserMapping() ([]model.UserMapping, error) {
	var userMappings []model.UserMapping
	err := u.db.Model(&model.UserMapping{}).Find(&userMappings).Error
	if err != nil {
		return nil, err
	}
	return userMappings, nil
}

// ResetIP resets the user's Last login ip
func (u *UserDao) ResetIP(projectID string, userId string, rawIp string) error {
	var user model.UserRecord
	err := u.db.Model(&model.UserRecord{}).Where("project_id = ? AND user_id = ?", projectID, userId).First(&user).Error
	if err != nil {
		return errors.New("user not found")
	}
	user.LastLoginIP = rawIp
	err = u.db.Model(&model.UserRecord{}).Where("id = ?", user.ID).Save(&user).Error
	if err != nil {
		return err
	}
	return nil
}

// RefreshTokenExpired 重置用户的 refresh token 过期时间为 30 天后
func (u *UserDao) RefreshTokenExpired(projectID string, userId string) error {
	now := time.Now()
	result := u.db.Model(&model.UserRecord{}).
		Where("project_id = ? AND user_id = ?", projectID, userId).
		Updates(map[string]interface{}{
			"refresh_token_expired": now.Add(30 * 24 * time.Hour).Unix(),
			"update_time":           now.Unix(),
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("user not found")
	}

	return nil
}

// GetAllUsersByProject 获取项目下所有用户ID
func (u *UserDao) GetAllUsersByProject(projectID string) ([]string, error) {
	if projectID == "" {
		return nil, errors.New("项目ID不能为空")
	}

	var userIDs []string
	err := u.db.Model(&model.UserRecord{}).
		Select("distinct user_id").
		Where("project_id = ?", projectID).
		Find(&userIDs).Error

	if err != nil {
		return nil, err
	}

	return userIDs, nil
}

// GetSubscribedUsersByProject 获取项目下所有有效订阅的用户ID
func (u *UserDao) GetSubscribedUsersByProject(projectID string) ([]string, error) {
	if projectID == "" {
		return nil, errors.New("项目ID不能为空")
	}

	var subscribedUserIDs []string
	err := u.db.Table("pay_user_subscription").
		Select("distinct user_id").
		Where("project_id = ? AND expire_date > ?", projectID, time.Now().UnixMilli()).
		Find(&subscribedUserIDs).Error

	if err != nil {
		return nil, err
	}

	return subscribedUserIDs, nil
}

func (u *UserDao) GetUserByAndroidId(androidId string) (*model.UserRecord, error) {
	if androidId == "" {
		return nil, errors.New("安卓ID不能为空")
	}

	// 查询user_device表中安卓ID对应的最新记录的用户ID
	var userDevice model.UserDeviceInfo
	err := u.db.Where("android_id = ?", androidId).
		Order("updated_at DESC").
		Limit(1).
		Find(&userDevice).Error

	if err != nil {
		return nil, err
	}

	// 如果没有找到记录
	if userDevice.ID == 0 {
		return nil, nil
	}

	// 根据用户ID查询用户信息
	var user model.UserRecord
	err = u.db.Where("user_id = ?", userDevice.UserID).
		First(&user).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &user, nil
}

func (u *UserDao) UpdateUser(projectID, userId string, user *model.UserRecord) error {
	return u.db.Model(&model.UserRecord{}).Where("project_id = ? AND user_id = ?", projectID, userId).Updates(user).Error
}

// CheckUserNameExists 检查用户名在同一个项目中是否已存在
func (u *UserDao) CheckUserNameExists(projectID, userName string) (bool, error) {
	var count int64
	err := u.db.Model(&model.UserRecord{}).
		Where("project_id = ? AND user_name = ?", projectID, userName).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// UpdateUserName 更新用户名
func (u *UserDao) UpdateUserName(projectID, userID, userName string) error {
	now := time.Now().Unix()
	return u.db.Model(&model.UserRecord{}).
		Where("project_id = ? AND user_id = ?", projectID, userID).
		Updates(map[string]interface{}{
			"user_name":   userName,
			"update_time": now,
		}).Error
}

// FindByProviderUser 通过提供商用户ID查找用户
func (u *UserDao) FindByProviderUser(projectID string, loginType string, providerUserID string) (*model.UserRecord, error) {
	var user model.UserRecord
	result := u.db.Where("project_id = ? AND login_type = ? AND provider_user_id = ?", projectID, loginType, providerUserID).First(&user)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

// ResetTokensWithEmptyValue 清空用户的令牌
func (u *UserDao) ResetTokensWithEmptyValue(projectID string, userId string) error {
	result := u.db.Model(&model.UserRecord{}).
		Where("project_id = ? AND user_id = ?", projectID, userId).
		Updates(map[string]interface{}{
			"access_token":          "",
			"refresh_token":         "",
			"access_token_expired":  0,
			"refresh_token_expired": 0,
			"update_time":           time.Now().Unix(),
		})
	return result.Error
}

// Update 更新用户信息
func (u *UserDao) Update(user *model.UserRecord) error {
	result := u.db.Save(user)
	return result.Error
}

// CheckUUIDExists 检查UUID在项目中是否已存在
func (u *UserDao) CheckUUIDExists(projectID, uuid string) (bool, error) {
	if projectID == "" {
		return false, errors.New("项目ID不能为空")
	}

	if uuid == "" {
		return false, errors.New("UUID不能为空")
	}

	var count int64
	err := u.db.Model(&model.UserRecord{}).
		Where("project_id = ? AND uuid = ?", projectID, uuid).
		Count(&count).Error

	if err != nil {
		zlog.Logger.Error("Check UUID exists error",
			zap.String("project_id", projectID),
			zap.String("uuid", uuid),
			zap.Error(err))
		return false, err
	}

	return count > 0, nil
}

// DeleteUser 软删除用户
func (u *UserDao) DeleteUser(projectID string, userID string) error {
	return u.db.
		Where("project_id = ? AND user_id = ?", projectID, userID).
		Delete(&model.UserRecord{}).
		Error
}

// UpdateUserEmail 更新用户邮箱
func (u *UserDao) UpdateUserEmail(projectID, userID, email string) error {
	if projectID == "" || userID == "" {
		return errors.New("项目ID和用户ID不能为空")
	}

	now := time.Now().Unix()
	result := u.db.Model(&model.UserRecord{}).
		Where("project_id = ? AND user_id = ?", projectID, userID).
		Updates(map[string]interface{}{
			"email":       email,
			"update_time": now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("用户不存在")
	}

	return nil
}

// GetUnsubscribedUsersByTimezones 获取指定时区的未订阅用户
// 未订阅用户：从未在 pay_user_subscription 表中出现过的用户
func (u *UserDao) GetUnsubscribedUsersByTimezones(timezones []string) ([]model.UserRecord, error) {
	if len(timezones) == 0 {
		return nil, nil
	}

	var users []model.UserRecord

	// 查询指定时区的用户，但排除在 pay_user_subscription 表中有记录的用户
	err := u.db.Model(&model.UserRecord{}).
		Where("timezone IN ?", timezones).
		Where("user_id NOT IN (?)",
			u.db.Table("pay_user_subscription").
				Select("DISTINCT user_id").Where("deleted_at is null and project_id in ('com.domob.piclib','com.bluex.picflow')")).
		Find(&users).Error

	if err != nil {
		return nil, err
	}

	return users, nil
}

// GetDistinctTimezones 获取数据库中所有不重复的用户时区
func (u *UserDao) GetDistinctTimezones() ([]string, error) {
	var timezones []string
	err := u.db.Model(&model.UserRecord{}).
		Select("DISTINCT timezone").
		Where("timezone IS NOT NULL AND timezone != ''").
		Pluck("timezone", &timezones).Error

	if err != nil {
		return nil, err
	}

	return timezones, nil
}

// GetUserByUUID 通过 UUID 查询用户
func (u *UserDao) GetUserByUUID(projectID string, uuid string) (*model.UserRecord, error) {
	if uuid == "" {
		return nil, errors.New("UUID不能为空")
	}

	var user model.UserRecord
	err := u.db.Model(&model.UserRecord{}).
		Where("project_id = ? AND uuid = ?", "com.web.genioai", uuid).
		First(&user).Error

	if err != nil {
		return nil, err
	}

	return &user, nil
}

// UpdateUserInfo 更新用户信息（支持部分更新）
func (u *UserDao) UpdateUserInfo(projectID, userID string, updates map[string]interface{}) error {
	updates["update_time"] = time.Now().Unix()
	return u.db.Model(&model.UserRecord{}).
		Where("project_id = ? AND user_id = ?", projectID, userID).
		Updates(updates).Error
}

// GetUsersByUserIDs 批量查询用户信息
func (u *UserDao) GetUsersByUserIDs(userIDs []string) ([]model.UserRecord, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	var users []model.UserRecord
	err := u.db.Model(&model.UserRecord{}).
		Where("user_id IN ?", userIDs).
		Find(&users).Error

	if err != nil {
		return nil, err
	}

	return users, nil
}
