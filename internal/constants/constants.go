package constants

import (
	"errors"

	"va_visionai_server/conf"
	vai "va_visionai_server/internal/va_interface"
)

const (
	VISIONAI            = "visionai:"
	RedisKeyUserSession = VISIONAI + "user:session"

	RedisKeyUserPopSubscribePage     = VISIONAI + "user:pop_subscribe_page"
	RedisKeyUserAmount               = VISIONAI + "user:amount"
	RedisKeyUserAmountHashFieldText  = "text_chat_amount"
	RedisKeyUserAmountHashFieldImage = "image_chat_amount"
	RedisKeyUserAmountLock           = VISIONAI + "user:amount:lock"
	RedisKeySubscriptionCheckerLock  = VISIONAI + "subscription:checker:lock"
	RedisKeyExpiredUsersTemp         = VISIONAI + "temp_expired_users"
	RedisKeyExpiredUsersProcessed    = VISIONAI + "processed_expired_users"
	RedisKeyExpiredUsersFailed       = VISIONAI + "failed_expired_users"
	RedisKeyUserIDMapping            = VISIONAI + "user_id_mapping"
	RedisKeyPictureTaskProgress      = VISIONAI + "picture_task_progress"

	RedisKeyUpdateChatLastMsgTimeLock = VISIONAI + "update_chat_last_msg_time_lock"
)
const (
	GOCALAI           = "gocalai:"
	RedisKeyNutrition = GOCALAI + "user:nutrition" // 营养数据的基础键
)
const (
	ProjectPicflow = "com.bluex.picflow"
)

type ServiceEventType string

