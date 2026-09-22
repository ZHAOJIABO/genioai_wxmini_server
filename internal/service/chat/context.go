package chat

import (
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/chat/common"
	vai "va_visionai_server/internal/va_interface"
)

// RequestType 区分请求类型
type RequestType common.RequestType

const (
	RequestTypeStandard             RequestType = RequestType(common.RequestTypeStandard)
	RequestTypeCustomPersonalInfoQA RequestType = RequestType(common.RequestTypeCustomPersonalInfoQA)
	// 更多类型可扩展
)

// ChatRequest 抽象不同类型的请求
type ChatRequest interface {
	GetRequestType() RequestType
	GetOriginalRequest() interface{}
}

// StandardRequest 封装标准聊天请求
type StandardRequest struct {
	Req *vai.ChatMessageSendRequest
}

func (r *StandardRequest) GetRequestType() RequestType     { return RequestTypeStandard }
func (r *StandardRequest) GetOriginalRequest() interface{} { return r.Req }

type CustomPersonalInfoChatRequest struct {
	Req *vai.ChatMessageSendRequest
}

func (r *CustomPersonalInfoChatRequest) GetRequestType() RequestType {
	return RequestTypeCustomPersonalInfoQA
}
func (r *CustomPersonalInfoChatRequest) GetOriginalRequest() interface{} { return r.Req }

// ChatContext 统一的处理上下文
type ChatContext struct {
	// 基础信息
	UserID           string
	ChatID           string
	Content          string
	MessageType      vai.MessageType
	ChatBizHandlerID vai.ChatBizHandlerID
	MessageID        string
	ReplyMessageID   string
	PromptID         string

	// 多媒体内容
	URLs           []string
	Blobs          []*vai.Blob
	VoiceContent   string
	VoiceHash      string
	FrameURL       string
	ParticipantIDs []string

	// 请求/响应处理
	Request      ChatRequest
	StreamServer interface{} // 不同类型的stream服务器

	// 扩展信息
	Chat *model.Chat // 会话模型
}

// 实现 common.ChatContext 接口
func (c *ChatContext) GetUserID() string                         { return c.UserID }
func (c *ChatContext) GetChatID() string                         { return c.ChatID }
func (c *ChatContext) GetContent() string                        { return c.Content }
func (c *ChatContext) GetMessageType() interface{}               { return c.MessageType }
func (c *ChatContext) GetChatBizHandlerID() vai.ChatBizHandlerID { return c.ChatBizHandlerID }
func (c *ChatContext) GetMessageID() string                      { return c.MessageID }
func (c *ChatContext) GetReplyMessageID() string                 { return c.ReplyMessageID }
func (c *ChatContext) GetPromptID() string                       { return c.PromptID }
func (c *ChatContext) GetURLs() []string                         { return c.URLs }
func (c *ChatContext) GetBlobs() interface{}                     { return c.Blobs }
func (c *ChatContext) GetVoiceContent() string                   { return c.VoiceContent }
func (c *ChatContext) GetVoiceHash() string                      { return c.VoiceHash }
func (c *ChatContext) GetFrameURL() string                       { return c.FrameURL }
func (c *ChatContext) GetRequest() interface{} {
	return requestTypeWrapper{req: c.Request}
}
func (c *ChatContext) GetStreamServer() interface{} { return c.StreamServer }
func (c *ChatContext) GetChat() *model.Chat         { return c.Chat }
func (c *ChatContext) SetChat(chat *model.Chat)     { c.Chat = chat }
func (c *ChatContext) GetParticipantIDs() []string {
	return c.ParticipantIDs
}

// 自定义类型以实现请求接口
type requestTypeWrapper struct {
	req ChatRequest
}

func (r requestTypeWrapper) GetRequestType() common.RequestType {
	return common.RequestType(r.req.GetRequestType())
}

func (r requestTypeWrapper) GetOriginalRequest() interface{} {
	return r.req.GetOriginalRequest()
}

func (r requestTypeWrapper) GetChatBizHandlerID() vai.ChatBizHandlerID {
	if req, ok := r.req.GetOriginalRequest().(*vai.ChatMessageSendRequest); ok {
		return req.GetMessage().GetChatBizHandlerId()
	}
	return vai.ChatBizHandlerID_CHAT_BIZ_HANDLER_ID_DEFAULT
}
