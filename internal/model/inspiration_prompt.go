package model

import (
	"time"

	"gorm.io/gorm"
)

// InspirationPrompt 灵感提示词表
type InspirationPrompt struct {
	ID         uint           `gorm:"primarykey" json:"id"`
	PromptID   string         `gorm:"type:varchar(64);uniqueIndex:uk_prompt_id;not null" json:"prompt_id"`
	Title      string         `gorm:"type:varchar(255);not null" json:"title"`                                     // 灵感标题(输入框底部展示)
	Content    string         `gorm:"type:text;not null" json:"content"`                                           // 灵感详细内容(点击后填充到输入框)
	ToolType   string         `gorm:"type:varchar(64);not null;index:idx_tool_type_status" json:"tool_type"`       // 工具类型(text2img/text2video等)
	Language   string         `gorm:"type:varchar(10);not null;default:'en';index:idx_language" json:"language"`   // 语言(zh/en)
	Status     int8           `gorm:"type:tinyint(1);not null;default:1;index:idx_tool_type_status" json:"status"` // 状态: 1-启用 0-禁用
	SortWeight int            `gorm:"type:int;not null;default:0" json:"sort_weight"`                              // 基础排序权重(数值越大越靠前)
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index:idx_tool_type_status" json:"deleted_at,omitempty"`
}

// IsEnabled 判断是否启用
func (ip *InspirationPrompt) IsEnabled() bool {
	return ip.Status == 1
}

// InspirationApplication 灵感应用统计表
type InspirationApplication struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	PromptID  string    `gorm:"type:varchar(64);not null;index:idx_prompt_recent" json:"prompt_id"` // 灵感ID
	UserID    string    `gorm:"type:varchar(64);not null;index:idx_user_tool" json:"user_id"`       // 用户ID
	ToolID    string    `gorm:"type:varchar(64);not null;index:idx_user_tool" json:"tool_id"`       // 工具ID
	ToolType  string    `gorm:"type:varchar(64);not null;index:idx_tool_type" json:"tool_type"`     // 工具类型
	AppliedAt time.Time `gorm:"not null;index:idx_prompt_recent" json:"applied_at"`                 // 应用时间
}
