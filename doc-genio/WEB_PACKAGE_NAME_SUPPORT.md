# Web 端 Package Name 支持

## 变更说明

为了让 Web 端也能使用 `package_name` 字段来标识应用（与移动端的 `App.package_name` 作用相同），我们对 proto 定义和代码进行了以下更新。

---

## Proto 定义变更

### WebClient 消息新增字段

**文件**: `pkg/va_interface/web.proto`

```protobuf
// Web客户端信息
message WebClient {
  // Web应用包名/标识符（与App.package_name作用相同，用于project_id判断）
  // 例如: "com.example.web"、"web.genio.ai" 等
  string package_name = 1;
  // Web应用版本号
  string client_version = 2;
  // 构建版本号（可选）
  string build_number = 3;
  // Web应用平台类型（可选）：admin、user、mobile_web等
  string platform = 4;
}
```

**新增字段说明**：
- `package_name`: Web 应用包名/标识符，用于区分不同的 Web 应用，与后端的 project_id 对应
- `platform`: 可选字段，用于区分不同类型的 Web 应用（管理后台、用户端、移动Web等）

---

## 代码变更

### 1. 新增统一的 GetPackageName 辅助函数

**文件**: `internal/api/common.go`

```go
// GetPackageName 统一获取包名/应用标识符
// 优先从 App 获取，如果不存在则从 WebClient 获取
// 用于获取 project_id
func GetPackageName(header *vai.RequestHeader) string {
	// 优先从 App 获取（移动端）
	if app := header.GetApp(); app != nil && app.GetPackageName() != "" {
		return app.GetPackageName()
	}

	// 从 WebClient 获取（Web端）
	if webClient := header.GetWebClient(); webClient != nil && webClient.GetPackageName() != "" {
		return webClient.GetPackageName()
	}

	// 如果都没有，返回空字符串
	return ""
}
```

**函数特点**：
- 统一处理移动端（App）和 Web 端（WebClient）的 package_name 获取
- 优先级：App > WebClient
- 向后兼容现有代码

### 2. 更新已有代码使用 GetPackageName

**已更新的文件**：
- `internal/api/common.go` - isVerifyAccessToken 函数
- `internal/api/user.go` - GetUserInfo 和 UserNotifyToken 函数

**修改示例**：

```go
// ❌ 旧代码（只支持移动端）
projectID := reqHeader.GetApp().GetPackageName()

// ✅ 新代码（同时支持移动端和Web端）
projectID := GetPackageName(reqHeader)
```

---

## 前端使用指南

### 移动端（无变化）

```javascript
// App 端
{
  "request_header": {
    "req_id": "req-001",
    "access_token": "token",
    "user_id": "user-123",
    "user_type": 0,
    "request_time_ms": 1706515200000,
    "app": {
      "package_name": "com.example.app",  // ✓ 移动端使用
      "version": "1.0.0"
    },
    "device": {
      "os": 1,
      "language": "zh-CN"
    }
  }
}
```

### Web 端（新增支持）

```javascript
// Web 端
{
  "request_header": {
    "req_id": "req-001",
    "access_token": "token",
    "user_id": "user-123",
    "user_type": 0,
    "request_time_ms": 1706515200000,
    "web_client": {
      "package_name": "com.example.web",  // ✓ Web端使用（新增）
      "client_version": "1.0.0",
      "platform": "user"  // 可选：user、admin、mobile_web 等
    },
    "browser_info": {
      "user_agent": "Mozilla/5.0...",
      "language": "zh-CN",
      "platform": "MacOS"
    }
  }
}
```

### TypeScript 接口定义示例

```typescript
interface WebClient {
  package_name: string;      // 必填：Web 应用标识符
  client_version: string;    // 必填：版本号
  build_number?: string;     // 可选：构建号
  platform?: string;         // 可选：平台类型
}

interface RequestHeader {
  req_id: string;
  access_token?: string;
  user_id: string;
  user_type: number;
  request_time_ms: number;
  // 移动端使用 app
  app?: {
    package_name: string;
    version: string;
  };
  device?: Device;
  // Web 端使用 web_client
  web_client?: WebClient;
  browser_info?: BrowserInfo;
}
```

---

## package_name 规范建议

### 移动端
- Android: `com.company.app`
- iOS: `com.company.app`

