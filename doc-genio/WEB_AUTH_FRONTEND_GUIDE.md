# Web端认证接入指南

## 概述

本文档为前端开发者提供完整的认证功能接入指南，包括：
- Google登录
- 邮箱注册
- 邮箱验证码登录
- 邮箱密码登录

**API Base URL**: `http://your-server-domain:8081` (默认HTTP端口)

---

## 目录

1. [通用说明](#通用说明)
2. [Google登录](#google登录)
3. [邮箱注册](#邮箱注册)
4. [邮箱登录](#邮箱登录)
5. [Token管理](#token管理)
6. [错误处理](#错误处理)
7. [完整示例代码](#完整示例代码)

---

## 通用说明

### 请求Header结构

所有API请求都需要包含 `request_header` 字段：

```typescript
interface RequestHeader {
  req_id: string;              // 请求唯一ID，建议使用UUID
  access_token?: string;       // 登录后携带，登录接口不需要
  app: {
    app_version: string;       // 应用版本，如 "1.0.0"
    package_name: string;      // 包名，如 "com.example.web"
  };
  device: {
    os: string;                // 固定为 "web"
    language: string;          // 语言代码，如 "zh-CN", "en-US"
  };
  user_id?: string;            // 登录后携带，登录接口不需要
  request_time_ms: number;     // 请求时间戳（毫秒）
}
```

### 响应Header结构

所有API响应都包含 `response_header` 字段：

```typescript
interface ResponseHeader {
  req_id: string;                    // 对应请求ID
  msg: string;                       // 消息描述
  response_status_code: number;      // 状态码：200=成功
  server_time: string;               // 服务器时间（ISO 8601格式）
  user_id?: string;                  // 用户ID
}
```

### 通用响应状态码

| 状态码 | 说明 |
|--------|------|
| 200 | 成功 |
| 1001 | 无效参数 |
| 1002 | 无效请求 |
| 1003 | 请求失败 |
| 2001 | 无效Access Token |
| 2002 | Access Token过期 |
| 2003 | 无效Refresh Token |
| 2004 | 无效用户ID |
| 4002 | 验证码错误 |
| 4011 | 用户名已存在 |

### 生成请求ID示例

```javascript
function generateRequestId() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
    const r = Math.random() * 16 | 0;
    const v = c === 'x' ? r : (r & 0x3 | 0x8);
    return v.toString(16);
  });
}
```

---

## Google登录

### 1. 前端集成Google登录按钮

首先在HTML中引入Google登录SDK：

```html
<!DOCTYPE html>
<html>
<head>
  <script src="https://accounts.google.com/gsi/client" async defer></script>
</head>
<body>
  <!-- Google登录按钮 -->
  <div id="g_id_onload"
       data-client_id="YOUR_GOOGLE_CLIENT_ID"
       data-callback="handleGoogleSignIn">
  </div>
  <div class="g_id_signin" data-type="standard"></div>
</body>
</html>
```

### 2. 处理Google登录回调

```javascript
// Google登录回调函数
async function handleGoogleSignIn(response) {
  const googleIdToken = response.credential;

  try {
    // 调用后端API
    const result = await loginWithGoogle(googleIdToken);

    // 保存token和用户信息
    localStorage.setItem('access_token', result.access_token);
    localStorage.setItem('refresh_token', result.refresh_token);
    localStorage.setItem('user_info', JSON.stringify(result.user_info));

    // 跳转到主页面
    window.location.href = '/home';
  } catch (error) {
    console.error('Google登录失败:', error);
    alert('登录失败，请重试');
  }
}
```

### 3. 调用后端Google登录API

**接口**: `POST /v1/auth/google_auth`

**请求示例**:

```javascript
async function loginWithGoogle(googleIdToken) {
  const requestBody = {
    request_header: {
      req_id: generateRequestId(),
      app: {
        app_version: "1.0.0",
        package_name: "com.example.web"
      },
      device: {
        os: "web",
        language: navigator.language || "zh-CN"
      },
      request_time_ms: Date.now()
    },
    google_id_token: googleIdToken  // Google提供的ID Token
  };

  const response = await fetch('http://your-server:8081/v1/auth/google_auth', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(requestBody)
  });

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }

  const data = await response.json();

  // 检查业务状态码
  if (data.response_header.response_status_code !== 200) {
    throw new Error(data.response_header.msg);
  }

  return data;
}
```

**成功响应示例**:

```json
{
  "response_header": {
    "req_id": "abc123",
    "msg": "success",
    "response_status_code": 200,
    "server_time": "2024-01-28T10:30:00Z",
    "user_id": "google_user_12345678"
  },
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expire_time": "2024-01-28T11:30:00Z",
  "refresh_token": "refresh_token_string",
  "user_info": {
    "user_id": "google_user_12345678",
    "user_name": "user@gmail.com",
    "avatar": "https://lh3.googleusercontent.com/...",
    "nickname": "张三",
    "register_time": "2024-01-28T10:30:00Z"
  },
  "auth_type": 3
}
```

### 4. TypeScript类型定义

```typescript
interface GoogleAuthRequest {
  request_header: RequestHeader;
  google_id_token: string;
}

interface AuthResponse {
  response_header: ResponseHeader;
  access_token: string;
  expire_time: string;
  refresh_token: string;
  user_info: {
    user_id: string;
    user_name: string;
    avatar: string;
    gender?: number;
    nickname: string;
    register_time: string;
  };
  auth_type: number;  // 3 = Google登录
}
```

---

## 邮箱注册

邮箱注册分为两步：
1. 发送验证码到邮箱
2. 提交注册信息（邮箱、验证码、密码）

### 步骤1: 发送邮箱验证码

**接口**: `POST /v1/auth/send_email_verify_code`

**请求示例**:

```javascript
async function sendEmailVerifyCode(email) {
  const requestBody = {
    request_header: {
      req_id: generateRequestId(),
      app: {
        app_version: "1.0.0",
        package_name: "com.example.web"
      },
      device: {
        os: "web",
        language: navigator.language || "zh-CN"
      },
      request_time_ms: Date.now()
    },
    email: email  // 用户输入的邮箱地址
  };

  const response = await fetch('http://your-server:8081/v1/auth/send_email_verify_code', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(requestBody)
  });

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }

  const data = await response.json();

  if (data.response_header.response_status_code !== 200) {
    throw new Error(data.response_header.msg);
  }

  return data;
}
```

**成功响应示例**:

```json
{
  "response_header": {
    "req_id": "def456",
    "msg": "success",
    "response_status_code": 200,
    "server_time": "2024-01-28T10:30:00Z"
  }
}
```

**注意事项**:
- 验证码有效期为5分钟
- 同一邮箱60秒内只能发送一次验证码
- 验证码为6位数字

### 步骤2: 提交注册信息

**接口**: `POST /v1/auth/email_register`

**请求示例**:

```javascript
async function registerWithEmail(email, verifyCode, password, confirmPassword) {
  const requestBody = {
    request_header: {
      req_id: generateRequestId(),
      app: {
        app_version: "1.0.0",
        package_name: "com.example.web"
      },
      device: {
        os: "web",
        language: navigator.language || "zh-CN"
      },
      request_time_ms: Date.now()
    },
    email: email,
    verify_code: verifyCode,
    password: password,
    confirm_password: confirmPassword
  };

  const response = await fetch('http://your-server:8081/v1/auth/email_register', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(requestBody)
  });

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }

  const data = await response.json();

  if (data.response_header.response_status_code !== 200) {
    throw new Error(data.response_header.msg);
  }

  return data;
}
```

**成功响应示例**:

```json
{
  "response_header": {
    "req_id": "ghi789",
    "msg": "success",
    "response_status_code": 200,
    "server_time": "2024-01-28T10:30:00Z"
  },
  "need_set_password": false
}
```

**密码规则**:
- 长度: 6-20个字符
- 必须与确认密码一致

### 完整注册流程示例

```javascript
// HTML表单示例
function renderRegisterForm() {
  return `
    <form id="registerForm">
      <input type="email" id="email" placeholder="请输入邮箱" required>
      <button type="button" onclick="sendCode()">发送验证码</button>
      <span id="countdown"></span>

      <input type="text" id="verifyCode" placeholder="请输入验证码" maxlength="6" required>
      <input type="password" id="password" placeholder="请设置密码(6-20位)" minlength="6" maxlength="20" required>
      <input type="password" id="confirmPassword" placeholder="请确认密码" minlength="6" maxlength="20" required>

      <button type="submit">注册</button>
    </form>
  `;
}

// 发送验证码
let countdown = 0;
async function sendCode() {
  const email = document.getElementById('email').value;

  // 验证邮箱格式
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  if (!emailRegex.test(email)) {
    alert('请输入有效的邮箱地址');
    return;
  }

  // 防止重复发送
  if (countdown > 0) {
    return;
  }

  try {
    await sendEmailVerifyCode(email);
    alert('验证码已发送到您的邮箱，请查收');

    // 开始倒计时
    countdown = 60;
    const countdownElement = document.getElementById('countdown');
    const interval = setInterval(() => {
      countdown--;
      countdownElement.textContent = `${countdown}秒后可重发`;
      if (countdown <= 0) {
        clearInterval(interval);
        countdownElement.textContent = '';
      }
    }, 1000);
  } catch (error) {
    console.error('发送验证码失败:', error);
    alert('发送验证码失败: ' + error.message);
  }
}

// 提交注册
document.getElementById('registerForm').addEventListener('submit', async (e) => {
  e.preventDefault();

  const email = document.getElementById('email').value;
  const verifyCode = document.getElementById('verifyCode').value;
  const password = document.getElementById('password').value;
  const confirmPassword = document.getElementById('confirmPassword').value;

  // 前端验证
  if (password !== confirmPassword) {
    alert('两次输入的密码不一致');
    return;
  }

  if (password.length < 6 || password.length > 20) {
    alert('密码长度必须为6-20个字符');
    return;
  }

  if (verifyCode.length !== 6) {
    alert('请输入6位验证码');
    return;
  }

  try {
    await registerWithEmail(email, verifyCode, password, confirmPassword);
    alert('注册成功！请使用邮箱和密码登录');
    // 跳转到登录页面
    window.location.href = '/login';
  } catch (error) {
    console.error('注册失败:', error);
    alert('注册失败: ' + error.message);
  }
});
```

---

## 邮箱登录

支持两种邮箱登录方式：
1. 邮箱验证码登录（无需密码）
2. 邮箱密码登录

### 方式1: 邮箱验证码登录

适用于忘记密码或快速登录场景。

**步骤1**: 发送验证码（同注册流程，调用 `/v1/auth/send_email_verify_code`）

**步骤2**: 验证码登录

**接口**: `POST /v1/auth/email_auth`

```javascript
async function loginWithEmailCode(email, verifyCode) {
  const requestBody = {
    request_header: {
      req_id: generateRequestId(),
      app: {
        app_version: "1.0.0",
        package_name: "com.example.web"
      },
      device: {
        os: "web",
        language: navigator.language || "zh-CN"
      },
      request_time_ms: Date.now()
    },
    email: email,
    verify_code: verifyCode
  };

  const response = await fetch('http://your-server:8081/v1/auth/email_auth', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(requestBody)
  });

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }

  const data = await response.json();

  if (data.response_header.response_status_code !== 200) {
    throw new Error(data.response_header.msg);
  }

  return data;  // 返回AuthResponse，包含access_token和user_info
}
```

**成功响应**: 同Google登录的 `AuthResponse` 格式

### 方式2: 邮箱密码登录

适用于已注册用户的常规登录。

**接口**: `POST /v1/auth/email_password_auth`

```javascript
async function loginWithEmailPassword(email, password) {
  const requestBody = {
    request_header: {
      req_id: generateRequestId(),
      app: {
        app_version: "1.0.0",
        package_name: "com.example.web"
      },
      device: {
        os: "web",
        language: navigator.language || "zh-CN"
      },
      request_time_ms: Date.now()
    },
    email: email,
    password: password
  };

  const response = await fetch('http://your-server:8081/v1/auth/email_password_auth', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(requestBody)
  });

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }

  const data = await response.json();

  if (data.response_header.response_status_code !== 200) {
    throw new Error(data.response_header.msg);
  }

  return data;  // 返回AuthResponse
}
```

**成功响应**: 同Google登录的 `AuthResponse` 格式

### 登录页面完整示例

```javascript
// HTML表单
function renderLoginForm() {
  return `
    <div>
      <h2>登录</h2>

      <!-- 密码登录 -->
      <form id="passwordLoginForm">
        <h3>密码登录</h3>
        <input type="email" id="emailPassword" placeholder="邮箱" required>
        <input type="password" id="password" placeholder="密码" required>
        <button type="submit">登录</button>
      </form>

      <hr>

      <!-- 验证码登录 -->
      <form id="codeLoginForm">
        <h3>验证码登录</h3>
        <input type="email" id="emailCode" placeholder="邮箱" required>
        <button type="button" onclick="sendLoginCode()">发送验证码</button>
        <span id="loginCountdown"></span>
        <input type="text" id="loginVerifyCode" placeholder="验证码" maxlength="6" required>
        <button type="submit">登录</button>
      </form>

      <hr>

      <!-- Google登录 -->
      <div id="g_id_onload"
           data-client_id="YOUR_GOOGLE_CLIENT_ID"
           data-callback="handleGoogleSignIn">
      </div>
      <div class="g_id_signin" data-type="standard"></div>
    </div>
  `;
}

// 密码登录处理
document.getElementById('passwordLoginForm').addEventListener('submit', async (e) => {
  e.preventDefault();

  const email = document.getElementById('emailPassword').value;
  const password = document.getElementById('password').value;

  try {
    const result = await loginWithEmailPassword(email, password);

    // 保存token和用户信息
    saveAuthData(result);

    // 跳转到主页面
    window.location.href = '/home';
  } catch (error) {
    console.error('登录失败:', error);
    alert('登录失败: ' + error.message);
  }
});

// 验证码登录处理
document.getElementById('codeLoginForm').addEventListener('submit', async (e) => {
  e.preventDefault();

  const email = document.getElementById('emailCode').value;
  const verifyCode = document.getElementById('loginVerifyCode').value;

  try {
    const result = await loginWithEmailCode(email, verifyCode);

    // 保存token和用户信息
    saveAuthData(result);

    // 跳转到主页面
    window.location.href = '/home';
  } catch (error) {
    console.error('登录失败:', error);
    alert('登录失败: ' + error.message);
  }
});

// 保存认证数据
function saveAuthData(authResponse) {
  localStorage.setItem('access_token', authResponse.access_token);
  localStorage.setItem('refresh_token', authResponse.refresh_token);
  localStorage.setItem('user_info', JSON.stringify(authResponse.user_info));
  localStorage.setItem('token_expire_time', authResponse.expire_time);
}
```

---

## Token管理

### Token使用

登录成功后，后续所有API请求都需要在 `request_header` 中携带 `access_token` 和 `user_id`：

```javascript
async function callProtectedAPI(endpoint, data) {
  const accessToken = localStorage.getItem('access_token');
  const userInfo = JSON.parse(localStorage.getItem('user_info'));

  if (!accessToken || !userInfo) {
    // 未登录，跳转到登录页
    window.location.href = '/login';
    return;
  }

  const requestBody = {
    request_header: {
      req_id: generateRequestId(),
      access_token: accessToken,
      user_id: userInfo.user_id,
      app: {
        app_version: "1.0.0",
        package_name: "com.example.web"
      },
      device: {
        os: "web",
        language: navigator.language || "zh-CN"
      },
      request_time_ms: Date.now()
    },
    ...data  // 业务数据
  };

  const response = await fetch(`http://your-server:8081${endpoint}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(requestBody)
  });

  const result = await response.json();

  // 处理token过期
  if (result.response_header.response_status_code === 2002) {
    // Access Token过期，尝试刷新
    await refreshAccessToken();
    // 重新调用API
    return callProtectedAPI(endpoint, data);
  }

  return result;
}
```

### Token刷新

当 `access_token` 过期时，使用 `refresh_token` 获取新的token。

**接口**: `POST /v1/auth/refresh_token`

```javascript
async function refreshAccessToken() {
  const refreshToken = localStorage.getItem('refresh_token');
  const userInfo = JSON.parse(localStorage.getItem('user_info'));

  if (!refreshToken || !userInfo) {
    // 没有refresh token，需要重新登录
    window.location.href = '/login';
    throw new Error('需要重新登录');
  }

  const requestBody = {
    request_header: {
      req_id: generateRequestId(),
      user_id: userInfo.user_id,
      app: {
        app_version: "1.0.0",
        package_name: "com.example.web"
      },
      device: {
        os: "web",
        language: navigator.language || "zh-CN"
      },
      request_time_ms: Date.now()
    },
    refresh_token: refreshToken
  };

  const response = await fetch('http://your-server:8081/v1/auth/refresh_token', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(requestBody)
  });

  if (!response.ok) {
    // 刷新失败，需要重新登录
    localStorage.clear();
    window.location.href = '/login';
    throw new Error('Token刷新失败，请重新登录');
  }

  const data = await response.json();

  if (data.response_header.response_status_code !== 200) {
    // 刷新失败，需要重新登录
    localStorage.clear();
    window.location.href = '/login';
    throw new Error('Token刷新失败，请重新登录');
  }

  // 更新access token
  localStorage.setItem('access_token', data.access_token);
  localStorage.setItem('token_expire_time', data.expire_time);

  return data;
}
```

**成功响应示例**:

```json
{
  "response_header": {
    "req_id": "jkl012",
    "msg": "success",
    "response_status_code": 200,
    "server_time": "2024-01-28T11:30:00Z",
    "user_id": "google_user_12345678"
  },
  "access_token": "new_access_token_here",
  "expire_time": "2024-01-28T12:30:00Z"
}
```

### 登出

**接口**: `POST /v1/auth/logout`

```javascript
async function logout() {
  const accessToken = localStorage.getItem('access_token');
  const userInfo = JSON.parse(localStorage.getItem('user_info'));

  if (accessToken && userInfo) {
    try {
      const requestBody = {
        request_header: {
          req_id: generateRequestId(),
          access_token: accessToken,
          user_id: userInfo.user_id,
          app: {
            app_version: "1.0.0",
            package_name: "com.example.web"
          },
          device: {
            os: "web",
            language: navigator.language || "zh-CN"
          },
          request_time_ms: Date.now()
        }
      };

      await fetch('http://your-server:8081/v1/auth/logout', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(requestBody)
      });
    } catch (error) {
      console.error('登出API调用失败:', error);
    }
  }

  // 清空本地存储
  localStorage.clear();

  // 跳转到登录页
  window.location.href = '/login';
}
```

---

## 错误处理

### 错误响应格式

所有错误都通过 `response_status_code` 和 `msg` 字段返回：

```json
{
  "response_header": {
    "req_id": "abc123",
    "msg": "验证码错误",
    "response_status_code": 4002,
    "server_time": "2024-01-28T10:30:00Z"
  }
}
```

### 常见错误处理

```javascript
function handleAPIError(responseHeader) {
  const code = responseHeader.response_status_code;
  const msg = responseHeader.msg;

  switch (code) {
    case 200:
      return null;  // 成功

    case 1001:
      return '请求参数错误，请检查输入';

    case 2001:
    case 2002:
      // Token无效或过期
      return '登录已过期，请重新登录';

    case 4002:
      return '验证码错误或已过期';

    case 4011:
      return '该邮箱已被注册';

    default:
      return msg || '请求失败，请稍后重试';
  }
}

// 使用示例
async function safeAPICall(apiFunction, ...args) {
  try {
    const result = await apiFunction(...args);

    const errorMsg = handleAPIError(result.response_header);
    if (errorMsg) {
      alert(errorMsg);

      // 如果是认证错误，跳转到登录页
      const code = result.response_header.response_status_code;
      if (code === 2001 || code === 2002 || code === 2004) {
        localStorage.clear();
        window.location.href = '/login';
      }

      return null;
    }

    return result;
  } catch (error) {
    console.error('API调用失败:', error);
    alert('网络错误，请检查网络连接');
    return null;
  }
}
```

---

## 完整示例代码

### 认证工具类 (auth.js)

```javascript
// ==================== 配置 ====================
const API_BASE_URL = 'http://your-server:8081';
const APP_VERSION = '1.0.0';
const PACKAGE_NAME = 'com.example.web';

// ==================== 工具函数 ====================
function generateRequestId() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
    const r = Math.random() * 16 | 0;
    const v = c === 'x' ? r : (r & 0x3 | 0x8);
    return v.toString(16);
  });
}

