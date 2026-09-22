package invite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/zlog"
)

var (
	ErrInviteCodeNotFound    = errors.New(constants.ErrMsgInviteCodeNotFound)
	ErrCannotInviteSelf      = errors.New(constants.ErrMsgCannotInviteSelf)
	ErrAlreadyUsedInviteCode = errors.New(constants.ErrMsgAlreadyUsedInviteCode)
)

// ConfigService 配置服务接口（避免循环依赖）
type ConfigService interface {
	GetIntConfig(key string) (int, error)
}

// InviteInfo 邀请信息
type InviteInfo struct {
	InviteCode        string `json:"invite_code"`
	InvitedCount      int64  `json:"invited_count"`
	TotalCredits      int64  `json:"total_credits"`
	HasUsedInviteCode bool   `json:"has_used_invite_code"`
	UsedInviteCode    string `json:"used_invite_code,omitempty"`
	CanUseInviteCode  bool   `json:"can_use_invite_code"`
}

// InviteRecordItem 邀请记录项
type InviteRecordItem struct {
	InviteUserID string    `json:"invite_user_id"`
	InviteTime   time.Time `json:"invite_time"`
	Credits      int64     `json:"credits"`
}

// InviteService 邀请码服务
type InviteService struct {
	inviteDao     *dao.InviteDao
	creditService credit.Service
	configService ConfigService
	db            *gorm.DB
}

// NewInviteService 创建邀请码服务
func NewInviteService(
	inviteDao *dao.InviteDao,
	creditService credit.Service,
	configService ConfigService,
	db *gorm.DB,
) *InviteService {
	return &InviteService{
		inviteDao:     inviteDao,
		creditService: creditService,
		configService: configService,
		db:            db,
	}
}

// GetInviteCreditAmount 获取每次邀请奖励积分（可配置，默认300）
func (s *InviteService) GetInviteCreditAmount() int64 {
	return s.getInviteCreditAmount()
}

// getInviteCreditAmount 获取每次邀请奖励积分（可配置，默认300）
func (s *InviteService) getInviteCreditAmount() int64 {
	if s.configService != nil {
		if amount, err := s.configService.GetIntConfig(constants.ConfigKeyInviteCreditAmount); err == nil && amount > 0 {
			return int64(amount)
		}
	}
	return constants.InviteCreditAmount
}

// getInviteMaxCredits 获取邀请积分封顶值（0表示无上限）
func (s *InviteService) getInviteMaxCredits() int64 {
	if s.configService != nil {
		if maxCredits, err := s.configService.GetIntConfig(constants.ConfigKeyInviteMaxCredits); err == nil {
			return int64(maxCredits)
		}
	}
	return 0
}

