# 后端 Web 端迁移指南

## 概述

`RequestHeader` 协议已升级，现在同时支持**移动端（App）**和 **Web 端**客户端。本文档面向后端开发者，说明如何修改现有代码以兼容 Web 端。

## 变更内容

### 1. Protocol Buffer 定义变更

#### 新增消息类型
- `WebClient` - Web 客户端信息（版本、构建号）
- `BrowserInfo` - 浏览器信息（浏览器名称/版本、平台、屏幕尺寸等）

#### RequestHeader 字段变更
```protobuf
message RequestHeader {
  // 通用字段
  string req_id = 1;
  string access_token = 2;
  string user_id = 5;
  UserType user_type = 7;
  int64 request_time_ms = 8;

  // 移动端字段（optional）
  optional App app = 3;
  optional Device device = 4;
  optional AppStore app_store = 6;

  // Web 端字段（optional）⭐ 新增
  optional WebClient web_client = 9;
  optional BrowserInfo browser_info = 10;
}
```

### 2. 拦截器修改

[internal/rpc/interceptor.go](../internal/rpc/interceptor.go) 已经更新，提供了兼容 Web 端和移动端的辅助函数。

#### 新增辅助函数

```go
// 判断是否为 Web 端客户端
func IsWebClient(header *vai.RequestHeader) bool

// 获取应用版本（兼容移动端和 Web 端）
func GetAppVersion(header *vai.RequestHeader) string

// 获取操作系统/平台（兼容移动端和 Web 端）
func GetPlatformOS(header *vai.RequestHeader) string

// 获取语言（兼容移动端和 Web 端）
func GetLanguage(header *vai.RequestHeader) vai.Language

// 获取包名/项目 ID（兼容移动端和 Web 端）
func GetPackageName(header *vai.RequestHeader) string
```

## 迁移步骤

### Step 1: 识别需要修改的代码

搜索以下模式的代码：
```bash
grep -r "\.GetDevice()" internal/
grep -r "\.GetApp()\.Get" internal/
grep -r "header\.GetAppStore()" internal/
```

### Step 2: 使用辅助函数替换直接访问

#### ❌ 错误写法（仅支持移动端）

```go
// 直接访问移动端字段
appVersion := req.GetRequestHeader().GetApp().GetAppVersion()
platform := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())
language := req.GetRequestHeader().GetDevice().GetLanguage()
```

**问题**：Web 端请求时 `GetApp()` 和 `GetDevice()` 返回 nil，导致空指针异常。

#### ✅ 正确写法（兼容移动端和 Web 端）

```go
import "va_visionai_server/internal/rpc"

// 使用辅助函数
header := req.GetRequestHeader()
appVersion := rpc.GetAppVersion(header)
platform := rpc.GetPlatformOS(header)
language := rpc.GetLanguage(header)
```

### Step 3: 修改具体文件

以下是需要修改的关键文件：

#### 1. internal/api/prompt.go

**Before:**
```go
language := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
```

**After:**
```go
language := constants.LanguageMap(rpc.GetLanguage(req.GetRequestHeader()))
```

#### 2. internal/api/tool.go

**Before:**
```go
appVersion := req.GetRequestHeader().GetApp().GetAppVersion()
platform := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())
```

**After:**
```go
header := req.GetRequestHeader()
appVersion := rpc.GetAppVersion(header)
platform := rpc.GetPlatformOS(header)
```

#### 3. internal/api/picture_forge.go

**Before:**
```go
appVersion := req.GetRequestHeader().GetApp().GetAppVersion()
platform := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())
```

**After:**
```go
header := req.GetRequestHeader()
appVersion := rpc.GetAppVersion(header)
platform := rpc.GetPlatformOS(header)
```

#### 4. internal/service/event/event_service.go

**Before:**
```go
brand := req.GetDevice().GetBrand()
model := req.GetDevice().GetModel()
projectID := req.GetApp().GetPackageName()
version := req.GetApp().GetAppVersion()
ip := req.GetDevice().GetIp()
language := req.GetDevice().GetLanguage().String()
```

**After:**
```go
var brand, model, ip string
var language vai.Language

if rpc.IsWebClient(req) {
    // Web 端
    if browserInfo := req.GetBrowserInfo(); browserInfo != nil {
        brand = browserInfo.GetBrowserName()
        model = browserInfo.GetBrowserVersion()
        ip = browserInfo.GetIp()
        language = browserInfo.GetLanguage()
    }
} else {
    // 移动端
    if device := req.GetDevice(); device != nil {
        brand = device.GetBrand()
        model = device.GetModel()
        ip = device.GetIp()
        language = device.GetLanguage()
    }
}

projectID := rpc.GetPackageName(req)
version := rpc.GetAppVersion(req)
```

