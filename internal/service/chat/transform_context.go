package chat

import (
	"context"

	vai "va_visionai_server/internal/va_interface"
)

// TransformContext 转换上下文，包含转换过程中需要的所有信息
type TransformContext struct {
	ctx         context.Context
	message     *vai.Message
	language    string
	isVisual    bool
	modelResult vai.Model
	err         error
}

// NewTransformContext 创建新的转换上下文
func NewTransformContext(ctx context.Context, message *vai.Message, language string) *TransformContext {
	return &TransformContext{
		ctx:      ctx,
		message:  message,
		language: language,
	}
}

// SetModelResult 设置模型选择结果
func (tc *TransformContext) SetModelResult(model vai.Model) {
	tc.modelResult = model
}

// SetError 设置错误信息
func (tc *TransformContext) SetError(err error) {
	tc.err = err
}

// SetIsVisual 设置是否为视觉消息
func (tc *TransformContext) SetIsVisual(isVisual bool) {
	tc.isVisual = isVisual
}

// GetContext 获取上下文
func (tc *TransformContext) GetContext() context.Context {
	return tc.ctx
}

// GetMessage 获取消息
func (tc *TransformContext) GetMessage() *vai.Message {
	return tc.message
}

// GetLanguage 获取语言
func (tc *TransformContext) GetLanguage() string {
	return tc.language
}

// IsVisual 是否为视觉消息
func (tc *TransformContext) IsVisual() bool {
	return tc.isVisual
}

// GetModelResult 获取模型选择结果
func (tc *TransformContext) GetModelResult() vai.Model {
	return tc.modelResult
}

// GetError 获取错误信息
func (tc *TransformContext) GetError() error {
	return tc.err
}
