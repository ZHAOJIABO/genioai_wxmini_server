package biz

// RegisterHandlers 批量注册多个处理器
func RegisterHandlers(handlers ...BizHandler) {
	Handlers = append(Handlers, handlers...)
}

// ClearHandlers 清空已注册处理器（用于测试）
func ClearHandlers() {
	Handlers = nil
}