const (
	ServiceEvent         = "ServiceEvent"
	ServiceEventComplete = "Complete"

	EventLogin                         ServiceEventType = "Login"
	EventLoginComplete                 ServiceEventType = EventLogin + ServiceEventComplete
	EventNewUser                       ServiceEventType = "NewUser"
	EventNewUserComplete               ServiceEventType = EventNewUser + ServiceEventComplete
	EventUploadFile                    ServiceEventType = "UploadFile"
	EventUploadFileComplete            ServiceEventType = EventUploadFile + ServiceEventComplete
	EventSysUploadFile                 ServiceEventType = "SysUploadFile"
	EventSysUploadFileComplete         ServiceEventType = EventSysUploadFile + ServiceEventComplete
	EventSendChatMessageStream         ServiceEventType = "SendChatMessageStream"
	EventSendChatMessageStreamComplete ServiceEventType = EventSendChatMessageStream + ServiceEventComplete
	EventHeatTracker                   ServiceEventType = "HeatTracker"
	EventHeatTrackerComplete           ServiceEventType = EventHeatTracker + ServiceEventComplete
	EventNewChat                       ServiceEventType = "NewChat"
	EventNewChatComplete               ServiceEventType = EventNewChat + ServiceEventComplete
	EventNewAudio                      ServiceEventType = "NewAudio"
	EventNewAudioComplete              ServiceEventType = EventNewAudio + ServiceEventComplete
	EventCreateOrder                   ServiceEventType = "CreateOrder"
	EventCreateOrderComplete           ServiceEventType = EventCreateOrder + ServiceEventComplete
	EventPaymentResultCallback         ServiceEventType = "PaymentResultCallback"
	EventPaymentResultCallbackComplete ServiceEventType = EventPaymentResultCallback + ServiceEventComplete
	EventResetUserAmounts              ServiceEventType = "ResetUserAmounts"
	EventResetUserAmountsComplete      ServiceEventType = EventResetUserAmounts + ServiceEventComplete
	EventPaymentSuccess                                 = "PaymentSuccess"
	EventGetASAToken                                    = "GetASAToken"
	// EventTypeSubscriptionCreated 订阅创建事件
	EventTypeSubscriptionCreated = "subscription.created"
	// EventTypeSubscriptionRenewed 订阅续订事件
	EventTypeSubscriptionRenewed = "subscription.renewed"
	// EventTypeOnetimePurchase 一次性购买事件
	EventTypeOnetimePurchase = "onetime.purchase"
	//大健康相关
	EventGetHealthProfileComplete            ServiceEventType = "GetHealthProfileComplete"
	EventEditHealthProfileComplete           ServiceEventType = "EditHealthProfileComplete"
	EventCreateHealthProfile                 ServiceEventType = "CreateHealthProfile"
	EventUploadFoodImage                     ServiceEventType = "UploadFoodImage"
	EventUploadFoodImageComplete             ServiceEventType = EventUploadFile + ServiceEventComplete
	EventUpsertMealRecord                    ServiceEventType = "UpsertMealRecord"
	EventUpsertMealRecordComplete                             = EventUpsertMealRecord + ServiceEventComplete
	EventGetMealRecord                                        = "GetMealRecord"
	EventGetMealRecordComplete                                = EventGetMealRecord + ServiceEventComplete
	EventGetMonthlyNutrition                 ServiceEventType = "GetMonthlyNutrition"
	EventGetMonthlyNutritionComplete         ServiceEventType = EventGetMonthlyNutrition + ServiceEventComplete
	EventGetVitAndMinRecommendIntake         ServiceEventType = "GetVitAndMinRecommendIntake"
	EventGetVitAndMinRecommendIntakeComplete                  = EventGetVitAndMinRecommendIntake + ServiceEventComplete
	EventGetDailyRecipeRecommend             ServiceEventType = "GetDailyRecipeRecommend"
	EventGetDailyRecipeRecommendComplete                      = EventGetDailyRecipeRecommend + ServiceEventComplete
	EventDeleteMealRecord                    ServiceEventType = "DeleteMealRecord"
	EventDeleteMealRecordComplete                             = EventDeleteMealRecord + ServiceEventComplete
	EventDeleteHealthAccount                 ServiceEventType = "DeleteHealthAccount"
	EventDeleteHealthAccountComplete                          = EventDeleteHealthAccount + ServiceEventComplete
	EventGetFloatingWindowConfig             ServiceEventType = "GetFloatingWindowConfig"
	EventGetFloatingWindowConfigComplete                      = EventGetFloatingWindowConfig + ServiceEventComplete
	EventUpsertMealRecordAmountNotEnough     ServiceEventType = "UpsertMealRecordAmountNotEnough"
	EventUploadFoodImageAmountNotEnough      ServiceEventType = "UploadFoodImageAmountNotEnough"
	EventGetSubscriptionExitConfig           ServiceEventType = "GetSubscriptionExitConfig"
	EventGetSubscriptionExitConfigComplete   ServiceEventType = EventGetSubscriptionExitConfig + ServiceEventComplete
	//starlit
	EventGetStarlitQuestionsWithProfile         ServiceEventType = "GetStarlitQuestionsWithProfile"
	EventGetStarlitQuestionsWithProfileComplete                  = EventGetStarlitQuestionsWithProfile + ServiceEventComplete
	//soulmate
	EventGetSoulmateInfo                  ServiceEventType = "GetSoulmateInfo"
	EventGetSoulmateInfoComplete                           = EventGetSoulmateInfo + ServiceEventComplete
	EventGenerateSoulmateRoleText         ServiceEventType = "GenerateSoulmateRoleText"
	EventGenerateSoulmateRoleTextComplete                  = EventGenerateSoulmateRoleText + ServiceEventComplete
	EventGetUserWeeklyData                ServiceEventType = "GetUserWeeklyData"
	EventGetUserWeeklyDataComplete        ServiceEventType = EventGetUserWeeklyData + ServiceEventComplete
	EventSubmitSoulmateAvatarTask         ServiceEventType = "SubmitSoulmateAvatarTask"
	EventSubmitSoulmateAvatarTaskComplete ServiceEventType = EventSubmitSoulmateAvatarTask + ServiceEventComplete
)

const (
	VoiceWaiting = 0
	VoicePending = 1
	VoiceSuccess = 2
	VoiceError   = 100
)

const (
	ANDROID = "Android"
	IOS     = "IOS"
	MAC     = "MAC"
)

const (
	DateOnly = "20060102"
	TimeOnly = "20060102150405"
)

