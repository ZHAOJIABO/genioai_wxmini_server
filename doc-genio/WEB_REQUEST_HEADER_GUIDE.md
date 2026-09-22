# Web 端 RequestHeader 使用指南

## 概述

`RequestHeader` 协议已升级，现在同时支持**移动端（App）**和 **Web 端**客户端。本文档面向前端开发者，说明如何在 Web 端正确使用新的 `RequestHeader` 结构。

## 变更内容

### 1. 新增消息类型

创建了两个新的消息类型用于 Web 端：

#### `WebClient` - Web 客户端信息
```protobuf
message WebClient {
  string client_version = 1;  // Web应用版本号
  string build_number = 2;    // 构建版本号（可选）
}
```

#### `BrowserInfo` - 浏览器信息
```protobuf
message BrowserInfo {
  string ip = 1;                // IP地址（IPv4）
  string user_agent = 2;        // 浏览器User Agent
  string browser_name = 3;      // 浏览器名称（Chrome、Firefox、Safari、Edge等）
  string browser_version = 4;   // 浏览器版本
  int32 screen_width = 5;       // 屏幕宽度（像素）
  int32 screen_height = 6;      // 屏幕高度（像素）
  Language language = 7;        // 语言
  string timezone = 8;          // 时区
  string platform = 9;          // 操作系统平台（Windows、MacOS、Linux等）
  string ipv6 = 10;             // IPv6地址（可选）
  double pixel_ratio = 11;      // 屏幕像素密度
}
```

### 2. `RequestHeader` 结构调整

`RequestHeader` 现在分为三部分：

#### 通用字段（所有端必填）
- `req_id` - 请求ID
- `access_token` - 访问令牌（登录后携带）
- `user_id` - 用户ID
- `user_type` - 用户类型（0: 注册用户, 1: 游客）
- `request_time_ms` - 请求时间戳（毫秒）

#### 移动端专用字段（optional）
- `app` - App版本信息
- `device` - 设备信息
- `app_store` - 应用商店

#### **Web 端专用字段（optional）** ⭐ 新增
- `web_client` - Web客户端信息
- `browser_info` - 浏览器信息

## Web 端使用方法

### 字段填充规则

1. **必须填充**：通用字段
2. **Web 端必须填充**：`web_client` 和 `browser_info`
3. **不要填充**：移动端专用字段（`app`、`device`、`app_store`）

### JSON 示例

#### 完整示例（注册用户）
```json
{
  "request_header": {
    "req_id": "web-req-1738137600000-abc123",
    "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "user_id": "user_12345",
    "user_type": 0,
    "request_time_ms": 1738137600000,

    "web_client": {
      "client_version": "1.0.0",
      "build_number": "20250129001"
    },

    "browser_info": {
      "ip": "192.168.1.100",
      "user_agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
      "browser_name": "Chrome",
      "browser_version": "122.0.0.0",
      "screen_width": 1920,
      "screen_height": 1080,
      "language": 0,
      "timezone": "Asia/Shanghai",
      "platform": "MacOS",
      "pixel_ratio": 2.0
    }
  }
}
```

#### 简化示例（游客用户）
```json
{
  "request_header": {
    "req_id": "web-req-1738137600000-xyz789",
    "user_type": 1,
    "request_time_ms": 1738137600000,

    "web_client": {
      "client_version": "1.0.0"
    },

    "browser_info": {
      "ip": "192.168.1.100",
      "user_agent": "Mozilla/5.0...",
      "browser_name": "Chrome",
      "browser_version": "122.0.0.0",
      "screen_width": 1920,
      "screen_height": 1080,
      "language": 0,
      "timezone": "Asia/Shanghai",
      "platform": "MacOS"
    }
  }
}
```

## 字段获取方式

### JavaScript/TypeScript 实现

