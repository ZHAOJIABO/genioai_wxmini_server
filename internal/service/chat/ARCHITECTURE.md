# Chat 服务编排技术方案  

本文档讨论 ChatService 业务编排的两套方案：

---

## 一、方案1：增强型策略模式 + 通用请求接口  

### 1.1 目录结构  
```
internal/service/chat/
├── service.go            # ChatService 核心编排入口
├── context.go            # 通用上下文和请求接口定义
├── llm.go                # LLM 相关实现
├── biz/                  # 业务或功能模块目录
│   ├── handler.go        # BizHandler 接口与 Register() 函数
│   ├── base_handler.go   # 基础处理器实现
│   ├── standard_llm.go   # 标准 LLM 处理流程
│   ├── profile_qa.go     # 示例业务：用户档案查询
│   └── ...               # 更多自定义业务文件
```

### 1.2 核心接口与上下文定义  
```go
// context.go
package chat

// RequestType 区分请求类型
type RequestType int

const (
    RequestTypeStandard RequestType = iota
    RequestTypeProfileQA
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

func (r *StandardRequest) GetRequestType() RequestType { return RequestTypeStandard }
func (r *StandardRequest) GetOriginalRequest() interface{} { return r.Req }

// ChatContext 统一的处理上下文
type ChatContext struct {
    // 基础信息
    UserID         string
    ChatID         string
    Content        string
    MessageType    vai.MessageType
    MessageID      string
    ReplyMessageID string
    PromptID       string
    
    // 多媒体内容
    URLs           []string
    Blobs          []*vai.Blob
    VoiceContent   string
    VoiceHash      string
    FrameURL       string
    
    // 请求/响应处理
    Request        ChatRequest
    StreamServer   interface{}  // 不同类型的stream服务器
    
    // 扩展信息
    Chat           *model.Chat  // 会话模型
    Extra          map[string]interface{}
}
```

```go
// biz/handler.go
package biz

import (
    "context"
    "va_visionai_server/internal/model"
    chat "va_visionai_server/internal/service/chat"
)

// BizHandler 定义业务编排策略
type BizHandler interface {
    // Match 匹配条件
    Match(ctx *chat.ChatContext) bool
    
    // PrepareHistory 准备消息历史，不同请求可能有不同处理
    PrepareHistory(ctx context.Context, chatCtx *chat.ChatContext) ([]model.MessageHistory, error)
    
    // Handle 执行业务处理
    Handle(ctx context.Context, 
           chatCtx *chat.ChatContext, 
           history []model.MessageHistory) (content string, modelID interface{}, err error)
    
    // StreamResponse 处理流式响应（可选实现）
    StreamResponse(ctx context.Context, 
                  chatCtx *chat.ChatContext, 
                  content string) error
}

// 注册表与注册函数
var Handlers []BizHandler

func Register(h BizHandler) {
    Handlers = append(Handlers, h)
}
```

### 1.3 基础处理器设计  
```go
// biz/base_handler.go
package biz

// BaseBizHandler 提供默认实现以简化业务代码
type BaseBizHandler struct{}

// 默认不匹配任何请求
func (h *BaseBizHandler) Match(ctx *chat.ChatContext) bool {
    return false
}

// 默认的历史处理
func (h *BaseBizHandler) PrepareHistory(ctx context.Context, chatCtx *chat.ChatContext) ([]model.MessageHistory, error) {
    // 基于请求类型调用不同实现
    switch chatCtx.Request.GetRequestType() {
    case chat.RequestTypeStandard:
        return prepareStandardHistory(ctx, chatCtx)
    default:
        return nil, errors.New("unsupported request type")
    }
}

// 空实现的流式响应
func (h *BaseBizHandler) StreamResponse(ctx context.Context, chatCtx *chat.ChatContext, content string) error {
    return nil
}

// 辅助函数：处理标准请求的历史
func prepareStandardHistory(ctx context.Context, chatCtx *chat.ChatContext) ([]model.MessageHistory, error) {
    // 原有 prepareMsgHistory 的逻辑
    return nil, nil
}
```

