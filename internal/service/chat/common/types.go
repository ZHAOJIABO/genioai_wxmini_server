package common

import (
	"context"
	"time"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
)

// RequestType 区分请求类型
type RequestType int

const (
	RequestTypeStandard RequestType = iota
	RequestTypeCustomPersonalInfoQA
	// 更多类型可扩展
)

// ChatRequest 抽象不同类型的请求
type ChatRequest interface {
	GetRequestType() RequestType
	GetOriginalRequest() interface{}
}

// ChatContext 通用的处理上下文接口
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
	GetParticipantIDs() []string
	GetChatBizHandlerID() vai.ChatBizHandlerID

	// 辅助setter
	SetChat(chat *model.Chat)
}

// LLMOptions 配置LLM调用参数
type LLMOptions struct {
	InitialModel   vai.Model     // 初始模型ID
	GlobalTimeout  time.Duration // 全局超时时间
	GapTimeout     time.Duration // 消息间隔超时
	ImageContent   string        // 预处理的图片内容（可为空）
	ThinkingConfig *common.ThinkingConfig
}

// LLMService 定义了 LLM 服务的接口
type LLMService interface {
	// ResolveModel 解析模型和处理图片
	ResolveModel(ctx context.Context, req *vai.ChatMessageSendRequest, history []model.MessageHistory) (context.Context, vai.Model, string, error)

	// StreamProcess 处理流式请求
	StreamProcess(ctx context.Context, req *vai.ChatMessageSendRequest, history []model.MessageHistory, callback func(context.Context, string) error, opts LLMOptions) (string, error)
}
