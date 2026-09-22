# 用户登录接口完整指南

## 重要提示 ⚠️

**登录成功后，响应中已包含完整的用户信息（UserInfo），前端无需再次调用 GetUserInfo 接口！**

所有登录方式（手机号、Apple、Google、邮箱、游客）的响应都包含：
- ✅ access_token（访问令牌）
- ✅ refresh_token（刷新令牌）
- ✅ **user_info（完整的用户信息）**
- ✅ is_first_login（是否首次登录）
- ✅ auth_type（认证类型）

---

## 统一的登录响应结构

### AuthResponse

```json
{
  "response_header": {
    "req_id": "unique-request-id",
    "msg": "Success",
    "server_time": "2024-01-29 10:00:00",
    "response_status_code": 200
  },
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "expire_time": "2024-01-30 10:00:00",
  "refresh_token": "refresh_token_string",
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
      "pop_subscribe": true,
      "pop_free_daily_score": true,
      "free_daily_credit": 10,
      "should_show": true,
      "home_popup": {
        "title": "欢迎使用",
        "content": "...",
        "button_text": "立即体验"
      },
      "float_tool_bar_tool_id": "tool-001"
    },
    "subscribe_info": {
      "product_id": "monthly_vip",
      "status": 1,
      "subscribe_expired_time": "2024-02-29 23:59:59",
      "subscribe_level": 1,
      "credit": 1000
    }
  },
  "is_first_login": true,
  "auth_type": 1
}
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| access_token | string | 访问令牌，后续请求需携带 |
| expire_time | string | access_token 过期时间 |
| refresh_token | string | 刷新令牌，用于获取新的 access_token |
| **user_info** | **UserInfo** | **⭐ 完整的用户信息，无需再次请求** |
| is_first_login | bool | 是否首次登录 |
| auth_type | int | 认证类型：1-手机号，2-Apple，3-Google，5-邮箱，6-游客 |

### UserInfo 完整结构

| 字段 | 类型 | 说明 |
|------|------|------|
| user_id | string | 用户ID |
| user_name | string | 用户名 |
| phone_number | string | 手机号（格式：+区号+手机号） |
| avatar | string | 头像URL |
| gender | int | 0-未知，1-男，2-女，3-无性别，4-保密 |
| birthday | string | 生日 |
| nickname | string | 昵称 |
| register_time | string | 注册时间 |
| last_login_time | string | 最后登录时间 |
| user_config | UserConfig | 用户配置（含弹窗信息） |
| subscribe_info | SubscribeInfo | 订阅信息 |

---

## 支持的登录方式

### 1. 游客登录

最简单的登录方式，无需任何凭证。

#### HTTP 接口

```
POST /v1/auth/guest_login
```

#### 请求示例

```json
{
  "request_header": {
    "req_id": "req-001",
    "request_time_ms": 1706515200000,
    "device": {
      "os": 1,
      "device_id": "device-unique-id",
      "language": "zh-CN"
    },
    "app": {
      "package_name": "com.example.app",
      "version": "1.0.0"
    }
  }
}
```

#### 响应示例

```json
{
  "response_header": {
    "req_id": "req-001",
    "msg": "Success",
    "response_status_code": 200
  },
  "access_token": "guest_access_token",
  "expire_time": "2024-01-30 10:00:00",
  "refresh_token": "guest_refresh_token",
  "user_info": {
    "user_id": "temp-user-123",
    "user_name": "",
    "nickname": "游客",
    "user_config": {
      "pop_subscribe": true,
      "free_daily_credit": 5
    },
    "subscribe_info": {
      "status": 0,
      "credit": 5
    }
  },
  "is_first_login": true,
  "auth_type": 6
}
```

---

### 2. 手机号登录

#### 步骤 1: 发送验证码

```
POST /v1/auth/send_phone_verify_code
```

**请求：**
```json
{
  "request_header": {
    "req_id": "req-001",
    "request_time_ms": 1706515200000
  },
  "phone_number": "+8613800138000"
}
```

**响应：**
```json
{
  "response_header": {
    "req_id": "req-001",
    "msg": "验证码已发送",
    "response_status_code": 200
  }
}
```

#### 步骤 2: 验证码登录

```
POST /v1/auth/phone_auth
```

**请求：**
```json
{
  "request_header": {
    "req_id": "req-002",
    "request_time_ms": 1706515200000,
    "device": {
      "os": 1,
      "language": "zh-CN"
    }
  },
  "phone_number": "+8613800138000",
  "verify_code": "123456"
}
```

**响应：** 返回完整的 AuthResponse（包含 user_info）

---

### 3. Apple ID 登录

```
POST /v1/auth/apple_auth
```

#### 请求示例

```json
{
  "request_header": {
    "req_id": "req-003",
    "request_time_ms": 1706515200000,
    "device": {
      "os": 2,
      "language": "zh-CN"
    }
  },
  "apple_id_token": "apple_id_token_string",
  "apple_authorization_code": "authorization_code_string"
}
```

#### 响应示例

返回完整的 AuthResponse，包含用户信息。

---

### 4. Google 账号登录

```
POST /v1/auth/google_auth
```

#### 请求示例

```json
{
  "request_header": {
    "req_id": "req-004",
    "request_time_ms": 1706515200000
  },
  "google_id_token": "google_id_token_string"
}
```

#### 响应示例

返回完整的 AuthResponse，包含用户信息。

---

### 5. 邮箱验证码登录

#### 步骤 1: 发送邮箱验证码

```
POST /v1/auth/send_email_verify_code
```

**请求：**
```json
{
  "request_header": {
    "req_id": "req-005",
    "request_time_ms": 1706515200000
  },
  "email": "user@example.com"
}
```

#### 步骤 2: 验证码登录

```
POST /v1/auth/email_auth
```

**请求：**
```json
{
  "request_header": {
    "req_id": "req-006",
    "request_time_ms": 1706515200000
  },
  "email": "user@example.com",
  "verify_code": "123456"
}
```

**响应：** 返回完整的 AuthResponse（包含 user_info）

---

### 6. 邮箱密码登录

```
POST /v1/auth/email_password_auth
```

#### 请求示例

```json
{
  "request_header": {
    "req_id": "req-007",
    "request_time_ms": 1706515200000
  },
  "email": "user@example.com",
  "password": "SecurePassword123!"
}
```

#### 响应示例

返回完整的 AuthResponse，包含用户信息。

---

### 7. 邮箱注册

```
POST /v1/auth/email_register
```

#### 请求示例

```json
{
  "request_header": {
    "req_id": "req-008",
    "request_time_ms": 1706515200000
  },
  "email": "newuser@example.com",
  "verify_code": "123456",
  "password": "SecurePassword123!"
}
```

#### 响应示例

```json
{
  "response_header": {
    "req_id": "req-008",
    "msg": "注册成功",
    "response_status_code": 200
  },
  "need_set_password": false
}
```

---

## 前端实现建议

### 登录流程最佳实践

```javascript
// 登录成功后的处理
async function handleLogin(loginResponse) {
  const { access_token, refresh_token, user_info, is_first_login } = loginResponse;

  // 1. 保存 token
  localStorage.setItem('access_token', access_token);
  localStorage.setItem('refresh_token', refresh_token);

  // 2. 保存用户信息（无需再次请求）
  localStorage.setItem('user_info', JSON.stringify(user_info));

  // 3. 根据是否首次登录决定跳转
  if (is_first_login) {
    // 首次登录，跳转到新手引导
    router.push('/onboarding');
  } else {
    // 老用户，直接进入主页
    router.push('/home');
  }

  // 4. 检查是否需要弹出订阅页面
  if (user_info.user_config.pop_subscribe) {
    showSubscriptionModal();
  }

  // 5. 检查首页弹窗
  if (user_info.user_config.should_show && user_info.user_config.home_popup) {
    showHomePopup(user_info.user_config.home_popup);
  }
}
```

### 完整的登录示例（TypeScript + Axios）

```typescript
import axios from 'axios';

