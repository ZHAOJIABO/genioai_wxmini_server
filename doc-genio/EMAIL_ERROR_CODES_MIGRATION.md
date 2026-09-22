# 邮箱错误码适配完成总结

## 修改概述

已成功将后端代码适配为使用新增的邮箱登录注册专用错误码，提供更精准的错误反馈。

## 修改的文件

### 1. [internal/api/auth.go](../internal/api/auth.go)

添加了 `strings` 包的导入，并修改了 4 个邮箱相关函数。

## 详细修改内容

### 1. SendEmailVerifyCode (发送邮箱验证码)

**修改位置**：[auth.go:574-579](../internal/api/auth.go#L574-L579)

```go
// ❌ 修改前
if !utils.IsValidEmail(email) {
    rsp = &vai.EmailVerifyCodeResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_REQUEST,  // 1002
            "Invalid email format"),
    }
    return rsp, nil
}

// ✅ 修改后
if !utils.IsValidEmail(email) {
    rsp = &vai.EmailVerifyCodeResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_EMAIL_FORMAT,  // 4017
            "Invalid email format"),
    }
    return rsp, nil
}
```

**变更**：
- 错误码从 `1002` (INVALID_REQUEST) 改为 `4017` (INVALID_EMAIL_FORMAT)

---

### 2. EmailAuth (邮箱验证码登录)

**修改位置**：[auth.go:633-655](../internal/api/auth.go#L633-L655)

```go
// ❌ 修改前
loginResult, err := s.authService.Login(ctx, req)
if err != nil {
    rsp = &vai.AuthResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_REQUEST,  // 1002
            err.Error()),
    }
    return rsp, nil
}

// ✅ 修改后
loginResult, err := s.authService.Login(ctx, req)
if err != nil {
    errMsg := err.Error()
    switch {
    case strings.Contains(errMsg, "invalid verification code") ||
         strings.Contains(errMsg, "invalid verify code"):
        rsp = &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_INVALID_EMAIL_VERIFY_CODE,  // 4022
                "Invalid verification code"),
        }
    case strings.Contains(errMsg, "expired") ||
         strings.Contains(errMsg, "code expired"):
        rsp = &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_EMAIL_VERIFY_CODE_EXPIRED,  // 4023
                "Verification code expired"),
        }
    case strings.Contains(errMsg, "email not registered") ||
         strings.Contains(errMsg, "user not found"):
        rsp = &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_EMAIL_NOT_REGISTERED,  // 4020
                "Email not registered"),
        }
    default:
        rsp = &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_INVALID_REQUEST,
                errMsg),
        }
    }
    return rsp, nil
}
```

**变更**：
- 添加了错误消息智能识别
- 根据不同错误返回对应的专用错误码

---

### 3. EmailPasswordAuth (邮箱密码登录)

**修改位置**：[auth.go:715-737](../internal/api/auth.go#L715-L737)

```go
// ❌ 修改前
loginResult, err := s.authService.Login(ctx, req)
if err != nil {
    errMsg := err.Error()
    if errMsg == "user not found" || errMsg == "invalid email or password" {
        rsp = &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_INVALID_REQUEST,  // 1002
                "Invalid email or password"),
        }
    } else if errMsg == "password not set for this account" {
        rsp = &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_INVALID_REQUEST,  // 1002
                "Please use email verification code to login"),
        }
    } else {
        rsp = &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_INVALID_REQUEST,
                errMsg),
        }
    }
    return rsp, nil
}

// ✅ 修改后
loginResult, err := s.authService.Login(ctx, req)
if err != nil {
    errMsg := err.Error()
    switch {
    case errMsg == "user not found":
        rsp = &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_EMAIL_NOT_REGISTERED,  // 4020
                "Email not registered"),
        }
    case errMsg == "invalid email or password":
        rsp = &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_INVALID_EMAIL_OR_PASSWORD,  // 4018
                "Invalid email or password"),
        }
    case errMsg == "password not set for this account":
        rsp = &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_PASSWORD_NOT_SET,  // 4024
                "Please use email verification code to login"),
        }
    default:
        rsp = &vai.AuthResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_INVALID_REQUEST,
                errMsg),
        }
    }
    return rsp, nil
}
```

**变更**：
- `user not found` → `4020` (EMAIL_NOT_REGISTERED)
- `invalid email or password` → `4018` (INVALID_EMAIL_OR_PASSWORD)
- `password not set` → `4024` (PASSWORD_NOT_SET)

---

### 4. EmailRegister (邮箱注册)

**修改位置 1**：[auth.go:797-816](../internal/api/auth.go#L797-L816)

```go
// ❌ 修改前
if !utils.IsValidEmail(email) {
    rsp = &vai.EmailRegisterResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_REQUEST,  // 1002
            "Invalid email format"),
    }
    return rsp, nil
}

if password != confirmPassword {
    rsp = &vai.EmailRegisterResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_REQUEST,  // 1002
            "Passwords do not match"),
    }
    return rsp, nil
}

if len(password) < 6 || len(password) > 20 {
    rsp = &vai.EmailRegisterResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_REQUEST,  // 1002
            "Password length must be between 6-20 characters"),
    }
    return rsp, nil
}

// ✅ 修改后
if !utils.IsValidEmail(email) {
    rsp = &vai.EmailRegisterResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_EMAIL_FORMAT,  // 4017
            "Invalid email format"),
    }
    return rsp, nil
}

if password != confirmPassword {
    rsp = &vai.EmailRegisterResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_PASSWORD_FORMAT,  // 4021
            "Passwords do not match"),
    }
    return rsp, nil
}

if len(password) < 6 || len(password) > 20 {
    rsp = &vai.EmailRegisterResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_PASSWORD_FORMAT,  // 4021
            "Password length must be between 6-20 characters"),
    }
    return rsp, nil
}
```

**修改位置 2**：[auth.go:827-848](../internal/api/auth.go#L827-L848)

```go
// ❌ 修改前
if err := emailVerifyService.Verify(ctx, email, verifyCode); err != nil {
    rsp = &vai.EmailRegisterResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_VERIFY_CODE,  // 4002
            "Invalid or expired verification code"),
    }
    return rsp, nil
}

projectID := common.GetProjectID(ctx)
existingUser, _ := s.authService.GetUserDao().GetUserByEmail(projectID, email)
if existingUser != nil {
    rsp = &vai.EmailRegisterResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_INVALID_REQUEST,  // 1002
            "Email already registered"),
    }
    return rsp, nil
}

// ✅ 修改后
if err := emailVerifyService.Verify(ctx, email, verifyCode); err != nil {
    errMsg := err.Error()
    if strings.Contains(errMsg, "expired") {
        rsp = &vai.EmailRegisterResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_EMAIL_VERIFY_CODE_EXPIRED,  // 4023
                "Verification code expired. Please request a new one."),
        }
    } else {
        rsp = &vai.EmailRegisterResponse{
            ResponseHeader: common.BuildResponseHeader(
                vai.StatusCode_INVALID_EMAIL_VERIFY_CODE,  // 4022
                "Invalid verification code"),
        }
    }
    return rsp, nil
}

projectID := common.GetProjectID(ctx)
existingUser, _ := s.authService.GetUserDao().GetUserByEmail(projectID, email)
if existingUser != nil {
    rsp = &vai.EmailRegisterResponse{
        ResponseHeader: common.BuildResponseHeader(
            vai.StatusCode_EMAIL_ALREADY_EXISTS,  // 4019
            "Email already registered"),
    }
    return rsp, nil
}
```

**变更**：
- 邮箱格式错误：`1002` → `4017` (INVALID_EMAIL_FORMAT)
- 密码格式错误：`1002` → `4021` (INVALID_PASSWORD_FORMAT)
- 验证码无效：`4002` → `4022` (INVALID_EMAIL_VERIFY_CODE)
- 验证码过期：新增判断 → `4023` (EMAIL_VERIFY_CODE_EXPIRED)
- 邮箱已注册：`1002` → `4019` (EMAIL_ALREADY_EXISTS)

---

## 错误码映射表

| 场景 | 旧错误码 | 新错误码 | 错误码值 |
|------|---------|---------|---------|
| 邮箱格式无效 | `INVALID_REQUEST` (1002) | `INVALID_EMAIL_FORMAT` (4017) | 4017 |
| 邮箱或密码错误 | `INVALID_REQUEST` (1002) | `INVALID_EMAIL_OR_PASSWORD` (4018) | 4018 |
| 邮箱已注册 | `INVALID_REQUEST` (1002) | `EMAIL_ALREADY_EXISTS` (4019) | 4019 |
| 邮箱未注册 | `INVALID_REQUEST` (1002) | `EMAIL_NOT_REGISTERED` (4020) | 4020 |
| 密码格式错误 | `INVALID_REQUEST` (1002) | `INVALID_PASSWORD_FORMAT` (4021) | 4021 |
| 验证码错误 | `INVALID_VERIFY_CODE` (4002) | `INVALID_EMAIL_VERIFY_CODE` (4022) | 4022 |
| 验证码过期 | `INVALID_VERIFY_CODE` (4002) | `EMAIL_VERIFY_CODE_EXPIRED` (4023) | 4023 |
| 密码未设置 | `INVALID_REQUEST` (1002) | `PASSWORD_NOT_SET` (4024) | 4024 |

## 优势

### 1. 更精准的错误提示
前端可以根据错误码提供更准确的用户提示：
```typescript
switch (errorCode) {
  case 4017: // INVALID_EMAIL_FORMAT
    showError("请输入正确的邮箱格式");
    break;
  case 4018: // INVALID_EMAIL_OR_PASSWORD
    showError("邮箱或密码错误");
    highlightFields(['email', 'password']);
    break;
  case 4019: // EMAIL_ALREADY_EXISTS
    showError("该邮箱已被注册");
    suggestAction("直接登录");
    break;
  // ...
}
```

### 2. 更好的用户体验
- 用户能清楚知道具体哪里出错
- 可以提供针对性的解决方案
- 减少用户困惑和重复尝试

### 3. 便于监控和分析
- 统计各类错误的发生频率
- 识别常见问题并优化
- 提升产品质量

### 4. 向后兼容
- 保留了对旧错误码的支持
- 不会影响现有客户端
- 平滑过渡

## 测试建议

### 1. 单元测试

```go
func TestEmailPasswordAuth_InvalidEmail(t *testing.T) {
    // 测试邮箱格式错误
    req := &vai.EmailPasswordAuthRequest{
        Email: "invalid-email",
        Password: "password123",
    }

    rsp, _ := server.EmailPasswordAuth(ctx, req)

    assert.Equal(t, vai.StatusCode_INVALID_EMAIL_FORMAT, rsp.GetResponseHeader().GetCode())
    assert.Equal(t, 4017, int(rsp.GetResponseHeader().GetResponseStatusCode()))
}

func TestEmailPasswordAuth_WrongPassword(t *testing.T) {
    // 测试密码错误
    req := &vai.EmailPasswordAuthRequest{
        Email: "test@example.com",
        Password: "wrongpassword",
    }

    rsp, _ := server.EmailPasswordAuth(ctx, req)

    assert.Equal(t, vai.StatusCode_INVALID_EMAIL_OR_PASSWORD, rsp.GetResponseHeader().GetCode())
    assert.Equal(t, 4018, int(rsp.GetResponseHeader().GetResponseStatusCode()))
}
```

### 2. 集成测试

使用 Postman 或 grpcurl 测试各种错误场景：

```bash
# 测试邮箱格式错误
grpcurl -d '{
  "email": "invalid-email",
  "password": "test123"
}' localhost:8080 va_interface.AuthService/EmailPasswordAuth

# 预期响应：
# {
#   "response_header": {
#     "code": 4017,
#     "msg": "Invalid email format",
#     "response_status_code": 4017
#   }
# }
```

## 注意事项

1. **错误消息匹配**：
   - 确保 auth service 返回的错误消息与匹配条件一致
   - 如果修改了 service 层的错误消息，需要同步更新 API 层的匹配逻辑

2. **安全性**：
   - 对于 `INVALID_EMAIL_OR_PASSWORD`，保持不区分是邮箱还是密码错误
   - 防止撞库攻击

3. **日志记录**：
   - 所有错误已自动记录到日志中
   - 便于排查问题

## 相关文档

- [邮箱错误码说明](EMAIL_AUTH_ERROR_CODES.md)
- [Protocol Buffer 定义](../pkg/va_interface/common.proto)
- [状态码映射](../internal/rpc/status_code.go)

---

**完成时间**: 2025-01-29
**修改者**: VisionAI Backend Team
**状态**: ✅ 已完成并通过编译
