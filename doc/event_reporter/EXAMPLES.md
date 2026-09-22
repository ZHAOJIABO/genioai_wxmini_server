# EventReporter 使用示例

## 1. 基础使用

### 异步上报（推荐）

```go
// 用户登录事件
reporter.NewEvent(event_reporter.EventTypeUserLogin).
    UserID("user-123").
    SessionID("session-456").
    Payload(map[string]any{
        "method": "password",
        "ip":     "192.168.1.1",
    }).
    Track()
```

### 同步上报（关键事件）

```go
// 支付成功事件 - 必须确保发送成功
err := reporter.NewEvent(event_reporter.EventTypePaymentSuccess).
    UserID("user-123").
    Payload(map[string]any{
        "order_id": "order-789",
        "amount":   100,
    }).
    TrackNow(ctx)

if err != nil {
    log.Printf("支付事件上报失败: %v", err)
}
```

## 2. 设置上下文

### 从 Context 自动提取

```go
// 假设 ctx 中已设置 trace_id 和 user_id
ctx = event_reporter.WithTraceID(ctx, "trace-abc")
ctx = event_reporter.WithUserID(ctx, "user-123")

reporter.NewEvent("API_CALL").
    FromContext(ctx).  // 自动提取 trace_id, user_id
    Track()
```

### 设置 ActionContext

```go
// 记录 API 调用
start := time.Now()
resp, err := httpClient.Do(req)
duration := time.Since(start)

reporter.NewEvent(event_reporter.EventTypeAPICall).
    UserID(userID).
    Action(
        event_reporter.DomainExternal,
        event_reporter.TypeAPI,
        event_reporter.OpCall,
        "/api/users",
    ).
    ActionSuccess(duration.Milliseconds()).
    ActionMetadata(map[string]any{
        "method":      "POST",
        "status_code": resp.StatusCode,
    }).
    Track()
```

### 使用预定义工厂函数

```go
// API 调用
actionCtx := event_reporter.APICallAction("GET", "/api/users", 200, 150*time.Millisecond)
reporter.NewEvent("API_CALL").
    ActionContext(actionCtx).
    Track()

// 数据库操作
actionCtx := event_reporter.DatabaseAction("SELECT", "users", 10, 5*time.Millisecond)
reporter.NewEvent("DATABASE_OPERATION").
    ActionContext(actionCtx).
    Track()
```

## 3. 设置客户端上下文

```go
clientCtx := event_reporter.NewClientContext().
    DeviceID("device-123").
    OS("ios", "17.0").
    Device("Apple", "iPhone 15").
    App("com.example.app", "2.0.0").
    Network("WiFi").
    Locale("zh_CN", "Asia/Shanghai").
    Build()

reporter.NewEvent("APP_OPEN").
    UserID(userID).
    ClientContext(clientCtx).
    Track()
```

## 4. 设置服务端上下文

```go
// 从 HTTP 请求自动提取
serverCtx := event_reporter.NewServerContext().
    FromHTTPRequest(r).
    ServerNode("node-1").
    ProcessLatency(50).
    Build()

reporter.NewEvent("REQUEST_RECEIVED").
    ServerContext(serverCtx).
    Track()
```

## 5. 使用拦截器

### 添加采样控制

```go
// 只上报 10% 的事件（高频场景）
sampling := event_reporter.NewSamplingInterceptor(0.1)

chain := event_reporter.NewInterceptorChain(
    event_reporter.NewDefaultInterceptor(source, serverNode),
    sampling,
)

// 设置到 reporter（需要类型断言）
if r, ok := reporter.(*event_reporter.Reporter); ok {
    r.SetInterceptor(chain)
}
```

### 添加过滤器

```go
// 过滤掉没有 UserID 的事件
filter := event_reporter.NewFilterInterceptor(func(e *eventsv1.Event) bool {
    return e.GetUserId() != ""
})
```

## 6. 手动控制

### 手动 Flush

```go
// 立即发送队列中的事件
err := reporter.Flush(ctx)
```

### 监控队列

```go
// 获取当前队列长度
queueLen := reporter.QueueLen()
log.Printf("事件队列长度: %d", queueLen)
```

## 7. 常见场景示例

### 任务生命周期

```go
// 任务创建
reporter.NewEvent(event_reporter.EventTypeTaskCreated).
    UserID(userID).
    Payload(map[string]any{"task_id": taskID, "task_type": "image_gen"}).
    Track()

// 任务完成
reporter.NewEvent(event_reporter.EventTypeTaskCompleted).
    UserID(userID).
    Payload(map[string]any{"task_id": taskID, "duration_ms": 5000}).
    Track()

// 任务失败
reporter.NewEvent(event_reporter.EventTypeTaskFailed).
    UserID(userID).
    Payload(map[string]any{"task_id": taskID, "error": err.Error()}).
    Track()
```

### 积分变动

```go
reporter.NewEvent(event_reporter.EventTypeCreditDeduct).
    UserID(userID).
    Payload(map[string]any{
        "amount":  10,
        "reason":  "image_generation",
        "task_id": taskID,
        "balance": newBalance,
    }).
    Track()
```