#### 5. internal/common/submitcontext/submit_context.go

**Before:**
```go
if h == nil || h.GetDevice() == nil {
    return nil
}
dev := h.GetDevice()
// ... 使用 dev 的字段
ctx.App = AppInfo{Version: h.GetApp().GetAppVersion()}
```

**After:**
```go
if h == nil {
    return nil
}

// 获取设备信息
var brand, model string
var osVersion string

if rpc.IsWebClient(h) {
    // Web 端
    if browserInfo := h.GetBrowserInfo(); browserInfo != nil {
        brand = browserInfo.GetBrowserName()
        model = browserInfo.GetPlatform()
        osVersion = browserInfo.GetBrowserVersion()
    }
} else {
    // 移动端
    if device := h.GetDevice(); device != nil {
        brand = device.GetBrand()
        model = device.GetModel()
        osVersion = device.GetOsv()
    }
}

ctx.App = AppInfo{Version: rpc.GetAppVersion(h)}
```

#### 6. internal/rpc/init.go

**Before:**
```go
packageName := result.RequestHeader.GetApp().GetPackageName()
os := result.RequestHeader.GetDevice().GetOs()
osName := constants.MappingOS(reqHeader.GetDevice().GetOs())
```

**After:**
```go
packageName := rpc.GetPackageName(result.RequestHeader)
platform := rpc.GetPlatformOS(result.RequestHeader)
osName := rpc.GetPlatformOS(reqHeader)
```

## 辅助函数详细说明

### IsWebClient

判断是否为 Web 端客户端。

```go
func IsWebClient(header *vai.RequestHeader) bool
```

**返回值**：
- `true` - Web 端客户端（存在 `web_client` 或 `browser_info`）
- `false` - 移动端客户端

**使用场景**：
当需要针对不同端做不同处理时使用。

```go
if rpc.IsWebClient(header) {
    // Web 端特殊处理
} else {
    // 移动端特殊处理
}
```

### GetAppVersion

获取应用版本号（自动兼容移动端和 Web 端）。

```go
func GetAppVersion(header *vai.RequestHeader) string
```

**返回值**：
- Web 端：返回 `web_client.client_version`
- 移动端：返回 `app.app_version`
- 未设置：返回空字符串 `""`

### GetPlatformOS

获取操作系统/平台（自动兼容移动端和 Web 端）。

```go
func GetPlatformOS(header *vai.RequestHeader) string
```

**返回值**：
- Web 端：返回 `browser_info.platform`（如 "MacOS"、"Windows"、"Linux"）
- 移动端：返回映射后的 OS 名称（如 "iOS"、"Android"）
- 未设置：返回 `"Unknown"`

### GetLanguage

获取语言（自动兼容移动端和 Web 端）。

```go
func GetLanguage(header *vai.RequestHeader) vai.Language
```

**返回值**：
- Web 端：返回 `browser_info.language`
- 移动端：返回 `device.language`
- 未设置：返回 `vai.Language_ENGLISH`

### GetPackageName

获取包名/项目 ID（自动兼容移动端和 Web 端）。

```go
func GetPackageName(header *vai.RequestHeader) string
```

**返回值**：
- Web 端：返回默认项目 ID `constants.ProjectIdVisionAI`（"com.domob.visionai"）
- 移动端：返回 `app.package_name`
- 未设置：返回默认项目 ID

## 字段映射表

| 用途 | 移动端字段 | Web 端字段 | 辅助函数 |
|------|-----------|-----------|---------|
| 应用版本 | `app.app_version` | `web_client.client_version` | `GetAppVersion()` |
| 操作系统 | `device.os` | `browser_info.platform` | `GetPlatformOS()` |
| 语言 | `device.language` | `browser_info.language` | `GetLanguage()` |
| 项目 ID | `app.package_name` | （使用默认值） | `GetPackageName()` |
| 设备品牌 | `device.brand` | `browser_info.browser_name` | - |
| 设备型号 | `device.model` | `browser_info.browser_version` | - |
| IP 地址 | `device.ip` | `browser_info.ip` | - |
| 屏幕尺寸 | `device.width/height` | `browser_info.screen_width/height` | - |

## 注意事项

### 1. 空值检查

即使使用辅助函数，在访问移动端或 Web 端特有字段时仍需进行空值检查：

```go
header := req.GetRequestHeader()
if rpc.IsWebClient(header) {
    if browserInfo := header.GetBrowserInfo(); browserInfo != nil {
        // 安全访问 browserInfo 的字段
        userAgent := browserInfo.GetUserAgent()
    }
} else {
    if device := header.GetDevice(); device != nil {
        // 安全访问 device 的字段
        imei := device.GetImei()
    }
}
```

