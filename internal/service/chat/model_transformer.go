package chat

import (
	"context"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// ModelTransformer 定义模型转换器接口
type ModelTransformer interface {
	Transform(ctx context.Context, req *vai.ChatMessageSendRequest)
}

// DoubaoTransformer 将所有 Doubao 系列强制映射到 Gemini 2.0 Flash
type DoubaoTransformer struct{}

func NewDoubaoTransformer() ModelTransformer {
	return &DoubaoTransformer{}
}

func (t *DoubaoTransformer) Transform(ctx context.Context, req *vai.ChatMessageSendRequest) {
	msg := req.GetMessage()

	// 通过 ModelID 映射
	switch msg.GetModelId() {
	case vai.Model_MODEL_DOUBAO_VISION_PRO,
		vai.Model_MODEL_DOUBAO_LITE,
		vai.Model_MODEL_DOUBAO_PRO:

		msg.ModelId = vai.Model_MODEL_GEMINI_2_0_FLASH
		msg.ModelName = vai.Model_MODEL_GEMINI_2_0_FLASH.String()
		zlog.LogWithContext(ctx).Info("transportDoubaoToGemini By ModelID",
			zap.String("ModelID", msg.GetModelId().String()),
			zap.String("ModelName", msg.GetModelName()),
		)
	}

	// 通过 ModelName 映射
	switch msg.GetModelName() {
	case vai.Model_MODEL_DOUBAO.String(),
		vai.Model_MODEL_DOUBAO_VISION_PRO.String(),
		vai.Model_MODEL_DOUBAO_LITE.String():
		msg.ModelId = vai.Model_MODEL_GEMINI_2_0_FLASH
		msg.ModelName = vai.Model_MODEL_GEMINI_2_0_FLASH.String()
		zlog.LogWithContext(ctx).Info("transportDoubaoToGemini By ModelName",
			zap.String("ModelID", msg.GetModelId().String()),
			zap.String("ModelName", msg.GetModelName()),
		)
	}
}

// VideoTransformer 视频消息强制使用 Gemini Flash
type VideoTransformer struct{}

func NewVideoTransformer() ModelTransformer {
	return &VideoTransformer{}
}

func (t *VideoTransformer) Transform(ctx context.Context, req *vai.ChatMessageSendRequest) {
	if req.GetMessage().GetMessageType() == vai.MessageType_MT_VIDEO {
		msg := req.GetMessage()
		msg.ModelId = vai.Model_MODEL_GEMINI_2_0_FLASH
		msg.ModelName = vai.Model_MODEL_GEMINI_2_0_FLASH.String()
		zlog.LogWithContext(ctx).Info("视频消息检测到，强制使用 Gemini Flash 模型",
			zap.String(constants.CtxModelName, msg.GetModelId().String()))
	}
}
