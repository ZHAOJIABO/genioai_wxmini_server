package constants

import vai "va_visionai_server/internal/va_interface"

const (
	ZH = "zh"
	EN = "en"
	RU = "ru"
	VI = "vi"
)

var languageMap = map[vai.Language]string{
	vai.Language_CHINESE:    ZH,
	vai.Language_ENGLISH:    EN,
	vai.Language_RUSSIAN:    RU,
	vai.Language_VIETNAMESE: VI,
}

func LanguageMap(lang vai.Language) string {
	if v, ok := languageMap[lang]; ok {
		return v
	}
	return "zh"
}