const (
	ErrMsgSuccess             = "Success"
	ErrMsgInvalidParam        = "Invalid Param"
	ErrMsgInvalidRequest      = "Invalid Request"
	ErrMsgRequestFailed       = "Request Failed"
	ErrMsgInvalidAccessToken  = "Invalid Access Token"
	ErrMsgExpiredAccessToken  = "Expired Access Token"
	ErrMsgInvalidRefreshToken = "Invalid Refresh Token"
	ErrMsgExpiredRefreshToken = "Expired Refresh Token"
	ErrMsgInvalidUser         = "Invalid User"
	ErrMsgInvalidUserID       = "Invalid UserID"
	ErrMsgUserBlocked         = "User Blocked"

	ErrMsgInvalidApp    = "Invalid App"
	ErrMsgInvalidDevice = "Invalid Device"

	ErrMsgInvalidModelID = "Invalid ModelID"
	// 用户行为
	ErrMsgInvalidPhoneNumber        = "Invalid Phone Number"
	ErrMsgInvalidVerifyCode         = "Invalid Verify Code"
	ErrMsgInvalidEmail              = "Invalid Email"
	ErrMsgAmountExhausted           = "Amount Exhausted"
	ErrMsgSendMessageTooOften       = "Send Message Too Often"
	ErrMsgFileTooLarge              = "File Too Large"
	ErrMsgDailyAmountLimitExceeded  = "Daily Amount Limit Exceeded"
	ErrMsgFailedByAlreadySignin     = "Failed By Already Signin"
	ErrMsgFailedByAdAwardLimit      = "Failed By Ad Award Limit"
	ErrMsgDailyTokenLimitExceeded   = "Daily Token Limit Exceeded"
	ErrMsgUnknown                   = "Unknown"
	ErrMsgInternalServer            = "Internal Server Error"
	ErrMsgDailyTokenLimitExceededEN = "Daily limit reached. Subscribe to continue."

	ErrMsgContentSafePolicy = "Content Safe Policy"
	ErrMsgUserNameExists    = "User Name Already Exists"

	ErrMsgRequestTooMany              = "Request Too Many"
	ErrMsgTaskProcessingLimitExceeded = "Processing Task Limit Exceeded"
	ErrMsgTaskQueueLimitExceeded      = "Task Queue Limit Exceeded, Please Wait for Existing Tasks to Complete"
	ErrMsgTaskConcurrentLimitExceeded = "Task Concurrent Limit Exceeded, Task Will Be Queued"
	ErrMsgRequestFailedEN             = "Request Failed"
	ErrMsgSubscribeExpired            = "Subscription Expired"
	ErrMsgUserAmountNotEnough         = "User Amount Not Enough"
	ErrMsgSubscribeNotActivate        = "Subscription Not Activate"
	ErrMsgHealthNoFood                = "No Food Detected"
	ErrMsgInvalidCoupon               = "Invalid Coupon"
	ErrMsgInvalidInviteCode           = "Invalid Invite Code"
)

var (
	ERR_INVALID_REQUEST                = errors.New(ErrMsgInvalidRequest)
	ERR_REQUEST_FAILED                 = errors.New(ErrMsgRequestFailed)
	ERR_INVALID_TOKEN                  = errors.New(ErrMsgInvalidAccessToken)
	ERR_TOKEN_EXPIRED                  = errors.New(ErrMsgExpiredAccessToken)
	ERR_INVALID_REFRESH_TOKEN          = errors.New(ErrMsgInvalidRefreshToken)
	ERR_REFRESH_TOKEN_EXPIRED          = errors.New(ErrMsgExpiredRefreshToken)
	ERR_INVALID_PARAM                  = errors.New(ErrMsgInvalidParam)
	ERR_INVALID_EMAIL                  = errors.New(ErrMsgInvalidEmail)
	ERR_INVALID_MODELID                = errors.New(ErrMsgInvalidModelID)
	ERR_USER_BLOCKED                   = errors.New(ErrMsgUserBlocked)
	ERR_USER_INVALID                   = errors.New(ErrMsgInvalidUser)
	ERR_INVALID_USER_ID                = errors.New(ErrMsgInvalidUserID)
	ERR_INTERNAL_SERVER                = errors.New(ErrMsgInternalServer)
	ERR_CONTENT_SAFE_POLICY            = errors.New(ErrMsgContentSafePolicy)
	ERR_DAILY_TOLEN_LIMIT              = errors.New(ErrMsgDailyTokenLimitExceeded)
	ERR_DAILY_TOLEN_LIMIT_EN           = errors.New(ErrMsgDailyTokenLimitExceededEN)
	ERR_USER_NAME_EXISTS               = errors.New(ErrMsgUserNameExists)
	ERR_REQUEST_TOO_MANY               = errors.New(ErrMsgRequestTooMany)
	ERR_TASK_QUEUE_LIMIT_EXCEEDED      = errors.New(ErrMsgTaskQueueLimitExceeded)
	ERR_TASK_CONCURRENT_LIMIT_EXCEEDED = errors.New(ErrMsgTaskConcurrentLimitExceeded)
	ERR_TASK_PROCESSING_LIMIT_EXCEEDED = errors.New(ErrMsgTaskProcessingLimitExceeded)
	ERR_REQUEST_FAILED_EN              = errors.New(ErrMsgRequestFailedEN)
	ERR_SUBSCRIBE_EXPIRED              = errors.New(ErrMsgSubscribeExpired)
	ERR_USER_AMOUNT_NOT_ENOUGH         = errors.New(ErrMsgUserAmountNotEnough)
	ERR_TASK_CANNOT_BE_REtried         = errors.New("task cannot be retried")
	ERR_SUBSCRIBE_NOT_ACTIVATE         = errors.New(ErrMsgSubscribeNotActivate)
	ERR_HEALTH_NO_FOOD                 = errors.New("empty response from model: no food detected")
	ERR_INVALID_COUPON                 = errors.New(ErrMsgInvalidCoupon)
	ERR_INVALID_INVITE_CODE            = errors.New(ErrMsgInvalidInviteCode)
)

