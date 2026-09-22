package biz

import (
	"context"

	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/chat/common"
	vai "va_visionai_server/internal/va_interface"
)

// ChatContext 类型的前向声明
type ChatContext interface {
	GetUserID() string
	GetChatID() string
	GetContent() string
	GetMessageType() interface{}
	GetMessageID() string
	GetReplyMessageID() string
	GetPromptID() string
	GetURLs() []string
	GetBlobs() interface{}
	GetVoiceContent() string
	GetVoiceHash() string
	GetFrameURL() string
	GetRequest() interface{}
	GetStreamServer() interface{}
	GetChat() *model.Chat
	GetChatBizHandlerID() vai.ChatBizHandlerID

	// 辅助setter
	SetChat(chat *model.Chat)
}

// BizHandler 定义业务编排策略
type BizHandler interface {
	// Name 返回处理器名称
	Name() string

	// Match 匹配条件
	Match(ctx common.ChatContext) bool

	// EnsureChatSession 确保会话存在，由 Handler 自己实现或继承实现
	EnsureChatSession(ctx context.Context, chatCtx common.ChatContext) (isNew bool, err error)

	// PrepareHistory 准备消息历史，不同请求可能有不同处理
	PrepareHistory(ctx context.Context, chatCtx common.ChatContext) ([]model.MessageHistory, error)

	// PrepareSystemPrompt 准备系统提示词
	PrepareSystemPrompt(ctx context.Context, chatCtx common.ChatContext) error

	// Handle 执行业务处理
	Handle(ctx context.Context,
		chatCtx common.ChatContext,
		history []model.MessageHistory) (string, interface{}, error)

	// StreamResponse 处理流式响应（可选实现）
	StreamResponse(ctx context.Context,
		chatCtx common.ChatContext,
		content string) error
}

// 注册表与注册函数
var Handlers []BizHandler

func Register(h BizHandler) {
	Handlers = append(Handlers, h)
}
