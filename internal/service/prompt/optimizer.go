package prompt

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/service"
	aigcclient "va_visionai_server/internal/service/ai_clients/aigc_core"
	"va_visionai_server/internal/service/event_reporter"
	"va_visionai_server/internal/zlog"
	v1 "va_visionai_server/internal/aibrain/v1"
)

// OptimizerService 独立的 Prompt 优化服务
type OptimizerService struct {
	aigcClient    *aigcclient.Client
	configService *service.ConfigService
	eventReporter event_reporter.EventReporter
}

func NewOptimizerService(aigcClient *aigcclient.Client, configService *service.ConfigService, eventReporter event_reporter.EventReporter) *OptimizerService {
	return &OptimizerService{
		aigcClient:    aigcClient,
		configService: configService,
		eventReporter: eventReporter,
	}
}

// EnhancePrompt 基于全局配置和可选图片，对用户的原始 prompt 进行优化。
// 失败时返回原始 prompt，并在服务端打印日志。
func (s *OptimizerService) EnhancePrompt(ctx context.Context, originalPrompt string, imageURLs []string, toolType string) ([]string, error) {
	log := zlog.LogWithContext(ctx).With(zap.String("user_id", common.GetUserID(ctx)))
	original := []string{originalPrompt}
	if s.aigcClient == nil {
		log.Warn("OptimizerService: aigc client not available, return original prompt")
		return original, io.EOF
	}

	// 从独立的配置key读取模型和系统prompt
	var model, systemPrompt string
	if s.configService != nil {
		model, _ = s.configService.GetStringConfig(constants.ConfigKeyPromptOptimizationDefaultModel)
		systemPrompt, _ = s.configService.GetStringConfig(constants.ConfigKeyPromptOptimizationDefaultPrompt)
	}
	model = strings.TrimSpace(model)
	if model == "" {
		log.Warn("OptimizerService: model not configured, return original prompt")
		return original, io.EOF
	}

	// 组装 system 消息（可选）
	messages := make([]*v1.ChatMessage, 0, 2)
	if sys := strings.TrimSpace(systemPrompt); sys != "" {
		messages = append(messages, &v1.ChatMessage{
			Role: v1.ChatRole_CHAT_ROLE_SYSTEM,
			Parts: []*v1.ContentPart{
				{Data: &v1.ContentPart_Text{Text: sys}},
			},
			Timestamp: time.Now().Unix(),
		})
	}

	// 组装 user 消息：JSON 文本 + 多张图片
	userParts := make([]*v1.ContentPart, 0, len(imageURLs)+1)
	hasAnyContent := strings.TrimSpace(originalPrompt) != "" || len(imageURLs) > 0 || strings.TrimSpace(toolType) != ""
	if hasAnyContent {
		// 以结构化 JSON 形式提供上下文
		type polishInput struct {
			ToolType   string `json:"tool_type"`
			UserPrompt string `json:"user_prompt"`
		}
		payload := polishInput{
			ToolType:   strings.TrimSpace(toolType),
			UserPrompt: strings.TrimSpace(originalPrompt),
		}
		if b, err := json.Marshal(payload); err == nil {
			userParts = append(userParts, &v1.ContentPart{Data: &v1.ContentPart_Text{Text: string(b)}})
		} else {
			// JSON 组装失败则退化为原文本
			if txt := strings.TrimSpace(originalPrompt); txt != "" {
				userParts = append(userParts, &v1.ContentPart{Data: &v1.ContentPart_Text{Text: txt}})
			}
		}
	}
	for _, u := range imageURLs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		userParts = append(userParts, &v1.ContentPart{Data: &v1.ContentPart_ImageUrl{ImageUrl: u}})
	}
	if len(userParts) == 0 {
		log.Warn("OptimizerService: empty user content, return original prompt")
		return original, io.EOF
	}
	messages = append(messages, &v1.ChatMessage{
		Role:      v1.ChatRole_CHAT_ROLE_USER,
		Parts:     userParts,
		Timestamp: time.Now().Unix(),
	})

	req := &v1.ChatStreamRequest{
		ModelName: model,
		Messages:  messages,
		Parameters: &v1.ChatParameters{
			Temperature: 0.7,
			MaxTokens:   4096,
		},
	}

	stream, err := s.aigcClient.ChatStream(ctx, req)
	if err != nil {
		log.Warn("OptimizerService: create chat stream failed, return original", zap.Error(err))
		return original, err
	}

	var buf strings.Builder
	for {
		resp, rerr := stream.Recv()
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			log.Warn("OptimizerService: receive stream chunk failed, return original", zap.Error(rerr))
			return original, rerr
		}
		buf.WriteString(resp.GetContentChunk())
		if resp.GetIsFinish() {
			break
		}
	}

	final := s.postProcessLLMOutput(buf.String())
	if len(final) == 0 {
		log.Warn("OptimizerService: empty optimized result, return original")
		return original, io.EOF
	}
	return final, nil
}