interface AuthResponse {
  response_header: {
    response_status_code: number;
    msg: string;
  };
  access_token: string;
  refresh_token: string;
  user_info: UserInfo;
  is_first_login: boolean;
  auth_type: number;
}

interface UserInfo {
  user_id: string;
  user_name: string;
  phone_number: string;
  avatar: string;
  nickname: string;
  user_config: UserConfig;
  subscribe_info: SubscribeInfo;
  // ... 其他字段
}

// 游客登录
async function guestLogin() {
  try {
    const response = await axios.post<AuthResponse>(
      'https://api.example.com/v1/auth/guest_login',
      {
        request_header: {
          req_id: `req-${Date.now()}`,
          request_time_ms: Date.now(),
          device: {
            os: 1,
            device_id: getDeviceId(),
            language: navigator.language
          },
          app: {
            package_name: 'com.example.web',
            version: '1.0.0'
          }
        }
      }
    );

    if (response.data.response_header.response_status_code === 200) {
      // ✅ 直接使用响应中的 user_info，无需再次请求
      await handleLogin(response.data);
      return response.data;
    } else {
      throw new Error(response.data.response_header.msg);
    }
  } catch (error) {
    console.error('游客登录失败:', error);
    throw error;
  }
}

// 手机号登录
async function phoneLogin(phoneNumber: string, verifyCode: string) {
  try {
    const response = await axios.post<AuthResponse>(
      'https://api.example.com/v1/auth/phone_auth',
      {
        request_header: {
          req_id: `req-${Date.now()}`,
          request_time_ms: Date.now(),
          device: {
            os: 1,
            language: navigator.language
          }
        },
        phone_number: phoneNumber,
        verify_code: verifyCode
      }
    );

    if (response.data.response_header.response_status_code === 200) {
      // ✅ 响应中已包含完整的用户信息
      await handleLogin(response.data);
      return response.data;
    } else {
      throw new Error(response.data.response_header.msg);
    }
  } catch (error) {
    console.error('手机号登录失败:', error);
    throw error;
  }
}

