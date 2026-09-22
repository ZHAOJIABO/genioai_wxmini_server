package biz

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	chatcommon "va_visionai_server/internal/service/chat/common"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// BaseHandler 提供 BizHandler 接口的基础实现
type BaseHandler struct{}

// Name 返回处理器名称
func (h *BaseHandler) Name() string {
	return "base_handler"
}

// Match 默认不匹配任何请求
func (h *BaseHandler) Match(ctx chatcommon.ChatContext) bool {
	return false
}

// PrepareHistory 的默认实现
func (h *BaseHandler) PrepareHistory(ctx context.Context, chatCtx chatcommon.ChatContext) ([]model.MessageHistory, error) {
	return nil, nil
}

// PrepareSystemPrompt 的默认实现
func (h *BaseHandler) PrepareSystemPrompt(ctx context.Context, chatCtx chatcommon.ChatContext) error {
	return nil
}

// Handle 的默认实现
func (h *BaseHandler) Handle(ctx context.Context, chatCtx chatcommon.ChatContext, history []model.MessageHistory) (string, interface{}, error) {
	return "", nil, errors.New("not implemented")
}

// StreamResponse 的默认实现
func (h *BaseHandler) StreamResponse(ctx context.Context, chatCtx chatcommon.ChatContext, content string) error {
	return nil
}

// BaseBizHandler 提供默认实现以简化业务代码
type BaseBizHandler struct {
	msgService MessageService
	chatDao    *dao.ChatDao // 新增依赖
}

// 添加 MessageService 接口
type MessageService interface {
	GetMsgHistory(ctx context.Context, chatID string) ([]model.MessageHistory, error)
}

// 修改 BaseBizHandler 构造函数
func NewBaseBizHandler(msgService MessageService, chatDao *dao.ChatDao) BaseBizHandler {
	return BaseBizHandler{
		msgService: msgService,
		chatDao:    chatDao,
	}
}

// EnsureChatSession 实现默认的会话创建/获取逻辑
func (h *BaseBizHandler) EnsureChatSession(ctx context.Context, chatCtx chatcommon.ChatContext) (isNew bool, err error) {
	if h.chatDao == nil {
		zlog.LogWithContext(ctx).Error("ChatDao is nil in BaseBizHandler")
		return false, errors.New("internal configuration error: ChatDao not initialized")
	}

	chatStartTime := time.Now()
	messageType, ok := chatCtx.GetMessageType().(vai.MessageType)
	if !ok {
		zlog.LogWithContext(ctx).Error("Failed to assert MessageType", zap.Any("type", chatCtx.GetMessageType()))
		return false, errors.New("invalid message type in context")
	}
	projectID := common.GetProjectID(ctx)

	chat := &model.Chat{
		ChatID:     chatCtx.GetChatID(),
		ProjectID:  projectID,
		Title:      constants.NewChatTitle,
		UserID:     chatCtx.GetUserID(),
		Status:     0,
		CreateTime: time.Now().Unix(),
		PromptID:   chatCtx.GetPromptID(),
		IsVideo:    messageType == vai.MessageType_MT_VIDEO,
	}

	newRow, err := h.chatDao.FirstOrCreate(projectID, chatCtx.GetChatID(), chat)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to create or get chat (base)",
			zap.String(constants.CtxChatID, chatCtx.GetChatID()),
			zap.Error(err))
		return false, err
	}

	chatCtx.SetChat(chat)

	zlog.LogWithContext(ctx).Info("Base chat session ensured",
		zap.Duration("duration", time.Since(chatStartTime)),
		zap.Bool("isNewChat", newRow))

	return newRow, nil
}

// 默认的历史处理
func (h *BaseBizHandler) PrepareHistory(ctx context.Context, chatCtx chatcommon.ChatContext) ([]model.MessageHistory, error) {
	// 基于请求类型调用不同实现
	req := chatCtx.GetRequest()
	if req == nil {
		return nil, errors.New("request is nil")
	}

	// 类型断言获取请求类型
	if chatReq, ok := req.(interface{ GetRequestType() chatcommon.RequestType }); ok {
		reqType := chatReq.GetRequestType()

		switch reqType {
		case chatcommon.RequestTypeStandard:
			return h.prepareStandardHistory(ctx, chatCtx)
		default:
			return nil, errors.New("unsupported request type")
		}
	}

	return nil, errors.New("invalid request type")
}

// prepareStandardHistory 处理标准请求的历史
func (h *BaseBizHandler) prepareStandardHistory(ctx context.Context, chatCtx chatcommon.ChatContext) ([]model.MessageHistory, error) {
	var msgHistory []model.MessageHistory
	var err error

	// 使用 msgService，如果在 chatCtx.GetExtra() 中有覆盖则使用覆盖版本
	svc := h.msgService

	if len(chatCtx.GetURLs()) == 0 && svc != nil {
		msgHistory, err = svc.GetMsgHistory(ctx, chatCtx.GetChatID())
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			zlog.LogWithContext(ctx).Error("GetMsgHistory Error", zap.Error(err))
			return nil, err
		}
	}

	// 构建当前用户消息
	msgType, ok := chatCtx.GetMessageType().(vai.MessageType)
	if !ok {
		zlog.LogWithContext(ctx).Error("Failed to assert MessageType in prepareStandardHistory")
		msgType = vai.MessageType_MT_TEXT // 使用默认值
	}

	userMsg := model.MessageHistory{
		Content:  chatCtx.GetContent(),
		URLs:     chatCtx.GetURLs(),
		Sender:   "user",
		FileType: msgType,
	}

	if chatCtx.GetVoiceContent() != "" {
		userMsg.Content = chatCtx.GetVoiceContent()
	}

	if blobs, ok := chatCtx.GetBlobs().([]*vai.Blob); ok &&
		msgType == vai.MessageType_MT_VIDEO &&
		len(blobs) > 0 {
		userMsg.VideoBlob = blobs[0].GetData()
	}

	return append(msgHistory, userMsg), nil
}

// PrepareSystemPrompt 的默认实现
func (h *BaseBizHandler) PrepareSystemPrompt(ctx context.Context, chatCtx chatcommon.ChatContext) error {
	return nil
}
