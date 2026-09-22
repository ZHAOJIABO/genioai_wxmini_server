# UserService HTTP/REST API 接口文档

## 概述

UserService 已支持 HTTP/REST API 调用（通过 grpc-gateway），所有接口均使用 **POST** 方法，请求体为 JSON 格式。

## 基础信息

- **协议**: HTTP/1.1 或 HTTP/2
- **Content-Type**: application/json
- **接口前缀**: /v1/user
- **认证方式**: 在 request_header 中携带 access_token

---

## 1. 获取用户信息

### 接口地址

```
POST /v1/user/get_user_info
```

### 请求参数

```json
{
  "request_header": {
    "req_id": "unique-request-id",
    "access_token": "user-access-token",
    "user_id": "user-id-123",
    "user_type": 0,
    "request_time_ms": 1706515200000,
    "web_client": {
      "client_version": "1.0.0",
      "platform": "web"
    },
    "browser_info": {
      "user_agent": "Mozilla/5.0...",
      "language": "zh-CN"
    }
  }
}
```

### 响应示例

```json
{
  "response_header": {
    "req_id": "unique-request-id",
    "msg": "Success",
    "server_time": "2024-01-29 10:00:00",
    "server_version": "1.0.0",
    "response_time_ms": 1706515200000,
    "user_id": "user-id-123",
    "response_status_code": 200
  },
  "user_info": {
    "user_id": "user-id-123",
    "user_name": "john_doe",
    "phone_number": "+8613800138000",
    "avatar": "https://example.com/avatar.jpg",
    "gender": 1,
    "birthday": "1990-01-01 00:00:00",
    "nickname": "John",
    "register_time": "2024-01-01 10:00:00",
    "last_login_time": "2024-01-29 09:00:00",
    "user_config": {
      "share_url": "https://example.com/share",
      "pop_subscribe": false,
      "pop_free_daily_score": true,
      "free_daily_credit": 10,
      "should_show": false,
      "float_tool_bar_tool_id": "tool-001"
    },
    "subscribe_info": {
      "product_id": "monthly_vip",
      "status": 1,
      "subscribe_expired_time": "2024-02-29 23:59:59",
      "subscribe_level": 1,
      "credit": 1000
    }
  }
}
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| user_type | int | 0-注册用户，1-游客 |
| gender | int | 0-未知，1-男，2-女，3-无性别，4-保密 |
| subscribe_info.status | int | 0-未订阅，1-生效中，2-已过期 |

---

## 2. 修改用户名

### 接口地址

```
POST /v1/user/edit_user_name
```

### 请求参数

```json
{
  "request_header": {
    "req_id": "unique-request-id",
    "access_token": "user-access-token",
    "user_id": "user-id-123",
    "user_type": 0,
    "request_time_ms": 1706515200000
  },
  "user_name": "new_username"
}
```

### 响应示例

```json
{
  "response_header": {
    "req_id": "unique-request-id",
    "msg": "Success",
    "server_time": "2024-01-29 10:00:00",
    "response_status_code": 200
  }
}
```

### 业务规则

1. **用户名长度**: 7-15 个字符
2. **允许字符**: 大小写字母、数字、下划线(_)、连字符(-)
3. **唯一性**: 用户名不能重复
4. **认证**: 必须是已注册用户（user_type=0）

### 常见错误

| 错误码 | 说明 | 处理建议 |
|--------|------|----------|
| 1001 | 用户名格式不符合要求 | 检查用户名长度和字符 |
| 4011 | 用户名已存在 | 提示用户更换用户名 |
| 2001 | access_token 无效 | 重新登录 |

---

## 3. 注销账号

### 接口地址

```
POST /v1/user/delete_user
```

### 请求参数

```json
{
  "request_header": {
    "req_id": "unique-request-id",
    "access_token": "user-access-token",
    "user_id": "user-id-123",
    "user_type": 0,
    "request_time_ms": 1706515200000,
    "device": {
      "os": 2,
      "language": "zh-CN"
    }
  }
}
```

### 响应示例

```json
{
  "response_header": {
    "req_id": "unique-request-id",
    "msg": "Success",
    "server_time": "2024-01-29 10:00:00",
    "response_status_code": 200
  }
}
```

### 业务逻辑

**注销账号后将执行以下操作：**

1. **撤销 Apple RefreshToken**（iOS 用户）
2. **存档所有聊天记录**（软删除）
3. **删除用户个人资料数据**
4. **软删除用户账号**（标记为已删除，数据保留）

⚠️ **注意事项：**
- 注销是不可逆操作
- 建议在调用前向用户进行二次确认
- 注销后用户需要重新注册才能使用

---

## 4. 推送Token上报

### 接口地址

```
POST /v1/user/notify_token
```

### 请求参数

```json
{
  "request_header": {
    "req_id": "unique-request-id",
    "access_token": "user-access-token",
    "user_id": "user-id-123",
    "user_type": 0,
    "request_time_ms": 1706515200000,
    "device": {
      "os": 1,
      "language": "zh-CN"
    },
    "app": {
      "package_name": "com.example.app"
    }
  },
  "token": "fcm-or-apns-token-string"
}
```

### 响应示例

```json
{
  "response_header": {
    "req_id": "unique-request-id",
    "msg": "Success",
    "server_time": "2024-01-29 10:00:00",
    "response_status_code": 200
  }
}
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| token | string | FCM token（Android）或 APNs token（iOS） |
| device.os | int | 1-Android，2-iOS |

### 业务说明

