# EventReporter 异步事件上报客户端

## 概述

EventReporter 是一个高性能的异步事件上报客户端，用于将业务事件发送到 `event_sink` 服务进行存储和分析。

**核心特性**：
- 异步批量上报，不阻塞业务逻辑
- 同步实时上报，用于关键事件
- 链式 Builder API，简洁易用
- 拦截器机制，统一处理公共字段
- 优雅关闭，确保事件不丢失

## 目录

- [技术设计](./DESIGN.md) - 架构设计和实现细节
- [使用示例](./EXAMPLES.md) - 代码使用示例

## 快速开始

```go
// 1. 从 ServiceProvider 获取 EventReporter
reporter := serviceProvider.EventReporter

// 2. 异步上报事件（推荐）
reporter.NewEvent(event_reporter.EventTypeUserLogin).
    UserID("user-123").
    Payload(map[string]any{"method": "password"}).
    Track()

// 3. 同步上报关键事件
err := reporter.NewEvent("PAYMENT_SUCCESS").
    UserID("user-123").
    Payload(map[string]any{"amount": 100}).
    TrackNow(ctx)
```

## 文件结构

```
internal/service/event_reporter/
├── reporter.go           # 核心实现（EventReporter 接口、reporter、noopReporter）
├── builder.go            # EventBuilder 链式构建器
├── context_builders.go   # ServerContext/ActionContext/ClientContext 构建器
├── interceptor.go        # 拦截器机制
├── constants.go          # 事件类型和常量定义
├── context_extractor.go  # Context 提取器
├── config.go             # 配置结构体
└── *_test.go             # 测试文件
```

## 配置

在 `conf/server.yaml` 中配置：

```yaml
event_sink:
  enabled: true
  addr: "event-sink-service:50051"
  queue_size: 10000      # 事件队列大小
  batch_size: 100        # 批量发送阈值
  flush_interval: 5s     # 时间触发阈值
  shutdown_timeout: 10s  # 关闭超时
```
