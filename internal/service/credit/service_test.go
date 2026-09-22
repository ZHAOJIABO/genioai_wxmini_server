package credit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
)

// MockCreditDao 是 CreditDaoInterface 的 mock 实现
type MockCreditDao struct {
	mock.Mock
}

func (m *MockCreditDao) GetUserAmounts(ctx context.Context, tx *gorm.DB, projectID, userID string) ([]*model.UserAmount, error) {
	args := m.Called(ctx, tx, projectID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.UserAmount), args.Error(1)
}

func (m *MockCreditDao) CreateTransaction(ctx context.Context, tx *gorm.DB, transaction *model.CreditTransaction) error {
	args := m.Called(ctx, tx, transaction)
	return args.Error(0)
}

func (m *MockCreditDao) UpdateUserAmount(ctx context.Context, tx *gorm.DB, userAmount *model.UserAmount) error {
	args := m.Called(ctx, tx, userAmount)
	return args.Error(0)
}

func (m *MockCreditDao) FindTransactionBySourceID(ctx context.Context, tx *gorm.DB, projectID, sourceID string, transactionType constants.CreditTransactionType) (*model.CreditTransaction, error) {
	args := m.Called(ctx, tx, projectID, sourceID, transactionType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.CreditTransaction), args.Error(1)
}

func (m *MockCreditDao) GetUserTransactions(ctx context.Context, tx *gorm.DB, projectID, userID string, limit, offset int, creditChangeType vai.CreditChangeType) ([]*model.CreditTransaction, int64, error) {
	args := m.Called(ctx, tx, projectID, userID, limit, offset, creditChangeType)
	if args.Get(0) == nil {
		return nil, 0, args.Error(2)
	}
	return args.Get(0).([]*model.CreditTransaction), args.Get(1).(int64), args.Error(2)
}

func (m *MockCreditDao) ExtendActiveGiftCredits(ctx context.Context, tx *gorm.DB, userID string, newExpiresAt time.Time) error {
	args := m.Called(ctx, tx, userID, newExpiresAt)
	return args.Error(0)
}

