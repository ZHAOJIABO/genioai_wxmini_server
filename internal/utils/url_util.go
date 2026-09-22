package utils

import (
	"net/url"
	"regexp"
	"strings"

	"va_visionai_server/conf"
)

// CleanURL 移除URL中的查询参数，返回干净的URL
// 例如: "https://example.com/image.jpg?size=1080x525" -> "https://example.com/image.jpg"
func CleanURL(urlStr string) string {
	if !strings.HasPrefix(urlStr, "http") || !strings.Contains(urlStr, "?") {
		return urlStr
	}
	u, err := url.Parse(urlStr)
	if err == nil {
		u.RawQuery = ""
		return u.String()
	}
	parts := strings.Split(urlStr, "?")
	return parts[0]
}

// NormalizeUgcURL 规范化UGC图片URL
func NormalizeUgcURL(urlStr string) string {
	clean := CleanURL(urlStr)
	u, err := url.Parse(clean)
	if err != nil {
		return clean
	}
	u.Host = strings.ToLower(u.Host)
	return u.String()
}

// ExtractUgcMD5 从UGC URL路径中提取32位十六进制MD5（兼容 -low 与常见扩展名）
// 返回 (md5, true) 表示提取成功；否则返回 ("", false)
func ExtractUgcMD5(urlStr string) (string, bool) {
	clean := CleanURL(urlStr)
	u, err := url.Parse(clean)
	var path string
	if err == nil {
		path = u.Path
	} else {
		path = clean
	}
	lastSlash := strings.LastIndex(path, "/")
	file := path
	if lastSlash >= 0 {
		file = path[lastSlash+1:]
	}

	if dot := strings.LastIndex(file, "."); dot >= 0 {
		file = file[:dot]
	}
	file = strings.TrimSuffix(file, "-low")

	re := regexp.MustCompile(`(?i)[a-f0-9]{32}`)
	md5 := re.FindString(file)
	if md5 == "" {
		return "", false
	}
	return strings.ToLower(md5), true
}

// IsUgcHost 判断给定URL是否属于受信UGC域名（来源于配置）
func IsUgcHost(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)

	var allowed []string
	if conf.GlobalConfig.OssConfig.UgcAddr != "" {
		if ou, err := url.Parse(conf.GlobalConfig.OssConfig.UgcAddr); err == nil && ou.Host != "" {
			allowed = append(allowed, strings.ToLower(ou.Host))
		}
	}
	if conf.GlobalConfig.CosConfig.UgcAddr != "" {
		if cu, err := url.Parse(conf.GlobalConfig.CosConfig.UgcAddr); err == nil && cu.Host != "" {
			allowed = append(allowed, strings.ToLower(cu.Host))
		}
	}
	for _, h := range allowed {
		if host == h {
			return true
		}
	}
	return false
}

func GetLowQualityImageURL(urlStr string) string {
	if strings.HasSuffix(urlStr, "-low.jpg") {
		return urlStr
	}
	return strings.Replace(urlStr, ".jpg", "-low.jpg", 1)
}