- **Android**: 上报 FCM (Firebase Cloud Messaging) Token
- **iOS**: 上报 APNs (Apple Push Notification service) Token
- 用于向用户推送消息通知
- Token 失效时需要重新上报

---

## HTTP 调用示例

### cURL

```bash
# 获取用户信息
curl -X POST https://your-api-domain.com/v1/user/get_user_info \
  -H "Content-Type: application/json" \
  -d '{
    "request_header": {
      "req_id": "req-001",
      "access_token": "your-access-token",
      "user_id": "user-123",
      "user_type": 0,
      "request_time_ms": 1706515200000
    }
  }'
```

### JavaScript (Fetch API)

```javascript
async function getUserInfo(accessToken, userId) {
  const response = await fetch('https://your-api-domain.com/v1/user/get_user_info', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      request_header: {
        req_id: `req-${Date.now()}`,
        access_token: accessToken,
        user_id: userId,
        user_type: 0,
        request_time_ms: Date.now(),
        web_client: {
          client_version: '1.0.0',
          platform: 'web'
        },
        browser_info: {
          user_agent: navigator.userAgent,
          language: navigator.language
        }
      }
    })
  });

  const data = await response.json();

  if (data.response_header.response_status_code === 200) {
    console.log('用户信息:', data.user_info);
    return data.user_info;
  } else {
    throw new Error(data.response_header.msg);
  }
}
```

### JavaScript (Axios)

```javascript
import axios from 'axios';

// 修改用户名
async function editUserName(accessToken, userId, newUserName) {
  try {
    const response = await axios.post(
      'https://your-api-domain.com/v1/user/edit_user_name',
      {
        request_header: {
          req_id: `req-${Date.now()}`,
          access_token: accessToken,
          user_id: userId,
          user_type: 0,
          request_time_ms: Date.now()
        },
        user_name: newUserName
      },
      {
        headers: {
          'Content-Type': 'application/json'
        }
      }
    );

    if (response.data.response_header.response_status_code === 200) {
      console.log('用户名修改成功');
      return true;
    } else {
      throw new Error(response.data.response_header.msg);
    }
  } catch (error) {
    console.error('修改用户名失败:', error);
    throw error;
  }
}
```

### Python (requests)

```python
import requests
import time

def get_user_info(access_token, user_id):
    url = "https://your-api-domain.com/v1/user/get_user_info"

    payload = {
        "request_header": {
            "req_id": f"req-{int(time.time() * 1000)}",
            "access_token": access_token,
            "user_id": user_id,
            "user_type": 0,
            "request_time_ms": int(time.time() * 1000)
        }
    }

    headers = {
        "Content-Type": "application/json"
    }

    response = requests.post(url, json=payload, headers=headers)
    data = response.json()

    if data["response_header"]["response_status_code"] == 200:
        return data["user_info"]
    else:
        raise Exception(data["response_header"]["msg"])
```

---

## 统一错误码

### 成功状态

| 状态码 | 说明 |
|--------|------|
| 200 | 请求成功 |

### 通用错误

| 状态码 | 说明 | 处理建议 |
|--------|------|----------|
| 1001 | 非法参数 | 检查请求参数格式 |
| 1002 | 非法请求 | 检查请求体结构 |
| 1003 | 请求失败 | 服务器内部错误，稍后重试 |

### 认证错误

| 状态码 | 说明 | 处理建议 |
|--------|------|----------|
| 2001 | AccessToken 无效 | 提示用户重新登录 |
| 2002 | AccessToken 过期 | 调用 RefreshToken 接口 |
| 2003 | RefreshToken 无效 | 提示用户重新登录 |
| 2004 | 无效的用户 | 用户可能被封禁或已注销 |

### 业务错误

| 状态码 | 说明 | 处理建议 |
|--------|------|----------|
| 4011 | 用户名已存在 | 提示用户更换用户名 |

---

## 注意事项

### 1. 请求头（request_header）

**必填字段：**
- `req_id`: 唯一请求ID，用于追踪和排查问题
- `access_token`: 用户访问令牌（游客除外）
- `user_id`: 用户ID
- `user_type`: 0-注册用户，1-游客
- `request_time_ms`: 请求时间戳（毫秒）

**平台特定字段：**
- **移动端**: 提供 `app` 和 `device` 信息
- **Web端**: 提供 `web_client` 和 `browser_info` 信息

### 2. 认证机制

- **注册用户** (user_type=0): 必须提供有效的 `access_token`
- **游客用户** (user_type=1): 无需 token 验证

### 3. 响应处理

建议按以下优先级检查响应：
1. 检查 HTTP 状态码（应为 200）
2. 检查 `response_status_code` 字段（200 表示成功）
3. 失败时读取 `msg` 字段获取错误信息

### 4. 超时设置

建议设置合理的超时时间：
- 连接超时: 5秒
- 读取超时: 30秒

### 5. 重试策略

对于以下错误码，建议实现自动重试：
- 1003: 请求失败（最多重试 3 次）
- 2002: Token 过期（自动刷新 token 后重试）

---

## 相关文档

- [GetUserInfo 详细接口文档](./GET_USER_INFO_API.md)
- [认证服务 HTTP API](./AUTH_SERVICE_HTTP_API.md)
- [gRPC 调用指南](./GRPC_CALLING_GUIDE.md)

---

## 更新日志

| 日期 | 版本 | 说明 |
|------|------|------|
| 2026-01-29 | v1.0 | UserService 新增 HTTP/REST API 支持 |

## 技术支持

如有疑问，请联系后端开发团队。
