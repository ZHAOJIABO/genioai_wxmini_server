package constants

import "fmt"

// Task Progress Constants
const (
	ProgressSubmitted            = 5   // Task submitted and initial processing
	ProgressProviderProcessing   = 10  // Submitted to provider, awaiting completion
	ProgressProviderMaxSimulated = 80  // Maximum simulated progress while provider processes
	ProgressDownloading          = 85  // Provider completed, downloading results
	ProgressUploading            = 90  // Uploading to OSS
	ProgressFinalizing           = 95  // Final processing steps
	ProgressCompleted            = 100 // Task completed successfully
	ProgressFailed               = -1  // Task failed
)

const (
	// 工作流类型 0: 非banner
	WorkflowKindTypeNormal = 0
	// 工作流类型 1: 个人
	WorkflowBannerTypePersonal = 1
	// 工作流类型 2: 宠物
	WorkflowBannerTypePet = 2
)

const (
	// 工作流Banner 固定ID
	// 人物
	WorkflowBannerKindIDPerson = "person_banner"
	// 宠物
	WorkflowBannerKindIDPet = "pet_banner"
)

// Minimax API 类型常量
const (
	MinimaxApiTypeText2Video    = "text2video"
	MinimaxApiTypeImage2Video   = "image2video"
	MinimaxApiTypeTemplateVideo = "video_template_generation"
)

// Minimax 模板ID常量
const (
	MinimaxTemplateIDDiving      = "392753057216684038" // 跳水
	MinimaxTemplateIDRings       = "393881433990066176" // 吊环
	MinimaxTemplateIDPUBG        = "393769180141805569" // 绝地求生
	MinimaxTemplateIDLabubu      = "394246956137422856" // 万物皆可labubu
	MinimaxTemplateIDMcDelivery  = "393879757702918151" // 麦当劳宠物外卖员
	MinimaxTemplateIDTibetan     = "393766210733957121" // 藏族风写真
	MinimaxTemplateIDDepressed   = "394125185182695432" // 生无可恋
	MinimaxTemplateIDLoveLetter  = "393857704283172864" // 情书写真
	MinimaxTemplateIDFemaleModel = "393866076583718914" // 女模特试穿广告
	MinimaxTemplateIDSeasons     = "398574688191234048" // 四季写真
	MinimaxTemplateIDMaleModel   = "393876118804459526" // 男模特试穿广告
)

// GetMinimaxTemplateID 根据模板名获取模板ID
func GetMinimaxTemplateID(templateName string) (string, error) {
	templateMap := map[string]string{
		"跳水":         MinimaxTemplateIDDiving,
		"吊环":         MinimaxTemplateIDRings,
		"绝地求生":       MinimaxTemplateIDPUBG,
		"万物皆可labubu": MinimaxTemplateIDLabubu,
		"麦当劳宠物外卖员":   MinimaxTemplateIDMcDelivery,
		"藏族风写真":      MinimaxTemplateIDTibetan,
		"生无可恋":       MinimaxTemplateIDDepressed,
		"情书写真":       MinimaxTemplateIDLoveLetter,
		"女模特试穿广告":    MinimaxTemplateIDFemaleModel,
		"四季写真":       MinimaxTemplateIDSeasons,
		"男模特试穿广告":    MinimaxTemplateIDMaleModel,
	}

	if templateID, exists := templateMap[templateName]; exists {
		return templateID, nil
	}
	return "", fmt.Errorf("未知的模板名: %s", templateName)
}

// MinimaxErrorInfo 用于对齐 Minimax 错误码的人类可读信息
type MinimaxErrorInfo struct {
	// Title 给用户展示的简要标题，如“系统异常/系统繁忙/信息异常/未知错误”
	Title string
	// Description 详细描述，给用户或日志展示的更友好提示
	Description string
	// Category 分类标签，便于统计与后续扩展（与 Title 保持一致即可）
	Category string
}

// 预设的错误分类与默认描述
var (
	minimaxCategorySystemException = "System Error"
	minimaxCategorySystemBusy      = "System Busy"
	minimaxCategoryInfoException   = "NSFW"

	// 默认描述
	minimaxDescSystemException = "System busy. Please try again later."
	minimaxDescSystemBusy      = "Request timed out. Please try again later."
	minimaxDescInfoException   = "Please check your input and try again."
)

