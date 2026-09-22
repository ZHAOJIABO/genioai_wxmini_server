# 前端接口对接文档总览

## 📚 文档导航

欢迎使用 VA Vision AI Server API！本文档汇总了所有前端对接所需的接口文档。

---

## 🔑 认证相关（必读）

### [用户登录完整指南](./AUTH_LOGIN_GUIDE.md) ⭐ 推荐首先阅读

**重要提示**: 登录成功后的响应中已包含完整的用户信息，无需再次调用 GetUserInfo！

- ✅ 所有登录方式及示例
- ✅ 登录响应中包含完整 user_info
- ✅ Token 管理和刷新
- ✅ 前端最佳实践
- ✅ 何时需要调用 GetUserInfo

**支持的登录方式：**
- 游客登录
- 手机号验证码登录
- Apple ID 登录
- Google 账号登录
- 邮箱验证码登录
- 邮箱密码登录

---

## 👤 用户服务

### [UserService HTTP API](./USER_SERVICE_HTTP_API.md)

用户相关的所有 HTTP/REST 接口，包括：

- **GET** `/v1/user/get_user_info` - 获取用户信息
- **POST** `/v1/user/edit_user_name` - 修改用户名
- **POST** `/v1/user/delete_user` - 注销账号
- **POST** `/v1/user/notify_token` - 推送Token上报

### [GetUserInfo 详细文档](./GET_USER_INFO_API.md)

GetUserInfo 接口的详细说明：
- 完整的请求/响应结构
- gRPC 调用示例
- 业务逻辑说明

---

## 🌐 Web 端开发

### [Web 端请求头规范](./WEB_REQUEST_HEADER_GUIDE.md)

Web 端调用 API 时的请求头配置指南：
- RequestHeader 结构说明
- Web 客户端字段
- 浏览器信息字段
- 完整示例代码

### [Web 端认证前端指南](./WEB_AUTH_FRONTEND_GUIDE.md)

Web 端认证功能的前端实现指南：
- 邮箱注册/登录流程
- 表单验证规则
- 错误处理
- UI/UX 建议

---

## 🔄 项目迁移

### [Web 后端迁移指南](./BACKEND_WEB_MIGRATION_GUIDE.md)

后端如何支持 Web 端的技术方案：
- proto 文件修改
- 代码改动说明
- 测试验证

### [Web 迁移总结](./WEB_MIGRATION_SUMMARY.md)

Web 端支持的完整改动总结。

---

## 📋 错误码对照表

### [邮箱认证错误码](./EMAIL_AUTH_ERROR_CODES.md)

邮箱登录/注册相关的错误码说明：
- 邮箱格式错误
- 密码错误
- 验证码错误
- 处理建议

### [错误码迁移对照](./EMAIL_ERROR_CODES_MIGRATION.md)

新旧错误码的对照表，方便代码迁移。

### [前端错误码使用指南](./FRONTEND_EMAIL_ERROR_CODES.md)

前端如何处理各种错误码：
- 错误提示文案
- 用户引导流程
- 代码示例

---

## ☁️ 基础设施

### [Cloudflare R2 对象存储](./R2_SETUP.md)

图片、文件上传相关的配置说明：
- R2 配置
- 上传接口
- 访问方式

---

## 🚀 快速开始

### 第一次接入？请按以下顺序阅读：

1. **[用户登录完整指南](./AUTH_LOGIN_GUIDE.md)** ⭐
   - 了解所有登录方式
   - 理解登录响应包含用户信息
   - 掌握 Token 管理

2. **[Web 端请求头规范](./WEB_REQUEST_HEADER_GUIDE.md)**（Web 端）
   - 了解如何构造请求头
   - Web 端特定字段说明

3. **[UserService HTTP API](./USER_SERVICE_HTTP_API.md)**
   - 了解用户相关的其他接口
   - 学习 HTTP/REST 调用方式

4. **[错误码对照表](./EMAIL_AUTH_ERROR_CODES.md)**
   - 了解所有错误码及处理方式

---

## 📊 接口概览

### 认证服务 (AuthService)