function createRequestHeader(includeAuth = false) {
  const header = {
    req_id: generateRequestId(),
    app: {
      app_version: APP_VERSION,
      package_name: PACKAGE_NAME
    },
    device: {
      os: "web",
      language: navigator.language || "zh-CN"
    },
    request_time_ms: Date.now()
  };

  if (includeAuth) {
    const accessToken = localStorage.getItem('access_token');
    const userInfo = JSON.parse(localStorage.getItem('user_info') || '{}');

    if (accessToken) header.access_token = accessToken;
    if (userInfo.user_id) header.user_id = userInfo.user_id;
  }

  return header;
}

async function apiRequest(endpoint, data, includeAuth = false) {
  const requestBody = {
    request_header: createRequestHeader(includeAuth),
    ...data
  };

  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(requestBody)
  });

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`);
  }

  const result = await response.json();

  // 检查业务状态码
  if (result.response_header.response_status_code !== 200) {
    throw new Error(result.response_header.msg || '请求失败');
  }

  return result;
}

// ==================== 认证API ====================
const AuthAPI = {
  // Google登录
  async loginWithGoogle(googleIdToken) {
    const result = await apiRequest('/v1/auth/google_auth', {
      google_id_token: googleIdToken
    });

    this.saveAuthData(result);
    return result;
  },

  // 发送邮箱验证码
  async sendEmailVerifyCode(email) {
    return await apiRequest('/v1/auth/send_email_verify_code', {
      email: email
    });
  },

  // 邮箱注册
  async registerWithEmail(email, verifyCode, password, confirmPassword) {
    return await apiRequest('/v1/auth/email_register', {
      email: email,
      verify_code: verifyCode,
      password: password,
      confirm_password: confirmPassword
    });
  },

  // 邮箱验证码登录
  async loginWithEmailCode(email, verifyCode) {
    const result = await apiRequest('/v1/auth/email_auth', {
      email: email,
      verify_code: verifyCode
    });

    this.saveAuthData(result);
    return result;
  },

  // 邮箱密码登录
  async loginWithEmailPassword(email, password) {
    const result = await apiRequest('/v1/auth/email_password_auth', {
      email: email,
      password: password
    });

    this.saveAuthData(result);
    return result;
  },

  // 刷新Token
  async refreshAccessToken() {
    const refreshToken = localStorage.getItem('refresh_token');
    if (!refreshToken) {
      throw new Error('没有refresh token');
    }

    const result = await apiRequest('/v1/auth/refresh_token', {
      refresh_token: refreshToken
    }, true);

    // 更新access token
    localStorage.setItem('access_token', result.access_token);
    localStorage.setItem('token_expire_time', result.expire_time);

    return result;
  },

  // 登出
  async logout() {
    try {
      await apiRequest('/v1/auth/logout', {}, true);
    } catch (error) {
      console.error('登出API调用失败:', error);
    }

    this.clearAuthData();
  },

  // 保存认证数据
  saveAuthData(authResponse) {
    localStorage.setItem('access_token', authResponse.access_token);
    localStorage.setItem('refresh_token', authResponse.refresh_token);
    localStorage.setItem('user_info', JSON.stringify(authResponse.user_info));
    localStorage.setItem('token_expire_time', authResponse.expire_time);
  },

  // 清空认证数据
  clearAuthData() {
    localStorage.clear();
  },

  // 检查是否已登录
  isLoggedIn() {
    return !!localStorage.getItem('access_token');
  },

  // 获取用户信息
  getUserInfo() {
    const userInfoStr = localStorage.getItem('user_info');
    return userInfoStr ? JSON.parse(userInfoStr) : null;
  }
};

// ==================== 导出 ====================
export { AuthAPI, generateRequestId, createRequestHeader, apiRequest };
```