// GetOrCreateInviteCode 获取或创建用户的邀请码
func (s *InviteService) GetOrCreateInviteCode(ctx context.Context, projectID, userID string) (string, error) {
	// 1. 尝试获取已有邀请码
	existing, err := s.inviteDao.GetInviteCodeByUserID(ctx, projectID, userID)
	if err == nil {
		return existing.Code, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	// 2. 生成新的唯一邀请码
	code, err := s.inviteDao.GenerateUniqueCode(ctx)
	if err != nil {
		return "", err
	}

	// 3. 创建邀请码记录
	inviteCode := &model.InviteCode{
		ProjectID: projectID,
		UserID:    userID,
		Code:      code,
	}

	err = s.inviteDao.CreateInviteCode(ctx, inviteCode)
	if err != nil {
		// 处理并发冲突
		if dao.IsDuplicateError(err) {
			existing, err = s.inviteDao.GetInviteCodeByUserID(ctx, projectID, userID)
			if err == nil {
				return existing.Code, nil
			}
		}
		return "", err
	}

	return code, nil
}

// UseInviteCode 使用邀请码
func (s *InviteService) UseInviteCode(ctx context.Context, projectID, inviteUserID, code string) error {
	logger := zlog.LogWithContext(ctx)

	// 1. 检查用户是否已经使用过邀请码（全局检查，不按项目隔离）
	existingRecord, err := s.inviteDao.GetInviteRecordByInvite(ctx, inviteUserID)
	if err == nil && existingRecord != nil {
		return ErrAlreadyUsedInviteCode
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// 2. 查找邀请码对应的邀请人
	inviteCode, err := s.inviteDao.GetInviteCodeByCode(ctx, code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInviteCodeNotFound
		}
		return err
	}

	// 3. 检查是否使用自己的邀请码
	if inviteCode.UserID == inviteUserID {
		return ErrCannotInviteSelf
	}

	// 4. 获取配置的积分值
	creditAmount := s.getInviteCreditAmount()
	maxCredits := s.getInviteMaxCredits()

	// 5. 使用事务处理邀请记录和积分发放（封顶检查在事务内执行）
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 在事务内检查邀请人是否已达到积分封顶（全局统计，避免竞态条件）
		inviterCreditGranted := true
		if maxCredits > 0 {
			grantedCount, err := s.inviteDao.GetGrantedCreditsCountByInviterWithTx(ctx, tx, inviteCode.UserID)
			if err != nil {
				logger.Error("failed to get granted credits count", zap.Error(err))
				return err
			}
			grantedCredits := grantedCount * creditAmount
			if grantedCredits >= maxCredits {
				inviterCreditGranted = false
				logger.Info("inviter reached credit cap",
					zap.String("inviter_user_id", inviteCode.UserID),
					zap.Int64("granted_credits", grantedCredits),
					zap.Int64("max_credits", maxCredits))
			}
		}
		// 创建邀请记录（ProjectID 使用邀请人的项目，用于记录归属）
		record := &model.InviteRecord{
			ProjectID:     inviteCode.ProjectID,
			InviterUserID: inviteCode.UserID,
			InviteUserID:  inviteUserID,
			InviteCode:    code,
		}

		if err := s.inviteDao.CreateInviteRecord(ctx, tx, record); err != nil {
			if dao.IsDuplicateError(err) {
				return ErrAlreadyUsedInviteCode
			}
			return err
		}

		// 积分永久有效（使用2099年表示永不过期，避免MySQL零值问题）
		expiredAt := time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)

		// 发放被邀请人积分（始终发放）
		inviteSourceID := generateInviteSourceID("invite", projectID, inviteUserID, code)
		if err := s.creditService.AddCreditsWithTx(
			ctx, tx, projectID, inviteUserID, inviteSourceID,
			constants.CreditTypeEventGrant,
			constants.TransactionTypeInviteReward,
			creditAmount,
			expiredAt,
			"邀请码奖励",
			0,
		); err != nil {
			logger.Error("failed to grant invite credits", zap.Error(err))
			return err
		}

		// 发放邀请人积分（仅在未达到封顶时发放，积分发到邀请人的项目）
		if inviterCreditGranted {
			inviterSourceID := generateInviterSourceID(inviteCode.ProjectID, inviteCode.UserID, inviteUserID, code)
			if err := s.creditService.AddCreditsWithTx(
				ctx, tx, inviteCode.ProjectID, inviteCode.UserID, inviterSourceID,
				constants.CreditTypeEventGrant,
				constants.TransactionTypeInviteReward,
				creditAmount,
				expiredAt,
				"邀请好友奖励",
				0,
			); err != nil {
				logger.Error("failed to grant inviter credits", zap.Error(err))
				return err
			}
		}

		// 更新邀请记录状态
		return s.inviteDao.UpdateInviteRecordCreditStatus(ctx, tx, record.ID, inviterCreditGranted, true)
	})
}

