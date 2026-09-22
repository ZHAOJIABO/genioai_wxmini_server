package conf

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type (
	ServerName string
	LogConfig  struct {
		LogLevel   string `required:"true"`
		LogPath    string `required:"true"`
		MaxAgeDays int    `required:"true"`
		MaxSize    int    `required:"true"`
		MaxBackups int    `required:"true"`
		Compress   bool
	}
	TencentSms struct {
		SecretID      string
		SecretKey     string
		AppID         string
		TemplateID    string
		Endpoint      string
		Region        string
		SignName      string
		ExpireMinutes int
	}

	// EmailConfig 邮件配置
	EmailConfig struct {
		SMTPHost      string `yaml:"smtp_host" mapstructure:"smtp_host"`           // SMTP服务器地址
		SMTPPort      int    `yaml:"smtp_port" mapstructure:"smtp_port"`           // SMTP端口
		Username      string `yaml:"username" mapstructure:"username"`             // 发件人邮箱
		Password      string `yaml:"password" mapstructure:"password"`             // 邮箱密码或授权码
		FromAddress   string `yaml:"from_address" mapstructure:"from_address"`     // 发件人地址
		ExpireMinutes int    `yaml:"expire_minutes" mapstructure:"expire_minutes"` // 验证码过期时间（分钟）
	}

	Redis struct {
		Addr     string
		Db       int
		AuthInfo string
	}

	Metrics struct {
		Addr      string
		Path      string
		Namespace string
	}

	GeoIP struct {
		Enabled      bool   `yaml:"enabled"`
		DatabasePath string `yaml:"database_path"`
	}

	VisionAiServerConfig struct {
		Port        int
		HTTPPort    int
		DialTimeout time.Duration `required:"true"`
		Mode        string
		// 服务部署地区
		Region string
	}
	Mysql struct {
		Addr          string `required:"true"`
		User          string
		Password      string
		Db            string
		Port          int
		MigrationsDir string
	}
	MongoDB struct {
		Addr       string `required:"true"`
		User       string
		Password   string
		Db         string
		Collection string
	}
	AmountConfig struct {
		// FreeImageGeneration makes image tasks free for all users. Defaults to false.
		FreeImageGeneration bool
		SigninDayOfAmount   []DayOfAmount
		SignMaxDays         int
		AdRewardMaxPerDay   int64
		AdRewardPlay        int64
		AdRewardClick       int64
		AdRewardAction      int64
	}
	DayOfAmount struct {
		Day    int
		Amount int64
	}
	LlmConfig struct {
		GPT           GPTModel
		Doubao        DoubaoModel
		DoubaoVision  DoubaoVisionModel
		VolcEngineASR VolcEngineASR
		VolcEngineTTS VolcEngineTTS
		Kimi          KimiConfig
		DeepSeek      DeepSeek
		AzureOpenAI   AzureOpenAI
		Tencent       Tencent
		Gemini        Gemini
		TitlePrompt   string
		GptImage      GptImage
		KlingAPI      KlingAPI
		MinimaxAPI    MinimaxAPI
		GptText2Image GptImage
	}
	GPTModel struct {
		APIKey   string
		Endpoint string
	}
	OssConfig struct {
		Endpoint        string
		AccessKeyID     string
		AccessKeySecret string
		Bucket          string
		UgcAddr         string
	}
	CosConfig struct {
		Endpoint        string
		AccessKeyID     string
		AccessKeySecret string
		Bucket          string
		UgcAddr         string
	}
	R2Config struct {
		Endpoint        string // Cloudflare R2 endpoint (e.g., https://<account_id>.r2.cloudflarestorage.com)
		AccessKeyID     string // R2 Access Key ID
		AccessKeySecret string // R2 Secret Access Key
		Bucket          string // R2 Bucket name
		UgcAddr         string // Public access URL (e.g., https://your-domain.com or R2 public URL)
	}
	DoubaoModel struct {
		APIKey          string
		Endpoint        string
		Region          string
		Doubao          string
		DoubaoPro       string
		DeepSeekR1      string
		DoubaoProVision string
	}
	DoubaoVisionModel struct {
		APIKey       string
		Endpoint     string
		Region       string
		DoubaoVision string
	}
	VolcEngineASR struct {
		AppKey    string
		AccessKey string
		Endpoint  string
	}
	VolcEngineTTS struct {
		AppKey    string
		AccessKey string
		Endpoint  string
	}

	KimiConfig struct {
		Endpoint  string `yaml:"endpoint"`
		APIKey    string `yaml:"api_key"`
		ModelName string `yaml:"model_name"`
	}

	Tencent struct {
		Endpoint string `yaml:"endpoint"`
		APIKey   string `yaml:"api_key"`
	}

	DeepSeek struct {
		Endpoint   string `yaml:"endpoint"`
		APIKey     string `yaml:"api_key"`
		DeepSeekR1 string `yaml:"deepseek_r1"`
		DeepSeekV3 string `yaml:"deepseek_v3"`
	}
	AzureOpenAI struct {
		Endpoint string `yaml:"endpoint"`
		APIKey   string `yaml:"api_key"`
		GPT4O    string `yaml:"gpt_4o"`
	}

	Gemini struct {
		Endpoint string `yaml:"endpoint"`
		APIKey   string `yaml:"api_key"`
		AuthFile string `yaml:"auth_file"`
	}
	GptImage struct {
		Endpoint string `yaml:"endpoint"`
		APIKey   string `yaml:"api_key"`
	}

	KlingAPI struct {
		Endpoint string `validate:"required"`
		AppID    string `validate:"required"`
		Secret   string `validate:"required"`
	}

	MinimaxAPI struct {
		Endpoint string `yaml:"endpoint" validate:"required"`
		AppID    string `yaml:"app_id" validate:"required"`
		Secret   string `yaml:"secret" validate:"required"`
	}

	ComfyUIConfig struct {
		Server   string `yaml:"server"`
		Port     int    `yaml:"port"`
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	}

	PayLinker struct {
		Addr string
	}

	AstroServer struct {
		Addr string
	}

	AIBrainServer struct {
		Addr string
	}

	Apple struct {
		PrivateKey string
		TeamID     string
		KID        string
	}

	HealthConfig struct {
		NutritionFile string
	}
	SlotReconciler struct {
		Enabled          bool                 `yaml:"enabled" mapstructure:"enabled"`
		Interval         time.Duration        `yaml:"interval" mapstructure:"interval"`
		RedisScanCount   int64                `yaml:"redis_scan_count" mapstructure:"redis_scan_count"`
		MaxTasksPerRound int                  `yaml:"max_tasks_per_round" mapstructure:"max_tasks_per_round"`
		WorkerPoolSize   int                  `yaml:"worker_pool_size" mapstructure:"worker_pool_size"`
		KeyTTLSeconds    int                  `yaml:"key_ttl_seconds" mapstructure:"key_ttl_seconds"`
		Queue            QueueReconciler      `yaml:"queue" mapstructure:"queue"`
		Concurrent       ConcurrentReconciler `yaml:"concurrent" mapstructure:"concurrent"`
	}

	QueueReconciler struct {
		Enabled          bool          `yaml:"enabled" mapstructure:"enabled"`
		Interval         time.Duration `yaml:"interval" mapstructure:"interval"`
		RedisScanCount   int64         `yaml:"redis_scan_count" mapstructure:"redis_scan_count"`
		MaxTasksPerRound int           `yaml:"max_tasks_per_round" mapstructure:"max_tasks_per_round"`
		WorkerPoolSize   int           `yaml:"worker_pool_size" mapstructure:"worker_pool_size"`
	}

	ConcurrentReconciler struct {
		Enabled          bool          `yaml:"enabled" mapstructure:"enabled"`
		Interval         time.Duration `yaml:"interval" mapstructure:"interval"`
		RedisScanCount   int64         `yaml:"redis_scan_count" mapstructure:"redis_scan_count"`
		MaxTasksPerRound int           `yaml:"max_tasks_per_round" mapstructure:"max_tasks_per_round"`
		WorkerPoolSize   int           `yaml:"worker_pool_size" mapstructure:"worker_pool_size"`
		AwaitingTimeout  time.Duration `yaml:"awaiting_timeout" mapstructure:"awaiting_timeout"` // AWAITING_PROVIDER_COMPLETION超时释放并发槽
	}

	// EventSink 事件上报配置
	EventSink struct {
		Enabled         bool          `yaml:"enabled" mapstructure:"enabled"`                   // 是否启用
		Addr            string        `yaml:"addr" mapstructure:"addr"`                         // gRPC 服务地址
		QueueSize       int           `yaml:"queue_size" mapstructure:"queue_size"`             // 队列容量
		BatchSize       int           `yaml:"batch_size" mapstructure:"batch_size"`             // 批量大小阈值
		FlushInterval   time.Duration `yaml:"flush_interval" mapstructure:"flush_interval"`     // 刷新间隔
		ShutdownTimeout time.Duration `yaml:"shutdown_timeout" mapstructure:"shutdown_timeout"` // 关闭超时
		Source          string        `yaml:"source" mapstructure:"source"`                     // 事件来源标识
		ServerNode      string        `yaml:"server_node" mapstructure:"server_node"`           // 服务节点标识
	}

	// PushGateway 推送网关配置
	PushGateway struct {
		Enabled bool          `yaml:"enabled" mapstructure:"enabled"` // 是否启用
		Addr    string        `yaml:"addr" mapstructure:"addr"`       // HTTP 服务地址
		Timeout time.Duration `yaml:"timeout" mapstructure:"timeout"` // 请求超时
		Env     string        `yaml:"env" mapstructure:"env"`
	}

	// AdminConfig 后台管理配置
	AdminConfig struct {
		JWTSecret   string `yaml:"jwt_secret" mapstructure:"jwt_secret"`
		TokenExpiry string `yaml:"token_expiry" mapstructure:"token_expiry"` // e.g. "24h"
	}
)