```typescript
// 获取浏览器信息
function getBrowserInfo(): BrowserInfo {
  const ua = navigator.userAgent;

  // 解析浏览器名称和版本（简化版，建议使用 UAParser.js 等库）
  let browserName = 'Unknown';
  let browserVersion = 'Unknown';

  if (ua.includes('Chrome')) {
    browserName = 'Chrome';
    browserVersion = ua.match(/Chrome\/([\d.]+)/)?.[1] || 'Unknown';
  } else if (ua.includes('Firefox')) {
    browserName = 'Firefox';
    browserVersion = ua.match(/Firefox\/([\d.]+)/)?.[1] || 'Unknown';
  } else if (ua.includes('Safari')) {
    browserName = 'Safari';
    browserVersion = ua.match(/Version\/([\d.]+)/)?.[1] || 'Unknown';
  } else if (ua.includes('Edge')) {
    browserName = 'Edge';
    browserVersion = ua.match(/Edge\/([\d.]+)/)?.[1] || 'Unknown';
  }

  // 获取平台信息
  let platform = 'Unknown';
  if (ua.includes('Mac')) platform = 'MacOS';
  else if (ua.includes('Win')) platform = 'Windows';
  else if (ua.includes('Linux')) platform = 'Linux';

  return {
    ip: '', // 需要从服务器获取或使用客户端IP检测服务
    user_agent: ua,
    browser_name: browserName,
    browser_version: browserVersion,
    screen_width: window.screen.width,
    screen_height: window.screen.height,
    language: getLanguageCode(), // 见下方函数
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
    platform: platform,
    pixel_ratio: window.devicePixelRatio
  };
}

// 获取语言代码
function getLanguageCode(): number {
  const lang = navigator.language.toLowerCase();
  if (lang.startsWith('zh')) return 0; // CHINESE
  if (lang.startsWith('en')) return 1; // ENGLISH
  if (lang.startsWith('ru')) return 2; // RUSSIAN
  if (lang.startsWith('vi')) return 3; // VIETNAMESE
  if (lang.startsWith('pt')) return 4; // PORTUGUESE
  if (lang.startsWith('id')) return 5; // INDONESIAN
  if (lang.startsWith('ms')) return 6; // MALAY
  if (lang.startsWith('th')) return 7; // THAI
  if (lang.startsWith('fil')) return 8; // FILIPINO
  return 1; // 默认 ENGLISH
}

// 生成请求ID
function generateRequestId(): string {
  const timestamp = Date.now();
  const random = Math.random().toString(36).substring(2, 10);
  return `web-req-${timestamp}-${random}`;
}

// 构建 RequestHeader
function buildRequestHeader(
  accessToken?: string,
  userId?: string
): RequestHeader {
  return {
    req_id: generateRequestId(),
    access_token: accessToken || '',
    user_id: userId || '',
    user_type: userId ? 0 : 1, // 有 userId 为注册用户，否则为游客
    request_time_ms: Date.now(),

    web_client: {
      client_version: '1.0.0', // 从配置中读取
      build_number: import.meta.env.VITE_BUILD_NUMBER || ''
    },

    browser_info: getBrowserInfo()
  };
}
```

## Language 枚举值

```typescript
enum Language {
  CHINESE = 0,      // 中文
  ENGLISH = 1,      // 英语
  RUSSIAN = 2,      // 俄语
  VIETNAMESE = 3,   // 越南语
  PORTUGUESE = 4,   // 葡萄牙语
  INDONESIAN = 5,   // 印尼语
  MALAY = 6,        // 马来语
  THAI = 7,         // 泰语
  FILIPINO = 8      // 菲律宾语
}
```

## 迁移指南

### 如果你的项目之前使用了移动端的 RequestHeader

#### 需要修改的地方：

1. **删除移动端字段**
   ```diff
   {
     "request_header": {
       "req_id": "...",
       "access_token": "...",
   -   "app": { ... },
   -   "device": { ... },
   -   "app_store": 1,
   +   "web_client": { ... },
   +   "browser_info": { ... }
     }
   }
   ```

2. **更新客户端信息采集逻辑**
   - 移除：设备IMEI、IDFA、OAID等移动设备标识
   - 移除：应用商店信息、已安装应用列表
   - 新增：浏览器名称/版本、平台信息、像素密度

3. **更新请求拦截器/中间件**

   **Before:**
   ```typescript
   // 旧的移动端实现
   const header = {
     req_id: generateId(),
     access_token: token,
     app: getAppInfo(),        // ❌ 移除
     device: getDeviceInfo(),  // ❌ 移除
     app_store: 1              // ❌ 移除
   };
   ```

   **After:**
   ```typescript
   // 新的 Web 端实现
   const header = {
     req_id: generateId(),
     access_token: token,
     user_id: getUserId(),
     user_type: getUserType(),
     request_time_ms: Date.now(),
     web_client: {               // ✅ 新增
       client_version: '1.0.0',
       build_number: BUILD_NUMBER
     },
     browser_info: getBrowserInfo() // ✅ 新增
   };
   ```

