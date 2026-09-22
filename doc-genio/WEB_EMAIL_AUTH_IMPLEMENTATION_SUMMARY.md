# Web端邮箱注册/登录实现总结文档

## ✅ 已完成工作

### 1. 数据库模型扩展
- **文件**: `internal/model/user.go:32`
- **改动**: 添加 `PasswordHash string` 字段用于存储bcrypt加密的密码

### 2. Proto定义更新
- **文件**: `pkg/va_interface/auth.proto`, `pkg/va_interface/service.proto`
- **新增消息类型**:
  - `EmailVerifyCodeRequest/Response` - 发送邮箱验证码
  - `EmailAuthRequest` - 邮箱验证码登录
  - `EmailPasswordAuthRequest` - 邮箱密码登录
  - `EmailRegisterRequest/Response` - 邮箱注册
- **新增RPC方法** (在AuthService中):
  - `SendEmailVerifyCode`
  - `EmailAuth`
  - `EmailPasswordAuth`
  - `EmailRegister`

### 3. 参数类型定义
- **文件**: `internal/service/auth/types/params.go:113-197`
- **新增类型**:
  - `EmailVerifyLoginParams` - 邮箱验证码登录参数
  - `EmailPasswordLoginParams` - 邮箱密码登录参数
  - `EmailRegisterParams` - 邮箱注册参数

### 4. 邮箱验证工具
- **文件**: `internal/utils/phone_number.go:24-31`
- **新增函数**: `IsValidEmail()` - RFC 5322标准的邮箱格式验证

### 5. 邮箱验证码服务
- **文件**: `internal/service/email_verify_service.go`
- **功能**:
  - 发送HTML格式验证码邮件
  - Redis存储验证码(5分钟过期)
  - 验证码验证和清理
- **依赖**: gomail.v2 (需要安装)

### 6. 认证Provider实现
#### EmailVerifyProvider (验证码登录)
- **文件**: `internal/service/auth/providers/email/email_verify_provider.go`
- **功能**: 验证邮箱验证码后返回用户信息

#### EmailPasswordProvider (密码登录)
- **文件**: `internal/service/auth/providers/email/email_password_provider.go`
- **功能**:
  - 验证邮箱和密码
  - 使用bcrypt加密/验证密码
  - 提供 `HashPassword()` 和 `VerifyPassword()` 工具函数

### 7. DAO层扩展
- **文件**: `internal/dao/user.go:63-73`
- **新增方法**: `GetUserByEmail()` - 根据邮箱查询用户

### 8. AuthService更新
- **文件**: `internal/service/auth/auth_service.go:114-133`
- **改动**: 在Login()方法的switch中添加邮箱登录case

### 9. 配置结构
- **文件**: `conf/conf.go:31-39`
- **新增结构**: `EmailConfig` - SMTP配置

---

## 📝 需要完成的剩余工作

### 1. 添加API接口方法到auth.go

在 `internal/api/auth.go` 文件末尾添加以下4个方法 (代码已经写好，见下方):

```go
// SendEmailVerifyCode 发送邮箱验证码
func (s *AuthServer) SendEmailVerifyCode(ctx context.Context, req *vai.EmailVerifyCodeRequest) (*vai.EmailVerifyCodeResponse, error)

// EmailAuth 邮箱验证码登录
func (s *AuthServer) EmailAuth(ctx context.Context, req *vai.EmailAuthRequest) (*vai.AuthResponse, error)

// EmailPasswordAuth 邮箱密码登录
func (s *AuthServer) EmailPasswordAuth(ctx context.Context, req *vai.EmailPasswordAuthRequest) (*vai.AuthResponse, error)

// EmailRegister 邮箱注册
func (s *AuthServer) EmailRegister(ctx context.Context, req *vai.EmailRegisterRequest) (*vai.EmailRegisterResponse, error)
```

**完整实现代码保存在**: `d:\GolandProjects\genioAI_server\IMPL_EMAIL_API_METHODS.txt` (见文档末尾)

### 2. 注册Provider到ProviderManager

在 `internal/service/auth/init.go` 或Provider初始化的地方添加:

```go
// 注册邮箱验证码Provider
pm.RegisterFactory("email", func() types.LoginProvider {
    return email.NewEmailVerifyProvider(userDao, emailVerifyService)
})

// 注册邮箱密码Provider
pm.RegisterFactory("email_password", func() types.LoginProvider {
    return email.NewEmailPasswordProvider(userDao)
})
```

### 3. 重新生成Proto代码

```bash
cd d:\GolandProjects\genioAI_server
# 根据项目的构建脚本执行
make proto
# 或者
protoc --go_out=. --go-grpc_out=. pkg/va_interface/*.proto
```

