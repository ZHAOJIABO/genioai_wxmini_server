package model

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type WorkflowKind struct {
	gorm.Model
	KindID    string      `json:"kind_id" gorm:"column:kind_id;type:varchar(64)"`
	Label     string      `json:"label" gorm:"column:label;type:varchar(64)"`
	Icon      string      `json:"icon" gorm:"column:icon;type:varchar(64)"`
	Language  string      `json:"language" gorm:"column:language;type:varchar(64)"`
	Workflows []*Workflow `json:"workflows" gorm:"-"`
	Status    int32       `json:"status" gorm:"column:status;type:int"`
	SortOrder int32       `json:"sort_order" gorm:"column:sort_order;type:int"`

	KindType               string    `json:"kind_type" gorm:"column:kind_type;type:varchar(64)"`
	Theme                  ThemeList `json:"theme" gorm:"column:theme;type:json"`
	BannerType             int       `json:"banner_type" gorm:"column:banner_type;type:tinyint(1);default:0"`
	CoverPicturesJson      string    `json:"cover_pictures_json" gorm:"column:cover_pictures_json;type:text"`
	DisplayStyle           string    `json:"display_style" gorm:"column:display_style;type:varchar(64)"`
	Description            string    `json:"description" gorm:"column:description;type:varchar(255)"`
	IOSupportVersions      string    `json:"ios_support_versions" gorm:"column:ios_support_versions;type:varchar(16)"`
	AndroidSupportVersions string    `json:"android_support_versions" gorm:"column:android_support_versions;type:varchar(16)"`
}

type ThemeList []string

func (tl *ThemeList) Value() (driver.Value, error) {
	if len(*tl) == 0 {
		return nil, nil
	}
	return json.Marshal(tl)
}

func (tl *ThemeList) Scan(value interface{}) error {
	if value == nil {
		*tl = ThemeList{}
		return nil
	}

	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, tl)
	case string:
		return json.Unmarshal([]byte(v), tl)
	default:
		*tl = ThemeList{}
		return nil
	}
}

func (wk *WorkflowKind) GetThemes() []string {
	return []string(wk.Theme)
}

func (wk *WorkflowKind) SetThemes(themes []string) {
	wk.Theme = ThemeList(themes)
}

func (wk *WorkflowKind) GetPrimaryTheme() string {
	if len(wk.Theme) > 0 {
		return wk.Theme[0]
	}
	return ""
}

type Workflow struct {
	gorm.Model

	WorkflowID         string                  `json:"workflow_id" gorm:"column:workflow_id;type:varchar(64);uniqueIndex"`
	KindID             string                  `json:"kind_id" gorm:"column:kind_id;type:varchar(64)"`
	Language           string                  `json:"language" gorm:"column:language;type:varchar(64)"`
	Title              string                  `json:"title" gorm:"column:title;type:varchar(64)"`
	Description        string                  `json:"description" gorm:"column:description;type:text"`
	Icon               string                  `json:"icon" gorm:"column:icon;type:text"`
	ExampleImages      []*WorkflowExampleImage `json:"example_images" gorm:"-"`
	ExampleJson        string                  `json:"example_json" gorm:"column:example_json"`
	InputsJSON         string                  `json:"-" gorm:"column:inputs_json"`
	Inputs             []*WorkflowInput        `json:"inputs" gorm:"-"`
	Status             int32
	SortOrder          int32
	WorkflowJson       string
	RefCount           int64                `json:"ref_count" gorm:"column:ref_count;default:0"`
	FaceCount          int32                `json:"face_count" gorm:"column:face_count;default:0"`
	UseApi             bool                 `json:"use_api" gorm:"column:use_api;default:0"`
	Prompt             string               `json:"prompt" gorm:"column:prompt;type:text"`
	RecreatePrompt     string               `json:"recreate_prompt" gorm:"column:recreate_prompt;type:text;comment:二创prompt，用于图生视频"`
	ApiConfig          WorkflowApiConfig    `json:"api_config" gorm:"column:api_config;type:json;serializer:json"`
	Provider           string               `json:"provider" gorm:"column:provider;type:varchar(64);comment:执行器名称，如comfyui、gpt4o、kling"`
	CreditPoints       int                  `json:"credit_points" gorm:"column:credit_points;default:0"`
	Tags               []string             `json:"tags" gorm:"-"`
	SupportExtraParams []*WorkflowToolParam `json:"support_extra_params" gorm:"column:support_extra_params;type:json;serializer:json;comment:支持的额外参数配置"`
	QualityCreditRules map[string]int       `json:"quality_credit_rules" gorm:"-"` // 内存字段：quality→额外积分映射（模型直连模式使用）
}