### 1.4 标准 LLM 处理器  
```go
// biz/standard_llm.go
package biz

// StandardLLMHandler 处理常规请求
type StandardLLMHandler struct {
    BaseBizHandler
    service *chat.ChatService
}

func init() {
    // 注册到全局处理器列表
    Register(&StandardLLMHandler{service: globalService})
}

func (h *StandardLLMHandler) Match(ctx *chat.ChatContext) bool {
    // 标准聊天请求且没有特殊的 promptID
    return ctx.Request.GetRequestType() == chat.RequestTypeStandard
}

func (h *StandardLLMHandler) Handle(ctx context.Context, chatCtx *chat.ChatContext, history []model.MessageHistory) (string, interface{}, error) {
    // 实现原有 streamLLM 的逻辑
    // ...省略具体实现
    return "llm response", vai.Model_MODEL_GPT4O, nil
}

func (h *StandardLLMHandler) StreamResponse(ctx context.Context, chatCtx *chat.ChatContext, content string) error {
    // 实现流式响应逻辑
    if chatCtx.StreamServer == nil {
        return nil
    }
    
    streamServer, ok := chatCtx.StreamServer.(vai.VisionAiService_SendChatMessageStreamServer)
    if !ok {
        return errors.New("invalid stream server type")
    }
    
    // 实现 sendEndMessage 的逻辑
    // ...省略具体实现
    return nil
}
```

### 1.5 在 `Process` 中整合新设计  
```go
// service.go
func (s *ChatService) Process(
    ctx context.Context,
    req *vai.ChatMessageSendRequest,
    streamServer vai.VisionAiService_SendChatMessageStreamServer,
) (*vai.Message, error) {
    // 包装为标准请求并交由通用处理流程
    return s.ProcessRequest(ctx, &StandardRequest{Req: req}, streamServer)
}

// ProcessRequest 通用处理入口
func (s *ChatService) ProcessRequest(
    ctx context.Context,
    chatReq ChatRequest,
    streamServer interface{},
) (*vai.Message, error) {
    // 1. 准备上下文
    chatCtx, err := s.prepareContext(ctx, chatReq, streamServer)
    if err != nil {
        return nil, err
    }
    
    // 2. 创建或获取会话
    chat, _, err := s.createOrGetChat(ctx, chatCtx)
    if err != nil {
        return nil, err
    }
    chatCtx.Chat = chat
    
    // 3. 遍历所有处理器
    for _, handler := range biz.Handlers {
        if !handler.Match(chatCtx) {
            continue
        }
        
        // 4. 准备消息历史
        history, err := handler.PrepareHistory(ctx, chatCtx)
        if err != nil {
            return nil, err
        }
        
        // 5. 执行业务处理
        content, modelID, err := handler.Handle(ctx, chatCtx, history)
        if err != nil {
            return nil, err
        }
        
        // 6. 流式响应处理
        if err := handler.StreamResponse(ctx, chatCtx, content); err != nil {
            // 记录错误但继续执行
            zlog.LogWithContext(ctx).Error("Stream response failed", zap.Error(err))
        }
        
        // 7. 归档与异步任务
        return s.finalizeResponse(ctx, chatCtx, content, modelID, history)
    }
    
    // 未找到匹配处理器
    return nil, errors.New("no suitable handler found")
}
```

