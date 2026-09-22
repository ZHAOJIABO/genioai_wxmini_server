package constants

// 上下文键
const (
	// CtxLanguage            = "language" // 语言上下文键
	CtxUserID              = "UserID"
	CtxOSName              = "OSName"
	CtxAppVersiopn         = "AppVersion"
	CtxAppStore            = "AppStore"
	CtxTraceID             = "TraceID"
	CtxSessionID           = "SessionID"
	CtxPromptID            = "PromptID"
	CtxLang                = "Lang"
	CtxMessageID           = "MessageID"
	CtxChatID              = "ChatID"
	CtxOrderID             = "OrderID"
	CtxIP                  = "IP"
	CtxProductID           = "ProductID"
	CtxPaymentWay          = "PaymentWay"
	CtxIdfv                = "Idfv"
	CtxAndroidId           = "AndroidId"
	CtxPackageName         = "PackageName"
	CtxProjectID           = "ProjectID"
	CtxEventType           = "EventType"
	CtxUploaderRole        = "UploaderRole"
	CtxModelID             = "ModelID"
	CtxModelName           = "ModelName"
	CtxModelDeploymentName = "ModelDeploymentName"
	CtxServerName          = "ServerName"
	// CtxSubmitContextJSON 提交任务时的客户端上下文（JSON字符串）
	CtxSubmitContextJSON = "SubmitContextJSON"
	// CtxTaskSubmitSource 任务提交来源 (normal/tool/random/guide)
	CtxTaskSubmitSource = "TaskSubmitSource"
	// CtxToolID 工具ID（工具任务时使用）
	CtxToolID = "ToolID"
)

// 任务提交来源常量
const (
	SubmitSourceNormal = "NORMAL" // 普通任务提交
	SubmitSourceTool   = "TOOL"   // 工具任务提交
	SubmitSourceRandom = "RANDOM" // 随机工作流提交
	SubmitSourceGuide  = "GUIDE"  // 图转视频引导提交
)