| 接口 | HTTP 路径 | 说明 |
|------|-----------|------|
| 游客登录 | POST /v1/auth/guest_login | 无需凭证的临时登录 |
| 发送手机验证码 | POST /v1/auth/send_phone_verify_code | 获取手机验证码 |
| 手机号登录 | POST /v1/auth/phone_auth | 手机号+验证码登录 |
| Apple ID 登录 | POST /v1/auth/apple_auth | 苹果ID登录 |
| Google 登录 | POST /v1/auth/google_auth | 谷歌账号登录 |
| 发送邮箱验证码 | POST /v1/auth/send_email_verify_code | 获取邮箱验证码 |
| 邮箱验证码登录 | POST /v1/auth/email_auth | 邮箱+验证码登录 |
| 邮箱密码登录 | POST /v1/auth/email_password_auth | 邮箱+密码登录 |
| 邮箱注册 | POST /v1/auth/email_register | 邮箱注册 |
| 刷新Token | POST /v1/auth/refresh_token | 刷新访问令牌 |
| 退出登录 | POST /v1/auth/logout | 退出登录 |

### 用户服务 (UserService)

| 接口 | HTTP 路径 | 说明 |
|------|-----------|------|
| 获取用户信息 | POST /v1/user/get_user_info | 获取完整用户信息 |
| 修改用户名 | POST /v1/user/edit_user_name | 修改用户名 |
| 注销账号 | POST /v1/user/delete_user | 注销用户账号 |
| 推送Token上报 | POST /v1/user/notify_token | 上报FCM/APNs token |

---

## 🎯 常见场景

### 场景 1: 用户首次登录

```
1. 调用登录接口（任意一种）
2. 获取响应中的 user_info（包含完整信息）
3. 保存 access_token 和 refresh_token
4. 根据 is_first_login 判断是否显示引导
5. 根据 user_config 判断是否弹窗
```

**无需调用 GetUserInfo！**

### 场景 2: 用户修改了资料

```
1. 调用修改接口（如 edit_user_name）
2. 修改成功后调用 GetUserInfo 获取最新信息
3. 更新本地缓存
```

### 场景 3: Token 过期

```
1. 请求返回错误码 2002（Token过期）
2. 使用 refresh_token 调用 /v1/auth/refresh_token
3. 获取新的 access_token
4. 重试原请求
```

### 场景 4: 页面刷新

```
1. 检查本地是否有 access_token
2. 如果有，验证 token 是否过期
3. 如果过期，使用 refresh_token 刷新
4. 可选：调用 GetUserInfo 获取最新用户信息
```

---

## 🔒 安全建议

1. **使用 HTTPS**: 生产环境必须使用 HTTPS
2. **Token 安全存储**: 不要存储在 localStorage，推荐使用 httpOnly cookie
3. **敏感信息**: 不要在日志中打印 token、密码等
4. **Token 过期处理**: 实现自动刷新机制
5. **退出登录**: 清除所有本地存储的认证信息

---

## 💡 最佳实践

### 1. 登录流程优化

```javascript
// ✅ 推荐：登录后直接使用响应中的 user_info
async function login(email, password) {
  const response = await emailPasswordLogin(email, password);
  const { access_token, user_info } = response;

  // 保存 token 和用户信息
  saveToken(access_token);
  saveUserInfo(user_info);

  // 直接跳转，无需再次请求
  router.push('/home');
}

// ❌ 不推荐：登录后再次请求用户信息
async function loginOld(email, password) {
  const response = await emailPasswordLogin(email, password);
  const { access_token } = response;

  saveToken(access_token);

  // 多余的请求！
  const userInfo = await getUserInfo(access_token);
  saveUserInfo(userInfo);

  router.push('/home');
}
```

### 2. Token 管理

```javascript
// 设置 axios 拦截器自动处理 token 过期
axios.interceptors.response.use(
  response => response,
  async error => {
    if (error.response?.data?.response_header?.response_status_code === 2002) {
      // Token 过期，自动刷新
      const newToken = await refreshToken();
      // 重试原请求
      return axios(error.config);
    }
    return Promise.reject(error);
  }
);
```

### 3. 错误处理

```javascript
// 统一的错误处理
function handleApiError(error) {
  const statusCode = error.response?.data?.response_header?.response_status_code;
  const msg = error.response?.data?.response_header?.msg;

  switch (statusCode) {
    case 2001: // Token 无效
    case 2003: // RefreshToken 无效
      // 跳转到登录页
      router.push('/login');
      break;
    case 2002: // Token 过期
      // 自动刷新（由拦截器处理）
      break;
    case 4018: // 邮箱或密码错误
      showError('邮箱或密码错误，请重新输入');
      break;
    default:
      showError(msg || '请求失败，请稍后重试');
  }
}
```

---

## 📞 技术支持

如有疑问，请联系后端开发团队或查看项目代码仓库。

## 📝 文档更新

| 日期 | 说明 |
|------|------|
| 2026-01-29 | 创建文档总览，整理所有接口文档 |
