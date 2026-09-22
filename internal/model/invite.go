package model

import "gorm.io/gorm"

// InviteCode 用户邀请码
type InviteCode struct {
	gorm.Model
	ProjectID string `gorm:"column:project_id;type:varchar(100);not null;uniqueIndex:idx_invite_code_project_user"`
	UserID    string `gorm:"column:user_id;type:varchar(100);not null;uniqueIndex:idx_invite_code_project_user"`
	Code      string `gorm:"column:code;type:varchar(20);not null;uniqueIndex:idx_invite_code"`
}

// InviteRecord 邀请记录
type InviteRecord struct {
	gorm.Model
	ProjectID            string `gorm:"column:project_id;type:varchar(100);not null;uniqueIndex:idx_invite_record_project_invite;index:idx_invite_record_inviter"`
	InviterUserID        string `gorm:"column:inviter_user_id;type:varchar(100);not null;index:idx_invite_record_inviter"`
	InviteUserID         string `gorm:"column:invite_user_id;type:varchar(100);not null;uniqueIndex:idx_invite_record_project_invite"`
	InviteCode           string `gorm:"column:invite_code;type:varchar(20);not null"`
	InviterCreditGranted bool   `gorm:"column:inviter_credit_granted;type:tinyint(1);not null;default:0"`
	InviteCreditGranted  bool   `gorm:"column:invite_credit_granted;type:tinyint(1);not null;default:0"`
}
