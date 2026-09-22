# 邮箱登录注册错误码说明

## 概述

本文档说明邮箱登录注册功能相关的错误码定义和使用场景。

## 错误码列表

| 错误码 | 枚举名称 | 错误消息建议 | 使用场景 |
|--------|---------|-------------|---------|
| **4017** | `INVALID_EMAIL_FORMAT` | "Invalid email format" | 邮箱格式不符合规范（如缺少@、域名错误等） |
| **4018** | `INVALID_EMAIL_OR_PASSWORD` | "Invalid email or password" | 邮箱或密码错误（登录时） |
| **4019** | `EMAIL_ALREADY_EXISTS` | "Email already registered" | 邮箱已被注册（注册时） |
| **4020** | `EMAIL_NOT_REGISTERED` | "Email not registered" | 邮箱未注册（登录或找回密码时） |
| **4021** | `INVALID_PASSWORD_FORMAT` | "Password does not meet requirements" | 密码格式不符合要求（如长度、复杂度等） |
| **4022** | `INVALID_EMAIL_VERIFY_CODE` | "Invalid verification code" | 邮箱验证码错误 |
| **4023** | `EMAIL_VERIFY_CODE_EXPIRED` | "Verification code expired" | 邮箱验证码已过期 |
| **4024** | `PASSWORD_NOT_SET` | "Please use verification code to login" | 账号未设置密码，需要使用验证码登录 |

## 详细说明

### 1. INVALID_EMAIL_FORMAT (4017)

**使用场景**：
- 用户输入的邮箱格式不正确
- 邮箱验证失败

**示例**：
```go
// 使用 Go
rsp = &vai.AuthResponse{
    ResponseHeader: common.BuildResponseHeader(
        vai.StatusCode_INVALID_EMAIL_FORMAT,
        "Invalid email format"),
}
```

```json
// JSON 响应
{
  "response_header": {
    "code": 4017,
    "msg": "Invalid email format",
    "response_status_code": 4017
  }
}
```

---

### 2. INVALID_EMAIL_OR_PASSWORD (4018)

**使用场景**：
- 用户登录时邮箱或密码错误
- 为了安全，不区分是邮箱还是密码错误

**建议**：
使用此错误码替代之前的 `INVALID_REQUEST (1002)`，以提供更明确的错误信息。

**示例**：
```go
// 使用 Go
rsp = &vai.AuthResponse{
    ResponseHeader: common.BuildResponseHeader(
        vai.StatusCode_INVALID_EMAIL_OR_PASSWORD,
        "Invalid email or password"),
}
```

**迁移建议**：
```go
// 旧代码
if errMsg == "user not found" || errMsg == "invalid email or password" {
    rsp = &vai.AuthResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_REQUEST,  // ❌ 旧的
            "Invalid email or password"),
    }
}

// 新代码
if errMsg == "user not found" || errMsg == "invalid email or password" {
    rsp = &vai.AuthResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_EMAIL_OR_PASSWORD,  // ✅ 新的
            "Invalid email or password"),
    }
}
```

---

### 3. EMAIL_ALREADY_EXISTS (4019)

**使用场景**：
- 用户注册时邮箱已被使用
- 绑定邮箱时该邮箱已被其他账号绑定

**示例**：
```go
// 使用 Go
rsp = &vai.EmailRegisterResponse{
    ResponseHeader: common.BuildResponseHeader(
        vai.StatusCode_EMAIL_ALREADY_EXISTS,
        "Email already registered"),
}
```

---

### 4. EMAIL_NOT_REGISTERED (4020)

**使用场景**：
- 用户尝试登录未注册的邮箱
- 找回密码时邮箱未注册

**示例**：
```go
// 使用 Go
rsp = &vai.AuthResponse{
    ResponseHeader: common.BuildResponseHeader(
        vai.StatusCode_EMAIL_NOT_REGISTERED,
        "Email not registered. Please sign up first."),
}
```

---

### 5. INVALID_PASSWORD_FORMAT (4021)

