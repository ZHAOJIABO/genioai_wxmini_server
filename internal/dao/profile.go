package dao

import (
	"context"

	"gorm.io/gorm"

	"va_visionai_server/internal/model"
)

type ProfileDao struct {
	db *gorm.DB
}

func NewProfileDao(db *gorm.DB) *ProfileDao {
	return &ProfileDao{db: db}
}

func (d *ProfileDao) GetUserProfile(ctx context.Context, projectID, userID string) (*model.UserProfile, error) {
	var userPersonalInfo model.UserProfile
	if err := d.db.Where("user_id = ? AND project_id = ?", userID, projectID).First(&userPersonalInfo).Error; err != nil {
		return nil, err
	}
	return &userPersonalInfo, nil
}

func (d *ProfileDao) UpdateUserProfile(ctx context.Context, userProfile *model.UserProfile) error {
	return d.db.Model(&model.UserProfile{}).
		Where("profile_id = ? AND project_id = ?", userProfile.ProfileID, userProfile.ProjectID).
		Updates(userProfile).Error
}

// create
func (d *ProfileDao) CreateUserProfile(ctx context.Context, userProfile *model.UserProfile) error {
	return d.db.Create(userProfile).Error
}

//--------------------------------- 用户自定义档案 ------------------------------------------------------------

func (d *ProfileDao) GetCustomerProfile(ctx context.Context, projectID string, profileIDs []string) ([]*model.CustomProfile, error) {
	var userPersonalInfos []*model.CustomProfile
	if err := d.db.Where("project_id = ? AND profile_id IN ?", projectID, profileIDs).Find(&userPersonalInfos).Error; err != nil {
		return nil, err
	}
	return userPersonalInfos, nil
}

func (d *ProfileDao) UpdateCustomProfile(ctx context.Context, projectID, profileID string, profile *model.CustomProfile) error {
	return d.db.Model(&model.CustomProfile{}).
		Where("profile_id = ? AND project_id = ?", profileID, projectID).
		Updates(profile).Error
}

func (d *ProfileDao) CreateCustomProfile(ctx context.Context, projectID string, profile *model.CustomProfile) error {
	return d.db.Create(profile).Error
}

func (d *ProfileDao) ListCustomProfile(ctx context.Context, projectID, userID string) ([]*model.CustomProfile, error) {
	var profiles []*model.CustomProfile
	if err := d.db.Where("project_id = ? AND create_by = ?", projectID, userID).Find(&profiles).Error; err != nil {
		return nil, err
	}
	return profiles, nil
}

func (d *ProfileDao) DeleteCustomProfile(ctx context.Context, projectID, createBy, profileID string) error {
	query := d.db.Where("project_id = ? AND create_by = ?", projectID, createBy)
	if profileID != "" {
		query = query.Where("profile_id = ?", profileID)
	}
	return query.Delete(&model.CustomProfile{}).Error
}

// // GetByProfileID 通过 ProfileID 获取档案信息
// func (d *ProfileDao) GetByProfileID(profileID string) (*model.CustomProfile, error) {
// 	var profile model.CustomProfile
// 	err := d.db.Where("profile_id = ?", profileID).First(&profile).Error
// 	return &profile, err
// }

// DeleteUserProfile 删除用户的基本档案
func (d *ProfileDao) DeleteUserProfile(ctx context.Context, projectID, userID string) error {
	return d.db.Where("project_id = ? AND user_id = ?", projectID, userID).Delete(&model.UserProfile{}).Error
}

// GetCustomProfileByProfileID 通过 ProjectID 和 ProfileID 获取单个自定义档案
func (d *ProfileDao) GetCustomProfileByProfileID(ctx context.Context, projectID, profileID string) (*model.CustomProfile, error) {
	var customProfile model.CustomProfile
	if err := d.db.WithContext(ctx).Where("project_id = ? AND profile_id = ?", projectID, profileID).First(&customProfile).Error; err != nil {
		return nil, err
	}
	return &customProfile, nil
}