### 2. 项目 ID 处理

Web 端统一使用默认项目 ID `com.domob.visionai`。如果你的业务逻辑依赖不同的项目 ID，需要：
- 方案 A：在 Web 端前端传递项目 ID（添加新字段）
- 方案 B：在后端根据域名或其他信息推断项目 ID

### 3. 日志记录

在日志中记录客户端类型有助于调试：

```go
zlog.LogWithContext(ctx).Info("Request received",
    zap.Bool("is_web_client", rpc.IsWebClient(header)),
    zap.String("platform", rpc.GetPlatformOS(header)),
    zap.String("version", rpc.GetAppVersion(header)))
```

### 4. 统计和监控

如果你的监控系统依赖 `AppStore` 字段，需要注意：
- Web 端的 `AppStore` 为 `"WEB"`
- 在 Prometheus 等监控中需要添加对应的标签

### 5. 向后兼容

这些修改保持了向后兼容性：
- 移动端客户端无需修改，继续使用原有字段
- 只有 Web 端需要填充新字段

## 测试建议

### 单元测试

```go
func TestGetAppVersion(t *testing.T) {
    // 测试 Web 端
    webHeader := &vai.RequestHeader{
        WebClient: &vai.WebClient{
            ClientVersion: "1.0.0",
        },
    }
    assert.Equal(t, "1.0.0", rpc.GetAppVersion(webHeader))

    // 测试移动端
    mobileHeader := &vai.RequestHeader{
        App: &vai.App{
            AppVersion: "2.0.0",
        },
    }
    assert.Equal(t, "2.0.0", rpc.GetAppVersion(mobileHeader))

    // 测试空值
    assert.Equal(t, "", rpc.GetAppVersion(nil))
}
```

### 集成测试

使用 Postman 或 grpcurl 测试 Web 端请求：

```bash
# Web 端请求示例
grpcurl -d '{
  "request_header": {
    "req_id": "test-123",
    "user_id": "user-456",
    "user_type": 0,
    "request_time_ms": 1738137600000,
    "web_client": {
      "client_version": "1.0.0"
    },
    "browser_info": {
      "platform": "MacOS",
      "browser_name": "Chrome",
      "browser_version": "122.0.0",
      "language": 0
    }
  }
}' localhost:8080 va_interface.YourService/YourMethod
```

## 需要修改的文件清单

根据代码扫描，以下文件需要更新（按优先级排序）：

### 高优先级（核心功能）
- ✅ `internal/rpc/interceptor.go` - 已完成
- ⚠️ `internal/rpc/init.go` - 需要修改
- ⚠️ `internal/api/prompt.go` - 需要修改
- ⚠️ `internal/api/tool.go` - 需要修改
- ⚠️ `internal/common/submitcontext/submit_context.go` - 需要修改

### 中优先级（图片相关）
- ⚠️ `internal/api/picture_forge.go` - 需要修改（多处）

### 低优先级（事件上报）
- ⚠️ `internal/service/event/event_service.go` - 需要修改

## 完成检查清单

- [x] 更新 protobuf 定义
- [x] 生成 protobuf Go 代码
- [x] 修改拦截器（interceptor.go）
- [x] 创建辅助函数
- [ ] 修改 API 层代码
- [ ] 修改 Service 层代码
- [ ] 更新单元测试
- [ ] 更新集成测试
- [ ] 更新文档

## 常见问题

### Q1: 如果我直接访问 `header.GetDevice()` 会怎样？

**A**: 如果请求来自 Web 端，`header.GetDevice()` 返回 nil，后续访问其字段会导致空指针异常（panic）。必须使用辅助函数或做空值检查。

### Q2: 是否需要同时支持移动端和 Web 端的所有接口？

**A**: 不一定。你可以根据业务需求：
- 部分接口仅支持移动端（检查并拒绝 Web 端请求）
- 部分接口仅支持 Web 端（检查并拒绝移动端请求）
- 大部分接口同时支持两端

### Q3: 如何在日志中区分移动端和 Web 端？

**A**: 拦截器已经在 context 中设置了 `CtxAppStore`：
```go
appStore := common.GetCtxValue(ctx, constants.CtxAppStore)
// Web 端: appStore == "WEB"
// 移动端: appStore == "GOOGLE_PLAY"、"OPPO" 等
```

### Q4: 性能影响如何？

**A**: 辅助函数只是简单的字段访问和条件判断，性能影响可以忽略不计（纳秒级）。

## 联系方式

如有疑问，请联系：
- 后端团队：[backend-team@example.com]
- 架构组：[architecture@example.com]

---

**更新时间**: 2025-01-29
**版本**: v1.0
**维护者**: VisionAI Backend Team
