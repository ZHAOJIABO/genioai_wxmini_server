package dao

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strings"

	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/model"
)

// InviteDao 邀请码数据访问层
type InviteDao struct {
	db *gorm.DB
}

// NewInviteDao 创建 InviteDao 实例
func NewInviteDao(db *gorm.DB) *InviteDao {
	return &InviteDao{db: db}
}

// GetInviteCodeByUserID 根据用户ID获取邀请码
func (d *InviteDao) GetInviteCodeByUserID(ctx context.Context, projectID, userID string) (*model.InviteCode, error) {
	var inviteCode model.InviteCode
	err := d.db.WithContext(ctx).
		Where("project_id = ? AND user_id = ?", projectID, userID).
		First(&inviteCode).Error
	if err != nil {
		return nil, err
	}
	return &inviteCode, nil
}

// GetInviteCodeByCode 根据邀请码获取记录
func (d *InviteDao) GetInviteCodeByCode(ctx context.Context, code string) (*model.InviteCode, error) {
	var inviteCode model.InviteCode
	err := d.db.WithContext(ctx).
		Where("code = ?", code).
		First(&inviteCode).Error
	if err != nil {
		return nil, err
	}
	return &inviteCode, nil
}

// CreateInviteCode 创建邀请码
func (d *InviteDao) CreateInviteCode(ctx context.Context, inviteCode *model.InviteCode) error {
	return d.db.WithContext(ctx).Create(inviteCode).Error
}

// GenerateUniqueCode 生成唯一邀请码
func (d *InviteDao) GenerateUniqueCode(ctx context.Context) (string, error) {
	charset := constants.InviteCodeCharset
	length := constants.InviteCodeLength

	for attempts := 0; attempts < 10; attempts++ {
		code, err := generateRandomCode(charset, length)
		if err != nil {
			return "", err
		}

		// 检查是否已存在
		var count int64
		err = d.db.WithContext(ctx).
			Model(&model.InviteCode{}).
			Where("code = ?", code).
			Count(&count).Error
		if err != nil {
			return "", err
		}

		if count == 0 {
			return code, nil
		}
	}

	return "", errors.New("failed to generate unique invite code after 10 attempts")
}

// GetInviteRecordByInvite 获取被邀请人的邀请记录（全局查询，不按项目隔离）
func (d *InviteDao) GetInviteRecordByInvite(ctx context.Context, inviteUserID string) (*model.InviteRecord, error) {
	var record model.InviteRecord
	err := d.db.WithContext(ctx).
		Where("invite_user_id = ?", inviteUserID).
		First(&record).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// CreateInviteRecord 创建邀请记录
func (d *InviteDao) CreateInviteRecord(ctx context.Context, tx *gorm.DB, record *model.InviteRecord) error {
	return tx.WithContext(ctx).Create(record).Error
}

// UpdateInviteRecordCreditStatus 更新邀请记录的积分发放状态
func (d *InviteDao) UpdateInviteRecordCreditStatus(ctx context.Context, tx *gorm.DB, recordID uint, inviterGranted, inviteGranted bool) error {
	return tx.WithContext(ctx).
		Model(&model.InviteRecord{}).
		Where("id = ?", recordID).
		Updates(map[string]interface{}{
			"inviter_credit_granted": inviterGranted,
			"invite_credit_granted":  inviteGranted,
		}).Error
}

// GetInviteStatsByInviter 获取邀请人的邀请统计（全局统计，不按项目隔离）
func (d *InviteDao) GetInviteStatsByInviter(ctx context.Context, inviterUserID string) (int64, error) {
	var count int64
	err := d.db.WithContext(ctx).
		Model(&model.InviteRecord{}).
		Where("inviter_user_id = ?", inviterUserID).
		Count(&count).Error
	return count, err
}

// GetGrantedCreditsCountByInviter 获取邀请人已获得积分的邀请记录数（全局统计，不按项目隔离）
func (d *InviteDao) GetGrantedCreditsCountByInviter(ctx context.Context, inviterUserID string) (int64, error) {
	var count int64
	err := d.db.WithContext(ctx).
		Model(&model.InviteRecord{}).
		Where("inviter_user_id = ? AND inviter_credit_granted = ?", inviterUserID, true).
		Count(&count).Error
	return count, err
}

// GetGrantedCreditsCountByInviterWithTx 获取邀请人已获得积分的邀请记录数（事务内，全局统计）
func (d *InviteDao) GetGrantedCreditsCountByInviterWithTx(ctx context.Context, tx *gorm.DB, inviterUserID string) (int64, error) {
	var count int64
	// 统计该邀请人在所有项目的已获得积分记录数
	err := tx.WithContext(ctx).
		Model(&model.InviteRecord{}).
		Where("inviter_user_id = ? AND inviter_credit_granted = ?", inviterUserID, true).
		Count(&count).Error
	return count, err
}

// GetInviteRecordsByInviter 获取邀请人的邀请记录列表（全局查询，不按项目隔离）
func (d *InviteDao) GetInviteRecordsByInviter(ctx context.Context, inviterUserID string, limit, offset int) ([]*model.InviteRecord, error) {
	var records []*model.InviteRecord
	err := d.db.WithContext(ctx).
		Where("inviter_user_id = ?", inviterUserID).
		Order("id ASC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}

// generateRandomCode 生成随机码
func generateRandomCode(charset string, length int) (string, error) {
	result := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))

	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}

	return string(result), nil
}

// IsDuplicateError 判断是否为重复键错误
func IsDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "Duplicate entry") ||
		strings.Contains(errMsg, "duplicate key") ||
		strings.Contains(errMsg, "UNIQUE constraint")
}