// 邮箱密码登录
async function emailPasswordLogin(email: string, password: string) {
  try {
    const response = await axios.post<AuthResponse>(
      'https://api.example.com/v1/auth/email_password_auth',
      {
        request_header: {
          req_id: `req-${Date.now()}`,
          request_time_ms: Date.now()
        },
        email: email,
        password: password
      }
    );

    if (response.data.response_header.response_status_code === 200) {
      // ✅ 响应中已包含完整的用户信息
      await handleLogin(response.data);
      return response.data;
    } else {
      throw new Error(response.data.response_header.msg);
    }
  } catch (error) {
    console.error('邮箱登录失败:', error);
    throw error;
  }
}
```

---

## 何时需要调用 GetUserInfo？

虽然登录响应已包含用户信息，但在以下场景仍需调用 GetUserInfo：

### ✅ 需要调用的场景

1. **用户修改了资料后**
   - 修改了昵称、头像、性别等
   - 需要获取最新的用户信息

2. **页面刷新或重新进入应用**
   - 用户已登录，但本地缓存可能过期
   - 建议获取最新的用户信息

3. **订阅状态可能变化**
   - 用户完成了订阅购买
   - 需要刷新订阅状态

4. **定期刷新**
   - 长时间使用应用后
   - 建议定期刷新用户信息（如每小时一次）

### ❌ 不需要调用的场景

1. **刚登录成功后**
   - 登录响应已包含最新的用户信息

2. **每次打开页面时**
   - 如果本地缓存的用户信息仍然有效

---

## Token 管理

### Access Token 过期处理

```javascript
// 设置 axios 拦截器
axios.interceptors.response.use(
  response => response,
  async error => {
    const originalRequest = error.config;

    // Token 过期（错误码 2002）
    if (error.response?.data?.response_header?.response_status_code === 2002
        && !originalRequest._retry) {
      originalRequest._retry = true;

      try {
        // 刷新 token
        const refreshToken = localStorage.getItem('refresh_token');
        const newToken = await refreshAccessToken(refreshToken);

        // 更新 token
        localStorage.setItem('access_token', newToken);
        originalRequest.headers['Authorization'] = `Bearer ${newToken}`;

        // 重试原请求
        return axios(originalRequest);
      } catch (refreshError) {
        // 刷新失败，跳转到登录页
        router.push('/login');
        return Promise.reject(refreshError);
      }
    }

    return Promise.reject(error);
  }
);
```

### 刷新 Token

```
POST /v1/auth/refresh_token
```

```json
{
  "request_header": {
    "req_id": "req-009",
    "request_time_ms": 1706515200000
  },
  "refresh_token": "your_refresh_token"
}
```

---

## 常见错误码

### 认证相关

| 错误码 | 说明 | 处理建议 |
|--------|------|----------|
| 4001 | 无效手机号 | 检查手机号格式 |
| 4002 | 验证码错误 | 提示用户重新输入 |
| 4017 | 邮箱格式无效 | 检查邮箱格式 |
| 4018 | 邮箱或密码错误 | 提示用户重新输入 |
| 4019 | 邮箱已注册 | 提示用户直接登录 |
| 4020 | 邮箱未注册 | 引导用户注册 |
| 4021 | 密码格式不符 | 提示密码要求 |
| 4022 | 邮箱验证码错误 | 提示重新输入 |
| 4023 | 验证码过期 | 提示重新发送 |
| 4024 | 未设置密码 | 引导使用验证码登录 |

---

## 安全建议

1. **HTTPS**: 生产环境必须使用 HTTPS
2. **Token 存储**: 建议使用 httpOnly cookie 或 secure storage
3. **敏感信息**: 不要在前端日志中打印 token 和密码
4. **Token 刷新**: 在 token 即将过期时主动刷新
5. **退出登录**: 清除本地所有 token 和用户信息

---

## 相关文档

- [UserService HTTP API](./USER_SERVICE_HTTP_API.md)
- [GetUserInfo 接口文档](./GET_USER_INFO_API.md)
- [Web端请求头规范](./WEB_REQUEST_HEADER_GUIDE.md)
- [错误码对照表](./EMAIL_ERROR_CODES_MIGRATION.md)

---

## 更新日志

| 日期 | 版本 | 说明 |
|------|------|------|
| 2026-01-29 | v1.0 | 创建登录接口完整指南 |
