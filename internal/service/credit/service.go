package credit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

var (
	ErrInsufficientCredits = errors.New("insufficient credits")
)

// CreditDetails 定义了支付事件消息中与积分相关的具体信息
type CreditDetails struct {
	Type            string    `json:"type"`
	TransactionType string    `json:"transaction_type"`
	Amount          int64     `json:"amount"`
	ExpiresAt       time.Time `json:"expires_at,omitempty"`
	Description     string    `json:"description"`
}

type SubscriptionDetails struct {
	Level     int       `json:"level"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Service defines the interface for credit management operations.
type Service interface {
	DeductCredits(ctx context.Context, projectID, userID, sourceID string, transactionType constants.CreditTransactionType, amount int64, description string, isMember bool) (map[uint]int64, error)
	DeductCreditsWithTx(ctx context.Context, tx *gorm.DB, projectID, userID, sourceID string, transactionType constants.CreditTransactionType, amount int64, description string, isMember bool) (map[uint]int64, error)
	AddCredits(ctx context.Context, projectID, userID, sourceID string, creditType constants.CreditType, transactionType constants.CreditTransactionType, amount int64, expiredAt time.Time, description string, subscribeLevel int) error
	AddCreditsWithTx(ctx context.Context, tx *gorm.DB, projectID, userID, sourceID string, creditType constants.CreditType, transactionType constants.CreditTransactionType, amount int64, expiredAt time.Time, description string, subscribeLevel int) error
	RefundCredits(ctx context.Context, task *model.PictureTask, description string) error
	ClearExpiredCredits(ctx context.Context, projectID, userID string, creditType constants.CreditType) (int64, error)
	HandleMembershipUpgrade(ctx context.Context, projectID, userID, orderID string, creditInfo CreditDetails, subDetails SubscriptionDetails) error

	// 查询接口
	GetUserCreditBalance(ctx context.Context, projectID, userID string) (*UserCreditBalance, error)
	GetUserCreditTransactions(ctx context.Context, projectID, userID string, limit, offset int, creditChangeType vai.CreditChangeType) ([]*model.CreditTransaction, int64, error)
	GetUserCreditOverview(ctx context.Context, projectID, userID string) (int64, int64, int64, int64, error)
	GetTransactionsBySourceIDs(ctx context.Context, sourceIDs []string, transactionType constants.CreditTransactionType) ([]*model.CreditTransaction, error)

	// 统计接口
	GetExpiredCreditsStats(ctx context.Context, projectID string, startTime, endTime time.Time) (*ExpiredCreditsStats, error)
}

// UserCreditBalance 用户额度余额信息
type UserCreditBalance struct {
	UserID              string              `json:"user_id"`
	ProjectID           string              `json:"project_id"`
	PurchasedCredits    int64               `json:"purchased_credits"`    // 内购次数余额
	SubscriptionCredits int64               `json:"subscription_credits"` // 会员赠送次数余额
	SystemGrantCredits  int64               `json:"system_grant_credits"` // 系统赠积分
	TotalCredits        int64               `json:"total_credits"`        // 总余额
	CreditDetails       []*UserCreditDetail `json:"credit_details"`       // 详细信息
}

// UserCreditDetail 用户额度详细信息
type UserCreditDetail struct {
	CreditType      string    `json:"credit_type"`
	RemainingAmount int64     `json:"remaining_amount"`
	ExpiredAt       time.Time `json:"expired_at"`
}

// ExpiredCreditsStats 过期额度统计信息
type ExpiredCreditsStats struct {
	ProjectID           string    `json:"project_id"`
	TotalExpiredCredits int64     `json:"total_expired_credits"`
	ExpiredUserCount    int64     `json:"expired_user_count"`
	StartTime           time.Time `json:"start_time"`
	EndTime             time.Time `json:"end_time"`
}

// creditService implements the Service interface.
type creditService struct {
	dao dao.CreditDaoInterface
	db  *gorm.DB
}

// NewService creates a new credit service.
func NewService(dao dao.CreditDaoInterface, db *gorm.DB) Service {
	return &creditService{
		dao: dao,
		db:  db,
	}
}

// DeductCredits deducts a specified amount of credits from a user's balance.
// It prioritizes deducting from credits that expire soonest (subscription gifts) before using non-expiring ones (purchased).
func (s *creditService) DeductCredits(ctx context.Context, projectID, userID, sourceID string, transactionType constants.CreditTransactionType, amount int64, description string, isMember bool) (map[uint]int64, error) {
	var deductionInfo map[uint]int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var txErr error
		deductionInfo, txErr = s.DeductCreditsWithTx(ctx, tx, projectID, userID, sourceID, transactionType, amount, description, isMember)
		return txErr
	})
	return deductionInfo, err
}

// DeductCreditsWithTx deducts credits within a given database transaction.
// For non-members: daily_free and purchased credits cannot be mixed in a single deduction.
// If amount <= daily_free_total, deduct only from daily_free.
// Else if amount <= purchased_total, deduct only from purchased.
// Otherwise, return ErrInsufficientCredits (no mixing allowed).
func (s *creditService) DeductCreditsWithTx(ctx context.Context, tx *gorm.DB, projectID, userID, sourceID string, transactionType constants.CreditTransactionType, amount int64, description string, isMember bool) (map[uint]int64, error) {
	userAmounts, err := s.dao.GetUserAmounts(ctx, tx, projectID, userID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user amounts")
	}

	// 会员：跳过 daily_free，其余类型可混用（维持原逻辑）
	if isMember {
		return s.deductCreditsForMember(ctx, tx, projectID, userID, sourceID, transactionType, amount, description, userAmounts)
	}

	// 非会员：按类型分组，不混用策略
	return s.deductCreditsForNonMember(ctx, tx, projectID, userID, sourceID, transactionType, amount, description, userAmounts)
}

// deductCreditsForMember 会员扣减逻辑：跳过 daily_free，其余按过期时间混用
func (s *creditService) deductCreditsForMember(ctx context.Context, tx *gorm.DB, projectID, userID, sourceID string, transactionType constants.CreditTransactionType, amount int64, description string, userAmounts []*model.UserAmount) (map[uint]int64, error) {
	var totalBalance int64
	availableAmounts := make([]*model.UserAmount, 0, len(userAmounts))
	for _, ua := range userAmounts {
		if ua.AmountIden == string(constants.CreditTypeDailyFree) {
			zlog.LogWithContext(ctx).Debug("skip daily free credits for member",
				zap.String("user_id", userID),
				zap.Uint("amount_id", ua.ID),
				zap.Int64("remaining", ua.RemainingAmount))
			continue
		}
		availableAmounts = append(availableAmounts, ua)
		totalBalance += ua.RemainingAmount
	}

	if totalBalance < amount {
		return nil, ErrInsufficientCredits
	}

	return s.performDeduction(ctx, tx, projectID, userID, sourceID, transactionType, amount, description, availableAmounts)
}

// deductCreditsForNonMember 非会员扣减逻辑：每日免费与其他类型不混用
func (s *creditService) deductCreditsForNonMember(ctx context.Context, tx *gorm.DB, projectID, userID, sourceID string, transactionType constants.CreditTransactionType, amount int64, description string, userAmounts []*model.UserAmount) (map[uint]int64, error) {
	// 按类型分组并汇总余额
	dailyFreeAmounts := make([]*model.UserAmount, 0)
	purchasedAmounts := make([]*model.UserAmount, 0)
	membershipAmounts := make([]*model.UserAmount, 0)
	eventGrantAmounts := make([]*model.UserAmount, 0)

	var dailyFreeTotal, purchasedTotal, membershipTotal, eventGrantTotal int64

	for _, ua := range userAmounts {
		switch ua.AmountIden {
		case string(constants.CreditTypeDailyFree):
			dailyFreeAmounts = append(dailyFreeAmounts, ua)
			dailyFreeTotal += ua.RemainingAmount
		case string(constants.CreditTypePurchased):
			purchasedAmounts = append(purchasedAmounts, ua)
			purchasedTotal += ua.RemainingAmount
		case string(constants.CreditTypeMembershipGrant):
			membershipAmounts = append(membershipAmounts, ua)
			membershipTotal += ua.RemainingAmount
		case string(constants.CreditTypeEventGrant):
			eventGrantAmounts = append(eventGrantAmounts, ua)
			eventGrantTotal += ua.RemainingAmount
		}
	}

	zlog.LogWithContext(ctx).Debug("non-member credit breakdown",
		zap.String("user_id", userID),
		zap.Int64("daily_free_total", dailyFreeTotal),
		zap.Int64("purchased_total", purchasedTotal),
		zap.Int64("membership_total", membershipTotal),
		zap.Int64("event_grant_total", eventGrantTotal),
		zap.Int64("amount_to_deduct", amount))

	// 策略：优先用每日免费（如果足够），否则用内购+会员赠送+活动赠送（如果足够），否则报错
	// 活动赠送视为可与内购、会员赠送混用的积分类型
	if amount <= dailyFreeTotal {
		// 仅扣每日免费
		zlog.LogWithContext(ctx).Info("deducting from daily_free only",
			zap.String("user_id", userID),
			zap.Int64("amount", amount))
		return s.performDeduction(ctx, tx, projectID, userID, sourceID, transactionType, amount, description, dailyFreeAmounts)
	}

	// 每日免费不足，检查内购+会员赠送+活动赠送合计
	paidCreditsTotal := purchasedTotal + membershipTotal + eventGrantTotal
	if amount <= paidCreditsTotal {
		// 仅扣内购、会员赠送和活动赠送（这三类可以混用）
		combinedAmounts := append(purchasedAmounts, membershipAmounts...)
		combinedAmounts = append(combinedAmounts, eventGrantAmounts...)
		zlog.LogWithContext(ctx).Info("deducting from purchased+membership+event_grant only",
			zap.String("user_id", userID),
			zap.Int64("amount", amount),
			zap.Int64("purchased_total", purchasedTotal),
			zap.Int64("membership_total", membershipTotal),
			zap.Int64("event_grant_total", eventGrantTotal))
		return s.performDeduction(ctx, tx, projectID, userID, sourceID, transactionType, amount, description, combinedAmounts)
	}

	// 不足且不混用
	zlog.LogWithContext(ctx).Debug("insufficient credits: no mixing allowed for non-member",
		zap.String("user_id", userID),
		zap.Int64("amount", amount),
		zap.Int64("daily_free_total", dailyFreeTotal),
		zap.Int64("paid_credits_total", paidCreditsTotal))
	return nil, ErrInsufficientCredits
}

// performDeduction 执行实际扣减操作（从指定的 userAmounts 列表中扣减）
func (s *creditService) performDeduction(ctx context.Context, tx *gorm.DB, projectID, userID, sourceID string, transactionType constants.CreditTransactionType, amount int64, description string, userAmounts []*model.UserAmount) (map[uint]int64, error) {
	deductionInfo := make(map[uint]int64)
	amountToDeduct := amount

	for _, ua := range userAmounts {
		if amountToDeduct == 0 {
			break
		}

		deduction := min(amountToDeduct, ua.RemainingAmount)
		if deduction == 0 {
			continue
		}
		ua.RemainingAmount -= deduction
		amountToDeduct -= deduction

		if err := s.dao.UpdateUserAmount(context.Background(), tx, ua); err != nil {
			return nil, errors.Wrap(err, "failed to update user amount")
		}
		deductionInfo[ua.ID] = deduction

		transaction := &model.CreditTransaction{
			ProjectID:       projectID,
			UserID:          userID,
			TransactionID:   uuid.NewString(),
			TransactionType: string(transactionType),
			AmountChange:    -deduction,
			CreditType:      ua.AmountIden,
			BalanceAfter:    ua.RemainingAmount,
			SourceID:        sourceID,
			Description:     description,
		}
		if err := s.dao.CreateTransaction(ctx, tx, transaction); err != nil {
			return nil, errors.Wrap(err, "failed to create transaction")
		}
	}

	return deductionInfo, nil
}

// AddCredits adds a specified amount of credits to a user's balance.
func (s *creditService) AddCredits(ctx context.Context, projectID, userID, sourceID string, creditType constants.CreditType, transactionType constants.CreditTransactionType, amount int64, expiredAt time.Time, description string, subscribeLevel int) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return s.addCreditsWithTx(ctx, tx, projectID, userID, sourceID, creditType, transactionType, amount, expiredAt, description, subscribeLevel)
	})
}

// AddCreditsWithTx adds credits using provided transaction (exposed API)
func (s *creditService) AddCreditsWithTx(ctx context.Context, tx *gorm.DB, projectID, userID, sourceID string, creditType constants.CreditType, transactionType constants.CreditTransactionType, amount int64, expiredAt time.Time, description string, subscribeLevel int) error {
	return s.addCreditsWithTx(ctx, tx, projectID, userID, sourceID, creditType, transactionType, amount, expiredAt, description, subscribeLevel)
}

// addCreditsWithTx adds credits within a given database transaction.
func (s *creditService) addCreditsWithTx(ctx context.Context, tx *gorm.DB, projectID, userID, sourceID string, creditType constants.CreditType, transactionType constants.CreditTransactionType, amount int64, expiredAt time.Time, description string, subscribeLevel int) error {
	newUserAmount := &model.UserAmount{
		ProjectID:       projectID,
		UserID:          userID,
		AmountIden:      string(creditType),
		RemainingAmount: amount,
		ExpiredAt:       expiredAt,
		SourceID:        sourceID,
		SubscribeLevel:  subscribeLevel,
	}

	if err := s.dao.CreateUserAmount(ctx, tx, newUserAmount); err != nil {
		return errors.Wrap(err, "failed to create user amount")
	}

	transaction := &model.CreditTransaction{
		ProjectID:       projectID,
		UserID:          userID,
		TransactionID:   uuid.NewString(),
		TransactionType: string(transactionType),
		AmountChange:    amount,
		CreditType:      string(creditType),
		BalanceAfter:    amount, // For new records, balance after is the amount itself
		SourceID:        sourceID,
		Description:     description,
	}

	if err := s.dao.CreateTransaction(ctx, tx, transaction); err != nil {
		return errors.Wrap(err, "failed to create transaction")
	}

	return nil
}

// RefundCredits refunds credits for a failed operation by parsing the deduction info from the task.
func (s *creditService) RefundCredits(ctx context.Context, task *model.PictureTask, description string) error {
	if task.CreditDeductionInfo == "" {
		// No deduction info, likely a free task or an old task before this mechanism.
		// For backward compatibility, we might log this but not return an error.
		return nil
	}

	var deductionInfo map[uint]int64
	if err := json.Unmarshal([]byte(task.CreditDeductionInfo), &deductionInfo); err != nil {
		return errors.Wrap(err, "failed to unmarshal credit deduction info")
	}

	if len(deductionInfo) == 0 {
		return nil // Nothing to refund
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Get all user amount records that were part of the deduction in one query
		amountIDs := make([]uint, 0, len(deductionInfo))
		for id := range deductionInfo {
			amountIDs = append(amountIDs, id)
		}

		userAmounts, err := s.dao.GetUserAmountsByIDs(ctx, tx, amountIDs)
		if err != nil {
			return errors.Wrap(err, "failed to get user amounts for refund")
		}

		userAmountsByID := make(map[uint]*model.UserAmount, len(userAmounts))
		for _, ua := range userAmounts {
			userAmountsByID[ua.ID] = ua
		}

		for amountID, refundAmount := range deductionInfo {
			ua, ok := userAmountsByID[amountID]
			if !ok {
				// This should not happen if DB is consistent.
				// Log a warning and continue.
				continue
			}

			ua.RemainingAmount += refundAmount
			if err := s.dao.UpdateUserAmount(ctx, tx, ua); err != nil {
				return errors.Wrapf(err, "failed to update user amount ID %d for refund", ua.ID)
			}

			// Create a refund transaction record
			refundTransaction := &model.CreditTransaction{
				ProjectID:       task.ProjectID,
				UserID:          task.UserID,
				TransactionID:   uuid.NewString(),
				TransactionType: string(constants.TransactionTypeCreationFailedRefund),
				AmountChange:    refundAmount,
				CreditType:      ua.AmountIden,
				BalanceAfter:    ua.RemainingAmount,
				SourceID:        task.TaskID, // Link transaction to the task
				Description:     description,
			}
			if err := s.dao.CreateTransaction(ctx, tx, refundTransaction); err != nil {
				return errors.Wrap(err, "failed to create refund transaction")
			}
		}
		return nil
	})
}

// ClearExpiredCredits clears expired credits of a specific type for a user.
// Returns the total amount of credits that were cleared.
func (s *creditService) ClearExpiredCredits(ctx context.Context, projectID, userID string, creditType constants.CreditType) (int64, error) {
	var totalCleared int64

	err := s.db.Transaction(func(tx *gorm.DB) error {
		userAmounts, err := s.dao.GetUserAmounts(ctx, tx, projectID, userID)
		if err != nil {
			return errors.Wrap(err, "failed to get user amounts")
		}

		now := time.Now()
		for _, ua := range userAmounts {
			// 只清理指定类型且已过期的额度
			if ua.AmountIden == string(creditType) && ua.ExpiredAt.Before(now) && ua.RemainingAmount > 0 {
				clearedAmount := ua.RemainingAmount
				ua.RemainingAmount = 0

				if err := s.dao.UpdateUserAmount(ctx, tx, ua); err != nil {
					return errors.Wrap(err, "failed to update user amount")
				}

				// 记录清理交易
				transaction := &model.CreditTransaction{
					ProjectID:       projectID,
					UserID:          userID,
					TransactionID:   uuid.NewString(),
					TransactionType: string(constants.TransactionTypeSubscriptionExpiredClear),
					AmountChange:    -clearedAmount,
					CreditType:      string(creditType),
					BalanceAfter:    0,
					SourceID:        "expired_cleanup_" + time.Now().Format("20060102150405"),
					Description:     "清理过期会员额度",
				}

				if err := s.dao.CreateTransaction(ctx, tx, transaction); err != nil {
					return errors.Wrap(err, "failed to create clear transaction")
				}

				totalCleared += clearedAmount
			}
		}

		return nil
	})

	return totalCleared, err
}

// GetUserCreditBalance 获取用户额度余额信息
func (s *creditService) GetUserCreditBalance(ctx context.Context, projectID, userID string) (*UserCreditBalance, error) {
	userAmounts, err := s.dao.GetUserAmounts(ctx, s.db, projectID, userID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get user amounts")
	}

	balance := &UserCreditBalance{
		UserID:        userID,
		ProjectID:     projectID,
		CreditDetails: make([]*UserCreditDetail, 0, len(userAmounts)),
	}

	for _, ua := range userAmounts {
		balance.CreditDetails = append(balance.CreditDetails, &UserCreditDetail{
			CreditType:      ua.AmountIden,
			RemainingAmount: ua.RemainingAmount,
			ExpiredAt:       ua.ExpiredAt,
		})

		balance.TotalCredits += ua.RemainingAmount

		switch ua.AmountIden {
		case string(constants.CreditTypePurchased):
			balance.PurchasedCredits += ua.RemainingAmount
		case string(constants.CreditTypeMembershipGrant):
			balance.SubscriptionCredits += ua.RemainingAmount
			//每日赠送和活动赠送都算做系统赠送
		case string(constants.CreditTypeDailyFree), string(constants.CreditTypeEventGrant):
			balance.SystemGrantCredits += ua.RemainingAmount
		}
	}

	return balance, nil
}

func (s *creditService) GetUserCreditOverview(ctx context.Context, projectID, userID string) (int64, int64, int64, int64, error) {
	return s.dao.GetUserCreditOverview(ctx, s.db, projectID, userID)
}

// GetUserCreditTransactions 获取用户额度交易记录
func (s *creditService) GetUserCreditTransactions(ctx context.Context, projectID, userID string, limit, offset int, creditChangeType vai.CreditChangeType) ([]*model.CreditTransaction, int64, error) {
	transactions, total, err := s.dao.GetUserTransactions(ctx, s.db, projectID, userID, limit, offset, creditChangeType)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to get user transactions")
	}
	return transactions, total, nil
}

// GetExpiredCreditsStats 获取过期额度统计信息
func (s *creditService) GetExpiredCreditsStats(ctx context.Context, projectID string, startTime, endTime time.Time) (*ExpiredCreditsStats, error) {
	// 这里需要在 DAO 层实现相应的统计查询
	// 目前返回一个简化的实现
	stats := &ExpiredCreditsStats{
		ProjectID: projectID,
		StartTime: startTime,
		EndTime:   endTime,
		// TODO: 实现实际的统计查询
		TotalExpiredCredits: 0,
		ExpiredUserCount:    0,
	}
	return stats, nil
}

// HandleMembershipUpgrade handles the credit logic for a membership upgrade.
// It extends the expiration of existing gift credits and adds the new credits from the upgrade.
func (s *creditService) HandleMembershipUpgrade(ctx context.Context, projectID, userID, eventID string, creditInfo CreditDetails, subDetails SubscriptionDetails) error {
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Error; err != nil {
		return err
	}

	// 1. Extend existing gift credits
	if err := s.dao.ExtendActiveGiftCredits(ctx, tx, userID, subDetails.ExpiresAt); err != nil {
		tx.Rollback()
		return errors.Wrap(err, "failed to extend active gift credits")
	}

	// 2. Add new credits from the upgrade
	err := s.addCreditsWithTx(
		ctx,
		tx,
		projectID,
		userID,
		eventID,
		constants.CreditType(creditInfo.Type),
		constants.CreditTransactionType(creditInfo.TransactionType),
		creditInfo.Amount,
		creditInfo.ExpiresAt,
		creditInfo.Description,
		subDetails.Level,
	)

	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "failed to add new credits for upgrade")
	}

	return tx.Commit().Error
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// GetTransactionsBySourceIDs 根据 sourceID 列表批量查询积分流水
func (s *creditService) GetTransactionsBySourceIDs(ctx context.Context, sourceIDs []string, transactionType constants.CreditTransactionType) ([]*model.CreditTransaction, error) {
	return s.dao.GetTransactionsBySourceIDs(ctx, s.db, sourceIDs, transactionType)
}