func (m *MockCreditDao) FindPrioritizedGiftAmount(ctx context.Context, tx *gorm.DB, userID string) (int64, error) {
	args := m.Called(ctx, tx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockCreditDao) CreateUserAmount(ctx context.Context, tx *gorm.DB, amount *model.UserAmount) error {
	args := m.Called(ctx, tx, amount)
	return args.Error(0)
}

func (m *MockCreditDao) GetUserAmountsByIDs(ctx context.Context, tx *gorm.DB, ids []uint) ([]*model.UserAmount, error) {
	args := m.Called(ctx, tx, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.UserAmount), args.Error(1)
}

func (m *MockCreditDao) GetUserCreditOverview(ctx context.Context, tx *gorm.DB, projectID, userID string) (int64, int64, int64, int64, error) {
	args := m.Called(ctx, tx, projectID, userID)
	return args.Get(0).(int64), args.Get(1).(int64), args.Get(2).(int64), args.Get(3).(int64), args.Error(4)
}

func (m *MockCreditDao) GetTransactionsBySourceIDs(ctx context.Context, tx *gorm.DB, sourceIDs []string, transactionType constants.CreditTransactionType) ([]*model.CreditTransaction, error) {
	args := m.Called(ctx, tx, sourceIDs, transactionType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.CreditTransaction), args.Error(1)
}

// TestDeductCreditsForNonMember_DailyFreeOnly 测试非会员仅从每日免费积分扣减
func TestDeductCreditsForNonMember_DailyFreeOnly(t *testing.T) {
	mockDao := new(MockCreditDao)
	svc := &creditService{
		dao: mockDao,
		db:  nil,
	}

	ctx := context.Background()
	projectID := "test-project"
	userID := "test-user"
	amount := int64(15)

	// 模拟用户有 20 每日免费积分 + 100 内购积分
	userAmounts := []*model.UserAmount{
		{
			Model:           gorm.Model{ID: 1},
			AmountIden:      string(constants.CreditTypeDailyFree),
			RemainingAmount: 20,
			ProjectID:       projectID,
			UserID:          userID,
			SourceID:        "daily_free_2025_01_01",
		},
		{
			Model:           gorm.Model{ID: 2},
			AmountIden:      string(constants.CreditTypePurchased),
			RemainingAmount: 100,
			ProjectID:       projectID,
			UserID:          userID,
			SourceID:        "purchase_123",
		},
	}

	mockDao.On("GetUserAmounts", ctx, (*gorm.DB)(nil), projectID, userID).Return(userAmounts, nil)
	mockDao.On("UpdateUserAmount", mock.Anything, (*gorm.DB)(nil), mock.MatchedBy(func(ua *model.UserAmount) bool {
		return ua.ID == 1 && ua.RemainingAmount == 5 // 20 - 15 = 5
	})).Return(nil)
	mockDao.On("CreateTransaction", ctx, (*gorm.DB)(nil), mock.MatchedBy(func(tx *model.CreditTransaction) bool {
		return tx.CreditType == string(constants.CreditTypeDailyFree) && tx.AmountChange == -15
	})).Return(nil)

	deductionInfo, err := svc.DeductCreditsWithTx(ctx, nil, projectID, userID, "", constants.TransactionTypeUsageDeduction, amount, "test deduction", false)

	require.NoError(t, err)
	assert.Equal(t, int64(15), deductionInfo[1])
	assert.Len(t, deductionInfo, 1) // 只扣减了一笔
	mockDao.AssertExpectations(t)
}

// TestDeductCreditsForNonMember_PurchasedOnly 测试非会员当每日免费不足时，仅从内购积分扣减
func TestDeductCreditsForNonMember_PurchasedOnly(t *testing.T) {
	mockDao := new(MockCreditDao)
	svc := &creditService{
		dao: mockDao,
		db:  nil,
	}

	ctx := context.Background()
	projectID := "test-project"
	userID := "test-user"
	amount := int64(30)

	userAmounts := []*model.UserAmount{
		{
			Model:           gorm.Model{ID: 1},
			AmountIden:      string(constants.CreditTypeDailyFree),
			RemainingAmount: 20,
			ProjectID:       projectID,
			UserID:          userID,
			SourceID:        "daily_free_2025_01_01",
		},
		{
			Model:           gorm.Model{ID: 2},
			AmountIden:      string(constants.CreditTypePurchased),
			RemainingAmount: 100,
			ProjectID:       projectID,
			UserID:          userID,
			SourceID:        "purchase_123",
		},
	}

	mockDao.On("GetUserAmounts", ctx, (*gorm.DB)(nil), projectID, userID).Return(userAmounts, nil)
	mockDao.On("UpdateUserAmount", mock.Anything, (*gorm.DB)(nil), mock.MatchedBy(func(ua *model.UserAmount) bool {
		return ua.ID == 2 && ua.RemainingAmount == 70 // 100 - 30 = 70
	})).Return(nil)
	mockDao.On("CreateTransaction", ctx, (*gorm.DB)(nil), mock.MatchedBy(func(tx *model.CreditTransaction) bool {
		return tx.CreditType == string(constants.CreditTypePurchased) && tx.AmountChange == -30
	})).Return(nil)

	deductionInfo, err := svc.DeductCreditsWithTx(ctx, nil, projectID, userID, "", constants.TransactionTypeUsageDeduction, amount, "test deduction", false)

	require.NoError(t, err)
	assert.Equal(t, int64(30), deductionInfo[2])
	assert.Len(t, deductionInfo, 1) // 只扣减了一笔（内购）
	// 每日免费积分未被触碰
	mockDao.AssertExpectations(t)
}

// TestDeductCreditsForNonMember_NoMixing 测试非会员不混用策略：总额足够但不混用时报错
func TestDeductCreditsForNonMember_NoMixing(t *testing.T) {
	mockDao := new(MockCreditDao)
	svc := &creditService{
		dao: mockDao,
		db:  nil,
	}

	ctx := context.Background()
	projectID := "test-project"
	userID := "test-user"
	amount := int64(22) // 需要 22，但 daily_free=20, purchased=5，总额足够但不能混用

	userAmounts := []*model.UserAmount{
		{
			Model:           gorm.Model{ID: 1},
			AmountIden:      string(constants.CreditTypeDailyFree),
			RemainingAmount: 20,
			ProjectID:       projectID,
			UserID:          userID,
			SourceID:        "daily_free_2025_01_01",
		},
		{
			Model:           gorm.Model{ID: 2},
			AmountIden:      string(constants.CreditTypePurchased),
			RemainingAmount: 5,
			ProjectID:       projectID,
			UserID:          userID,
			SourceID:        "purchase_123",
		},
	}

	mockDao.On("GetUserAmounts", ctx, (*gorm.DB)(nil), projectID, userID).Return(userAmounts, nil)

	deductionInfo, err := svc.DeductCreditsWithTx(ctx, nil, projectID, userID, "", constants.TransactionTypeUsageDeduction, amount, "test deduction", false)

	require.Error(t, err)
	assert.Equal(t, ErrInsufficientCredits, err)
	assert.Nil(t, deductionInfo)
	mockDao.AssertExpectations(t)
}

// TestDeductCreditsForNonMember_WithMembershipGrant 测试非会员同时有过期会员积分，可与内购混用
func TestDeductCreditsForNonMember_WithMembershipGrant(t *testing.T) {
	mockDao := new(MockCreditDao)
	svc := &creditService{
		dao: mockDao,
		db:  nil,
	}

	ctx := context.Background()
	projectID := "test-project"
	userID := "test-user"
	amount := int64(35) // 需要 35，daily_free=20（不足），purchased=20 + membership=20 = 40（足够）

	userAmounts := []*model.UserAmount{
		{
			Model:           gorm.Model{ID: 1},
			AmountIden:      string(constants.CreditTypeDailyFree),
			RemainingAmount: 20,
			ProjectID:       projectID,
			UserID:          userID,
			SourceID:        "daily_free_2025_01_01",
		},
		{
			Model:           gorm.Model{ID: 2},
			AmountIden:      string(constants.CreditTypePurchased),
			RemainingAmount: 20,
			ProjectID:       projectID,
			UserID:          userID,
			SourceID:        "purchase_123",
		},
		{
			Model:           gorm.Model{ID: 3},
			AmountIden:      string(constants.CreditTypeMembershipGrant),
			RemainingAmount: 20,
			ProjectID:       projectID,
			UserID:          userID,
			SourceID:        "membership_expired",
		},
	}

	mockDao.On("GetUserAmounts", ctx, (*gorm.DB)(nil), projectID, userID).Return(userAmounts, nil)
	mockDao.On("UpdateUserAmount", mock.Anything, (*gorm.DB)(nil), mock.MatchedBy(func(ua *model.UserAmount) bool {
		return ua.ID == 2 && ua.RemainingAmount == 0 // purchased 全部用完
	})).Return(nil)
	mockDao.On("UpdateUserAmount", mock.Anything, (*gorm.DB)(nil), mock.MatchedBy(func(ua *model.UserAmount) bool {
		return ua.ID == 3 && ua.RemainingAmount == 5 // membership 用掉 15
	})).Return(nil)
	mockDao.On("CreateTransaction", ctx, (*gorm.DB)(nil), mock.MatchedBy(func(tx *model.CreditTransaction) bool {
		return tx.CreditType == string(constants.CreditTypePurchased) && tx.AmountChange == -20
	})).Return(nil)
	mockDao.On("CreateTransaction", ctx, (*gorm.DB)(nil), mock.MatchedBy(func(tx *model.CreditTransaction) bool {
		return tx.CreditType == string(constants.CreditTypeMembershipGrant) && tx.AmountChange == -15
	})).Return(nil)

	deductionInfo, err := svc.DeductCreditsWithTx(ctx, nil, projectID, userID, "", constants.TransactionTypeUsageDeduction, amount, "test deduction", false)

	require.NoError(t, err)
	assert.Equal(t, int64(20), deductionInfo[2]) // purchased
	assert.Equal(t, int64(15), deductionInfo[3]) // membership
	assert.Len(t, deductionInfo, 2)
	mockDao.AssertExpectations(t)
}

// TestDeductCreditsForMember_SkipDailyFree 测试会员跳过每日免费积分，允许其他类型混用
func TestDeductCreditsForMember_SkipDailyFree(t *testing.T) {
	mockDao := new(MockCreditDao)
	svc := &creditService{
		dao: mockDao,
		db:  nil,
	}

	ctx := context.Background()
	projectID := "test-project"
	userID := "test-user"
	amount := int64(25) // 需要 25，membership=10 + purchased=20 = 30（足够）

	userAmounts := []*model.UserAmount{
		{
			Model:           gorm.Model{ID: 1},
			AmountIden:      string(constants.CreditTypeDailyFree),
			RemainingAmount: 50, // 会员应该跳过这个
			ProjectID:       projectID,
			UserID:          userID,
			SourceID:        "daily_free_2025_01_01",
		},
		{
			Model:           gorm.Model{ID: 2},
			AmountIden:      string(constants.CreditTypeMembershipGrant),
			RemainingAmount: 10,
			ProjectID:       projectID,
			UserID:          userID,
			SourceID:        "membership_active",
		},
		{
			Model:           gorm.Model{ID: 3},
			AmountIden:      string(constants.CreditTypePurchased),
			RemainingAmount: 20,
			ProjectID:       projectID,
			UserID:          userID,
			SourceID:        "purchase_123",
		},
	}

	mockDao.On("GetUserAmounts", ctx, (*gorm.DB)(nil), projectID, userID).Return(userAmounts, nil)
	mockDao.On("UpdateUserAmount", mock.Anything, (*gorm.DB)(nil), mock.MatchedBy(func(ua *model.UserAmount) bool {
		return ua.ID == 2 && ua.RemainingAmount == 0 // membership 用完
	})).Return(nil)
	mockDao.On("UpdateUserAmount", mock.Anything, (*gorm.DB)(nil), mock.MatchedBy(func(ua *model.UserAmount) bool {
		return ua.ID == 3 && ua.RemainingAmount == 5 // purchased 用掉 15
	})).Return(nil)
	mockDao.On("CreateTransaction", ctx, (*gorm.DB)(nil), mock.MatchedBy(func(tx *model.CreditTransaction) bool {
		return tx.CreditType == string(constants.CreditTypeMembershipGrant) && tx.AmountChange == -10
	})).Return(nil)
	mockDao.On("CreateTransaction", ctx, (*gorm.DB)(nil), mock.MatchedBy(func(tx *model.CreditTransaction) bool {
		return tx.CreditType == string(constants.CreditTypePurchased) && tx.AmountChange == -15
	})).Return(nil)

	deductionInfo, err := svc.DeductCreditsWithTx(ctx, nil, projectID, userID, "", constants.TransactionTypeUsageDeduction, amount, "test deduction", true)

	require.NoError(t, err)
	assert.Equal(t, int64(10), deductionInfo[2]) // membership
	assert.Equal(t, int64(15), deductionInfo[3]) // purchased
	assert.Len(t, deductionInfo, 2)
	// daily_free 未被触碰
	mockDao.AssertExpectations(t)
}

// TestRefundCredits_ExpiredCredit 测试退款到已过期的积分记录
// 说明退款逻辑的预期行为
func TestRefundCredits_ExpiredCredit(t *testing.T) {
	// 注意：由于 RefundCredits 内部使用 s.db.Transaction，完整测试需要真实 DB 或更复杂的 mock
	// 这里仅作为逻辑说明，展示退款到过期记录的预期行为

	t.Log("退款逻辑说明：")
	t.Log("1. 积分会返还到原 UserAmount 记录（根据 CreditDeductionInfo 中的 amountID）")
	t.Log("2. 即使该记录已过期，余额仍会增加")
	t.Log("3. 过期记录的积分将不会再被使用（因为 GetUserAmounts 会过滤 expired_at < now 的记录）")
	t.Log("4. 如需改进，可在退款时检测过期并转发到新的系统补偿记录，或延长有效期")

	// 预期行为示例：
	// - 用户在 2025-01-01 使用了 15 个每日免费积分（当天有效期到 23:59:59）
	// - 任务提交后记录了 CreditDeductionInfo: {"1":15}
	// - 2025-01-02 任务失败，触发退款
	// - RefundCredits 会将 15 积分加回到 ID=1 的 UserAmount 记录
	// - 但该记录的 ExpiredAt 是 2025-01-01 23:59:59，已过期
	// - 后续扣减时 GetUserAmounts 不会返回该记录，因此该 15 积分实际无法使用
	// - 这是当前设计的预期行为，确保每日积分的时效性
}