// WeChatMiniProgramConfig binds credentials to one project; there is no cross-project fallback.
type WeChatMiniProgramConfig struct {
	ProjectID string
	AppID     string
	AppSecret string
}

type Config struct {
	WeChatMiniPrograms   []WeChatMiniProgramConfig
	ServerName           ServerName
	LogConfig            LogConfig
	Metrics              Metrics
	GeoIP                GeoIP
	VisionAiServerConfig VisionAiServerConfig
	AmountConfig         AmountConfig
	Mysql                Mysql
	Redis                Redis
	MongoDB              MongoDB
	TencentSms           TencentSms
	Email                EmailConfig
	LlmConfig            LlmConfig
	OssConfig            OssConfig
	CosConfig            CosConfig
	R2Config             R2Config
	ComfyUI              ComfyUIConfig
	PayLinker            PayLinker
	AstroServer          AstroServer
	AIBrainServer        AIBrainServer
	Apple                Apple
	HealthConfig         HealthConfig
	Reconciler           SlotReconciler
	EventSink            EventSink
	PushGateway          PushGateway
	Admin                AdminConfig
}

var GlobalConfig Config

func ConfigInit(configPath string) (err error) {
	viper.SetConfigType("yaml")
	viper.SetConfigFile(configPath)
	if err = viper.ReadInConfig(); err != nil {
		return err
	}
	err = viper.Unmarshal(&GlobalConfig)
	if err != nil {
		return err
	}
	return nil
}

func (c LogConfig) String() string {
	return fmt.Sprintf(`LogConfig: LogPath:%s, LogLevel:%s`, c.LogPath, c.LogLevel)
}

func IsDev() bool {
	return GlobalConfig.VisionAiServerConfig.Mode == "DEV"
}
func IsProd() bool {
	return !IsDev() && !IsLocal()
}
func IsLocal() bool {
	return GlobalConfig.VisionAiServerConfig.Mode == "LOCAL"
}
