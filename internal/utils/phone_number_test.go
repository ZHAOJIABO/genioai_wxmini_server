package utils

import (
	"fmt"
	"testing"

	"github.com/nyaruka/phonenumbers"
	"github.com/stretchr/testify/assert"
)

func TestParsePhoneNumber(t *testing.T) {
	tests := []struct {
		name        string
		phoneNumber string
		wantErr     bool
		wantDetails func(num *phonenumbers.PhoneNumber) string
	}{
		{
			name:        "有效的中国手机号",
			phoneNumber: "+8613812345678",
			wantErr:     false,
			wantDetails: func(num *phonenumbers.PhoneNumber) string {
				countryCode := num.GetCountryCode()
				nationalNumber := num.GetNationalNumber()
				countryName := phonenumbers.GetRegionCodeForNumber(num)
				numberType := phonenumbers.GetNumberType(num)

				return fmt.Sprintf("国家代码: %d, 国家: %s, 号码类型: %v, 国内号码: %d",
					countryCode, countryName, numberType, nationalNumber)
			},
		},
		{
			name:        "有效的美国电话号码",
			phoneNumber: "+12025550179",
			wantErr:     false,
			wantDetails: func(num *phonenumbers.PhoneNumber) string {
				countryCode := num.GetCountryCode()
				nationalNumber := num.GetNationalNumber()
				countryName := phonenumbers.GetRegionCodeForNumber(num)
				numberType := phonenumbers.GetNumberType(num)

				return fmt.Sprintf("国家代码: %d, 国家: %s, 号码类型: %v, 国内号码: %d",
					countryCode, countryName, numberType, nationalNumber)
			},
		},
		{
			name:        "无效的电话号码格式",
			phoneNumber: "123",
			wantErr:     true,
			wantDetails: nil,
		},
		{
			name:        "空电话号码",
			phoneNumber: "",
			wantErr:     true,
			wantDetails: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phoneNumber, err := ParsePhoneNumber(tt.phoneNumber)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				// nolint:testifylint
				assert.NoError(t, err)
				assert.NotNil(t, phoneNumber)

				// 输出电话号码的详细信息
				if tt.wantDetails != nil {
					details := tt.wantDetails(phoneNumber)
					t.Logf("电话号码详细信息: %s", details)

					// 额外输出格式化的电话号码
					formattedNumber := phonenumbers.Format(phoneNumber, phonenumbers.INTERNATIONAL)
					t.Logf("国际格式: %s", formattedNumber)

					formattedNational := phonenumbers.Format(phoneNumber, phonenumbers.NATIONAL)
					t.Logf("国内格式: %s", formattedNational)

					formattedE164 := phonenumbers.Format(phoneNumber, phonenumbers.E164)
					t.Logf("E164格式: %s", formattedE164)
				}
			}
		})
	}
}

func TestIsValidPhoneNumber(t *testing.T) {
	tests := []struct {
		name        string
		phoneNumber string
		want        bool
	}{
		{
			name:        "有效的中国手机号",
			phoneNumber: "+8613812345678",
			want:        true,
		},
		{
			name:        "有效的美国电话号码",
			phoneNumber: "+12025550179",
			want:        true,
		},
		{
			name:        "无效的电话号码格式",
			phoneNumber: "123",
			want:        false,
		},
		{
			name:        "空电话号码",
			phoneNumber: "",
			want:        false,
		},
		{
			name:        "格式正确但不存在的号码",
			phoneNumber: "+12025550199",
			want:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidPhoneNumber(tt.phoneNumber)
			assert.Equal(t, tt.want, got)

			// 如果是有效号码，输出详细信息
			if got {
				phoneNumber, err := ParsePhoneNumber(tt.phoneNumber)
				if err == nil {
					countryCode := phoneNumber.GetCountryCode()
					nationalNumber := phoneNumber.GetNationalNumber()
					countryName := phonenumbers.GetRegionCodeForNumber(phoneNumber)
					numberType := phonenumbers.GetNumberType(phoneNumber)

					t.Logf("电话号码详细信息: 国家代码: %d, 国家: %s, 号码类型: %v, 国内号码: %d",
						countryCode, countryName, numberType, nationalNumber)

					// 格式化电话号码
					formattedNumber := phonenumbers.Format(phoneNumber, phonenumbers.INTERNATIONAL)
					t.Logf("国际格式: %s", formattedNumber)

					formattedNational := phonenumbers.Format(phoneNumber, phonenumbers.NATIONAL)
					t.Logf("国内格式: %s", formattedNational)

					formattedE164 := phonenumbers.Format(phoneNumber, phonenumbers.E164)
					t.Logf("E164格式: %s", formattedE164)
				}
			}
		})
	}
}
