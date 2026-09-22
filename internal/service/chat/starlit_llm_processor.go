package chat

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/zlog"
)

// callbackProcessorWithStarlit 专门处理分片 token 的业务逻辑。
type callbackProcessorWithStarlit struct {
	suffix             string
	processed          map[string]bool
	stop               bool
	followQuestionText string
	questionChan       chan string // 使用channel存储解析出的问题
	questionCount      int         // 问题计数器
	finished           bool        // 标记是否已完成处理
	mutex              sync.Mutex  // 用于保护共享变量
}

const (
	questionMarker = "*q*"
	questionDelay  = 800 * time.Millisecond // 问题间隔时间
	questionBuffer = 100                    // 问题缓冲区大小
)

// newCallbackProcessorWithStarlit 初始化一个 callbackProcessorWithStarlit
func newCallbackProcessorWithStarlit() *callbackProcessorWithStarlit {
	return &callbackProcessorWithStarlit{
		suffix: "",
		processed: map[string]bool{
			questionMarker: false,
		},
		stop:               false,
		followQuestionText: "",
		questionChan:       make(chan string, questionBuffer), // 创建带缓冲的channel
		questionCount:      0,
		finished:           false,
	}
}

// GetQuestionChannel 返回问题channel，用于接收解析出的问题
func (p *callbackProcessorWithStarlit) GetQuestionChannel() <-chan string {
	return p.questionChan
}

// Close 关闭问题channel
func (p *callbackProcessorWithStarlit) Close() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if !p.finished {
		p.finished = true
		close(p.questionChan)
	}
}

// Finish 标记处理完成并确保所有问题都被处理
func (p *callbackProcessorWithStarlit) Finish(ctx context.Context) {
	// 最后一次尝试解析所有问题
	p.extractAllCompleteQuestions(ctx)

	// 标记完成并关闭channel
	p.Close()
}

func (p *callbackProcessorWithStarlit) process(ctx context.Context, token string) (toSend string, stopNow bool) {
	//zlog.LogWithContext(ctx).Debug("[(p *callbackProcessorWithStarlit) process] Token", zap.String("token", token))
	// 累积字符
	p.followQuestionText += token

	// 持续解析所有完整的问题
	p.extractAllCompleteQuestions(ctx)

	// 这里不再直接返回问题，而是通过channel异步发送
	return "", false
}

// extractAllCompleteQuestions 提取所有完整的问题并发送到channel
func (p *callbackProcessorWithStarlit) extractAllCompleteQuestions(ctx context.Context) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.finished {
		return
	}

	for {
		// 查找第一个question标记的位置
		firstMarkerPos := strings.Index(p.followQuestionText, questionMarker)
		if firstMarkerPos == -1 {
			break
		}

		// 从标记后开始查找问题内容
		afterMarker := p.followQuestionText[firstMarkerPos+len(questionMarker):]

		// 查找格式 "*q*": "问题内容" 中的问题内容
		// 首先跳过 "*q*": 部分，找到问题内容的开始引号
		colonPos := strings.Index(afterMarker, `:`)
		if colonPos == -1 {
			break
		}

		// 找到问题内容的开始引号
		startQuote := strings.Index(afterMarker[colonPos:], `"`)
		if startQuote == -1 {
			break
		}
		startQuote = colonPos + startQuote + 1 // 调整为相对于afterMarker的位置，+1跳过引号本身

		// 找到问题内容的结束引号
		endQuote := strings.Index(afterMarker[startQuote:], `"`)
		if endQuote == -1 {
			// 问题不完整，等待更多token
			break
		}

		// 提取问题内容
		question := afterMarker[startQuote : startQuote+endQuote]
		question = strings.TrimSpace(question)

		if question != "" {
			// 发送问题到channel
			p.questionCount++
			p.questionChan <- question

			zlog.LogWithContext(ctx).Debug("Extracted and sent question to channel",
				zap.String("question", question),
				zap.Int("question_count", p.questionCount))
		}

		// 移除已处理的部分，继续查找下一个问题
		p.followQuestionText = afterMarker[startQuote+endQuote+1:]
	}
}
