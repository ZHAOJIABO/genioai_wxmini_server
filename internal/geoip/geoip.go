package geoip

import (
	"net"
	"sync"

	"github.com/oschwald/geoip2-golang"
	"go.uber.org/zap"

	"va_visionai_server/internal/zlog"
)

var (
	db   *geoip2.Reader
	once sync.Once
	mu   sync.RWMutex
)

// InitGeoIP 初始化GeoIP数据库
// 使用sync.Once确保只初始化一次
func InitGeoIP(dbPath string) error {
	var err error
	once.Do(func() {
		db, err = geoip2.Open(dbPath)
	})
	return err
}

// GetCountryCode 根据IP地址获取国家代码
// 返回ISO 3166-1 alpha-2国家代码（如 "US", "CN", "SG"）
// 如果无法解析或IP无效，返回 "unknown"
func GetCountryCode(ip string) string {
	mu.RLock()
	defer mu.RUnlock()

	// 如果数据库未初始化或IP为空，返回unknown
	if db == nil || ip == "" {
		if zlog.Logger != nil {
			zlog.Logger.Debug("GetCountryCode: db is nil or ip is empty", zap.String("ip", ip))
		}
		return "unknown"
	}

	// 解析IP地址
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		if zlog.Logger != nil {
			zlog.Logger.Debug("GetCountryCode: parsedIP is nil", zap.String("ip", ip))
		}
		return "unknown"
	}

	// 检查是否为私有IP
	if isPrivateIP(parsedIP) {
		if zlog.Logger != nil {
			zlog.Logger.Debug("GetCountryCode: isPrivateIP", zap.String("ip", ip))
		}
		return "unknown"
	}

	// 查询国家信息
	record, err := db.Country(parsedIP)
	if err != nil {
		if zlog.Logger != nil {
			zlog.Logger.Debug("GetCountryCode: err", zap.String("ip", ip), zap.Error(err))
		}
		return "unknown"
	}

	// 返回国家代码
	if record.Country.IsoCode != "" {
		if zlog.Logger != nil {
			zlog.Logger.Debug("GetCountryCode: record.Country.IsoCode", zap.String("ip", ip), zap.String("country", record.Country.IsoCode))
		}
		return record.Country.IsoCode
	}

	// 如果国家代码为空，尝试返回注册国家代码
	if record.RegisteredCountry.IsoCode != "" {
		if zlog.Logger != nil {
			zlog.Logger.Debug("GetCountryCode: record.RegisteredCountry.IsoCode", zap.String("ip", ip), zap.String("country", record.RegisteredCountry.IsoCode))
		}
		return record.RegisteredCountry.IsoCode
	}

	if zlog.Logger != nil {
		zlog.Logger.Debug("GetCountryCode: unknown", zap.String("ip", ip))
	}
	return "unknown"
}

// isPrivateIP 检查IP是否为私有IP
func isPrivateIP(ip net.IP) bool {
	// 检查是否为回环地址
	if ip.IsLoopback() {
		return true
	}

	// 检查是否为私有网段
	privateIPBlocks := []string{
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // RFC3927 link-local
		"127.0.0.0/8",    // IPv4 loopback
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local addr
	}

	for _, block := range privateIPBlocks {
		_, subnet, err := net.ParseCIDR(block)
		if err != nil {
			continue
		}
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}

// Close 关闭GeoIP数据库连接
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if db != nil {
		db.Close()
		db = nil
	}
}

// GetCountryAndTimezone 根据IP地址获取国家代码和时区
// 需要使用 GeoLite2-City 数据库才能获取时区信息
// 返回:
//   - country: ISO 3166-1 alpha-2国家代码（如 "US", "CN"），无法解析时返回 "unknown"
//   - timezone: IANA 时区标识符（如 "Asia/Shanghai", "America/New_York"），无法解析时返回空字符串
func GetCountryAndTimezone(ip string) (country, timezone string) {
	mu.RLock()
	defer mu.RUnlock()

	// 如果数据库未初始化或IP为空，返回默认值
	if db == nil || ip == "" {
		if zlog.Logger != nil {
			zlog.Logger.Debug("GetCountryAndTimezone: db is nil or ip is empty", zap.String("ip", ip))
		}
		return "unknown", ""
	}

	// 解析IP地址
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		if zlog.Logger != nil {
			zlog.Logger.Debug("GetCountryAndTimezone: parsedIP is nil", zap.String("ip", ip))
		}
		return "unknown", ""
	}

	// 检查是否为私有IP
	if isPrivateIP(parsedIP) {
		if zlog.Logger != nil {
			zlog.Logger.Debug("GetCountryAndTimezone: isPrivateIP", zap.String("ip", ip))
		}
		return "unknown", ""
	}

	// 使用 City 方法查询（包含国家和时区信息）
	record, err := db.City(parsedIP)
	if err != nil {
		if zlog.Logger != nil {
			zlog.Logger.Debug("GetCountryAndTimezone: City lookup err", zap.String("ip", ip), zap.Error(err))
		}
		return "unknown", ""
	}

	// 获取国家代码
	country = record.Country.IsoCode
	if country == "" {
		country = record.RegisteredCountry.IsoCode
	}
	if country == "" {
		country = "unknown"
	}

	// 获取时区
	timezone = record.Location.TimeZone

	if zlog.Logger != nil {
		zlog.Logger.Debug("GetCountryAndTimezone: success",
			zap.String("ip", ip),
			zap.String("country", country),
			zap.String("timezone", timezone))
	}

	return country, timezone
}