// postProcessLLMOutput 处理 LLM 输出，将字符串数组格式的字符串转换为真正的数组
// 支持以下格式：
// 1. JSON 数组格式：["prompt1", "prompt2"] 或带代码块标记的 ```json\n[...]\n```
// 2. 纯文本格式：单个字符串（兜底逻辑）
//
// 函数会自动：
// - 过滤空字符串
// - 清理前后空白
// - 处理 markdown 代码块包裹的 JSON
func (s *OptimizerService) postProcessLLMOutput(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}

	// 尝试提取 markdown 代码块中的 JSON（LLM 可能返回 ```json [...] ```）
	cleanedRaw := s.extractJSONFromMarkdown(raw)

	// 尝试解析为 JSON 数组
	var prompts []string
	if err := json.Unmarshal([]byte(cleanedRaw), &prompts); err == nil {
		// 过滤空字符串并返回
		result := make([]string, 0, len(prompts))
		for _, p := range prompts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		// 如果解析成功但结果为空，使用兜底逻辑
		if len(result) > 0 {
			return result
		}
	}

	// 兜底：作为单个字符串返回
	return []string{raw}
}

// extractJSONFromMarkdown 从 markdown 代码块中提取 JSON 内容
// 支持格式：```json\n[...]\n``` 或 ```\n[...]\n```
func (s *OptimizerService) extractJSONFromMarkdown(raw string) string {
	// 检查是否包含 markdown 代码块标记
	if !strings.Contains(raw, "```") {
		return raw
	}

	// 尝试提取代码块内容
	// 匹配 ```json 或 ``` 开头的代码块
	lines := strings.Split(raw, "\n")
	var inCodeBlock bool
	var jsonLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 检测代码块开始
		if !inCodeBlock && (strings.HasPrefix(trimmed, "```json") || strings.HasPrefix(trimmed, "```")) {
			inCodeBlock = true
			continue
		}

		// 检测代码块结束
		if inCodeBlock && strings.HasPrefix(trimmed, "```") {
			break
		}

		// 收集代码块内的内容
		if inCodeBlock {
			jsonLines = append(jsonLines, line)
		}
	}

	// 如果找到代码块内容，返回提取的内容
	if len(jsonLines) > 0 {
		extracted := strings.TrimSpace(strings.Join(jsonLines, "\n"))
		if extracted != "" {
			return extracted
		}
	}

	// 否则返回原始内容
	return raw
}

// PromptEnhanceEventParams 事件上报参数
type PromptEnhanceEventParams struct {
	UserID          string
	OriginalPrompt  string
	EnhancedPrompts []string
	ToolID          string
	ToolType        string
	ImageCount      int
	Enhanced        bool
}

// TrackPromptEnhanceEvent 上报 Prompt 增强事件
func (s *OptimizerService) TrackPromptEnhanceEvent(ctx context.Context, params PromptEnhanceEventParams) {
	if s.eventReporter == nil {
		return
	}

	originalPrompt := params.OriginalPrompt
	enhancedPrompts := params.EnhancedPrompts
	if len(originalPrompt) > 100 {
		originalPrompt = originalPrompt[:100]
	}
	enhancedPromptsStr := strings.Join(enhancedPrompts, ",")
	if len(enhancedPromptsStr) > 100 {
		enhancedPromptsStr = enhancedPromptsStr[:100]
	}
	s.eventReporter.NewEvent(event_reporter.EventTypePromptEnhance).
		UserID(params.UserID).
		FromContext(ctx).
		Payload(map[string]any{
			"original_prompt":  originalPrompt,
			"enhanced_prompts": enhancedPromptsStr,
			"tool_id":          params.ToolID,
			"tool_type":        params.ToolType,
			"image_count":      params.ImageCount,
			"enhanced":         params.Enhanced,
		}).
		Track()
}
