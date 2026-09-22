package service

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type UserPersonalInfoService struct {
	userPersonalInfoDao *dao.ProfileDao
	userDao             *dao.UserDao
}

func NewUserPersonalInfoService(userPersonalInfoDao *dao.ProfileDao, userDao *dao.UserDao, _ interface{}) *UserPersonalInfoService {
	return &UserPersonalInfoService{userPersonalInfoDao: userPersonalInfoDao, userDao: userDao}
}

func (s *UserPersonalInfoService) GetUserPersonalInfo(ctx context.Context, projectID, userID string) (*vai.UserProfile, error) {
	userPersonalInfo, err := s.userPersonalInfoDao.GetUserProfile(ctx, projectID, userID)
	if err != nil && err != gorm.ErrRecordNotFound {
		zlog.LogWithContext(ctx).Error("GetUserPersonalInfo Error", zap.Error(err))
		return nil, err
	}
	if err == gorm.ErrRecordNotFound {
		zlog.LogWithContext(ctx).Info("GetUserPersonalInfo RecordNotFound", zap.String("userID", userID))
		return &vai.UserProfile{}, nil
	}

	return &vai.UserProfile{
		ProfileId:      userPersonalInfo.ProfileID,
		Nickname:       userPersonalInfo.NickName,
		Avatar:         userPersonalInfo.AvatarUrl,
		EmotionalState: userPersonalInfo.EmotionalState,
		BirthTimestamp: userPersonalInfo.BirthTimestamp,
		Gender:         vai.Gender(userPersonalInfo.Sex),
		Constellation:  userPersonalInfo.Constellation,
		BirthAddress:   userPersonalInfo.BirthAddress,
		BirthProvince:  userPersonalInfo.BirthProvince,
		BirthCity:      userPersonalInfo.BirthCity,
		BirthCountry:   userPersonalInfo.BirthCountry,
		BirthLongitude: userPersonalInfo.BirthLongitude,
		BirthLatitude:  userPersonalInfo.BirthLatitude,
	}, nil
}

func (s *UserPersonalInfoService) CreateUserProfile(ctx context.Context, userProfile *model.UserProfile) error {
	return s.userPersonalInfoDao.CreateUserProfile(ctx, userProfile)
}

func (s *UserPersonalInfoService) UpdateUserProfile(ctx context.Context, userProfile *model.UserProfile) error {
	return s.userPersonalInfoDao.UpdateUserProfile(ctx, userProfile)
}

func (s *UserPersonalInfoService) ValidateUserPersonalInfo(ctx context.Context, userPersonalInfo *model.UserProfile) bool {
	logger := zlog.LogWithContext(ctx)
	logger.Info("Start validating user personal info")

	isValid := true
	if userPersonalInfo.ProfileID == "" {
		logger.Warn("ProfileID is empty")
		isValid = false
		return isValid
	}

	if userPersonalInfo.BirthTimestamp != 0 {
		// 检查时间戳是否合理（例如不能是未来日期，也不能太过久远）
		now := time.Now().Unix()
		minTime := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC).Unix()

		if userPersonalInfo.BirthTimestamp > now {
			logger.Warn("BirthTimestamp is in the future",
				zap.Int64("BirthTimestamp", userPersonalInfo.BirthTimestamp),
				zap.Int64("Now", now))
			isValid = false
			return isValid
		}

		if userPersonalInfo.BirthTimestamp < minTime {
			logger.Warn("BirthTimestamp is too old",
				zap.Int64("BirthTimestamp", userPersonalInfo.BirthTimestamp),
				zap.Int64("MinTime", minTime))
			isValid = false
			return isValid
		}
	}

	if userPersonalInfo.BirthLongitude != 0 {
		if userPersonalInfo.BirthLongitude < -180 || userPersonalInfo.BirthLongitude > 180 {
			logger.Warn("BirthLongitude out of range",
				zap.Float32("BirthLongitude", userPersonalInfo.BirthLongitude))
			isValid = false
			return isValid
		}
	}

	if userPersonalInfo.BirthLatitude != 0 {
		if userPersonalInfo.BirthLatitude < -90 || userPersonalInfo.BirthLatitude > 90 {
			logger.Warn("BirthLatitude out of range",
				zap.Float32("BirthLatitude", userPersonalInfo.BirthLatitude))
			isValid = false
			return isValid
		}
	}

	return isValid
}