## 注意事项

### 1. IP 地址获取
浏览器无法直接获取客户端真实 IP，建议：
- 方案A：在服务端拦截器中填充 IP 地址
- 方案B：调用第三方 IP 检测服务（如 ipify.org、ipapi.co）
- 方案C：暂时留空，完全由服务端填充

### 2. 隐私合规
采集浏览器信息需要遵守隐私法规（GDPR、CCPA等）：
- 在隐私政策中说明数据用途
- 对于敏感字段（IP、时区等），确保用户知情

### 3. 用户代理字符串（User Agent）
Chrome 正在逐步弃用完整的 User Agent：
- 考虑使用 [User-Agent Client Hints](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/User-Agent-Client-Hints)
- 后端应做好降级处理

### 4. 请求时间戳
`request_time_ms` 应使用客户端时间戳（毫秒），服务端会：
- 验证时间戳与服务器时间的偏差（防重放攻击）
- 允许合理的时间误差（如 ±5 分钟）

## 与移动端的主要区别

| 特性 | 移动端 | Web 端 |
|------|--------|--------|
| 客户端标识 | IMEI、IDFA、OAID | User Agent、浏览器指纹 |
| 应用信息 | 包名、版本、应用商店 | 客户端版本、构建号 |
| 设备信息 | 品牌、型号、OS | 浏览器名称、版本、平台 |
| 网络信息 | 2G/3G/4G/5G、运营商 | （通常不采集） |
| 定位信息 | GPS 经纬度 | （需单独授权） |
| 已安装应用 | 应用列表 | （无法获取） |

## 完整的 TypeScript 类型定义

```typescript
interface RequestHeader {
  req_id: string;
  access_token?: string;
  user_id?: string;
  user_type: 0 | 1; // 0: 注册用户, 1: 游客
  request_time_ms: number;

  // Web 端字段
  web_client?: WebClient;
  browser_info?: BrowserInfo;

  // 移动端字段（Web 端不使用）
  app?: App;
  device?: Device;
  app_store?: number;
}

interface WebClient {
  client_version: string;
  build_number?: string;
}

interface BrowserInfo {
  ip: string;
  user_agent: string;
  browser_name: string;
  browser_version: string;
  screen_width: number;
  screen_height: number;
  language: Language;
  timezone: string;
  platform: string;
  ipv6?: string;
  pixel_ratio?: number;
}

enum Language {
  CHINESE = 0,
  ENGLISH = 1,
  RUSSIAN = 2,
  VIETNAMESE = 3,
  PORTUGUESE = 4,
  INDONESIAN = 5,
  MALAY = 6,
  THAI = 7,
  FILIPINO = 8
}
```

## 常见问题（FAQ）

### Q1: 必须同时填充 `web_client` 和 `browser_info` 吗？
**A:** 是的，Web 端请求应该同时填充这两个字段。服务端可能会验证这些字段的存在性。

### Q2: `user_id` 和 `access_token` 在游客模式下可以为空吗？
**A:** 可以。游客用户（`user_type = 1`）可以不提供 `user_id` 和 `access_token`。但注册用户（`user_type = 0`）必须提供。

### Q3: `req_id` 有格式要求吗？
**A:** 建议格式为 `web-req-{timestamp}-{random}`，确保全局唯一即可。服务端会用它来追踪请求链路。

### Q4: 屏幕尺寸应该用 `window.innerWidth` 还是 `screen.width`？
**A:** 使用 `screen.width` 和 `screen.height`（物理屏幕尺寸），而不是窗口大小。

### Q5: 如何处理浏览器禁用 JavaScript 的情况？
**A:** 如果 JavaScript 被禁用，Web 应用通常无法正常工作。可以在服务端提供降级方案，从 HTTP 头中提取基本信息。

## 相关文档

- [Protocol Buffers 官方文档](https://protobuf.dev/)
- [gRPC Web 文档](https://grpc.io/docs/platforms/web/)
- [User-Agent Client Hints](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/User-Agent-Client-Hints)

## 联系方式

如有疑问，请联系：
- 后端团队：[backend-team@example.com]
- API 文档：[https://api-docs.example.com]

---

**更新时间**: 2025-01-29
**版本**: v1.0
**维护者**: VisionAI Backend Team
