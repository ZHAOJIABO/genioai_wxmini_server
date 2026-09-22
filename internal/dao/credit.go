package dao

import (
	"context"
	"time"

	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
)

// CreditDaoInterface defines the interface for credit data access operations.
type CreditDaoInterface interface {
	GetUserAmounts(ctx context.Context, tx *gorm.DB, projectID, userID string) ([]*model.UserAmount, error)
	CreateTransaction(ctx context.Context, tx *gorm.DB, transaction *model.CreditTransaction) error
	UpdateUserAmount(ctx context.Context, tx *gorm.DB, userAmount *model.UserAmount) error
	FindTransactionBySourceID(ctx context.Context, tx *gorm.DB, projectID, sourceID string, transactionType constants.CreditTransactionType) (*model.CreditTransaction, error)
	GetUserTransactions(ctx context.Context, tx *gorm.DB, projectID, userID string, limit, offset int, creditChangeType vai.CreditChangeType) ([]*model.CreditTransaction, int64, error)
	ExtendActiveGiftCredits(ctx context.Context, tx *gorm.DB, userID string, newExpiresAt time.Time) error
	FindPrioritizedGiftAmount(ctx context.Context, tx *gorm.DB, userID string) (int64, error)
	CreateUserAmount(ctx context.Context, tx *gorm.DB, amount *model.UserAmount) error
	GetUserAmountsByIDs(ctx context.Context, tx *gorm.DB, ids []uint) ([]*model.UserAmount, error)
	GetUserCreditOverview(ctx context.Context, tx *gorm.DB, projectID, userID string) (int64, int64, int64, int64, error)
	GetTransactionsBySourceIDs(ctx context.Context, tx *gorm.DB, sourceIDs []string, transactionType constants.CreditTransactionType) ([]*model.CreditTransaction, error)
}

// CreditDao handles database operations for credits.
type CreditDao struct {
	db *gorm.DB
}

// NewCreditDao creates a new CreditDao.
func NewCreditDao() *CreditDao {
	return &CreditDao{db: db.GetDB()}
}

// GetUserAmounts retrieves all non-expired, positive balance user amounts, ordered by expiration date (soonest first).
func (d *CreditDao) GetUserAmounts(ctx context.Context, tx *gorm.DB, projectID, userID string) ([]*model.UserAmount, error) {
	var userAmounts []*model.UserAmount
	err := tx.WithContext(ctx).
		Where("project_id = ? AND user_id = ? AND remaining_amount > 0 AND (expired_at > ? OR expired_at IS NULL)", projectID, userID, time.Now()).
		Order("expired_at ASC").
		Find(&userAmounts).Error
	return userAmounts, err
}

// CreateTransaction creates a new credit transaction record.
func (d *CreditDao) CreateTransaction(ctx context.Context, tx *gorm.DB, transaction *model.CreditTransaction) error {
	return tx.WithContext(ctx).Create(transaction).Error
}

// UpdateUserAmount updates a user's credit balance.
func (d *CreditDao) UpdateUserAmount(ctx context.Context, tx *gorm.DB, userAmount *model.UserAmount) error {
	return tx.WithContext(ctx).Save(userAmount).Error
}

// FindTransactionBySourceID finds a transaction by its source ID (e.g., task_id).
func (d *CreditDao) FindTransactionBySourceID(ctx context.Context, tx *gorm.DB, projectID, sourceID string, transactionType constants.CreditTransactionType) (*model.CreditTransaction, error) {
	var transaction model.CreditTransaction
	err := tx.WithContext(ctx).
		Where("project_id = ? AND source_id = ? AND transaction_type = ?", projectID, sourceID, transactionType).
		First(&transaction).Error
	if err != nil {
		return nil, err
	}
	return &transaction, nil
}

// GetUserTransactions retrieves a paginated list of credit transactions for a user.
func (d *CreditDao) GetUserTransactions(ctx context.Context, tx *gorm.DB, projectID, userID string, limit, offset int, creditChangeType vai.CreditChangeType) ([]*model.CreditTransaction, int64, error) {
	var transactions []*model.CreditTransaction
	var total int64

	baseQuery := tx.Model(&model.CreditTransaction{}).
		Where("project_id = ? AND user_id = ? AND transaction_type != ?", projectID, userID, constants.TransactionTypeSubscriptionExpiry)

	switch creditChangeType {
	case vai.CreditChangeType_CREDIT_CHANGE_TYPE_RECHARGE:
		baseQuery = baseQuery.Where("amount_change > ?", 0)
	case vai.CreditChangeType_CREDIT_CHANGE_TYPE_CONSUME:
		baseQuery = baseQuery.Where("amount_change < ?", 0)
	case vai.CreditChangeType_CREDIT_CHANGE_TYPE_ALL:
	}

	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	dataQuery := tx.Where("project_id = ? AND user_id = ? AND transaction_type != ?", projectID, userID, constants.TransactionTypeSubscriptionExpiry)

	switch creditChangeType {
	case vai.CreditChangeType_CREDIT_CHANGE_TYPE_RECHARGE:
		dataQuery = dataQuery.Where("amount_change > ?", 0)
	case vai.CreditChangeType_CREDIT_CHANGE_TYPE_CONSUME:
		dataQuery = dataQuery.Where("amount_change < ?", 0)
	}

	if err := dataQuery.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&transactions).Error; err != nil {
		return nil, 0, err
	}

	return transactions, total, nil
}