### 使用示例

```javascript
import { AuthAPI } from './auth.js';

// Google登录
async function handleGoogleLogin(googleIdToken) {
  try {
    await AuthAPI.loginWithGoogle(googleIdToken);
    alert('登录成功！');
    window.location.href = '/home';
  } catch (error) {
    alert('登录失败: ' + error.message);
  }
}

// 邮箱注册
async function handleRegister(email, verifyCode, password, confirmPassword) {
  try {
    // 1. 先发送验证码（用户点击发送按钮时）
    // await AuthAPI.sendEmailVerifyCode(email);

    // 2. 提交注册
    await AuthAPI.registerWithEmail(email, verifyCode, password, confirmPassword);
    alert('注册成功！请登录');
    window.location.href = '/login';
  } catch (error) {
    alert('注册失败: ' + error.message);
  }
}

// 邮箱密码登录
async function handleEmailLogin(email, password) {
  try {
    await AuthAPI.loginWithEmailPassword(email, password);
    alert('登录成功！');
    window.location.href = '/home';
  } catch (error) {
    alert('登录失败: ' + error.message);
  }
}

// 登出
async function handleLogout() {
  await AuthAPI.logout();
  window.location.href = '/login';
}

// 检查登录状态
function checkAuth() {
  if (!AuthAPI.isLoggedIn()) {
    window.location.href = '/login';
    return false;
  }
  return true;
}

// 获取用户信息
function displayUserInfo() {
  const userInfo = AuthAPI.getUserInfo();
  if (userInfo) {
    console.log('当前用户:', userInfo);
    document.getElementById('userName').textContent = userInfo.nickname || userInfo.user_name;
    document.getElementById('userAvatar').src = userInfo.avatar;
  }
}
```

