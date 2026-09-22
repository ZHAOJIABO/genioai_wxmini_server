package model

import "time"

// TaskChain 两步固定链（图→视频）
type TaskChain struct {
	ChainID             string    `json:"chain_id" gorm:"column:chain_id;primaryKey;type:varchar(64)"`
	RootTaskID          string    `json:"root_task_id" gorm:"column:root_task_id;type:varchar(64);index"`
	NextTaskID          string    `json:"next_task_id" gorm:"column:next_task_id;type:varchar(64);index"`
	UserID              string    `json:"user_id" gorm:"column:user_id;type:varchar(64);index"`
	ProjectID           string    `json:"project_id" gorm:"column:project_id;type:varchar(64)"`
	Status              int32     `json:"status" gorm:"column:status;type:int;default:0;comment:0-PENDING,1-RUNNING,2-DONE,3-FAILED"`
	CurrentStep         int32     `json:"current_step" gorm:"column:current_step;type:int;default:1"`
	ConfigJSON          string    `json:"config_json" gorm:"column:config_json;type:text"`
	AllTaskIDs          string    `json:"all_task_ids" gorm:"column:all_task_ids;type:text;comment:所有步骤任务ID的JSON数组"`
	TotalCreditCost     int       `json:"total_credit_cost" gorm:"column:total_credit_cost;type:int;default:0;comment:总积分消耗"`
	CreditDeductionInfo string    `json:"credit_deduction_info" gorm:"column:credit_deduction_info;type:text;comment:积分扣除详情"`
	CreatedAt           time.Time `json:"created_at" gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP"`
	UpdatedAt           time.Time `json:"updated_at" gorm:"column:updated_at;type:timestamp;default:CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP"`
}
