package profile

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type ProfileService struct {
	profileDao    *dao.ProfileDao
	configService *service.ConfigService
}

func NewProfileService(profileDao *dao.ProfileDao, configService *service.ConfigService) *ProfileService {
	return &ProfileService{
		profileDao:    profileDao,
		configService: configService,
	}
}

// CreateCustomProfile 创建用户自定义个人信息
func (s *ProfileService) CreateCustomProfile(ctx context.Context, projectID string, profile *model.CustomProfile) error {
	return s.profileDao.CreateCustomProfile(ctx, projectID, profile)
}

// UpdateCustomProfile 更新用户自定义个人信息
func (s *ProfileService) UpdateCustomProfile(ctx context.Context, projectID string, profileID string, profile *model.CustomProfile) error {
	return s.profileDao.UpdateCustomProfile(ctx, projectID, profileID, profile)
}

// ListCustomUserPersonalInfo 获取用户自定义个人信息列表
func (s *ProfileService) ListCustomProfile(ctx context.Context, projectID, userID, lang string) ([]*vai.CustomProfile, error) {
	customProfiles, err := s.profileDao.ListCustomProfile(ctx, projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("ListCustomProfile", zap.Error(err))
		return nil, err
	}
	relationList, err := s.configService.GetRelationList(ctx, lang)
	if err != nil {
		zlog.LogWithContext(ctx).Error("ListCustomProfile", zap.Error(err))
		return nil, err
	}
	relationMap := make(map[string]*vai.Relation)
	for _, relation := range relationList {
		relationMap[relation.GetRelationIden()] = relation
	}

	result := make([]*vai.CustomProfile, 0, len(customProfiles))
	for _, customProfile := range customProfiles {
		grpcProfile := customProfile.CustomProfileToGRPC()
		grpcProfile.Relation = relationMap[customProfile.Relation]

		result = append(result, grpcProfile)
	}

	return result, nil
}

// DeleteCustomProfile 删除用户自定义个人信息
func (s *ProfileService) DeleteCustomProfile(ctx context.Context, projectID, createBy, profileID string) error {
	return s.profileDao.DeleteCustomProfile(ctx, projectID, createBy, profileID)
}

// GetCustomProfile 获取用户自定义个人信息
func (s *ProfileService) GetCustomProfile(ctx context.Context, projectID, profileID string) (*model.CustomProfile, error) {
	customProfiles, err := s.profileDao.GetCustomerProfile(ctx, projectID, []string{profileID})
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetCustomProfile", zap.Error(err))
		return nil, err
	}
	if len(customProfiles) == 0 {
		return nil, errors.New("自定义个人信息不存在")
	}
	return customProfiles[0], nil
}

// DeleteUserProfile 删除用户的所有个人资料
func (s *ProfileService) DeleteUserProfile(ctx context.Context, projectID, userID string) error {
	// 删除用户的自定义档案
	if err := s.profileDao.DeleteCustomProfile(ctx, projectID, userID, ""); err != nil {
		return err
	}

	// 删除用户的基本档案
	if err := s.profileDao.DeleteUserProfile(ctx, projectID, userID); err != nil {
		return err
	}

	return nil
}