---

## 最佳实践

### 1. 安全性

- ✅ **HTTPS**: 生产环境必须使用HTTPS
- ✅ **Token安全**: 不要将token暴露在URL中
- ✅ **密码规则**: 前端验证密码强度
- ✅ **验证码**: 限制发送频率，防止滥用
- ✅ **XSS防护**: 对用户输入进行转义

### 2. 用户体验

- ✅ **自动刷新Token**: 在token即将过期时自动刷新
- ✅ **加载状态**: 显示加载动画
- ✅ **错误提示**: 友好的错误信息
- ✅ **记住登录**: 使用localStorage持久化
- ✅ **倒计时**: 验证码发送后显示倒计时

### 3. 性能优化

- ✅ **请求去重**: 防止重复提交
- ✅ **缓存用户信息**: 减少API调用
- ✅ **按需加载**: Google SDK按需加载

### 4. 错误处理

```javascript
// 统一错误处理
window.addEventListener('unhandledrejection', function(event) {
  console.error('未处理的Promise错误:', event.reason);

  // 如果是认证错误，跳转到登录页
  if (event.reason?.message?.includes('登录')) {
    AuthAPI.clearAuthData();
    window.location.href = '/login';
  }
});
```

---

## 常见问题 FAQ

### Q1: Google登录按钮不显示？
**A**: 检查以下几点：
1. 确认已正确引入Google SDK
2. 确认`data-client_id`配置正确
3. 检查浏览器控制台是否有跨域错误
4. 确认域名已在Google Cloud Console中配置

