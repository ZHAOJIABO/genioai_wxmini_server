package geoip

import (
	"net"
	"testing"
)

// TestIsPrivateIP_Loopback 测试回环地址
func TestIsPrivateIP_Loopback(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv4 loopback range", "127.0.0.100", true},
		{"IPv6 loopback", "::1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			result := isPrivateIP(ip)
			if result != tt.expected {
				t.Errorf("isPrivateIP(%s) = %v, expected %v", tt.ip, result, tt.expected)
			}
		})
	}
}

// TestIsPrivateIP_PrivateRanges 测试私有网段
func TestIsPrivateIP_PrivateRanges(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"10.0.0.0/8", "10.0.0.1", true},
		{"10.0.0.0/8", "10.255.255.255", true},
		{"172.16.0.0/12", "172.16.0.1", true},
		{"172.16.0.0/12", "172.31.255.255", true},
		{"192.168.0.0/16", "192.168.1.1", true},
		{"192.168.0.0/16", "192.168.255.255", true},
		{"Link-local", "169.254.1.1", true},
		{"IPv6 unique local", "fc00::1", true},
		{"IPv6 link-local", "fe80::1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			result := isPrivateIP(ip)
			if result != tt.expected {
				t.Errorf("isPrivateIP(%s) = %v, expected %v", tt.ip, result, tt.expected)
			}
		})
	}
}

// TestIsPrivateIP_PublicIPs 测试公网地址
func TestIsPrivateIP_PublicIPs(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"Google DNS", "8.8.8.8", false},
		{"Cloudflare DNS", "1.1.1.1", false},
		{"Public IPv4", "123.45.67.89", false},
		{"Public IPv6", "2001:4860:4860::8888", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			result := isPrivateIP(ip)
			if result != tt.expected {
				t.Errorf("isPrivateIP(%s) = %v, expected %v", tt.ip, result, tt.expected)
			}
		})
	}
}

// TestGetCountryCode_InvalidInputs 测试无效输入
func TestGetCountryCode_InvalidInputs(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected string
	}{
		{"Empty string", "", "unknown"},
		{"Invalid IP", "invalid-ip", "unknown"},
		{"Private IP", "192.168.1.1", "unknown"},
		{"Loopback", "127.0.0.1", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetCountryCode(tt.ip)
			if result != tt.expected {
				t.Errorf("GetCountryCode(%s) = %s, expected %s", tt.ip, result, tt.expected)
			}
		})
	}
}

// TestGetCountryAndTimezone_InvalidInputs 测试无效输入
func TestGetCountryAndTimezone_InvalidInputs(t *testing.T) {
	tests := []struct {
		name            string
		ip              string
		expectedCountry string
		expectedTZ      string
	}{
		{"Empty string", "", "unknown", ""},
		{"Invalid IP", "invalid-ip", "unknown", ""},
		{"Private IP", "192.168.1.1", "unknown", ""},
		{"Loopback", "127.0.0.1", "unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			country, timezone := GetCountryAndTimezone(tt.ip)
			if country != tt.expectedCountry {
				t.Errorf("GetCountryAndTimezone(%s) country = %s, expected %s", tt.ip, country, tt.expectedCountry)
			}
			if timezone != tt.expectedTZ {
				t.Errorf("GetCountryAndTimezone(%s) timezone = %s, expected %s", tt.ip, timezone, tt.expectedTZ)
			}
		})
	}
}

// TestGetCountryCode_WithoutDB 测试未初始化数据库的情况
func TestGetCountryCode_WithoutDB(t *testing.T) {
	// 确保数据库未初始化
	db = nil
	result := GetCountryCode("8.8.8.8")
	if result != "unknown" {
		t.Errorf("GetCountryCode without DB should return 'unknown', got %s", result)
	}
}

// TestGetCountryAndTimezone_WithoutDB 测试未初始化数据库的情况
func TestGetCountryAndTimezone_WithoutDB(t *testing.T) {
	// 确保数据库未初始化
	db = nil
	country, timezone := GetCountryAndTimezone("8.8.8.8")
	if country != "unknown" {
		t.Errorf("GetCountryAndTimezone without DB should return 'unknown' for country, got %s", country)
	}
	if timezone != "" {
		t.Errorf("GetCountryAndTimezone without DB should return empty string for timezone, got %s", timezone)
	}
}