type WorkflowApiConfig struct {
	ApiIden             string  `json:"api_iden"`
	EffectScene         string  `json:"effect_scene"`
	TaskType            string  `json:"task_type"`
	ModelName           string  `json:"model_name"`
	TimeRatio           float32 `json:"time_ratio"`
	DefaultTimeDuration uint16  `json:"default_time_duration"`
	DefaultResolution   string  `json:"default_resolution"`
	AuditReplaceBy      string  `json:"audit_replace_by"`
	// RequireAuditBypass 表示该工作流是否需要进行避审替换
	// true: 当满足其他条件时（如用户兑换了GOOGLENICE优惠券），进行工作流替换
	// false（默认）: 不进行替换
	RequireAuditBypass bool `json:"require_audit_bypass"`
}

type WorkflowToolParam struct {
	Label       string `json:"label"`
	Value       string `json:"value"`
	Credit      int32  `json:"credit"`
	SourceGroup string `json:"source_group"`
}

type WorkflowExampleImage struct {
	OriContent         []string `json:"ori_content"`
	ResultContent      string   `json:"result_content"`
	AspectRatio        float32  `json:"aspect_ratio"`
	ResultThumbnailUrl string   `json:"result_thumbnail_url"`
}

type WorkflowInput struct {
	InputName    string `json:"input_name"`
	InputType    int32  `json:"input_type"`
	InputContent string `json:"input_content"`
}

type UserPictureInfo struct {
	PhotoURL    string  `json:"photo_url"`
	AspectRatio float64 `json:"aspect_ratio"`
}

type PictureTask struct {
	TaskID                 string            `json:"task_id" gorm:"column:task_id;primaryKey;type:varchar(64)"`
	ProjectID              string            `json:"project_id" gorm:"column:project_id;type:varchar(64)"`
	UserID                 string            `json:"user_id" gorm:"column:user_id;index;type:varchar(64)"`
	WorkflowID             string            `json:"workflow_id" gorm:"column:workflow_id;type:varchar(64)"`
	Status                 int32             `json:"status" gorm:"column:status;type:int"`
	ParamJSON              string            `json:"param_json" gorm:"column:param_json;type:text"`
	UserPictureInfoJson    string            `json:"user_picture_info_json" gorm:"column:user_picture_info_json;type:text"`
	SubmitContextJson      string            `json:"submit_context_json" gorm:"column:submit_context_json;type:text;comment:客户端提交时上下文信息JSON"`
	ResultJSON             string            `json:"result_json" gorm:"column:result_json;type:text"`
	Error                  string            `json:"error" gorm:"column:error;type:text"`
	IsPublished            bool              `json:"is_published" gorm:"column:is_published;default:false"`
	Progress               int32             `json:"progress" gorm:"column:progress;type:int;default:0"`
	CreatedAt              time.Time         `json:"created_at" gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP"`
	UpdatedAt              time.Time         `json:"updated_at" gorm:"column:updated_at;type:timestamp;default:CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP"`
	DeletedAt              gorm.DeletedAt    `json:"deleted_at" gorm:"column:deleted_at;index"`
	ViewCount              int32             `json:"view_count" gorm:"column:view_count;type:int;default:0"`
	FakeViewCount          int32             `json:"fake_view_count" gorm:"column:fake_view_count;type:int;default:0"`
	ViewCountSeed          int64             `json:"view_count_seed" gorm:"column:view_count_seed;type:bigint;default:0"`
	ScoreJSON              string            `json:"score_json" gorm:"column:score_json;type:text"`
	Score                  float64           `json:"score" gorm:"column:score;type:float;default:0"`
	UnreadTaskResult       bool              `json:"unread_task_result" gorm:"column:unread_task_result;type:bool;default:false"`
	AllowShowOriPicture    bool              `json:"allow_show_ori_picture" gorm:"column:allow_show_ori_picture;type:bool;default:false"`
	Theme                  string            `json:"theme" gorm:"column:theme;type:varchar(64)"`
	WorkflowType           string            `json:"workflow_type" gorm:"column:workflow_type;type:varchar(255);comment:工作流类型"`
	Executer               string            `json:"executer" gorm:"column:executer;type:varchar(64);comment:任务执行器名称"`
	ExecuterTaskID         string            `json:"executer_task_id" gorm:"column:executer_task_id;type:varchar(255);comment:外部执行器返回的任务ID"`
	ExecuterTaskInfo       string            `json:"executer_task_info" gorm:"column:executer_task_info;type:text;comment:执行器相关的上下文信息，如提交参数、错误详情等"`
	RetryCount             int32             `json:"retry_count" gorm:"column:retry_count;type:int(11);default:0;comment:重试次数"`
	ProviderSyncErrorCount int32             `json:"provider_sync_error_count" gorm:"column:provider_sync_error_count;type:int;default:0;comment:外部API同步失败次数"`
	ApiConfig              WorkflowApiConfig `json:"api_config" gorm:"column:api_config;type:json;serializer:json"`
	CreditPoints           int               `json:"credit_points" gorm:"column:credit_points;type:int(11);comment:消耗点数"`
	CreditDeductionInfo    string            `json:"credit_deduction_info" gorm:"type:text;comment:积分扣除详情"`
	RequestApiParams       string            `json:"request_api_params" gorm:"type:text;comment:请求API的参数"`
	CompletedAt            sql.NullTime      `json:"completed_at" gorm:"column:completed_at;type:timestamp;null;comment:任务完成时间;default:null"`
	IsRefunded             bool              `json:"is_refunded" gorm:"column:is_refunded;type:tinyint(1);default:0;comment:是否已退款"`
	CanRetry               bool              `json:"can_retry" gorm:"column:can_retry;type:tinyint(1);comment:是否可重试"`
	VideoDuration          uint16            `json:"video_duration" gorm:"column:video_duration;default:0;comment:视频时长"`
	FailedReason           string            `json:"failed_reason" gorm:"column:failed_reason;type:text;comment:失败原因"`
	FailedReasonFull       string            `json:"failed_reason_full" gorm:"column:failed_reason_full;type:varchar(255);comment:失败原因详情"`
	Comment                string            `json:"comment" gorm:"column:comment;type:varchar(255);comment:任务备注信息"`
	// 模型直连模式字段
	TaskMode       int8   `json:"task_mode" gorm:"column:task_mode;type:tinyint(1);default:0;comment:任务模式 0-workflow 1-模型直连"`
	ModelName      string `json:"model_name" gorm:"column:model_name;type:varchar(64);comment:模型名称(模型直连模式)"`
	UserPrompt     string `json:"user_prompt" gorm:"column:user_prompt;type:text;comment:用户输入prompt(模型直连模式)"`
	NegativePrompt string `json:"negative_prompt" gorm:"column:negative_prompt;type:text;comment:负面prompt(模型直连模式)"`
}

