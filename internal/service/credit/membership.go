package credit

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/zlog"
)

// MembershipService 处理会员相关的额度管理
type MembershipService interface {
	// GrantMembershipCredits 为用户发放会员额度
	GrantMembershipCredits(ctx context.Context, projectID, userID string, membershipLevel int, expiredAt time.Time) error

	// ClearExpiredMembershipCredits 清理过期的会员额度
	ClearExpiredMembershipCredits(ctx context.Context, projectID, userID string) (int64, error)

	// ProcessMembershipUpgrade 处理会员升级，发放新的额度
	ProcessMembershipUpgrade(ctx context.Context, projectID, userID string, oldLevel, newLevel int, expiredAt time.Time) error

	// ProcessMembershipExpiry 处理会员到期，清零会员额度
	ProcessMembershipExpiry(ctx context.Context, projectID, userID string) (int64, error)
}

// membershipService 实现 MembershipService 接口
type membershipService struct {
	creditService Service
}

// NewMembershipService 创建新的会员额度管理服务
func NewMembershipService(creditService Service) MembershipService {
	return &membershipService{
		creditService: creditService,
	}
}

// getMembershipCreditAmount 根据会员等级获取应发放的额度数量
func (s *membershipService) getMembershipCreditAmount(membershipLevel int) int64 {
	// 根据会员等级返回不同的额度数量
	// 这里可以根据业务需求调整
	switch membershipLevel {
	case 10: // 基础会员
		return 100
	case 2: // 高级会员
		return 300
	case 3: // 尊享会员
		return 500
	default:
		return 0
	}
}

// GrantMembershipCredits 为用户发放会员额度
func (s *membershipService) GrantMembershipCredits(ctx context.Context, projectID, userID string, membershipLevel int, expiredAt time.Time) error {
	amount := s.getMembershipCreditAmount(membershipLevel)
	if amount <= 0 {
		zlog.LogWithContext(ctx).Warn("无效的会员等级，跳过额度发放",
			zap.String("userID", userID),
			zap.Int("membershipLevel", membershipLevel))
		return nil
	}

	sourceID := "membership_grant_" + time.Now().Format("20060102150405")
	description := "会员额度发放"

	err := s.creditService.AddCredits(
		ctx,
		projectID,
		userID,
		sourceID,
		constants.CreditTypeMembershipGrant,
		constants.TransactionTypeSubscriptionGrant,
		amount,
		expiredAt,
		description,
		0,
	)

	if err != nil {
		zlog.LogWithContext(ctx).Error("会员额度发放失败",
			zap.String("userID", userID),
			zap.Int("membershipLevel", membershipLevel),
			zap.Int64("amount", amount),
			zap.Error(err))
		return errors.Wrap(err, "failed to grant membership credits")
	}

	zlog.LogWithContext(ctx).Info("会员额度发放成功",
		zap.String("userID", userID),
		zap.Int("membershipLevel", membershipLevel),
		zap.Int64("amount", amount),
		zap.Time("expiredAt", expiredAt))

	return nil
}

// ClearExpiredMembershipCredits 清理过期的会员额度
func (s *membershipService) ClearExpiredMembershipCredits(ctx context.Context, projectID, userID string) (int64, error) {
	clearedAmount, err := s.creditService.ClearExpiredCredits(ctx, projectID, userID, constants.CreditTypeMembershipGrant)
	if err != nil {
		zlog.LogWithContext(ctx).Error("清理过期会员额度失败",
			zap.String("userID", userID),
			zap.String("projectID", projectID),
			zap.Error(err))
		return 0, errors.Wrap(err, "failed to clear expired membership credits")
	}

	if clearedAmount > 0 {
		zlog.LogWithContext(ctx).Info("清理过期会员额度成功",
			zap.String("userID", userID),
			zap.String("projectID", projectID),
			zap.Int64("clearedAmount", clearedAmount))
	}

	return clearedAmount, nil
}

// ProcessMembershipUpgrade 处理会员升级，发放新的额度
func (s *membershipService) ProcessMembershipUpgrade(ctx context.Context, projectID, userID string, oldLevel, newLevel int, expiredAt time.Time) error {
	// 先清理旧的会员额度（如果有的话）
	clearedAmount, err := s.ClearExpiredMembershipCredits(ctx, projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("清理旧会员额度失败",
			zap.String("userID", userID),
			zap.Int("oldLevel", oldLevel),
			zap.Error(err))
		// 不阻止升级流程，继续发放新额度
	}

	// 发放新的会员额度
	err = s.GrantMembershipCredits(ctx, projectID, userID, newLevel, expiredAt)
	if err != nil {
		return errors.Wrap(err, "failed to grant new membership credits")
	}

	zlog.LogWithContext(ctx).Info("会员升级处理完成",
		zap.String("userID", userID),
		zap.Int("oldLevel", oldLevel),
		zap.Int("newLevel", newLevel),
		zap.Int64("clearedAmount", clearedAmount))

	return nil
}

// ProcessMembershipExpiry 处理会员到期，清零会员额度
func (s *membershipService) ProcessMembershipExpiry(ctx context.Context, projectID, userID string) (int64, error) {
	clearedAmount, err := s.ClearExpiredMembershipCredits(ctx, projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("会员到期清理额度失败",
			zap.String("userID", userID),
			zap.Error(err))
		return 0, errors.Wrap(err, "failed to clear expired membership credits")
	}

	if clearedAmount > 0 {
		sourceID := "membership_expiry_" + time.Now().Format("20060102150405")
		description := "会员到期额度清零"

		// 记录清零操作（用于统计）
		err = s.creditService.AddCredits(
			ctx,
			projectID,
			userID,
			sourceID,
			constants.CreditTypeMembershipGrant,
			constants.TransactionTypeSubscriptionExpiry,
			-clearedAmount, // 负数表示扣减
			time.Time{},    // 无过期时间
			description,
			0,
		)

		if err != nil {
			zlog.LogWithContext(ctx).Error("记录会员到期清零失败",
				zap.String("userID", userID),
				zap.Int64("clearedAmount", clearedAmount),
				zap.Error(err))
			// 不返回错误，因为实际的清零操作已经完成
		}
	}

	zlog.LogWithContext(ctx).Info("会员到期处理完成",
		zap.String("userID", userID),
		zap.Int64("clearedAmount", clearedAmount))

	return clearedAmount, nil
}