**使用场景**：
- 注册或修改密码时，密码不符合安全要求
- 密码长度、复杂度验证失败

**密码要求示例**：
- 长度：8-32 字符
- 必须包含大小写字母、数字
- 可选：特殊字符

**示例**：
```go
// 使用 Go
rsp = &vai.EmailRegisterResponse{
    ResponseHeader: common.BuildResponseHeader(
        vai.StatusCode_INVALID_PASSWORD_FORMAT,
        "Password must be 8-32 characters with uppercase, lowercase, and numbers"),
}
```

---

### 6. INVALID_EMAIL_VERIFY_CODE (4022)

**使用场景**：
- 用户输入的邮箱验证码错误
- 验证码校验失败

**示例**：
```go
// 使用 Go
rsp = &vai.EmailVerifyResponse{
    ResponseHeader: common.BuildResponseHeader(
        vai.StatusCode_INVALID_EMAIL_VERIFY_CODE,
        "Invalid verification code"),
}
```

---

### 7. EMAIL_VERIFY_CODE_EXPIRED (4023)

**使用场景**：
- 验证码已过期（通常 5-10 分钟）
- 提示用户重新获取验证码

**示例**：
```go
// 使用 Go
rsp = &vai.EmailVerifyResponse{
    ResponseHeader: common.BuildResponseHeader(
        vai.StatusCode_EMAIL_VERIFY_CODE_EXPIRED,
        "Verification code expired. Please request a new one."),
}
```

---

### 8. PASSWORD_NOT_SET (4024)

**使用场景**：
- 用户使用邮箱验证码注册，未设置密码
- 用户尝试密码登录但账号未设置密码

**处理建议**：
- 引导用户使用验证码登录
- 或提示用户先设置密码

**示例**：
```go
// 使用 Go
rsp = &vai.AuthResponse{
    ResponseHeader: common.BuildResponseHeader(
        vai.StatusCode_PASSWORD_NOT_SET,
        "Please use verification code to login or set a password first"),
}
```

## 前端处理建议

### TypeScript/JavaScript

```typescript
enum EmailAuthErrorCode {
  INVALID_EMAIL_FORMAT = 4017,
  INVALID_EMAIL_OR_PASSWORD = 4018,
  EMAIL_ALREADY_EXISTS = 4019,
  EMAIL_NOT_REGISTERED = 4020,
  INVALID_PASSWORD_FORMAT = 4021,
  INVALID_EMAIL_VERIFY_CODE = 4022,
  EMAIL_VERIFY_CODE_EXPIRED = 4023,
  PASSWORD_NOT_SET = 4024,
}

function handleAuthError(errorCode: number, errorMsg: string) {
  switch (errorCode) {
    case EmailAuthErrorCode.INVALID_EMAIL_FORMAT:
      showError("请输入正确的邮箱格式");
      break;
    case EmailAuthErrorCode.INVALID_EMAIL_OR_PASSWORD:
      showError("邮箱或密码错误");
      break;
    case EmailAuthErrorCode.EMAIL_ALREADY_EXISTS:
      showError("该邮箱已被注册，请直接登录");
      break;
    case EmailAuthErrorCode.EMAIL_NOT_REGISTERED:
      showError("该邮箱未注册，请先注册");
      break;
    case EmailAuthErrorCode.INVALID_PASSWORD_FORMAT:
      showError("密码格式不符合要求（8-32位，包含大小写字母和数字）");
      break;
    case EmailAuthErrorCode.INVALID_EMAIL_VERIFY_CODE:
      showError("验证码错误，请重新输入");
      break;
    case EmailAuthErrorCode.EMAIL_VERIFY_CODE_EXPIRED:
      showError("验证码已过期，请重新获取");
      break;
    case EmailAuthErrorCode.PASSWORD_NOT_SET:
      showError("账号未设置密码，请使用验证码登录");
      // 可以自动切换到验证码登录模式
      switchToVerifyCodeLogin();
      break;
    default:
      showError(errorMsg || "未知错误");
  }
}
```