var (
	CONTENT_SAFE_POLICY_MSG_ZH = "检测到文本信息涉及敏感话题，请排除敏感信息后再进行问答。"
	CONTENT_SAFE_POLICY_MSG_EN = "The text information contains sensitive topics. Please exclude sensitive information before asking questions."
)

func MappingOS(os int32) string {
	osName := "Unknown"
	switch os {
	case 1:
		osName = ANDROID
	case 2:
		osName = IOS
	case 3:
		osName = MAC
	}
	return osName
}

func MappingModelName(modelID vai.Model) string {
	switch modelID {
	case vai.Model_MODEL_GPT4O:
		return "gpt-4o"
	case vai.Model_MODEL_GPT4O_MINI:
		return "gpt-4o-mini"
		// 暂不支持gpt4 如果选了4 启用4o模型
	case vai.Model_MODEL_GPT4:
		return "gpt-4o"
	case vai.Model_MODEL_DOUBAO:
		return "doubao"
	case vai.Model_MODEL_DOUBAO_PRO:
		return "doubao-pro"
	case vai.Model_MODEL_GPTO1_PREVIEW:
		return "o1-preview"
	case vai.Model_MODEL_GPTO1_MINI:
		return "o1-mini"
	case vai.Model_MODEL_KIMI_VISION_PREVIEW:
		return conf.GlobalConfig.LlmConfig.Kimi.ModelName
	case vai.Model_MODEL_GPT4_1:
		return "gpt-4.1"
	default:
		return ""
	}
}

var errMessageMap = map[vai.StatusCode]string{
	vai.StatusCode_SUCCESS: ErrMsgSuccess,
	// 非法参数
	vai.StatusCode_INVALID_PARAM: ErrMsgInvalidParam,
	// 非法请求
	vai.StatusCode_INVALID_REQUEST: ErrMsgInvalidRequest,
	// 请求失败
	vai.StatusCode_REQUEST_FAILED: ErrMsgRequestFailed,

	// 通用问题
	// AccessToken错误
	vai.StatusCode_INVALID_ACCESS_TOKEN: ErrMsgInvalidAccessToken,
	// AccessToken过期，需要刷新token
	vai.StatusCode_EXPIRED_ACCESS_TOKEN: ErrMsgExpiredAccessToken,
	// RefreshToken错误，需要重新登录
	vai.StatusCode_INVALID_REFRESH_TOKEN: ErrMsgInvalidRefreshToken,
	// 无效的用户ID
	vai.StatusCode_INVALID_USER: ErrMsgInvalidUser,
	// 无效的APP
	vai.StatusCode_INVALID_APP: ErrMsgInvalidApp,
	// 无效的设别信息（可能对部分设备ID进行禁止访问）
	vai.StatusCode_INVALID_DEVICE: ErrMsgInvalidDevice,

	// 用户行为
	// 无效手机号
	vai.StatusCode_INVALID_PHONE_NUMBER: ErrMsgInvalidPhoneNumber,
	// 验证码验证错误
	vai.StatusCode_INVALID_VERIFY_CODE: ErrMsgInvalidVerifyCode,
	// 积分消耗完毕
	vai.StatusCode_AMOUNT_EXHAUSTED: ErrMsgAmountExhausted,
	// 发送消息过于频繁
	vai.StatusCode_SEND_MESSAGE_TOO_OFTEN: ErrMsgSendMessageTooOften,
	// 文件过大
	vai.StatusCode_FILE_TOO_LARGE: ErrMsgFileTooLarge,
	// 达到日积分奖励上限
	vai.StatusCode_DAILY_AMOUNT_LIMIT_EXCEEDED: ErrMsgDailyAmountLimitExceeded,
	// 签到失败因为已签到
	vai.StatusCode_FAILED_BY_ALREADY_SIGNIN: ErrMsgFailedByAlreadySignin,
	// 广告激励次数超过限制
	vai.StatusCode_FAILED_BY_AD_AWARD_LIMIT: ErrMsgFailedByAdAwardLimit,
	// 达到日使用上限
	vai.StatusCode_DAILY_TOKEN_LIMIT_EXCEEDED: ErrMsgDailyTokenLimitExceeded,
	// 无效的优惠券
	vai.StatusCode_INVALID_COUPON: ErrMsgInvalidCoupon,

	// 用户名已存在
	vai.StatusCode(4011): ErrMsgUserNameExists,

	vai.StatusCode_TASK_PROCESSING_LIMIT_EXCEEDED: ErrMsgTaskProcessingLimitExceeded,

	vai.StatusCode_CREDIT_POINT_NOT_ENOUGH: ErrMsgUserAmountNotEnough,
	vai.StatusCode_SUBSCRIBE_EXPIRED:       ErrMsgSubscribeExpired,
	vai.StatusCode_SUBSCRIBE_NOT_ACTIVATE:  ErrMsgSubscribeNotActivate,
	//大健康相关
	// 未识别出食物
	vai.StatusCode_HEALTH_NO_FOOD: ErrMsgHealthNoFood,
}

