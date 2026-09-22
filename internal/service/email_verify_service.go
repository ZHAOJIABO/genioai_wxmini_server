package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/go-redis/redis"
	"go.uber.org/zap"
	"gopkg.in/gomail.v2"

	"va_visionai_server/conf"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/zlog"
)

const EmailVerifyCodeKeyPrefix = "EMAIL_VC:"
const PasswordResetCodeKeyPrefix = "PASSWORD_RESET_VC:"

// EmailVerifyService 邮箱验证码服务
type EmailVerifyService struct {
	emailCfg    conf.EmailConfig
	redisClient *redis.Client
	dialer      *gomail.Dialer
}

// NewEmailVerifyService 创建邮箱验证码服务
func NewEmailVerifyService() (*EmailVerifyService, error) {
	vs := &EmailVerifyService{
		emailCfg: conf.GlobalConfig.Email,
	}
	return vs, vs.init()
}

func (s *EmailVerifyService) init() error {
	s.redisClient = db.GetRedis()
	if err := s.redisClient.Ping().Err(); err != nil {
		return err
	}

	// 初始化邮件发送器
	s.dialer = gomail.NewDialer(
		s.emailCfg.SMTPHost,
		s.emailCfg.SMTPPort,
		s.emailCfg.Username,
		s.emailCfg.Password,
	)
	// 跳过证书验证（生产环境建议配置正确的TLS）
	s.dialer.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	return nil
}

// SendVerifyCode 发送邮箱验证码
func (s *EmailVerifyService) SendVerifyCode(ctx context.Context, email string) (string, error) {
	if email == "" {
		zlog.LogWithContext(ctx).Error("invalid email", zap.String("email", email))
		return "", constants.ERR_INVALID_PARAM
	}

	// 1. 生成6位数字验证码
	verifyCode := fmt.Sprintf("%06d", rand.New(rand.NewSource(time.Now().UnixNano())).Int31n(1000000))

	// 2. 发送验证码邮件
	if err := s.sendVerifyCodeEmail(ctx, email, verifyCode); err != nil {
		return "", err
	}

	// 3. 记录验证码到Redis（默认5分钟过期）
	expireMinutes := 5
	if s.emailCfg.ExpireMinutes > 0 {
		expireMinutes = s.emailCfg.ExpireMinutes
	}
	err := s.redisClient.Set(
		EmailVerifyCodeKeyPrefix+email,
		verifyCode,
		time.Minute*time.Duration(expireMinutes),
	).Err()

	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to save verify code to Redis",
			zap.String("email", email),
			zap.Error(err))
		return "", err
	}

	zlog.LogWithContext(ctx).Info("Email verify code sent successfully",
		zap.String("email", email),
		zap.String("code", verifyCode))

	return verifyCode, nil
}

// Verify 验证邮箱验证码
func (s *EmailVerifyService) Verify(ctx context.Context, email, verifyCode string) error {
	if email == "" || verifyCode == "" {
		return constants.ERR_INVALID_PARAM
	}

	// 从Redis获取验证码
	savedCode, err := s.redisClient.Get(EmailVerifyCodeKeyPrefix + email).Result()
	if err != nil {
		if err == redis.Nil {
			zlog.LogWithContext(ctx).Warn("Verify code not found or expired",
				zap.String("email", email))
			return errors.New("验证码不存在或已过期")
		}
		zlog.LogWithContext(ctx).Error("Failed to get verify code from Redis",
			zap.String("email", email),
			zap.Error(err))
		return errors.New("验证失败: " + err.Error())
	}

	// 验证码匹配检查
	if savedCode != verifyCode {
		zlog.LogWithContext(ctx).Warn("Verify code mismatch",
			zap.String("email", email),
			zap.String("expected", savedCode),
			zap.String("actual", verifyCode))
		return errors.New("验证码错误")
	}

	// 验证成功后，将验证码有效期缩短为30秒（防止重复使用）
	s.redisClient.Expire(EmailVerifyCodeKeyPrefix+email, time.Second*30)

	zlog.LogWithContext(ctx).Info("Email verify code validated successfully",
		zap.String("email", email))

	return nil
}

