package model

import (
	"gorm.io/gorm"
)

// CreditTransaction records every change in user's credits.
// It provides a detailed audit trail for all credit-related operations.
type CreditTransaction struct {
	gorm.Model

	ProjectID       string `gorm:"type:varchar(255);not null;index"`
	UserID          string `gorm:"type:varchar(255);not null;index"`
	TransactionID   string `gorm:"type:varchar(100);not null;uniqueIndex"`
	TransactionType string `gorm:"type:varchar(50);not null;index"`
	AmountChange    int64  `gorm:"not null"`                  // Positive for addition, negative for deduction
	CreditType      string `gorm:"type:varchar(50);not null"` // e.g., "purchased", "subscription_gift"

	// BalanceAfter is the balance of the specific credit_type for the user after this transaction.
	BalanceAfter int64 `gorm:"not null"`

	SourceID    string `gorm:"type:varchar(255);index"` // Optional: related task_id, order_id, etc.
	Description string `gorm:"type:varchar(255)"`
}
