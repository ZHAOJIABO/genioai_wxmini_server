package constants

const (
	LLMInputToken  = "input_token"
	LLMOutputToken = "output_token"
	LLMModelID     = "model_id"
)

const (
	NewChatTitle = "New Chat"

	SystemPromptBase            = "system_base"
	SystemPromptTitle           = "system_title"
	SystemPictureEvaluateScore  = "sys_picture_evaluate_score"
	SystemPictureQuestions      = "sys_picture_questions"
	SystemPictureVisionAnalysis = "sys_picture_vision_analysis"
	SystemPromptVideo           = "system_video"
	SystemPromptProfileChat     = "system_profile_chat"
	SystemSuggestQuestionModel  = "system_suggest_question_model"
	SystemStarlitQuestionModel  = "system_starlit_question_model"
	// Soulmate相关常量
	SystemSoulmateGenerateModel         = "system_soulmate_generate_model"
	SystemSoulmateGenerateLoverPrompt   = "system_soulmate_generate_lover_prompt"
	SystemSoulmateGenerateOtherPrompt   = "system_soulmate_generate_other_prompt"
	SystemSoulmateGeneratePicturePrompt = "system_soulmate_generate_picture_prompt"

	SystemConstellationEncouragingWords      = "sys_1"
	SystemConstellationFortune               = "sys_2"
	SystemConstellationFortuneGuide          = "sys_3"
	SystemConstellationFortuneOverall        = "sys_4"
	SystemConstellationFortuneInterpretation = "sys_5"

	SystemConstellationPowerTroubleSpirituality       = "sys_6"
	SystemConstellationPowerTroubleThinkingCreativity = "sys_7"
	SystemConstellationPowerTroubleSocialLife         = "sys_8"
	SystemConstellationPowerTroubleSelf               = "sys_9"
	SystemConstellationPowerTroubleLove               = "sys_10"
	SystemConstellationPowerTroubleConvention         = "sys_11"

	SystemConstellationUserProfileChatPrompt            = "sys_12"
	SystemConstellationUserSuggestQuestionsPrompt       = "sys_13"
	SystemConstellationChatTitlePrompt                  = "sys_14"
	SystemConstellationSingleUserStarlitQuestionsPrompt = "sys_16"
	SystemConstellationMultiUserStarlitQuestionsPrompt  = "sys_17"

	// SystemConstellationFortune               = "sys_constellation_fortune"
	// SystemConstellationEncouragingWords      = "sys_constellation_encouraging_words"
	// SystemConstellationFortuneGuide          = "sys_constellation_fortune_guide"
	// SystemConstellationFortuneOverall        = "sys_constellation_fortune_overall"
	// SystemConstellationFortuneInterpretation = "sys_constellation_fortune_interpretation"

	// SystemConstellationPowerTroubleSpirituality       = "sys_constellation_power_trouble_spirituality"
	// SystemConstellationPowerTroubleThinkingCreativity = "sys_constellation_power_trouble_thinking_creativity"
	// SystemConstellationPowerTroubleSocialLife         = "sys_constellation_power_trouble_social_life"
	// SystemConstellationPowerTroubleSelf               = "sys_constellation_power_trouble_self"
	// SystemConstellationPowerTroubleLove               = "sys_constellation_power_trouble_love"
	// SystemConstellationPowerTroubleConvention         = "sys_constellation_power_trouble_convention"

	MsgModelErr = "Model error, please try again later"

	//大健康prompt
	DailyRecipeRecommendPrompt = "sys_1"
	FoodAnalysisPrompt         = "sys_2"
	MealAnalysisPrompt         = "sys_3"
)

// 大健康模型配置
const (
	ConfigKeyRecipeRecommendModel = "_generate_recipe_recommend_model"
	ConfigKeyFoodAnalysisModel    = "_generate_food_analysis_model"
	ConfigKeyMealAnalysisModel    = "_generate_meal_analysis_model"
)

// 大健康content替换词
const (
	ReplaceContentUserHealthInfo  = "{health-replace-prompt-userHealthInfo}"
	ReplaceContentUserAndMealInfo = "{health-replace-prompt-userAndMealInfo}"
)

// astro 替换词
const (
	ReplaceContentSoulmateAge          = "{soulmate-replace-prompt-age}"
	ReplaceContentSoulmateGender       = "{soulmate-replace-prompt-gender}"
	ReplaceContentSoulmateRelationship = "{soulmate-replace-prompt-relationship}"
)
const (
	ComfyUIExecutorName            = "comfyui"
	Gpt4oExecutorName              = "gpt4o_old"
	Gpt4oExecutorNameV2            = "gpt4o"
	KlingExecutorName              = "kling"
	MinimaxExecutorName            = "minimax"
	CloudComfyExecutorName         = "cloud_comfy"
	GeminiExecutorName             = "gemini"
	Gpt4oText2ImageExecutorName    = "gpt4o_text2image"
	SoraExecutorName               = "sora"
	VectorEngineGeminiExecutorName = "vectorengine_gemini"
	QwenExecutorName               = "qwen"
	GPTImage2ExecutorName          = "gpt_image2"
	GPTImage2I2IExecutorName       = "gpt_image2_i2i"
)

// WANX模型名称常量
const (
	WANXModelName = "wanx"
)

// DefaultGPTImageModel 是 gptimage 执行器在 workflow 未配置 api_config.model_name 时使用的模型
const DefaultGPTImageModel = "gpt-image-2.5-sunburst-c"