### 1.6 业务实现示例  
```go
// biz/profile_qa.go
package biz

// ProfileQAHandler 处理用户档案问答
type ProfileQAHandler struct {
    BaseBizHandler
    profileService *service.ProfileService
}

func init() {
    Register(&ProfileQAHandler{profileService: globalProfileService})
}

func (h *ProfileQAHandler) Match(ctx *chat.ChatContext) bool {
    // 基于 PromptID 或请求类型判断
    return ctx.PromptID == "user_profile_qa" || 
           ctx.Request.GetRequestType() == chat.RequestTypeProfileQA
}

func (h *ProfileQAHandler) Handle(ctx context.Context, chatCtx *chat.ChatContext, history []model.MessageHistory) (string, interface{}, error) {
    // 1. 获取用户档案
    profile, err := h.profileService.GetUserProfile(chatCtx.UserID)
    if err != nil {
        return "", nil, err
    }
    
    // 2. 构建增强提示词
    prompt := fmt.Sprintf("用户档案信息：\n%s\n\n请回答用户问题：%s", 
                          profile.Summary, chatCtx.Content)
    
    // 3. 添加到历史
    enhancedHistory := append(history, model.MessageHistory{
        Content: prompt,
        Sender: "user",
    })
    
    // 4. 调用 LLM
    llmService := globalLLMService
    reply, err := llmService.Process(ctx, enhancedHistory, vai.Model_MODEL_GPT4O_MINI)
    if err != nil {
        return "", nil, err
    }
    
    return reply, vai.Model_MODEL_GPT4O_MINI, nil
}
```

### 1.7 优缺点  
- 优点：
  - 完全解耦请求类型与处理逻辑
  - 支持多样化的请求入口和处理流程
  - 统一的上下文管理，支持扩展信息
  - 兼容原有流程，无缝迁移
  - 保持了策略模式的简洁性
- 缺点：
  - 初始设置较为复杂
  - 需要更多的接口和辅助函数
  - Handler 实现者需了解更多细节

---

## 二、方案2：显式注册 + 优先级 + 中间件  

### 2.1 改进要点  
1. **显式注册**：在 `main` 或 `wire` 层统一调用 `RegisterHandlers()`，可见所有业务模块
2. **可控优先级**：`BizHandler` 增加 `Priority() int` 方法，注册时按优先级排序
3. **高效匹配**：基于 `map[promptID]BizHandler` 做一次性定位或分组索引
4. **中间件机制**：在 Handler 调用链上套用日志、限流、监控等切面逻辑
5. **上下文扩展**：`ChatRequestContext` 增加 `Values map[string]interface{}`，支持业务自定义数据传递

### 2.2 样例接口  
```go
// handler.go
package biz

type BizHandler interface {
    Priority() int                          // 优先级，值越小越优先
    Match(ctx *ChatRequestContext) bool
    Handle(ctx context.Context, reqCtx *ChatRequestContext, history []model.MessageHistory) (string, error)
}

func RegisterHandler(h BizHandler)
```

### 2.3 中间件示例  
```go
type BizHandleFunc func(context.Context, *ChatRequestContext, []model.MessageHistory) (string, error)
type BizMiddleware func(BizHandleFunc) BizHandleFunc

// 日志中间件
func LoggingMiddleware(next BizHandleFunc) BizHandleFunc {
  return func(ctx context.Context, reqCtx *ChatRequestContext, history []model.MessageHistory) (string, error) {
    start := time.Now()
    reply, err := next(ctx, reqCtx, history)
    zlog.LogWithContext(ctx).Info("BizHandler 完成",
        zap.String("biz", reqCtx.PromptID),
        zap.Duration("took", time.Since(start)))
    return reply, err
  }
}
```

### 2.4 何时过渡  
- 当自定义业务数量超过 5~10 个；
- 需要统一切面（日志/监控/限流/熔断）时；
- 有冲突或顺序控制需求时。

---

## 三、推荐实践与落地
1. **先使用方案1**：快速搭建，入门简单；
2. **保留过渡点**：在 `biz/handler.go` 中留下注释或 TODO，标记未来插入优先级或中间件链的位置；
3. **文档驱动**：本文件即为设计蓝图，开发时可及时更新；
4. **单元测试**：为每个 `BizHandler` 写简单的匹配与执行测试；
5. **持续演进**：业务增多时，逐步演化到方案2，不破坏现有注册逻辑。