## 后端使用示例

### 邮箱登录接口

```go
func (s *AuthServer) EmailPasswordAuth(ctx context.Context, req *vai.EmailPasswordAuthRequest) (*vai.AuthResponse, error) {
    // 验证邮箱格式
    if !isValidEmail(req.Email) {
        return &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_INVALID_EMAIL_FORMAT,
                "Invalid email format"),
        }, nil
    }

    // 验证密码格式
    if !isValidPassword(req.Password) {
        return &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_INVALID_PASSWORD_FORMAT,
                "Password does not meet requirements"),
        }, nil
    }

    // 登录验证
    loginResult, err := s.authService.Login(ctx, req)
    if err != nil {
        errMsg := err.Error()
        switch errMsg {
        case "user not found":
            return &vai.AuthResponse{
                ResponseHeader: common.BuildResponseHeader(
                    vai.StatusCode_EMAIL_NOT_REGISTERED,
                    "Email not registered"),
            }, nil
        case "invalid email or password":
            return &vai.AuthResponse{
                ResponseHeader: common.BuildResponseHeader(
                    vai.StatusCode_INVALID_EMAIL_OR_PASSWORD,
                    "Invalid email or password"),
            }, nil
        case "password not set for this account":
            return &vai.AuthResponse{
                ResponseHeader: common.BuildResponseHeader(
                    vai.StatusCode_PASSWORD_NOT_SET,
                    "Please use verification code to login"),
            }, nil
        default:
            return &vai.AuthResponse{
                ResponseHeader: common.BuildResponseHeader(
                    vai.StatusCode_INVALID_REQUEST,
                    errMsg),
            }, nil
        }
    }

    // 登录成功
    return &vai.AuthResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_SUCCESS,
            "Login successful"),
        UserInfo: loginResult.UserInfo,
        Token: loginResult.Token,
    }, nil
}
```

### 邮箱注册接口

```go
func (s *AuthServer) EmailRegister(ctx context.Context, req *vai.EmailRegisterRequest) (*vai.EmailRegisterResponse, error) {
    // 验证邮箱格式
    if !isValidEmail(req.Email) {
        return &vai.EmailRegisterResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_INVALID_EMAIL_FORMAT,
                "Invalid email format"),
        }, nil
    }

    // 检查邮箱是否已注册
    exists, _ := s.userService.CheckEmailExists(ctx, req.Email)
    if exists {
        return &vai.EmailRegisterResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_EMAIL_ALREADY_EXISTS,
                "Email already registered"),
        }, nil
    }

    // 验证验证码
    if !s.verifyCodeService.Verify(ctx, req.Email, req.VerifyCode) {
        return &vai.EmailRegisterResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_INVALID_EMAIL_VERIFY_CODE,
                "Invalid verification code"),
        }, nil
    }

    // 注册逻辑...
    // ...
}
```

## 相关文件

- **Protocol 定义**：[pkg/va_interface/common.proto](../pkg/va_interface/common.proto#L68-L84)
- **状态码映射**：[internal/rpc/status_code.go](../internal/rpc/status_code.go#L52-L60)
- **认证服务**：[internal/api/auth.go](../internal/api/auth.go)

## 注意事项

1. **安全性**：
   - 对于 `INVALID_EMAIL_OR_PASSWORD`，不要区分是邮箱还是密码错误，防止撞库攻击
   - 验证码错误次数应该有限制，防止暴力破解

2. **用户体验**：
   - 错误消息应该清晰明了
   - 提供明确的解决方案（如"请重新获取验证码"）

3. **向后兼容**：
   - 旧的错误码（如 `INVALID_REQUEST`）仍然可用
   - 建议逐步迁移到新的错误码

## 更新日志

- **2025-01-29**：初始版本，添加 8 个邮箱相关错误码（4017-4024）

---

**维护者**: VisionAI Backend Team
**最后更新**: 2025-01-29
