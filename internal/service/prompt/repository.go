package prompt

import (
	"time"

	"va_visionai_server/internal/model"
)

// InspirationRepository 定义灵感提示词数据访问接口
// 遵循Go最佳实践：接口由消费者（service包）定义，而不是提供者（dao包）
// 只包含service层实际需要的方法，符合接口隔离原则（ISP）
type InspirationRepository interface {
	// GetPromptsByToolType 根据工具类型获取所有启用的灵感（不筛选语言）
	GetPromptsByToolType(toolType string) ([]*model.InspirationPrompt, error)

	// GetPromptByID 根据 prompt_id 获取灵感
	GetPromptByID(promptID string) (*model.InspirationPrompt, error)

	// CountApplicationsBatch 批量统计多个灵感在指定时间后的应用数
	// 返回 map[promptID]count，未统计到的promptID自动填充为0
	CountApplicationsBatch(promptIDs []string, since time.Time) (map[string]int64, error)

	// CountApplications 统计指定灵感在指定时间后的应用数
	CountApplications(promptID string, since time.Time) (int64, error)

	// RecordApplication 记录灵感应用
	RecordApplication(application *model.InspirationApplication) error
}