### 4. 配置邮件服务

在 `config.yaml` 中添加:

```yaml
email:
  smtp_host: smtp.gmail.com
  smtp_port: 587
  username: your-email@gmail.com
  password: your-app-password  # Gmail应用专用密码
  from_address: your-email@gmail.com
  expire_minutes: 5
```

**Gmail配置步骤**:
1. 启用两步验证
2. 生成应用专用密码: https://myaccount.google.com/apppasswords
3. 使用生成的16位密码

### 5. 数据库迁移

执行SQL:
```sql
ALTER TABLE user_record
ADD COLUMN password_hash VARCHAR(255) DEFAULT ''
COMMENT '密码哈希（bcrypt）';
```

### 6. 安装依赖

```bash
go get -u gopkg.in/gomail.v2
go get -u golang.org/x/crypto/bcrypt
```

---

## 🎯 使用流程示例

### 注册流程
```
1. 客户端调用 SendEmailVerifyCode({email: "user@example.com"})
2. 用户收到邮件验证码
3. 客户端调用 EmailRegister({
     email: "user@example.com",
     verify_code: "123456",
     password: "mypassword",
     confirm_password: "mypassword"
   })
4. 注册成功，用户已创建并设置密码
```

### 登录流程（两种方式）

**方式1: 验证码登录**
```
1. SendEmailVerifyCode({email: "user@example.com"})
2. EmailAuth({email: "user@example.com", verify_code: "123456"})
```

**方式2: 密码登录**
```
1. EmailPasswordAuth({email: "user@example.com", password: "mypassword"})
```

**Google登录 (已有)**
```
1. GoogleAuth({google_id_token: "xxx"})
```

---

## 📂 文件清单

| 文件路径 | 状态 | 说明 |
|---------|------|------|
| `internal/model/user.go` | ✅ 已修改 | 添加password_hash字段 |
| `pkg/va_interface/auth.proto` | ✅ 已修改 | 添加邮箱相关消息定义 |
| `pkg/va_interface/service.proto` | ✅ 已修改 | 添加邮箱相关RPC方法 |
| `internal/service/auth/types/params.go` | ✅ 已创建 | 邮箱登录参数类型 |
| `internal/utils/phone_number.go` | ✅ 已修改 | 添加IsValidEmail() |
| `internal/service/email_verify_service.go` | ✅ 已创建 | 邮箱验证码服务 |
| `internal/service/auth/providers/email/email_verify_provider.go` | ✅ 已创建 | 邮箱验证码Provider |
| `internal/service/auth/providers/email/email_password_provider.go` | ✅ 已创建 | 邮箱密码Provider |
| `internal/dao/user.go` | ✅ 已修改 | 添加GetUserByEmail() |
| `internal/service/auth/auth_service.go` | ✅ 已修改 | 添加邮箱登录case |
| `conf/conf.go` | ✅ 已修改 | 添加EmailConfig |
| `internal/api/auth.go` | ⚠️ 需要添加 | 4个API方法 |
| `internal/service/auth/init.go` | ⚠️ 需要修改 | Provider注册 |

---

## ⚠️ 重要提醒

1. **Proto代码生成**: 必须重新生成proto代码才能编译通过
2. **邮件配置**: 必须正确配置SMTP才能发送验证码
3. **数据库迁移**: 必须执行SQL添加password_hash字段
4. **依赖安装**: 需要安装gomail.v2和bcrypt包
5. **Provider注册**: 必须在初始化时注册两个新Provider

---

## 🔧 API方法完整实现代码

将以下代码添加到 `internal/api/auth.go` 文件末尾:

```go
// SendEmailVerifyCode 发送邮箱验证码
func (s *AuthServer) SendEmailVerifyCode(ctx context.Context, req *vai.EmailVerifyCodeRequest) (*vai.EmailVerifyCodeResponse, error) {
	reqHeader := req.GetRequestHeader()
	email := req.GetEmail()
	var err error
	var rsp *vai.EmailVerifyCodeResponse

	entryLogFields := []zap.Field{
		zap.String("Email", email),
		zap.Any(constants.ServiceEvent, EventSendVerifyCode),
	}
	zlog.LogWithContext(ctx).Info("SendEmailVerifyCode", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, EventSendVerifyCodeComplete),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, reqHeader.GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("SendEmailVerifyCode", exitLogFields...)
		}
	}()

	if !utils.IsValidEmail(email) {
		rsp = &vai.EmailVerifyCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, "Invalid email format"),
		}
		return rsp, nil
	}

	emailVerifyService, err := service.NewEmailVerifyService()
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to create email verify service", zap.Error(err))
		rsp = &vai.EmailVerifyCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INTERNAL_SERVER_ERROR, constants.ErrMsgInternalServer),
		}
		return rsp, nil
	}

	_, err = emailVerifyService.SendVerifyCode(ctx, email)
	if err != nil {
		rsp = &vai.EmailVerifyCodeResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, "Failed to send verification code"),
		}
		return rsp, nil
	}

	rsp = &vai.EmailVerifyCodeResponse{
		ResponseHeader: common.BuildResponseHeader(vai.StatusCode_SUCCESS, "success"),
	}

	return rsp, nil
}

// EmailAuth 邮箱验证码登录
func (s *AuthServer) EmailAuth(ctx context.Context, req *vai.EmailAuthRequest) (*vai.AuthResponse, error) {
	reqHeader := req.GetRequestHeader()
	lang := constants.LanguageMap(reqHeader.GetDevice().GetLanguage())
	os := constants.MappingOS(reqHeader.GetDevice().GetOs())
	email := req.GetEmail()
	var err error
	var rsp *vai.AuthResponse

	entryLogFields := []zap.Field{
		zap.String("Email", email),
		zap.Any(constants.ServiceEvent, constants.EventLogin),
	}
	zlog.LogWithContext(ctx).Info("EmailAuth", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, constants.EventLoginComplete),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("EmailAuth", exitLogFields...)
		}
	}()

	loginResult, err := s.authService.Login(ctx, req)
	if err != nil {
		rsp = &vai.AuthResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, err.Error()),
		}
		return rsp, nil
	}

	user := loginResult.User
	userInfo := s.userService.BuildUserInfoByUserRecord(user)
	userPersonalInfo, err := s.userPersonalService.GetUserPersonalInfo(ctx, user.ProjectID, user.UserId)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetUserPersonalInfo Error", zap.Error(err))
	}
	userInfo.Gender = userPersonalInfo.GetGender()
	userInfo.Nickname = userPersonalInfo.GetNickname()
	userInfo.Avatar = userPersonalInfo.GetAvatar()
	userInfo.RegisterTime = time.Unix(user.CreateTime, 0).Format(common.TimestampFormat)
	err = s.BuildUserInfo(ctx, lang, os, userInfo, loginResult.CreditGranted, loginResult.CreditAmount)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_INVALID_REQUEST, err, constants.ErrMsgInvalidRequest)
		return rsp, nil
	}

	rsp = &vai.AuthResponse{
		ResponseHeader: common.BuildResponseHeader(vai.StatusCode_SUCCESS, "success"),
		AccessToken:    user.AccessToken,
		ExpireTime:     time.Unix(user.AccessTokenExpired, 0).Format(common.TimestampFormat),
		RefreshToken:   user.RefreshToken,
		UserInfo:       userInfo,
		IsFirstLogin:   loginResult.CreditGranted,
		AuthType:       vai.AuthType_AUTH_TYPE_EMAIL,
	}

	return rsp, nil
}

// EmailPasswordAuth 邮箱密码登录
func (s *AuthServer) EmailPasswordAuth(ctx context.Context, req *vai.EmailPasswordAuthRequest) (*vai.AuthResponse, error) {
	reqHeader := req.GetRequestHeader()
	lang := constants.LanguageMap(reqHeader.GetDevice().GetLanguage())
	os := constants.MappingOS(reqHeader.GetDevice().GetOs())
	email := req.GetEmail()
	var err error
	var rsp *vai.AuthResponse

	entryLogFields := []zap.Field{
		zap.String("Email", email),
		zap.Any(constants.ServiceEvent, constants.EventLogin),
	}
	zlog.LogWithContext(ctx).Info("EmailPasswordAuth", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, constants.EventLoginComplete),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("EmailPasswordAuth", exitLogFields...)
		}
	}()

	loginResult, err := s.authService.Login(ctx, req)
	if err != nil {
		errMsg := err.Error()
		if errMsg == "user not found" || errMsg == "invalid email or password" {
			rsp = &vai.AuthResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, "Invalid email or password"),
			}
		} else if errMsg == "password not set for this account" {
			rsp = &vai.AuthResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, "Please use email verification code to login"),
			}
		} else {
			rsp = &vai.AuthResponse{
				ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, errMsg),
			}
		}
		return rsp, nil
	}

	user := loginResult.User
	userInfo := s.userService.BuildUserInfoByUserRecord(user)
	userPersonalInfo, err := s.userPersonalService.GetUserPersonalInfo(ctx, user.ProjectID, user.UserId)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetUserPersonalInfo Error", zap.Error(err))
	}
	userInfo.Gender = userPersonalInfo.GetGender()
	userInfo.Nickname = userPersonalInfo.GetNickname()
	userInfo.Avatar = userPersonalInfo.GetAvatar()
	userInfo.RegisterTime = time.Unix(user.CreateTime, 0).Format(common.TimestampFormat)
	err = s.BuildUserInfo(ctx, lang, os, userInfo, loginResult.CreditGranted, loginResult.CreditAmount)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.AuthResponse](ctx, vai.StatusCode_INVALID_REQUEST, err, constants.ErrMsgInvalidRequest)
		return rsp, nil
	}

	rsp = &vai.AuthResponse{
		ResponseHeader: common.BuildResponseHeader(vai.StatusCode_SUCCESS, "success"),
		AccessToken:    user.AccessToken,
		ExpireTime:     time.Unix(user.AccessTokenExpired, 0).Format(common.TimestampFormat),
		RefreshToken:   user.RefreshToken,
		UserInfo:       userInfo,
		IsFirstLogin:   false,
		AuthType:       vai.AuthType_AUTH_TYPE_EMAIL,
	}

	return rsp, nil
}

// EmailRegister 邮箱注册
func (s *AuthServer) EmailRegister(ctx context.Context, req *vai.EmailRegisterRequest) (*vai.EmailRegisterResponse, error) {
	reqHeader := req.GetRequestHeader()
	email := req.GetEmail()
	verifyCode := req.GetVerifyCode()
	password := req.GetPassword()
	confirmPassword := req.GetConfirmPassword()
	var err error
	var rsp *vai.EmailRegisterResponse

	entryLogFields := []zap.Field{
		zap.String("Email", email),
		zap.Any(constants.ServiceEvent, "EmailRegister"),
	}
	zlog.LogWithContext(ctx).Info("EmailRegister", entryLogFields...)

	defer func() {
		if rsp != nil {
			exitLogFields := []zap.Field{
				zap.Any(constants.ServiceEvent, "EmailRegisterComplete"),
				zap.Error(err),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.String(constants.CtxPackageName, reqHeader.GetApp().GetPackageName()),
			}
			zlog.LogWithContext(ctx).Info("EmailRegister", exitLogFields...)
		}
	}()

	if !utils.IsValidEmail(email) {
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, "Invalid email format"),
		}
		return rsp, nil
	}

	if password != confirmPassword {
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, "Passwords do not match"),
		}
		return rsp, nil
	}

	if len(password) < 6 || len(password) > 20 {
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, "Password length must be between 6-20 characters"),
		}
		return rsp, nil
	}

	emailVerifyService, err := service.NewEmailVerifyService()
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to create email verify service", zap.Error(err))
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INTERNAL_SERVER_ERROR, constants.ErrMsgInternalServer),
		}
		return rsp, nil
	}

	if err := emailVerifyService.Verify(ctx, email, verifyCode); err != nil {
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_VERIFY_CODE, "Invalid or expired verification code"),
		}
		return rsp, nil
	}

	projectID := common.GetProjectID(ctx)
	existingUser, _ := s.userService.GetUserDao().GetUserByEmail(projectID, email)
	if existingUser != nil {
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INVALID_REQUEST, "Email already registered"),
		}
		return rsp, nil
	}

	emailAuthReq := &vai.EmailAuthRequest{
		RequestHeader: reqHeader,
		Email:         email,
		VerifyCode:    verifyCode,
	}

	loginResult, err := s.authService.Login(ctx, emailAuthReq)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to create user", zap.Error(err))
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_REQUEST_FAILED, "Registration failed: "+err.Error()),
		}
		return rsp, nil
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to hash password", zap.Error(err))
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INTERNAL_SERVER_ERROR, "Failed to set password"),
		}
		return rsp, nil
	}

	user := loginResult.User
	user.PasswordHash = string(passwordHash)
	if err := s.userService.GetUserDao().Update(user); err != nil {
		zlog.LogWithContext(ctx).Error("Failed to update user password", zap.Error(err))
		rsp = &vai.EmailRegisterResponse{
			ResponseHeader: common.BuildResponseHeader(vai.StatusCode_INTERNAL_SERVER_ERROR, "Failed to set password"),
		}
		return rsp, nil
	}

	emailVerifyService.DeleteVerifyCode(ctx, email)

	zlog.LogWithContext(ctx).Info("Email registration successful",
		zap.String("email", email),
		zap.String("user_id", user.UserId))

	rsp = &vai.EmailRegisterResponse{
		ResponseHeader:  common.BuildResponseHeader(vai.StatusCode_SUCCESS, "Registration successful"),
		NeedSetPassword: false,
	}

	return rsp, nil
}
```

---

完成以上步骤后，整个邮箱注册/登录系统就实现完毕了！