// ExtendActiveGiftCredits extends the expiration date of all active gift credits for a user.
func (d *CreditDao) ExtendActiveGiftCredits(ctx context.Context, tx *gorm.DB, userID string, newExpiresAt time.Time) error {
	giftCreditTypes := []string{
		string(constants.CreditTypeMembershipGrant),
	}
	return tx.WithContext(ctx).
		Model(&model.UserAmount{}).
		Where("user_id = ? AND amount_iden IN (?) AND remaining_amount > 0 AND expired_at > ? AND expired_at < ?", userID, giftCreditTypes, time.Now(), newExpiresAt).
		Update("expired_at", newExpiresAt).Error
}

func (d *CreditDao) FindPrioritizedGiftAmount(ctx context.Context, tx *gorm.DB, userID string) (int64, error) {
	giftCreditTypes := []string{
		string(constants.CreditTypeMembershipGrant),
	}
	return d.findTotalAmountByTypes(ctx, tx, userID, giftCreditTypes)
}

func (d *CreditDao) findTotalAmountByTypes(ctx context.Context, tx *gorm.DB, userID string, creditTypes []string) (int64, error) {
	var total int64
	err := tx.WithContext(ctx).
		Model(&model.UserAmount{}).
		Where("user_id = ? AND amount_iden IN (?) AND remaining_amount > 0", userID, creditTypes).
		Select("COALESCE(SUM(remaining_amount), 0)").
		Scan(&total).Error
	return total, err
}

// CreateUserAmount creates a new user amount record.
func (d *CreditDao) CreateUserAmount(ctx context.Context, tx *gorm.DB, amount *model.UserAmount) error {
	return tx.WithContext(ctx).Create(amount).Error
}

func (d *CreditDao) GetUserAmountsByIDs(ctx context.Context, tx *gorm.DB, ids []uint) ([]*model.UserAmount, error) {
	var userAmounts []*model.UserAmount
	if len(ids) == 0 {
		return userAmounts, nil
	}
	err := tx.WithContext(ctx).
		Where("id IN ?", ids).
		Find(&userAmounts).Error
	return userAmounts, err
}

// GetTransactionsBySourceIDs 根据 sourceID 列表批量查询积分流水
func (d *CreditDao) GetTransactionsBySourceIDs(ctx context.Context, tx *gorm.DB, sourceIDs []string, transactionType constants.CreditTransactionType) ([]*model.CreditTransaction, error) {
	var transactions []*model.CreditTransaction
	if len(sourceIDs) == 0 {
		return transactions, nil
	}
	err := tx.WithContext(ctx).
		Where("source_id IN ? AND transaction_type = ? AND amount_change > 0", sourceIDs, transactionType).
		Find(&transactions).Error
	return transactions, err
}

var _ CreditDaoInterface = &CreditDao{}

func (d *CreditDao) GetUserCreditOverview(ctx context.Context, tx *gorm.DB, projectID, userID string) (int64, int64, int64, int64, error) {
	var result struct {
		MembershipAmount int64 `gorm:"column:membership_amount"`
		PurchasedAmount  int64 `gorm:"column:purchased_amount"`
		DailyFreeAmount  int64 `gorm:"column:daily_free_amount"`
	}
	err := tx.WithContext(ctx).
		Model(&model.UserAmount{}).
		Select("SUM(CASE WHEN amount_iden = ? THEN remaining_amount ELSE 0 END) as membership_amount, SUM(CASE WHEN amount_iden = ? THEN remaining_amount ELSE 0 END) as purchased_amount, SUM(CASE WHEN amount_iden = ? OR amount_iden = ?  THEN remaining_amount ELSE 0 END) as daily_free_amount", constants.CreditTypeMembershipGrant, constants.CreditTypePurchased, constants.CreditTypeDailyFree, constants.CreditTypeEventGrant).
		Where("project_id = ? AND user_id = ? AND expired_at > ?", projectID, userID, time.Now()).
		Scan(&result).Error
	totalAmount := result.MembershipAmount + result.PurchasedAmount + result.DailyFreeAmount
	return totalAmount, result.MembershipAmount, result.PurchasedAmount, result.DailyFreeAmount, err
}
