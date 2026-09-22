package chat

import (
	"errors"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// ModelResolver 模型解析器接口，责任链模式的处理器
type ModelResolver interface {
	// Resolve 解析模型，返回解析后的模型和是否继续链式处理
	Resolve(ctx *TransformContext) (bool, error)
	// SetNext 设置下一个处理器
	SetNext(resolver ModelResolver)
}

// BaseResolver 基础解析器，提供责任链的基本框架
type BaseResolver struct {
	nextResolver ModelResolver
}

// SetNext 设置下一个处理器
func (r *BaseResolver) SetNext(resolver ModelResolver) {
	r.nextResolver = resolver
}

// ResolveNext 调用下一个处理器
func (r *BaseResolver) ResolveNext(ctx *TransformContext) error {
	if r.nextResolver != nil {
		continueChain, err := r.nextResolver.Resolve(ctx)
		if err != nil {
			return err
		}
		// 如果不继续链式处理，则返回
		if !continueChain {
			return nil
		}
	}
	return nil
}

// DefaultModelResolver 默认模型解析器，根据语言设置默认模型
type DefaultModelResolver struct {
	BaseResolver
	modelSelector *ModelSelector
}

// NewDefaultModelResolver 创建默认模型解析器
func NewDefaultModelResolver(modelSelector *ModelSelector) *DefaultModelResolver {
	return &DefaultModelResolver{
		modelSelector: modelSelector,
	}
}

// Resolve 根据语言和是否为视觉消息选择默认模型
func (r *DefaultModelResolver) Resolve(ctx *TransformContext) (bool, error) {
	message := ctx.GetMessage()
	// 如果已经指定了模型，则跳过默认模型解析
	if message.GetModelName() != "" || message.GetModelId() != 0 {
		return true, r.ResolveNext(ctx)
	}

	// 使用ModelSelector选择默认模型
	model, err := r.modelSelector.SelectModel(ctx.GetContext(), ctx.GetLanguage(), ctx.IsVisual())
	if err != nil {
		// 选型失败时静默降级到Gemini Flash
		model = vai.Model_MODEL_GEMINI_2_0_FLASH
		zlog.LogWithContext(ctx.GetContext()).Warn("默认模型选取失败，回退到 Gemini Flash",
			zap.Error(err),
			zap.String("fallback_model", model.String()))
	}

	// 设置模型结果
	ctx.SetModelResult(model)
	message.ModelId = model
	message.ModelName = model.String()

	zlog.LogWithContext(ctx.GetContext()).Info("最终选择模型",
		zap.String(constants.CtxModelName, model.String()),
		zap.Bool("isVisual", ctx.IsVisual()),
		zap.Bool("is_fallback", err != nil))

	return true, r.ResolveNext(ctx)
}

// ModelNameResolver 模型名称解析器，通过模型名称解析模型ID
type ModelNameResolver struct {
	BaseResolver
}

// NewModelNameResolver 创建模型名称解析器
func NewModelNameResolver() *ModelNameResolver {
	return &ModelNameResolver{}
}

// Resolve 通过模型名称解析模型ID
func (r *ModelNameResolver) Resolve(ctx *TransformContext) (bool, error) {
	message := ctx.GetMessage()
	modelName := message.GetModelName()

	// 如果没有指定模型名称，则跳过
	if modelName == "" {
		return true, r.ResolveNext(ctx)
	}

	// 根据模型名称解析模型ID
	if val, ok := vai.Model_value[modelName]; ok {
		modelID := vai.Model(val)
		message.ModelId = modelID
		ctx.SetModelResult(modelID)

		zlog.LogWithContext(ctx.GetContext()).Info("通过模型名称解析模型ID",
			zap.String("modelName", modelName),
			zap.Int32("modelID", int32(modelID)))
	} else {
		zlog.LogWithContext(ctx.GetContext()).Warn("无效的模型名称",
			zap.String("modelName", modelName))
	}

	return false, nil
}

// VideoModelResolver 视频模型解析器，处理视频消息
type VideoModelResolver struct {
	BaseResolver
}

// NewVideoModelResolver 创建视频模型解析器
func NewVideoModelResolver() *VideoModelResolver {
	return &VideoModelResolver{}
}

// Resolve 处理视频消息，强制使用Gemini模型
func (r *VideoModelResolver) Resolve(ctx *TransformContext) (bool, error) {
	message := ctx.GetMessage()

	// 如果是视频消息，强制使用Gemini模型
	if message.GetMessageType() == vai.MessageType_MT_VIDEO {
		geminiModel := vai.Model_MODEL_GEMINI_2_0_FLASH
		message.ModelId = geminiModel
		message.ModelName = geminiModel.String()
		ctx.SetModelResult(geminiModel)

		zlog.LogWithContext(ctx.GetContext()).Info("视频消息，强制使用Gemini模型",
			zap.String(constants.CtxModelName, geminiModel.String()))

		// 视频消息处理完成，不需要继续责任链
		return false, nil
	}

	return true, r.ResolveNext(ctx)
}

// ModelResolverChain 模型解析器链，管理所有解析器
type ModelResolverChain struct {
	head ModelResolver
}

// NewModelResolverChain 创建模型解析器链
func NewModelResolverChain(resolvers ...ModelResolver) *ModelResolverChain {
	if len(resolvers) == 0 {
		return &ModelResolverChain{}
	}

	// 建立责任链
	head := resolvers[0]
	current := head

	for i := 1; i < len(resolvers); i++ {
		current.SetNext(resolvers[i])
		current = resolvers[i]
	}

	return &ModelResolverChain{
		head: head,
	}
}

// ResolveModel 执行模型解析
func (c *ModelResolverChain) ResolveModel(ctx *TransformContext) error {
	if c.head == nil {
		return errors.New("模型解析链为空")
	}

	_, err := c.head.Resolve(ctx)

	// 即使有错误，也要确保至少有一个模型被选择
	if err != nil && ctx.GetModelResult() == 0 {
		// 责任链处理失败且没有设置任何模型，则设置默认模型
		defaultModel := vai.Model_MODEL_GEMINI_2_0_FLASH
		ctx.SetModelResult(defaultModel)
		zlog.LogWithContext(ctx.GetContext()).Warn("责任链处理失败且无模型结果，设置兜底模型",
			zap.Error(err),
			zap.String("fallback_model", defaultModel.String()))
	}

	return err
}