> **总结**：该思路可行且易于实现。方案1 满足现阶段需求，方案2 则为未来大规模业务演进提供保障。 

---

## 四、附录：核心实现参考

为确保方案1的实现不影响现有业务逻辑，这里提供完整的核心函数实现参考。

### 4.1 上下文准备与会话管理

```go
// 完整实现准备上下文函数
func (s *ChatService) prepareContext(ctx context.Context, req ChatRequest, streamServer interface{}) (*ChatContext, error) {
    switch req.GetRequestType() {
    case RequestTypeStandard:
        stdReq := req.GetOriginalRequest().(*vai.ChatMessageSendRequest)
        reqHeader := stdReq.GetRequestHeader()
        reqMsg := stdReq.GetMessage()
        
        userID := reqHeader.GetUserId()
        chatID := reqMsg.GetChatId()
        content := reqMsg.GetContent()
        messageType := reqMsg.GetMessageType()
        messageID := reqMsg.GetMessageId()
        promptID := reqMsg.GetPromptId()
        replyMessageID := generateMsgID()
        
        // 处理URL
        urls := reqMsg.GetUrls()
        blobs := reqMsg.GetBlobs()
        if reqMsg.GetUrl() != "" {
            urls = append(urls, reqMsg.GetUrl())
        }
        
        // 处理视频帧
        var frameURL string
        if messageType == vai.MessageType_MT_VIDEO && len(blobs) > 0 {
            frameURL = s.processVideoFrame(ctx, reqHeader, messageID, blobs[0].GetData())
        }
        
        // 处理语音
        var voiceContent string
        var voiceHash string
        if reqMsg.GetVoiceInfo() != nil {
            voiceHash = reqMsg.GetVoiceInfo().GetMd5()
            if voiceHash != "" {
                voiceContent = s.processVoice(ctx, voiceHash)
            }
        }
        
        // 确保消息ID不为空
        if strings.TrimSpace(messageID) == "" {
            messageID = generateMsgID()
        }
        
        return &ChatContext{
            UserID:         userID,
            ChatID:         chatID,
            Content:        content,
            MessageType:    messageType,
            MessageID:      messageID,
            ReplyMessageID: replyMessageID,
            PromptID:       promptID,
            URLs:           urls,
            Blobs:          blobs,
            VoiceContent:   voiceContent,
            VoiceHash:      voiceHash,
            FrameURL:       frameURL,
            Request:        req,
            StreamServer:   streamServer,
            Extra:          make(map[string]interface{}),
        }, nil
    default:
        return nil, errors.New("unsupported request type")
    }
}

// 创建或获取会话
func (s *ChatService) createOrGetChat(ctx context.Context, chatCtx *ChatContext) (*model.Chat, bool, error) {
    chatStartTime := time.Now()
    cDao := dao.NewChatDao()
    chat := &model.Chat{
        ChatID:     chatCtx.ChatID,
        Title:      constants.NewChatTitle,
        UserID:     chatCtx.UserID,
        Status:     0,
        CreateTime: time.Now().Unix(),
        PromptID:   chatCtx.PromptID,
        IsVideo:    chatCtx.MessageType == vai.MessageType_MT_VIDEO,
    }

    newRow, err := cDao.FirstOrCreate(chat)
    if err != nil {
        zlog.LogWithContext(ctx).Error("Failed to create or get chat",
            zap.String(constants.CtxChatID, chatCtx.ChatID),
            zap.Error(err))
        return nil, false, err
    }

    zlog.LogWithContext(ctx).Info("Chat session prepared",
        zap.Duration("duration", time.Since(chatStartTime)),
        zap.Bool("isNewChat", newRow))

    return chat, newRow, nil
}
```

### 4.2 消息历史处理