// 任务模式常量
const (
	TaskModeWorkflow    = 0 // Workflow模式
	TaskModeModelDirect = 1 // 模型直连模式
)

type ScoreData struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type PictureTaskResult struct {
	ResultURL             string  `json:"result_url"`
	UserShowImageURL      string  `json:"user_show_image_url"`
	Width                 int     `json:"width"`
	Height                int     `json:"height"`
	AspectRatio           float64 `json:"aspect_ratio"`
	AspectRatioText       string  `json:"aspect_ratio_text"` // 标准比例文本，如"1:1", "3:4", "4:3", "9:16", "16:9"
	VideoFrameURL         string  `json:"video_frame_url"`
	VideoFrameAspectRatio float64 `json:"video_frame_aspect_ratio"`
	// 多图结果列表（单图时为空）
	ImageResults []ImageResultItem `json:"image_results,omitempty"`
}

// ImageResultItem 多图生成时单张图片的结果信息
type ImageResultItem struct {
	ResultURL        string  `json:"result_url"`
	UserShowImageURL string  `json:"user_show_image_url"`
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	AspectRatio      float64 `json:"aspect_ratio"`
	AspectRatioText  string  `json:"aspect_ratio_text"`
}

type ApiConfig struct {
	ApiIden string `json:"api_iden"`

	KlingEffectScene string `json:"kling_effect_scene"`
}

