package event_reporter

// ============ 事件类型常量 ============

const (
	//BUSINESS
	// 用户相关事件
	EventTypeUserLogin    = "USER_LOGIN"
	EventTypeUserLogout   = "USER_LOGOUT"
	EventTypeUserRegister = "USER_REGISTER"

	// PROMPT 增强
	EventTypePromptEnhance = "PROMPT_ENHANCE"

	// API 调用事件
	EventTypeAPICall = "API_CALL"

	// 数据库操作事件
	EventTypeDatabaseOp = "DATABASE_OPERATION"

	// 缓存操作事件
	EventTypeCacheOp = "CACHE_OPERATION"

	// 任务相关事件
	EventTypeTaskCreated     = "TASK_CREATED"
	EventTypeTaskCompleted   = "TASK_COMPLETED"
	EventTypeTaskFailed      = "TASK_FAILED"
	EventTypeTaskSubmit      = "TASK_SUBMIT"       // 单任务提交事件
	EventTypeTaskChainSubmit = "TASK_CHAIN_SUBMIT" // 任务链提交事件

	// 支付相关事件
	EventTypePaymentSuccess = "PAYMENT_SUCCESS"
	EventTypePaymentFailed  = "PAYMENT_FAILED"

	// 积分相关事件
	EventTypeCreditDeduct  = "CREDIT_DEDUCT"
	EventTypeCreditRefund  = "CREDIT_REFUND"
	EventTypeCreditExpired = "CREDIT_EXPIRED"
)

// ============ ActionContext Domain 常量 ============

const (
	// DomainInternal 内部执行（函数调用等）
	DomainInternal = "INTERNAL"
	// DomainExternal 外部交互（API、数据库等）
	DomainExternal = "EXTERNAL"
)

// ============ ActionContext Type 常量 ============

const (
	// TypeFunction 函数执行
	TypeFunction = "FUNCTION"
	// TypeAPI HTTP/API 调用
	TypeAPI = "API"
	// TypeDatabase 数据库操作
	TypeDatabase = "DATABASE"
	// TypeCache 缓存操作
	TypeCache = "CACHE"
	// TypeQueue 消息队列操作
	TypeQueue = "QUEUE"
	// TypeRPC RPC 调用
	TypeRPC = "RPC"
)

// ============ ActionContext Operation 常量 ============

const (
	// OpExecute 执行
	OpExecute = "EXECUTE"
	// OpCall 调用
	OpCall = "CALL"
	// OpQuery 查询
	OpQuery = "QUERY"
	// OpCommand 命令（INSERT/UPDATE/DELETE）
	OpCommand = "COMMAND"
	// OpPublish 发布消息
	OpPublish = "PUBLISH"
	// OpConsume 消费消息
	OpConsume = "CONSUME"
	// OpGet 获取
	OpGet = "GET"
	// OpSet 设置
	OpSet = "SET"
	// OpDelete 删除
	OpDelete = "DELETE"
)