```go
// 基础处理器的历史消息准备实现
func prepareStandardHistory(ctx context.Context, chatCtx *chat.ChatContext) ([]model.MessageHistory, error) {
    var msgHistory []model.MessageHistory
    var err error
    
    msgService := globalMsgService
    
    // 仅当没有URLs时才获取历史
    if len(chatCtx.URLs) == 0 {
        msgHistory, err = msgService.GetMsgHistory(ctx, chatCtx.ChatID)
        if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, err
        }
    }
    
    // 构建当前用户消息
    userMsg := model.MessageHistory{
        Content:  chatCtx.Content,
        URLs:     chatCtx.URLs,
        Sender:   "user",
        FileType: chatCtx.MessageType,
    }
    
    if chatCtx.VoiceContent != "" {
        userMsg.Content = chatCtx.VoiceContent
    }
    
    if chatCtx.MessageType == vai.MessageType_MT_VIDEO && len(chatCtx.Blobs) > 0 {
        userMsg.VideoBlob = chatCtx.Blobs[0].GetData()
    }
    
    return append(msgHistory, userMsg), nil
}
```

### 4.3 标准 LLM 处理器实现

```go
// Handle 实现
func (h *StandardLLMHandler) Handle(ctx context.Context, chatCtx *chat.ChatContext, history []model.MessageHistory) (string, interface{}, error) {
    llmStartTime := time.Now()
    
    req := chatCtx.Request.GetOriginalRequest().(*vai.ChatMessageSendRequest)
    streamServer, _ := chatCtx.StreamServer.(vai.VisionAiService_SendChatMessageStreamServer)
    
    // 创建回调函数
    callback := func(ctx context.Context, token string) error {
        if streamServer == nil {
            return nil
        }
        
        res := &vai.ChatMessageStreamResponse{
            MessageToken:   token,
            ReqMessageId:   req.GetMessage().GetMessageId(),
            ReplyMessageId: chatCtx.ReplyMessageID,
            ResponseHeader: &vai.ResponseHeader{
                Code:  0,
                Msg:   "success",
                ReqId: req.GetRequestHeader().GetReqId(),
            },
            IsEnd: false,
        }
        
        if err := streamServer.Context().Err(); err != nil {
            zlog.LogWithContext(ctx).Error("Stream Context Done", zap.Error(err))
            return chat.ErrRemoteClose
        }
        
        if err := streamServer.Send(res); err != nil {
            zlog.LogWithContext(ctx).Error("StreamServer send failed", zap.Error(err))
            return chat.ErrRemoteClose
        }
        
        return nil
    }
    
    // 解析模型和处理图片
    finalModelID, imageContent, _ := h.service.llmService.ResolveModel(ctx, req, history)
    
    // 调用 LLM 服务
    replyContent, err := h.service.llmService.StreamProcess(
        ctx,
        req,
        history,
        callback,
        chat.LLMOptions{
            InitialModel:  finalModelID,
            GlobalTimeout: 120 * time.Second,
            GapTimeout:    10 * time.Second,
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

// StreamResponse 实现
func (h *StandardLLMHandler) StreamResponse(ctx context.Context, chatCtx *chat.ChatContext, content string) error {
    streamServer, ok := chatCtx.StreamServer.(vai.VisionAiService_SendChatMessageStreamServer)
    if !ok || streamServer == nil {
        return nil
    }
    
    req := chatCtx.Request.GetOriginalRequest().(*vai.ChatMessageSendRequest)
    reqHeader := req.GetRequestHeader()
    
    // 发送结束消息
    err := streamServer.Send(&vai.ChatMessageStreamResponse{
        ReqMessageId:   req.GetMessage().GetMessageId(),
        ReplyMessageId: chatCtx.ReplyMessageID,
        MessageToken:   "" + appendLowVersionWarningIfMac(reqHeader),
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
```

### 4.4 响应处理与归档

