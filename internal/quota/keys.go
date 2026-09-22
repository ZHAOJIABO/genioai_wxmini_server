package quota

// 统一的 Redis 键前后缀常量（供 service/task 共用）
const (
	ConcurrentTasksKeyPrefix = "visionai:concurrent:" // 并发任务集合前缀
	QueueTasksKeyPrefix      = "visionai:queue:"      // 队列任务集合前缀
	TasksKeySuffix           = ":tasks"               // 任务集合后缀
)