var ErrMsgMapCode = map[string]vai.StatusCode{
	ErrMsgSuccess:                     vai.StatusCode_SUCCESS,
	ErrMsgInvalidParam:                vai.StatusCode_INVALID_PARAM,
	ErrMsgInvalidRequest:              vai.StatusCode_INVALID_REQUEST,
	ErrMsgRequestFailed:               vai.StatusCode_REQUEST_FAILED,
	ErrMsgInvalidAccessToken:          vai.StatusCode_INVALID_ACCESS_TOKEN,
	ErrMsgExpiredAccessToken:          vai.StatusCode_EXPIRED_ACCESS_TOKEN,
	ErrMsgInvalidRefreshToken:         vai.StatusCode_INVALID_REFRESH_TOKEN,
	ErrMsgInvalidUser:                 vai.StatusCode_INVALID_USER,
	ErrMsgInvalidApp:                  vai.StatusCode_INVALID_APP,
	ErrMsgInvalidDevice:               vai.StatusCode_INVALID_DEVICE,
	ErrMsgInvalidPhoneNumber:          vai.StatusCode_INVALID_PHONE_NUMBER,
	ErrMsgInvalidVerifyCode:           vai.StatusCode_INVALID_VERIFY_CODE,
	ErrMsgInvalidEmail:                vai.StatusCode_INVALID_PARAM,
	ErrMsgAmountExhausted:             vai.StatusCode_AMOUNT_EXHAUSTED,
	ErrMsgSendMessageTooOften:         vai.StatusCode_SEND_MESSAGE_TOO_OFTEN,
	ErrMsgFileTooLarge:                vai.StatusCode_FILE_TOO_LARGE,
	ErrMsgDailyAmountLimitExceeded:    vai.StatusCode_DAILY_AMOUNT_LIMIT_EXCEEDED,
	ErrMsgFailedByAlreadySignin:       vai.StatusCode_FAILED_BY_ALREADY_SIGNIN,
	ErrMsgFailedByAdAwardLimit:        vai.StatusCode_FAILED_BY_AD_AWARD_LIMIT,
	ErrMsgDailyTokenLimitExceeded:     vai.StatusCode_DAILY_TOKEN_LIMIT_EXCEEDED,
	ErrMsgUserNameExists:              vai.StatusCode(4011),
	ErrMsgTaskProcessingLimitExceeded: vai.StatusCode_TASK_PROCESSING_LIMIT_EXCEEDED,
	ErrMsgTaskQueueLimitExceeded:      vai.StatusCode_TASK_PROCESSING_LIMIT_EXCEEDED,
	ErrMsgTaskConcurrentLimitExceeded: vai.StatusCode_TASK_PROCESSING_LIMIT_EXCEEDED,
	ErrMsgSubscribeNotActivate:        vai.StatusCode_SUBSCRIBE_NOT_ACTIVATE,
	ErrMsgUserAmountNotEnough:         vai.StatusCode_CREDIT_POINT_NOT_ENOUGH,
	ErrMsgSubscribeExpired:            vai.StatusCode_SUBSCRIBE_EXPIRED,
	ErrMsgInvalidCoupon:               vai.StatusCode_INVALID_COUPON,
}