### Q2: 验证码收不到？
**A**: 检查以下几点：
1. 邮箱地址是否正确
2. 检查垃圾邮件文件夹
3. 确认服务器SMTP配置正确
4. 查看服务器日志是否有发送失败记录

### Q3: Token刷新失败怎么办？
**A**: Token刷新失败通常意味着refresh token也过期了，需要用户重新登录。建议在refresh token即将过期前提醒用户。

### Q4: 如何实现"记住我"功能？
**A**: 可以使用`localStorage`（默认）或`sessionStorage`（关闭浏览器后失效）：
```javascript
// 记住我：使用localStorage（已实现）
// 不记住：使用sessionStorage
const storage = rememberMe ? localStorage : sessionStorage;
storage.setItem('access_token', token);
```

### Q5: 如何处理并发请求时的token过期？
**A**: 使用Promise缓存，确保同时只有一个刷新请求：
```javascript
let refreshPromise = null;

async function refreshAccessToken() {
  if (refreshPromise) {
    return refreshPromise;  // 返回正在进行的刷新请求
  }

  refreshPromise = AuthAPI.refreshAccessToken()
    .finally(() => {
      refreshPromise = null;  // 完成后清空
    });

  return refreshPromise;
}
```

---

## 附录

### TypeScript完整类型定义

