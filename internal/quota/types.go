package quota

// QuotaType 配额类型（计数型限制）
type QuotaType string

const (
	QuotaTypeQueue      QuotaType = "queue"      // 队列长度配额
	QuotaTypeConcurrent QuotaType = "concurrent" // 并发执行配额
)