// minimaxErrorCodeMap 维护明确列出的错误码与信息映射
// 覆盖图片中枚举的错误码；若未命中则由 GetMinimaxErrorInfo 做范围与默认处理
var minimaxErrorCodeMap = map[uint16]MinimaxErrorInfo{
	// 系统异常类（图片中示例）
	1000: {Title: minimaxCategorySystemException, Description: minimaxDescSystemException, Category: minimaxCategorySystemException},
	1004: {Title: minimaxCategorySystemException, Description: minimaxDescSystemException, Category: minimaxCategorySystemException},
	1008: {Title: minimaxCategorySystemException, Description: minimaxDescSystemException, Category: minimaxCategorySystemException},
	1024: {Title: minimaxCategorySystemException, Description: minimaxDescSystemException, Category: minimaxCategorySystemException},
	1033: {Title: minimaxCategorySystemException, Description: minimaxDescSystemException, Category: minimaxCategorySystemException},
	2018: {Title: minimaxCategorySystemException, Description: minimaxDescSystemException, Category: minimaxCategorySystemException},
	2049: {Title: minimaxCategorySystemException, Description: minimaxDescSystemException, Category: minimaxCategorySystemException},

	// 信息异常类（参数/内容问题）
	1026: {Title: minimaxCategoryInfoException, Description: minimaxDescInfoException, Category: minimaxCategoryInfoException},
	1027: {Title: minimaxCategoryInfoException, Description: minimaxDescInfoException, Category: minimaxCategoryInfoException},
	1039: {Title: minimaxCategoryInfoException, Description: minimaxDescInfoException, Category: minimaxCategoryInfoException},
	1041: {Title: minimaxCategoryInfoException, Description: minimaxDescInfoException, Category: minimaxCategoryInfoException},
	1042: {Title: minimaxCategoryInfoException, Description: minimaxDescInfoException, Category: minimaxCategoryInfoException},
	1043: {Title: minimaxCategoryInfoException, Description: minimaxDescInfoException, Category: minimaxCategoryInfoException},
	1044: {Title: minimaxCategoryInfoException, Description: minimaxDescInfoException, Category: minimaxCategoryInfoException},

	// 系统繁忙类
	1001: {Title: minimaxCategorySystemBusy, Description: minimaxDescSystemBusy, Category: minimaxCategorySystemBusy},
	1002: {Title: minimaxCategorySystemBusy, Description: minimaxDescSystemBusy, Category: minimaxCategorySystemBusy},

	// 保持兼容：旧实现中存在 10000 的兜底错误
	10000: {Title: minimaxCategorySystemException, Description: minimaxDescSystemException, Category: minimaxCategorySystemException},
}

// GetMinimaxErrorInfo 根据错误码返回结构化错误信息
// 覆盖范围：1000-1044, 2018, 2049；其余未知码给出“未知/系统异常”的默认提示
func GetMinimaxErrorInfo(code uint16) MinimaxErrorInfo {
	if info, ok := minimaxErrorCodeMap[code]; ok {
		return info
	}

	// 图片覆盖范围内但未显式枚举的码，按系统异常处理
	if (code >= 1000 && code <= 1044) || code == 2018 || code == 2049 {
		return MinimaxErrorInfo{
			Title:       minimaxCategorySystemException,
			Description: minimaxDescSystemException,
			Category:    minimaxCategorySystemException,
		}
	}

	// 其他未知码
	return MinimaxErrorInfo{
		Title:       minimaxCategorySystemException,
		Description: minimaxDescSystemException,
		Category:    minimaxCategorySystemException,
	}
}

// GetGenericErrorInfo provides unified error mapping for executors without specific error codes
// Currently returns "System Busy" for all errors, but can be extended in the future
func GetGenericErrorInfo(errMsg string) (reason, fullReason string) {
	// For now, all errors are mapped to "System Busy" as requested
	// This can be extended in the future to categorize different error types
	return "System Busy", "System busy. Please try again later."
}