```go
func (s *ChatService) finalizeResponse(ctx context.Context, chatCtx *ChatContext, content string, modelID interface{}, history []model.MessageHistory) (*vai.Message, error) {
    // 归档消息
    finalModelID, ok := modelID.(vai.Model)
    if !ok {
        finalModelID = vai.Model_MODEL_GPT4O // 默认模型
    }
    
    // 处理帧URL
    urls := chatCtx.URLs
    if chatCtx.FrameURL != "" {
        urls = append(urls, chatCtx.FrameURL)
    }
    
    // 归档用户消息
    stdReq := chatCtx.Request.GetOriginalRequest().(*vai.ChatMessageSendRequest)
    _, err := s.msgService.ArchiveChatMessage(
        chatCtx.ChatID,
        chatCtx.MessageID,
        chatCtx.UserID,
        chatCtx.Chat.CreateTime,
        chatCtx.Content,
        urls,
        chatCtx.VoiceHash,
        vai.MessageSender_USER,
        chatCtx.MessageType,
        finalModelID.String(),
    )
    if err != nil {
        return nil, err
    }
    
    // 归档系统回复
    replyMsgID, err := s.msgService.ArchiveChatMessage(
        chatCtx.ChatID,
        chatCtx.ReplyMessageID,
        chatCtx.UserID,
        chatCtx.Chat.CreateTime,
        content,
        []string{},
        "",
        vai.MessageSender_ASSISTANT,
        vai.MessageType_MT_TEXT,
        finalModelID.String(),
    )
    if err != nil {
        return nil, err
    }
    
    // 构建回复消息
    responseMsg := &vai.Message{
        ChatId:      chatCtx.ChatID,
        MessageId:   replyMsgID,
        Content:     content,
        CreateTime:  time.Now().Format(common.TimestampFormat),
        MessageType: vai.MessageType_MT_TEXT,
        Sender:      vai.MessageSender_ASSISTANT,
    }
    
    // 启动异步任务
    now := time.Now()
    go task.Chat.Submit(chatCtx.ChatID, &now)
    
    // 启动标题更新任务（非星座应用）
    if stdReq != nil && stdReq.GetRequestHeader().GetApp().GetAppName() != "com.bluex.astrox" {
        titleCtx := utils.CloneContext(ctx)
        go s.updateChatTitle(titleCtx, stdReq, chatCtx.ChatID, chatCtx.Chat.Title,
           append(history, model.MessageHistory{Content: content, Sender: "assistant"}))
    }
    
    return responseMsg, nil
}
```

### 4.5 全局服务引用设计

为了避免循环依赖问题，使用全局变量模式管理服务实例：

```go
// biz/global.go
package biz

import (
    "va_visionai_server/internal/service"
    "va_visionai_server/internal/service/chat"
)

// 全局服务实例
var (
    globalChatService    *chat.ChatService
    globalLLMService     *chat.ChatLLMService
    globalMsgService     *service.MessageService
    globalProfileService *service.ProfileService
)

// InitGlobalServices 在应用启动时初始化全局服务引用
func InitGlobalServices(
    chatSvc *chat.ChatService,
    llmSvc *chat.ChatLLMService,
    msgSvc *service.MessageService,
    profileSvc *service.ProfileService,
) {
    globalChatService = chatSvc
    globalLLMService = llmSvc
    globalMsgService = msgSvc
    globalProfileService = profileSvc
}
```

### 4.6 实现注意事项

1. **兼容性保证**：
   - 保持所有现有功能完整，包括消息归档、异步任务等
   - 确保特殊处理（如Mac版本警告）正常工作
   - 保持相同的日志级别和内容

2. **错误处理**：
   - 保持原有错误类型和处理逻辑，特别是 `ErrRemoteClose`
   - 在流式处理中正确传递错误状态

3. **性能考虑**：
   - 避免不必要的类型转换和内存分配
   - 合理使用 goroutine 处理异步任务
   - 保持原有的超时控制机制

4. **服务注入**：
   - 在应用初始化时设置全局服务引用
   - 或通过 Wire 框架进行依赖注入

5. **分步实施建议**：
   - 先实现核心框架和接口
   - 将原有 Process 改为调用 ProcessRequest
   - 实现标准 LLM 处理器
   - 逐步添加新的业务处理器 