package service

import (
	"context"
	"fmt"
	"time"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/credit"
	vai "va_visionai_server/internal/va_interface"

	"gorm.io/gorm"
)

type CreditService struct {
	db            *gorm.DB
	creditService credit.Service
}

func NewCreditService(db *gorm.DB, creditService credit.Service) *CreditService {
	return &CreditService{db: db, creditService: creditService}
}

type CreditBalance struct {
	Total      int64 `json:"total"`
	Membership int64 `json:"membership"`
	Purchased  int64 `json:"purchased"`
	System     int64 `json:"system"`
}

type CreditRecord struct {
	ID              uint   `json:"id"`
	TransactionID   string `json:"transaction_id"`
	TransactionType string `json:"transaction_type"`
	AmountChange    int64  `json:"amount_change"`
	CreditType      string `json:"credit_type"`
	Description     string `json:"description"`
	CreatedAt       string `json:"created_at"`
}

// AddCredits 给用户加积分
func (s *CreditService) AddCredits(ctx context.Context, projectID, userID string, amount int64, description string) error {
	expiredAt := time.Now().AddDate(1, 0, 0) // 1年后过期
	sourceID := fmt.Sprintf("admin_grant_%s_%d", userID, time.Now().UnixNano())
	return s.creditService.AddCredits(
		ctx,
		projectID,
		userID,
		sourceID,
		constants.CreditTypeEventGrant,
		constants.TransactionTypeSystemGrant,
		amount,
		expiredAt,
		description,
		0,
	)
}

// GetBalance 获取用户积分余额
func (s *CreditService) GetBalance(ctx context.Context, projectID, userID string) (*CreditBalance, error) {
	total, membership, purchased, system, err := s.creditService.GetUserCreditOverview(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	return &CreditBalance{
		Total:      total,
		Membership: membership,
		Purchased:  purchased,
		System:     system,
	}, nil
}

// GetHistory 获取积分流水
func (s *CreditService) GetHistory(ctx context.Context, projectID, userID string, page, size int) ([]CreditRecord, int64, error) {
	offset := (page - 1) * size
	transactions, total, err := s.creditService.GetUserCreditTransactions(ctx, projectID, userID, size, offset, vai.CreditChangeType_CREDIT_CHANGE_TYPE_ALL)
	if err != nil {
		return nil, 0, err
	}

	records := make([]CreditRecord, 0, len(transactions))
	for _, t := range transactions {
		createdAt := t.CreatedAt.Format("2006-01-02 15:04:05")
		records = append(records, CreditRecord{
			ID:              t.ID,
			TransactionID:   t.TransactionID,
			TransactionType: t.TransactionType,
			AmountChange:    t.AmountChange,
			CreditType:      t.CreditType,
			Description:     t.Description,
			CreatedAt:       createdAt,
		})
	}

	return records, total, nil
}

// FindUserIDByEmail 根据邮箱查找用户ID
func (s *CreditService) FindUserIDByEmail(ctx context.Context, email string) (string, error) {
	var user model.UserRecord
	err := s.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return "", err
	}
	return user.UserId, nil
}