func CodeMsg(code vai.StatusCode) string {
	if msg, ok := errMessageMap[code]; ok {
		return msg
	}
	return ErrMsgRequestFailed
}

func GetContentSafePolicyMsg(lang vai.Language) string {
	switch lang {
	case vai.Language_CHINESE:
		return CONTENT_SAFE_POLICY_MSG_ZH
	case vai.Language_ENGLISH:
		return CONTENT_SAFE_POLICY_MSG_EN
	default:
		return CONTENT_SAFE_POLICY_MSG_EN
	}
}

const (
	//ImageUploadFirst   = "ImageUploadFirst"
	QnACompletionFirst = "game_addiction"
	PaymentFirst       = "active_pay"
	RegisterByPhoneNum = "active_register"
	AppFirstOpen       = "active"
)

func MappingConstellation(constellation string) vai.Constellation {
	switch constellation {
	case "Aries":
		return vai.Constellation_CONSTELLATION_ARIES
	case "Taurus":
		return vai.Constellation_CONSTELLATION_TAURUS
	case "Gemini":
		return vai.Constellation_CONSTELLATION_GEMINI
	case "Cancer":
		return vai.Constellation_CONSTELLATION_CANCER
	case "Leo":
		return vai.Constellation_CONSTELLATION_LEO
	case "Virgo":
		return vai.Constellation_CONSTELLATION_VIRGO
	case "Libra":
		return vai.Constellation_CONSTELLATION_LIBRA
	case "Scorpio":
		return vai.Constellation_CONSTELLATION_SCORPIO
	case "Sagittarius":
		return vai.Constellation_CONSTELLATION_SAGITTARIUS
	case "Capricorn":
		return vai.Constellation_CONSTELLATION_CAPRICORN
	case "Aquarius":
		return vai.Constellation_CONSTELLATION_AQUARIUS
	case "Pisces":
		return vai.Constellation_CONSTELLATION_PISCES
	default:
		return vai.Constellation_CONSTELLATION_UNKNOWN
	}
}

const (
	ProjectIdPicLib      = "com.domob.piclib"
	ProjectIdPicFlow     = "com.bluex.picflow"
	ProjectIdVisionAI    = "com.domob.visionai"
	ProjectIdVisualAI    = "com.bluex.visualai"
	ProjectIdSolacex     = "com.bluex.solacex"
	ProjectIdDietAI      = "com.bluex.dietai"
	ProjectIdGenioAIWeb  = "com.web.genioai" // Web 端项目 ID
)

// 图片压缩尺寸
const MaxImageSize = 1024 * 1024 // 最大 1m

const (
	NoFoodWasDetectedEN              = "Recognition failed. No food was detected."
	NoFoodWasDetectedZH              = "识别失败，未检测到食物"
	ErrMsgGetJsonFailed              = "获取json失败"
	ErrMsgCheckProductIdConfigFailed = "校验套餐ID配置失败"
)

func IsPicflowProject(projectID string) bool {
	return projectID == ProjectPicflow || projectID == ProjectIdPicLib
}

// soulmate相关

const (
	SoulmateRoleNameLover     = "the_most_crush_worthy_lover"
	SoulmateRoleNameFriend    = "the_most_intimate_friend"
	SoulmateRoleNameColleague = "the_most_compatible_colleague"
	SoulmateRoleNameLeader    = "the_most_reliable_leader"
	SoulmateRoleNameClassmate = "the_most_unforgettable_classmate"
)

// MapSoulmateRoleNameToString 将角色类型映射为字符串
func MapSoulmateRoleNameToString(roleType string) string {
	switch roleType {
	case SoulmateRoleNameLover:
		return "Lover"
	case SoulmateRoleNameFriend:
		return "Friend"
	case SoulmateRoleNameColleague:
		return "Colleague"
	case SoulmateRoleNameLeader:
		return "Leader"
	case SoulmateRoleNameClassmate:
		return "Classmate"
	default:
		return "未知"
	}
}

// MapSoulmateGenderToString 将角色类型映射为字符串
func MapSoulmateGenderToString(gender vai.Gender) string {
	switch gender {
	case vai.Gender_Gender_MALE:
		return "Male"
	case vai.Gender_Gender_FEMALE:
		return "Female"
	default:
		return "Unknown"
	}
}