// sendVerifyCodeEmail 发送验证码邮件
func (s *EmailVerifyService) sendVerifyCodeEmail(ctx context.Context, email, verifyCode string) error {
	// 构建邮件内容
	subject := "Your Verification Code"
	body := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background-color: #4CAF50; color: white; padding: 20px; text-align: center; }
        .content { background-color: #f9f9f9; padding: 30px; border-radius: 5px; }
        .code { font-size: 32px; font-weight: bold; color: #4CAF50; text-align: center;
                letter-spacing: 5px; padding: 20px; background-color: white;
                border-radius: 5px; margin: 20px 0; }
        .footer { text-align: center; color: #666; font-size: 12px; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Email Verification</h1>
        </div>
        <div class="content">
            <p>Hello,</p>
            <p>Your verification code is:</p>
            <div class="code">%s</div>
            <p>This code will expire in <strong>5 minutes</strong>.</p>
            <p>If you didn't request this code, please ignore this email.</p>
        </div>
        <div class="footer">
            <p>This is an automated email, please do not reply.</p>
        </div>
    </div>
</body>
</html>
`, verifyCode)

	// 创建邮件消息
	m := gomail.NewMessage()
	m.SetHeader("From", s.emailCfg.FromAddress)
	m.SetHeader("To", email)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)

	// 发送邮件
	if err := s.dialer.DialAndSend(m); err != nil {
		zlog.LogWithContext(ctx).Error("Failed to send email",
			zap.String("email", email),
			zap.Error(err))
		return fmt.Errorf("邮件发送失败: %w", err)
	}

	return nil
}

// DeleteVerifyCode 删除验证码（用于注册成功后清理）
func (s *EmailVerifyService) DeleteVerifyCode(ctx context.Context, email string) error {
	return s.redisClient.Del(EmailVerifyCodeKeyPrefix + email).Err()
}

// SendPasswordResetCode 发送密码重置验证码
func (s *EmailVerifyService) SendPasswordResetCode(ctx context.Context, email string) (string, error) {
	if email == "" {
		zlog.LogWithContext(ctx).Error("invalid email", zap.String("email", email))
		return "", constants.ERR_INVALID_PARAM
	}

	// 1. 生成6位数字验证码
	verifyCode := fmt.Sprintf("%06d", rand.New(rand.NewSource(time.Now().UnixNano())).Int31n(1000000))

	// 2. 发送密码重置邮件
	if err := s.sendPasswordResetCodeEmail(ctx, email, verifyCode); err != nil {
		return "", err
	}

	// 3. 记录验证码到Redis（默认5分钟过期）
	expireMinutes := 5
	if s.emailCfg.ExpireMinutes > 0 {
		expireMinutes = s.emailCfg.ExpireMinutes
	}
	err := s.redisClient.Set(
		PasswordResetCodeKeyPrefix+email,
		verifyCode,
		time.Minute*time.Duration(expireMinutes),
	).Err()

	if err != nil {
		zlog.LogWithContext(ctx).Error("Failed to save password reset code to Redis",
			zap.String("email", email),
			zap.Error(err))
		return "", err
	}

	zlog.LogWithContext(ctx).Info("Password reset code sent successfully",
		zap.String("email", email),
		zap.String("code", verifyCode))

	return verifyCode, nil
}

// VerifyPasswordResetCode 验证密码重置验证码
func (s *EmailVerifyService) VerifyPasswordResetCode(ctx context.Context, email, verifyCode string) error {
	if email == "" || verifyCode == "" {
		return constants.ERR_INVALID_PARAM
	}

	// 从Redis获取验证码
	savedCode, err := s.redisClient.Get(PasswordResetCodeKeyPrefix + email).Result()
	if err != nil {
		if err == redis.Nil {
			zlog.LogWithContext(ctx).Warn("Password reset code not found or expired",
				zap.String("email", email))
			return errors.New("验证码不存在或已过期")
		}
		zlog.LogWithContext(ctx).Error("Failed to get password reset code from Redis",
			zap.String("email", email),
			zap.Error(err))
		return errors.New("验证失败: " + err.Error())
	}

	// 验证码匹配检查
	if savedCode != verifyCode {
		zlog.LogWithContext(ctx).Warn("Password reset code mismatch",
			zap.String("email", email),
			zap.String("expected", savedCode),
			zap.String("actual", verifyCode))
		return errors.New("验证码错误")
	}

	// 验证成功后，将验证码有效期缩短为30秒（防止重复使用）
	s.redisClient.Expire(PasswordResetCodeKeyPrefix+email, time.Second*30)

	zlog.LogWithContext(ctx).Info("Password reset code validated successfully",
		zap.String("email", email))

	return nil
}

// DeletePasswordResetCode 删除密码重置验证码
func (s *EmailVerifyService) DeletePasswordResetCode(ctx context.Context, email string) error {
	return s.redisClient.Del(PasswordResetCodeKeyPrefix + email).Err()
}

// sendPasswordResetCodeEmail 发送密码重置验证码邮件
func (s *EmailVerifyService) sendPasswordResetCodeEmail(ctx context.Context, email, verifyCode string) error {
	subject := "Password Reset Code"
	body := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background-color: #FF6B6B; color: white; padding: 20px; text-align: center; }
        .content { background-color: #f9f9f9; padding: 30px; border-radius: 5px; }
        .code { font-size: 32px; font-weight: bold; color: #FF6B6B; text-align: center;
                letter-spacing: 5px; padding: 20px; background-color: white;
                border-radius: 5px; margin: 20px 0; }
        .footer { text-align: center; color: #666; font-size: 12px; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Password Reset</h1>
        </div>
        <div class="content">
            <p>Hello,</p>
            <p>You are resetting your password. Your verification code is:</p>
            <div class="code">%s</div>
            <p>This code will expire in <strong>5 minutes</strong>.</p>
            <p>If you didn't request a password reset, please ignore this email.</p>
        </div>
        <div class="footer">
            <p>This is an automated email, please do not reply.</p>
        </div>
    </div>
</body>
</html>
`, verifyCode)

	m := gomail.NewMessage()
	m.SetHeader("From", s.emailCfg.FromAddress)
	m.SetHeader("To", email)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)

	if err := s.dialer.DialAndSend(m); err != nil {
		zlog.LogWithContext(ctx).Error("Failed to send password reset email",
			zap.String("email", email),
			zap.Error(err))
		return fmt.Errorf("邮件发送失败: %w", err)
	}

	return nil
}
