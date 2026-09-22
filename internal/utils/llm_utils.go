package utils

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/model"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// 定义错误类型
var (
	ErrLLMExecutionFailed = errors.New("LLM execution failed")
	ErrLLMProcessTimeout  = errors.New("LLM process timeout")
	ErrInvalidJSONFormat  = errors.New("invalid JSON format in LLM response")
)

type LLMQueryOptions struct {
	GlobalTimeout     time.Duration
	MessageGapTimeout time.Duration
	ImageContent      string
	Callback          StreamCallback
	ThinkingConfig    *common.ThinkingConfig
}

// ExecuteLLMQuery 执行LLM查询并处理超时控制
func ExecuteLLMQuery(
	ctx context.Context,
	llmHandler common.LlmHandler,
	req *vai.ChatMessageSendRequest,
	history []model.MessageHistory,
) (string, error) {
	output := make(model.Output)
	errChan := make(chan error)
	cancelChan := make(model.CancelCh)
	defer func() {
		select {
		case <-cancelChan:
		default:
			SafeCloseChan(cancelChan)
		}
	}()

	// 启动处理协程
	go func() {
		err := llmHandler.Process(ctx, common.LLMProcessParams{
			Request:  req,
			History:  history,
			Output:   output,
			CancelCh: cancelChan,
		})
		if err != nil {
			errChan <- err
		}
	}()

	// 收集响应
	var response strings.Builder
	done := false

	for !done {
		select {
		case outputs, ok := <-output:
			if !ok {
				done = true
				break
			}
			for _, text := range outputs {
				response.WriteString(text)
			}
		case err := <-errChan:
			zlog.LogWithContext(ctx).Error("LLM Process Error", zap.Error(err))
			return "", ErrLLMExecutionFailed
		case <-time.After(60 * time.Second):
			zlog.LogWithContext(ctx).Error("Wait LLM Response Timeout")
			SafeCloseChan(cancelChan)
			return "", ErrLLMProcessTimeout
		}
	}

	return response.String(), nil
}

