package types

import (
	"errors"
	"strings"

	"va_visionai_server/internal/utils"
)

// BaseLoginParams 基础登录参数
type BaseLoginParams struct {
	ProjectID  string
	DeviceInfo *DeviceInfo
	ClientIP   string
}

// DeviceInfo 设备信息结构体
type DeviceInfo struct {
	OS         string
	DeviceID   string
	AppVersion string
	UserType   string
}

// LoginParams 登录参数接口
type LoginParams interface {
	// 获取基础参数
	GetBaseParams() *BaseLoginParams
	// 验证参数有效性
	Validate() error
}

// PhoneVerifyLoginParams 手机验证码登录参数
type PhoneVerifyLoginParams struct {
	*BaseLoginParams
	PhoneNumber string
	VerifyCode  string
}

func (p *PhoneVerifyLoginParams) GetBaseParams() *BaseLoginParams {
	return p.BaseLoginParams
}

func (p *PhoneVerifyLoginParams) Validate() error {
	if p.PhoneNumber == "" || p.VerifyCode == "" {
		return errors.New("手机号和验证码不能为空")
	}
	// 校验手机号格式
	if !utils.IsValidPhoneNumber(p.PhoneNumber) {
		return errors.New("无效的手机号格式")
	}
	return nil
}

// AppleLoginParams 苹果登录参数
type AppleLoginParams struct {
	*BaseLoginParams
	IdToken           string
	AuthorizationCode string
}

func (p *AppleLoginParams) GetBaseParams() *BaseLoginParams {
	return p.BaseLoginParams
}

func (p *AppleLoginParams) Validate() error {
	if p.IdToken == "" {
		return errors.New("苹果ID令牌不能为空")
	}
	return nil
}

// GoogleLoginParams Google登录参数
type GoogleLoginParams struct {
	*BaseLoginParams
	IdToken string
	Code    string
}

func (p *GoogleLoginParams) GetBaseParams() *BaseLoginParams {
	return p.BaseLoginParams
}

func (p *GoogleLoginParams) Validate() error {
	if p.IdToken == "" && p.Code == "" {
		return errors.New("Google ID令牌或授权码不能为空")
	}
	return nil
}

// WeChatLoginParams 微信小程序登录参数。
type WeChatLoginParams struct {
	*BaseLoginParams
	Code     string
	AuthType string
}

func (p *WeChatLoginParams) GetBaseParams() *BaseLoginParams { return p.BaseLoginParams }

func (p *WeChatLoginParams) Validate() error {
	if p == nil || strings.TrimSpace(p.Code) == "" || len(p.Code) > 512 {
		return ErrInvalidCredentials
	}
	if p.AuthType != "" && p.AuthType != "miniprogram" {
		return ErrInvalidCredentials
	}
	if p.BaseLoginParams == nil || p.ProjectID == "" {
		return ErrInvalidLoginContext
	}
	return nil
}

// TempUserLoginParams 临时用户登录参数
type TempUserLoginParams struct {
	*BaseLoginParams
	TempUserIden string
}

func (p *TempUserLoginParams) GetBaseParams() *BaseLoginParams {
	return p.BaseLoginParams
}

func (p *TempUserLoginParams) Validate() error {
	if p.TempUserIden == "" {
		return errors.New("设备ID不能为空")
	}
	return nil
}

// NormalizePhoneNumber 标准化手机号格式（去除+号前缀）
func NormalizePhoneNumber(phoneNumber string) string {
	return strings.ReplaceAll(phoneNumber, "+", "")
}

// EmailVerifyLoginParams 邮箱验证码登录参数
type EmailVerifyLoginParams struct {
	*BaseLoginParams
	Email      string
	VerifyCode string
}

func (p *EmailVerifyLoginParams) GetBaseParams() *BaseLoginParams {
	return p.BaseLoginParams
}

func (p *EmailVerifyLoginParams) Validate() error {
	if p.Email == "" || p.VerifyCode == "" {
		return errors.New("邮箱和验证码不能为空")
	}
	// 校验邮箱格式
	if !utils.IsValidEmail(p.Email) {
		return errors.New("无效的邮箱格式")
	}
	return nil
}

// EmailPasswordLoginParams 邮箱密码登录参数
type EmailPasswordLoginParams struct {
	*BaseLoginParams
	Email    string
	Password string
}

func (p *EmailPasswordLoginParams) GetBaseParams() *BaseLoginParams {
	return p.BaseLoginParams
}

func (p *EmailPasswordLoginParams) Validate() error {
	if p.Email == "" || p.Password == "" {
		return errors.New("邮箱和密码不能为空")
	}
	// 校验邮箱格式
	if !utils.IsValidEmail(p.Email) {
		return errors.New("无效的邮箱格式")
	}
	// 密码长度检查
	if len(p.Password) < 6 || len(p.Password) > 20 {
		return errors.New("密码长度必须在6-20个字符之间")
	}
	return nil
}

// EmailRegisterParams 邮箱注册参数
type EmailRegisterParams struct {
	*BaseLoginParams
	Email           string
	VerifyCode      string
	Password        string
	ConfirmPassword string
}

func (p *EmailRegisterParams) GetBaseParams() *BaseLoginParams {
	return p.BaseLoginParams
}

func (p *EmailRegisterParams) Validate() error {
	if p.Email == "" {
		return errors.New("邮箱不能为空")
	}
	// 校验邮箱格式
	if !utils.IsValidEmail(p.Email) {
		return errors.New("无效的邮箱格式")
	}
	if p.VerifyCode == "" {
		return errors.New("验证码不能为空")
	}
	if p.Password == "" {
		return errors.New("密码不能为空")
	}
	// 密码长度检查
	if len(p.Password) < 6 || len(p.Password) > 20 {
		return errors.New("密码长度必须在6-20个字符之间")
	}
	// 确认密码
	if p.Password != p.ConfirmPassword {
		return errors.New("两次输入的密码不一致")
	}
	return nil
}
