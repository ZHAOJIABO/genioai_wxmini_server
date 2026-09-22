package constants

// 性别常量定义
const (
	GenderUnknown = 0
	GenderMale    = 1
	GenderFemale  = 2
)

// MapGenderToString 将性别数字映射为字符串
func MapGenderToString(gender int) string {
	switch gender {
	case GenderMale:
		return "Male"
	case GenderFemale:
		return "Female"
	default:
		return "Unknown"
	}
}

// MapGenderToInt 将性别字符串映射为数字
func MapGenderToInt(gender string) int {
	switch gender {
	case "Male":
		return GenderMale
	case "Female":
		return GenderFemale
	default:
		return GenderUnknown
	}
}