// ToolGroup 工具分组管理表
// 注意：不声明 TableName()，使用 GORM 统一前缀管理
// group_id 与 deleted_at 联合唯一索引，支持软删除后复用 group_id
type ToolGroup struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"uniqueIndex:idx_group_id_deleted_at"`
	GroupID   string         `json:"group_id" gorm:"column:group_id;type:varchar(64);uniqueIndex:idx_group_id_deleted_at;not null;comment:分组唯一标识"`
	GroupName string         `json:"group_name" gorm:"column:group_name;type:varchar(64);not null;comment:分组名称(AI Image/AI Video等)"`
	SortOrder int            `json:"sort_order" gorm:"column:sort_order;type:int;default:0;comment:分组排序"`
}

type PictureTools struct {
	gorm.Model

	ToolID      string `json:"tool_id" gorm:"column:tool_id;type:varchar(64);uniqueIndex;not null"`
	SimpleTitle string `json:"simple_title" gorm:"column:simple_title;type:varchar(64)"`
	Title       string `json:"title" gorm:"column:title;type:varchar(64)"`
	Description string `json:"description" gorm:"column:description;type:varchar(255)"`
	// @deprecated
	Icon                string `json:"icon" gorm:"column:icon;type:varchar(255)"`
	RecommendCover      string `json:"recommend_cover" gorm:"column:recommend_cover;type:varchar(255)"`
	Cover               string `json:"cover" gorm:"column:cover;type:json;"`
	RefWorkflowID       string `json:"ref_workflow_id" gorm:"column:ref_workflow_id;type:varchar(64)"`
	SupportCustomPrompt bool   `json:"support_custom_prompt" gorm:"column:support_custom_prompt;type:tinyint(1);default:0"`

	IsVipTool bool `json:"is_vip_tool" gorm:"column:is_vip_tool;type:tinyint(1);default:0"`

	SupportPromptOptimization bool `json:"support_prompt_optimization" gorm:"column:support_prompt_optimization;type:tinyint(1);default:0"`

	ParamsJSON string `json:"params_json" gorm:"column:params_json;type:text"`

	SupportVersions string `json:"support_versions" gorm:"column:support_versions;type:varchar(16)"`

	PlatformDisplay int32 `json:"platform_display" gorm:"column:platform_display;type:int;default:0;comment:平台显示控制 0-全平台 1-仅iOS 2-仅Android"`

	SortOrder int `json:"sort_order" gorm:"column:sort_order;type:int;default:0"`

	ToolType string `json:"tool_type" gorm:"column:tool_type;type:varchar(64);comment:工具类型"`

	// 首页工具ICON
	ToolIcon string `json:"tool_icon" gorm:"column:tool_icon;type:varchar(255);comment:工具图标"`

	// 聚合页工具ICON - 用于 ListToolsGroupByType 接口
	AggregateIcon string `json:"aggregate_icon" gorm:"column:aggregate_icon;type:varchar(255);comment:聚合页工具图标"`

	// 聚合页标签图片 - 用于 ListToolsGroupByType 接口
	AggregateTagIcon string `json:"aggregate_tag_icon" gorm:"column:aggregate_tag_icon;type:varchar(255);comment:聚合页标签图片"`

	// 标签图片 - 用于 ListPictureTools 接口
	TagIcon string `json:"tag_icon" gorm:"column:tag_icon;type:varchar(255);comment:工具标签图片"`

	// 工具分组ID - 关联 ToolGroup 表
	GroupID string `json:"group_id" gorm:"column:group_id;type:varchar(64);index;comment:工具分组ID"`

	// 首页隐藏控制 - true: 首页隐藏(聚合页仍显示), false: 首页显示(默认)
	HideOnHome bool `json:"hide_on_home" gorm:"column:hide_on_home;type:tinyint(1);default:0;comment:首页隐藏控制 0-首页显示 1-首页隐藏"`
}

type PictureToolCapability struct {
	gorm.Model

	ToolID    string `gorm:"column:tool_id;type:varchar(64);index;not null" json:"tool_id"`
	Title     string `gorm:"column:title;type:varchar(255);not null" json:"title"`
	Cover     string `gorm:"column:cover;type:text" json:"cover"`
	Prompt    string `gorm:"column:prompt;type:text" json:"prompt"`
	SortOrder int32  `gorm:"column:sort_order;type:int;default:0" json:"sort_order"`
}

// CustomPromptHistory 自定义 Prompt 历史记录
type CustomPromptHistory struct {
	gorm.Model

	HistoryID    string `json:"history_id" gorm:"column:history_id;type:varchar(64);uniqueIndex;not null"`
	UserID       string `json:"user_id" gorm:"column:user_id;type:varchar(64);index;not null"`
	ToolID       string `json:"tool_id" gorm:"column:tool_id;type:varchar(64);index;not null"`
	ToolType     string `json:"tool_type" gorm:"column:tool_type;type:varchar(64);index;not null"`
	CustomPrompt string `json:"custom_prompt" gorm:"column:custom_prompt;type:text;not null"`
	PromptHash   string `json:"prompt_hash" gorm:"column:prompt_hash;type:varchar(64);index:idx_user_tooltype_hash;not null;comment:Prompt内容的SHA256哈希值，用于快速去重"`
}

// BannerConfig banner配置表
type BannerConfig struct {
	gorm.Model

	// Banner唯一标识
	BannerKey string `json:"banner_key" gorm:"column:banner_key;type:varchar(64);uniqueIndex;not null;comment:banner唯一标识"`
	// Banner标题
	Title string `json:"title" gorm:"column:title;type:varchar(128);comment:banner标题"`
	// 图片地址
	ImageURL string `json:"image_url" gorm:"column:image_url;type:varchar(512);not null;comment:图片地址"`
	// 缩略图地址
	ThumbnailURL string `json:"thumbnail_url" gorm:"column:thumbnail_url;type:varchar(512);comment:缩略图地址"`

	// 跳转配置
	// 跳转类型：0-不跳转 1-工作流分类页(KIND) 2-工具详情页(TOOL)
	LinkType int8 `json:"link_type" gorm:"column:link_type;type:tinyint;default:0;comment:跳转类型 0-不跳转 1-KIND 2-TOOL"`
	// 跳转地址（存储kindID或toolID）
	LinkAddr string `json:"link_addr" gorm:"column:link_addr;type:varchar(128);comment:跳转地址 kindID或toolID"`

	// 展示控制
	// 排序权重
	SortOrder int `json:"sort_order" gorm:"column:sort_order;type:int;default:0;comment:排序权重"`
	// 状态：0-下架 1-上架
	Status int8 `json:"status" gorm:"column:status;type:tinyint;default:1;comment:状态 0-下架 1-上架"`

	// 积分配置
	// 是否开启积分奖励
	EnableReward bool `json:"enable_reward" gorm:"column:enable_reward;type:tinyint(1);default:0;comment:是否开启积分奖励"`
	// 奖励积分数
	RewardPoints int `json:"reward_points" gorm:"column:reward_points;type:int;default:0;comment:奖励积分数"`

	// 弹窗配置
	// 是否开启弹窗（第一次点击时是否弹窗）
	EnablePopup bool `json:"enable_popup" gorm:"column:enable_popup;type:tinyint(1);default:0;comment:是否开启弹窗"`
	// 弹窗图片地址
	PopupImageUrl string `json:"popup_image_url" gorm:"column:popup_image_url;type:varchar(512);comment:弹窗图片地址"`
	// 弹窗标题
	PopupTitle string `json:"popup_title" gorm:"column:popup_title;type:varchar(128);comment:弹窗标题"`
	// 弹窗内容
	PopupContent string `json:"popup_content" gorm:"column:popup_content;type:varchar(512);comment:弹窗内容"`
	// 弹窗按钮文案
	PopupButtonText string `json:"popup_button_text" gorm:"column:popup_button_text;type:varchar(64);comment:弹窗按钮文案"`
	// 弹窗按钮背景颜色
	PopupButtonColor string `json:"popup_button_color" gorm:"column:popup_button_color;type:varchar(32);comment:弹窗按钮背景颜色"`
	// 弹窗按钮文字颜色
	PopupButtonTextColor string `json:"popup_button_text_color" gorm:"column:popup_button_text_color;type:varchar(32);comment:弹窗按钮文字颜色"`
}

// BannerClickRecord Banner点击记录表（用于记录用户点击和积分发放）
type BannerClickRecord struct {
	gorm.Model

	// 用户ID
	UserID string `json:"user_id" gorm:"column:user_id;type:varchar(64);not null;index:idx_user_banner;comment:用户ID"`
	// Banner唯一标识
	BannerKey string `json:"banner_key" gorm:"column:banner_key;type:varchar(64);not null;uniqueIndex:idx_user_banner;comment:banner唯一标识"`

	// 本次发放的积分
	PointsAwarded int `json:"points_awarded" gorm:"column:points_awarded;type:int;default:0;comment:本次发放的积分"`
	// 是否展示了弹窗
	PopupShown bool `json:"popup_shown" gorm:"column:popup_shown;type:tinyint(1);default:0;comment:是否展示了弹窗"`
	// 点击时间
	ClickTime time.Time `json:"click_time" gorm:"column:click_time;type:timestamp;default:CURRENT_TIMESTAMP;comment:点击时间"`
}
