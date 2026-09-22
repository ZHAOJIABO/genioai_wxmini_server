package utils

import (
	"regexp"

	"github.com/nyaruka/phonenumbers"
	"go.uber.org/zap"

	"va_visionai_server/internal/zlog"
)

func ParsePhoneNumber(phone string) (*phonenumbers.PhoneNumber, error) {
	return phonenumbers.Parse(phone, "")
}

func IsValidPhoneNumber(phone string) bool {
	phoneNumber, err := phonenumbers.Parse(phone, "")
	if err != nil {
		zlog.Logger.Error("ParsePhoneNumber", zap.Error(err))
		return false
	}

	return phonenumbers.IsValidNumber(phoneNumber)
}

// IsValidEmail 验证邮箱格式
func IsValidEmail(email string) bool {
	// RFC 5322 标准的邮箱正则表达式（简化版）
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}
