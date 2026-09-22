package biz

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	chatcommon "va_visionai_server/internal/service/chat/common"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// StandardLLMHandler 处理常规请求
type StandardLLMHandler struct {
	BaseBizHandler
	llmService chatcommon.LLMService
}

// Name 返回处理器名称
func (h *StandardLLMHandler) Name() string {
	return "standard_llm_handler"
}

// NewStandardLLMHandler 构造函数
func NewStandardLLMHandler(llmSvc chatcommon.LLMService, msgSvc MessageService, chatDao *dao.ChatDao) *StandardLLMHandler {
	return &StandardLLMHandler{
		BaseBizHandler: NewBaseBizHandler(msgSvc, chatDao),
		llmService:     llmSvc,
	}
}

func (h *StandardLLMHandler) Match(ctx chatcommon.ChatContext) bool {
	// 标准聊天请求且没有特殊的promptID
	req := ctx.GetRequest()
	if req == nil {
		return false
	}

	if chatReq, ok := req.(interface{ GetChatBizHandlerID() vai.ChatBizHandlerID }); ok {
		return chatReq.GetChatBizHandlerID() == vai.ChatBizHandlerID_CHAT_BIZ_HANDLER_ID_DEFAULT
	}

	return false
}

func (h *StandardLLMHandler) Handle(ctx context.Context, chatCtx chatcommon.ChatContext, history []model.MessageHistory) (string, interface{}, error) {
	llmStartTime := time.Now()

	// 从上下文提取请求和流式服务器
	if chatCtx.GetRequest() == nil {
		return "", nil, errors.New("invalid request")
	}

	// 类型断言获取具体请求
	originalReq := chatCtx.GetRequest().(interface{ GetOriginalRequest() interface{} }).GetOriginalRequest()
	req, ok := originalReq.(*vai.ChatMessageSendRequest)
	if !ok {
		return "", nil, errors.New("invalid request type")
	}

	// 获取流式服务器
	streamServer, _ := chatCtx.GetStreamServer().(vai.ChatService_SendChatMessageStreamServer)

	// 创建回调函数
	callback := func(ctx context.Context, token string) error {
		if streamServer == nil {
			return nil
		}

		res := &vai.ChatMessageStreamResponse{
			MessageToken:   token,
			ReqMessageId:   req.GetMessage().GetMessageId(),
			ReplyMessageId: chatCtx.GetReplyMessageID(),
			ResponseHeader: &vai.ResponseHeader{
				Code:  0,
				Msg:   "success",
				ReqId: req.GetRequestHeader().GetReqId(),
			},
			IsEnd: false,
		}

		if err := streamServer.Context().Err(); err != nil {
			zlog.LogWithContext(ctx).Error("Stream Context Done", zap.Error(err))
			return ErrRemoteClose
		}

		if err := streamServer.Send(res); err != nil {
			zlog.LogWithContext(ctx).Error("StreamServer send failed", zap.Error(err))
			return ErrRemoteClose
		}

		return nil
	}

	// 解析模型和处理图片
	ctx, finalModelID, imageContent, _ := h.llmService.ResolveModel(ctx, req, history)

	// 调用 LLM 服务
	replyContent, err := h.llmService.StreamProcess(
		ctx,
		req,
		history,
		callback,
		chatcommon.LLMOptions{
			InitialModel:  finalModelID,
			GlobalTimeout: 120 * time.Second,
			GapTimeout:    60 * time.Second,
			ImageContent:  imageContent,
		},
	)

	llmDuration := time.Since(llmStartTime)
	zlog.LogWithContext(ctx).Info("LLM processing completed",
		zap.Duration("duration", llmDuration),
		zap.Int("replyLength", len(replyContent)),
		zap.String(constants.CtxChatID, req.GetMessage().GetChatId()))

	return replyContent, finalModelID, err
}

func (h *StandardLLMHandler) StreamResponse(ctx context.Context, chatCtx chatcommon.ChatContext, content string) error {
	// 获取流式服务器
	streamServer, ok := chatCtx.GetStreamServer().(vai.ChatService_SendChatMessageStreamServer)
	if !ok || streamServer == nil {
		return nil
	}

	// 从上下文提取请求
	originalReq := chatCtx.GetRequest().(interface{ GetOriginalRequest() interface{} }).GetOriginalRequest()
	req, ok := originalReq.(*vai.ChatMessageSendRequest)
	if !ok {
		return errors.New("invalid request type")
	}

	reqHeader := req.GetRequestHeader()

	// 发送结束消息
	err := streamServer.Send(&vai.ChatMessageStreamResponse{
		ReqMessageId:   req.GetMessage().GetMessageId(),
		ReplyMessageId: chatCtx.GetReplyMessageID(),
		MessageToken:   "" + AppendLowVersionWarningIfMac(reqHeader),
		IsEnd:          true,
		ResponseHeader: &vai.ResponseHeader{
			Code:           vai.StatusCode_SUCCESS,
			Msg:            "ok",
			ReqId:          reqHeader.GetReqId(),
			ResponseTimeMs: time.Now().UnixMilli(),
			ServerTime:     time.Now().Format(common.TimestampFormat),
		},
	})

	if err != nil {
		zlog.LogWithContext(ctx).Error("ChatStream Send EndTag Msg Error", zap.Error(err))
	}

	return nil
}

// AppendLowVersionWarningIfMac 添加Mac版本警告，与原有逻辑保持一致
func AppendLowVersionWarningIfMac(header *vai.RequestHeader) string {
	const lowVersionWarning = `

**温馨提示：您使用的是旧版本的Mac APP，为了保证体验，请[点击链接](https://bluefocus.feishu.cn/wiki/R3kgw6Qcji4pZjk9Q3wcbmpYnKe)升级到最新版本**`
	const lowVersion = "1.2.1"

	if !isMacOS(header) {
		return ""
	}

	if header.GetApp().GetAppVersion() < lowVersion {
		return lowVersionWarning
	}

	return ""
}

// isMacOS 判断请求是否来自MacOS
func isMacOS(header *vai.RequestHeader) bool {
	if header.GetDevice().GetOs() == 3 || header.GetDevice().GetOs() == 0 {
		return true
	}
	return false
}