// GetInviteInfo 获取用户的邀请信息
func (s *InviteService) GetInviteInfo(ctx context.Context, projectID, userID string) (*InviteInfo, error) {
	info := &InviteInfo{}

	// 获取用户的邀请码
	code, err := s.GetOrCreateInviteCode(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	info.InviteCode = code

	// 获取成功邀请人数（全局统计，可容忍错误）
	count, err := s.inviteDao.GetInviteStatsByInviter(ctx, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("failed to get invite stats, using default 0",
			zap.String("user_id", userID),
			zap.Error(err))
		count = 0
	}
	info.InvitedCount = count

	// 获取实际获得积分的邀请记录数（全局统计，可容忍错误）
	grantedCount, err := s.inviteDao.GetGrantedCreditsCountByInviter(ctx, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("failed to get granted credits count, using default 0",
			zap.String("user_id", userID),
			zap.Error(err))
		grantedCount = 0
	}
	info.TotalCredits = grantedCount * s.getInviteCreditAmount()

	// 检查用户是否已使用过邀请码（全局检查）
	record, err := s.inviteDao.GetInviteRecordByInvite(ctx, userID)
	if err == nil && record != nil {
		info.HasUsedInviteCode = true
		info.UsedInviteCode = record.InviteCode
	}

	// 未使用过邀请码即可使用
	info.CanUseInviteCode = !info.HasUsedInviteCode

	return info, nil
}

// GetInviteRecords 获取邀请记录列表
func (s *InviteService) GetInviteRecords(ctx context.Context, projectID, userID string, limit, offset int) ([]*InviteRecordItem, int64, error) {
	// 获取总数（全局统计，可容忍错误）
	total, err := s.inviteDao.GetInviteStatsByInviter(ctx, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("failed to get invite stats for records, using default 0",
			zap.String("user_id", userID),
			zap.Error(err))
		total = 0
	}

	// 获取记录列表（全局查询，可容忍错误）
	records, err := s.inviteDao.GetInviteRecordsByInviter(ctx, userID, limit, offset)
	if err != nil {
		zlog.LogWithContext(ctx).Warn("failed to get invite records, returning empty list",
			zap.String("user_id", userID),
			zap.Error(err))
		records = []*model.InviteRecord{}
	}

	// 构建 sourceID 映射，用于查询实际发放的积分
	sourceIDMap := make(map[uint]string, len(records)) // recordID -> sourceID
	sourceIDs := make([]string, 0, len(records))
	for _, r := range records {
		if r.InviterCreditGranted {
			sourceID := generateInviterSourceID(r.ProjectID, r.InviterUserID, r.InviteUserID, r.InviteCode)
			sourceIDMap[r.ID] = sourceID
			sourceIDs = append(sourceIDs, sourceID)
		}
	}

	// 批量查询积分流水，获取真实发放的积分
	creditMap := make(map[string]int64)
	if len(sourceIDs) > 0 {
		transactions, err := s.creditService.GetTransactionsBySourceIDs(ctx, sourceIDs, constants.TransactionTypeInviteReward)
		if err != nil {
			zlog.LogWithContext(ctx).Warn("failed to get invite credit transactions, using default 0",
				zap.String("user_id", userID),
				zap.Error(err))
		} else {
			for _, t := range transactions {
				creditMap[t.SourceID] = t.AmountChange
			}
		}
	}

	// 转换为返回结构
	items := make([]*InviteRecordItem, 0, len(records))
	for _, r := range records {
		credits := int64(0)
		if sourceID, ok := sourceIDMap[r.ID]; ok {
			credits = creditMap[sourceID]
		}
		items = append(items, &InviteRecordItem{
			InviteUserID: r.InviteUserID,
			InviteTime:   r.CreatedAt,
			Credits:      credits,
		})
	}

	return items, total, nil
}

// generateInviteSourceID 生成邀请奖励的 sourceID
func generateInviteSourceID(role, projectID, userID, code string) string {
	return fmt.Sprintf("invite_%s_%s_%s_%s", role, projectID, userID, code)
}

// generateInviterSourceID 生成邀请人奖励的 sourceID（包含被邀请人ID确保唯一）
func generateInviterSourceID(projectID, inviterUserID, inviteUserID, code string) string {
	return fmt.Sprintf("invite_inviter_%s_%s_%s_%s", projectID, inviterUserID, inviteUserID, code)
}