// MessageGapMonitor 监控消息间隔超时
// 当两条消息之间的间隔超过指定时间时，执行取消操作
func MessageGapMonitor(
	ctx context.Context,
	input model.Output,
	output model.Output,
	cancelCh model.CancelCh,
	gapTimeout time.Duration,
) chan struct{} {
	done := make(chan struct{})
	timer := time.NewTimer(gapTimeout)

	go func() {
		defer SafeCloseChan(done)
		defer timer.Stop()
		defer SafeCloseChan(output) // 确保在退出时关闭输出通道

		for {
			select {
			case <-ctx.Done():
				return

			case <-timer.C:
				zlog.Logger.Warn("消息间隔超时，取消LLM处理")
				select {
				case cancelCh <- struct{}{}:
					zlog.Logger.Info("成功发送取消信号")
				default:
					zlog.Logger.Warn("取消通道已关闭或阻塞")
				}
				return

			case msg, ok := <-input:
				if !ok {
					return
				}

				// 接收到消息，重置定时器
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(gapTimeout)

				// 将消息传递到输出通道
				select {
				case output <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return done
}

// StreamCallback 定义流式处理回调函数
type StreamCallback func(ctx context.Context, output []string) error

// ExecuteLLMQueryWithGapMonitor 执行LLM查询并处理全局和消息间隔超时控制
func ExecuteLLMQueryWithGapMonitor(
	ctx context.Context,
	llmHandler common.LlmHandler,
	req *vai.ChatMessageSendRequest,
	history []model.MessageHistory,
	options LLMQueryOptions,
) (string, error) {
	input := make(model.Output)
	output := make(model.Output)
	errChan := make(chan error)
	cancelChan := make(model.CancelCh)
	defer func() {
		select {
		case <-cancelChan:
		default:
			SafeCloseChan(cancelChan)
		}
	}()

	// 设置消息间隔超时检测
	messageGapTimeout := options.MessageGapTimeout
	if messageGapTimeout == 0 {
		messageGapTimeout = 20 * time.Second // 默认值
	}
	gapMonitorDone := MessageGapMonitor(ctx, input, output, cancelChan, messageGapTimeout)
	defer func() {
		select {
		case <-gapMonitorDone:
		case <-time.After(time.Second):
			// 确保不会无限等待
			zlog.Logger.Warn("消息间隔监控协程未及时退出")
		}
	}()
	hiddenReasoning := req.GetMessage().GetHiddenReasoning()
	// 启动处理协程
	go func() {
		err := llmHandler.Process(ctx, common.LLMProcessParams{
			Request:         req,
			History:         history,
			Output:          input, // 使用输入通道
			CancelCh:        cancelChan,
			ImageContent:    options.ImageContent, // 传递图片内容
			HiddenReasoning: hiddenReasoning,
			ThinkingConfig:  options.ThinkingConfig,
		})
		if err != nil {
			errChan <- err
		}
	}()

	// 设置全局超时定时器
	globalTimeoutDuration := options.GlobalTimeout
	if globalTimeoutDuration == 0 {
		globalTimeoutDuration = 120 * time.Second // 默认值
	}
	globalTimeout := time.NewTimer(globalTimeoutDuration)
	defer globalTimeout.Stop()

	// 收集响应
	var response strings.Builder
	done := false

	for !done {
		select {
		case <-ctx.Done():
			return response.String(), ctx.Err()

		case outputs, ok := <-output: // 从监控函数的输出通道读取
			if !ok {
				done = true
				break
			}
			for _, text := range outputs {
				response.WriteString(text)
			}
			// 调用回调函数处理流式输出
			if options.Callback != nil {
				if err := options.Callback(ctx, outputs); err != nil {
					zlog.LogWithContext(ctx).Error("流式回调处理错误", zap.Error(err))
					SafeCloseChan(cancelChan)
					return response.String(), err
				}
			}

		case err := <-errChan:
			zlog.LogWithContext(ctx).Error("LLM处理错误", zap.Error(err))
			return "", ErrLLMExecutionFailed

		case <-globalTimeout.C:
			zlog.LogWithContext(ctx).Error("LLM全局处理超时")
			SafeCloseChan(cancelChan)
			return "", ErrLLMProcessTimeout
		}
	}

	return response.String(), nil
}

// ExtractJSONFromLLMResponse 从LLM响应中提取JSON内容
// 返回提取到的JSON字符串和可能的错误
// 如果找不到有效的JSON或JSON格式不正确，将返回错误
func ExtractJSONFromLLMResponse(response string) (string, error) {
	// 查找第一个左花括号位置
	jsonStart := strings.Index(response, "{")
	if jsonStart < 0 {
		return response, ErrInvalidJSONFormat
	}

	// 查找最后一个右花括号位置
	jsonEnd := strings.LastIndex(response, "}")
	if jsonEnd < 0 || jsonEnd <= jsonStart {
		return response, ErrInvalidJSONFormat
	}

	// 提取JSON字符串并清理
	jsonStr := response[jsonStart : jsonEnd+1]
	jsonStr = strings.ReplaceAll(jsonStr, "\n", "")
	jsonStr = strings.TrimSpace(jsonStr)

	// 验证提取的内容是否以 { 开始，以 } 结束
	if !strings.HasPrefix(jsonStr, "{") || !strings.HasSuffix(jsonStr, "}") {
		return response, ErrInvalidJSONFormat
	}

	return jsonStr, nil
}

// RemoveMarkdownCodeBlock 尝试从字符串中移除常见的 Markdown 代码块标记 (```json ... ``` 或 ``` ... ```)
func RemoveMarkdownCodeBlock(text string) string {
	trimmed := strings.TrimSpace(text)
	// 检查是否以 ```json 开头并以 ``` 结尾
	if strings.HasPrefix(trimmed, "```json") && strings.HasSuffix(trimmed, "```") {
		content := strings.TrimPrefix(trimmed, "```json")
		content = strings.TrimSuffix(content, "```")
		return strings.TrimSpace(content)
	}
	// 检查是否以 ``` 开头并以 ``` 结尾
	if strings.HasPrefix(trimmed, "```") && strings.HasSuffix(trimmed, "```") {
		content := strings.TrimPrefix(trimmed, "```")
		content = strings.TrimSuffix(content, "```")
		return strings.TrimSpace(content)
	}
	// 如果没有找到代码块标记，返回原始（清理空格后）的文本
	return trimmed
}

// RemoveTrailingCommaInArray 移除数组中最后一个元素后的逗号
// 用于处理形如 ["1","2",] 这样的非法JSON格式
func RemoveTrailingCommaInArray(text string) string {
	// 使用 strings.Builder 来构建结果字符串
	var result strings.Builder
	result.Grow(len(text)) // 预分配空间

	// 记录当前字符是否在字符串内
	inString := false
	// 记录是否需要跳过当前字符
	skipChar := false

	// 只需遍历一次字符串
	for i := 0; i < len(text); i++ {
		if skipChar {
			skipChar = false
			continue
		}

		c := text[i]

		// 处理字符串引号
		if c == '"' && (i == 0 || text[i-1] != '\\') {
			inString = !inString
		}

		// 只在不在字符串内的情况下处理逗号
		if !inString && c == ',' {
			// 查找下一个非空白字符
			nextNonSpace := -1
			for j := i + 1; j < len(text); j++ {
				if text[j] != ' ' && text[j] != '\n' && text[j] != '\t' && text[j] != '\r' {
					nextNonSpace = j
					break
				}
			}

			// 如果逗号后面的非空白字符是右方括号，跳过这个逗号
			if nextNonSpace != -1 && text[nextNonSpace] == ']' {
				continue
			}
		}

		// 写入当前字符
		result.WriteByte(c)
	}

	return result.String()
}