### Web 端
- 用户端: `com.company.web` 或 `web.company.com`
- 管理后台: `com.company.admin` 或 `admin.company.com`
- 移动Web: `com.company.mobile` 或 `m.company.com`

**注意**：package_name 需要在后端配置为有效的 project_id，否则接口调用会失败。

---

## 迁移指南

### 对于后端开发者

如果你的代码中使用了 `reqHeader.GetApp().GetPackageName()`，请按以下步骤迁移：

1. **使用 GetPackageName 辅助函数**：
   ```go
   // 替换这种写法
   projectID := reqHeader.GetApp().GetPackageName()

   // 改为
   projectID := api.GetPackageName(reqHeader)
   ```

2. **检查代码中的其他使用**：
   ```bash
   # 搜索所有使用 GetApp().GetPackageName() 的地方
   grep -rn "GetApp().GetPackageName()" internal/
   ```

3. **确保空值处理**：
   ```go
   packageName := api.GetPackageName(reqHeader)
   if packageName == "" {
       // 处理未提供 package_name 的情况
       return errors.New("package_name is required")
   }
   ```

### 对于前端开发者

1. **Web 端必须提供 web_client.package_name**：
   ```javascript
   const requestHeader = {
     req_id: `req-${Date.now()}`,
     user_id: userId,
     user_type: 0,
     request_time_ms: Date.now(),
     web_client: {
       package_name: 'com.example.web', // 必须提供
       client_version: '1.0.0'
     },
     browser_info: {
       user_agent: navigator.userAgent,
       language: navigator.language
     }
   };
   ```

2. **配置环境变量**：
   ```javascript
   // .env
   VITE_APP_PACKAGE_NAME=com.example.web

   // 代码中使用
   const packageName = import.meta.env.VITE_APP_PACKAGE_NAME;
   ```

---

## 测试验证

### 测试移动端（应该继续正常工作）

```bash
curl -X POST http://localhost:8200/v1/user/get_user_info \
  -H "Content-Type: application/json" \
  -d '{
    "request_header": {
      "req_id": "test-001",
      "user_id": "test-user",
      "user_type": 0,
      "request_time_ms": 1706515200000,
      "app": {
        "package_name": "com.example.app",
        "version": "1.0.0"
      },
      "device": {
        "os": 1,
        "language": "zh-CN"
      }
    }
  }'
```

### 测试 Web 端（新功能）

```bash
curl -X POST http://localhost:8200/v1/user/get_user_info \
  -H "Content-Type: application/json" \
  -d '{
    "request_header": {
      "req_id": "test-002",
      "user_id": "test-user",
      "user_type": 0,
      "request_time_ms": 1706515200000,
      "web_client": {
        "package_name": "com.example.web",
        "client_version": "1.0.0",
        "platform": "user"
      },
      "browser_info": {
        "user_agent": "Mozilla/5.0...",
        "language": "zh-CN"
      }
    }
  }'
```

---

## 常见问题

### Q1: 为什么 Web 端也要 package_name？

**A**: 因为后端很多地方使用 `package_name` 作为 `project_id` 来区分不同的应用和项目。保持字段名一致可以避免代码中大量的条件判断和特殊处理。

### Q2: package_name 可以重复吗？

**A**: 不同平台可以使用不同的 package_name，但同一平台的不同环境应该使用不同的值。例如：
- 生产环境: `com.example.web`
- 测试环境: `com.example.web.test`

### Q3: 如果不提供 package_name 会怎样？

**A**: 某些接口可能会返回错误或使用默认值。建议始终提供 package_name。

### Q4: 我的代码已经在用 GetApp().GetPackageName()，会受影响吗？

**A**:
- 如果你的接口只被移动端调用：不受影响
- 如果你的接口需要支持 Web 端：需要改用 `GetPackageName(reqHeader)` 辅助函数

---

## 相关文档

- [Web 端请求头规范](./WEB_REQUEST_HEADER_GUIDE.md)
- [用户登录完整指南](./AUTH_LOGIN_GUIDE.md)
- [UserService HTTP API](./USER_SERVICE_HTTP_API.md)

---

## 更新日志

| 日期 | 版本 | 说明 |
|------|------|------|
| 2026-01-29 | v1.0 | 新增 Web 端 package_name 支持 |