```typescript
// types/auth.ts

export interface RequestHeader {
  req_id: string;
  access_token?: string;
  app: {
    app_version: string;
    package_name: string;
  };
  device: {
    os: string;
    android_id?: string;
    idfv?: string;
    language: string;
  };
  user_id?: string;
  user_type?: number;
  request_time_ms: number;
}

export interface ResponseHeader {
  req_id: string;
  msg: string;
  response_status_code: number;
  server_time: string;
  server_version?: string;
  response_time_ms?: number;
  user_id?: string;
}

export interface UserInfo {
  user_id: string;
  user_name: string;
  avatar: string;
  gender?: number;
  nickname: string;
  register_time: string;
}

export interface AuthResponse {
  response_header: ResponseHeader;
  access_token: string;
  expire_time: string;
  refresh_token: string;
  user_info: UserInfo;
  auth_type: number;
}

export interface GoogleAuthRequest {
  request_header: RequestHeader;
  google_id_token: string;
}

export interface EmailVerifyCodeRequest {
  request_header: RequestHeader;
  email: string;
}

export interface EmailRegisterRequest {
  request_header: RequestHeader;
  email: string;
  verify_code: string;
  password: string;
  confirm_password: string;
}

export interface EmailAuthRequest {
  request_header: RequestHeader;
  email: string;
  verify_code: string;
}

export interface EmailPasswordAuthRequest {
  request_header: RequestHeader;
  email: string;
  password: string;
}

export interface RefreshTokenRequest {
  request_header: RequestHeader;
  refresh_token: string;
}

export interface RefreshTokenResponse {
  response_header: ResponseHeader;
  access_token: string;
  expire_time: string;
}

export interface LogoutRequest {
  request_header: RequestHeader;
}

export interface BaseResponse {
  response_header: ResponseHeader;
}
```

---

## 联系支持

如有问题，请联系：
- 技术支持邮箱: support@example.com
- API文档: http://your-server:8081/docs
- 问题反馈: https://github.com/your-repo/issues

---

**文档版本**: v1.0.0
**最后更新**: 2024-01-28
**适用API版本**: v1
