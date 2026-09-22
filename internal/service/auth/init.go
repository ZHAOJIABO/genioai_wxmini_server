package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/go-redis/redis"

	"va_visionai_server/conf"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/service"
	"va_visionai_server/internal/service/auth/providers/apple"
	"va_visionai_server/internal/service/auth/providers/email"
	"va_visionai_server/internal/service/auth/providers/google"
	"va_visionai_server/internal/service/auth/providers/normal"
	"va_visionai_server/internal/service/auth/providers/temp"
	"va_visionai_server/internal/service/auth/providers/wechat"
	"va_visionai_server/internal/service/auth/types"
)

var (
	defaultProviderManager *ProviderManager
	defaultAuthService     *AuthService
	initOnce               sync.Once
)

// 创建普通登录提供商的适配器函数
func normalLoginProviderAdapter(config types.ProviderConfig) (types.LoginProvider, error) {
	provider := normal.NewNormalLoginProvider()
	err := provider.Initialize(context.Background(), config)
	return provider, err
}

// 创建Google登录提供商的适配器函数
func googleLoginProviderAdapter(
	config types.ProviderConfig,
	configService *service.ConfigService,
	userDao *dao.UserDao,
	redisClient *redis.Client,
) (types.LoginProvider, error) {
	provider := google.NewGoogleLoginProvider(configService, userDao, redisClient)
	err := provider.Initialize(context.Background(), config)
	return provider, err
}

// 创建Apple登录提供商的适配器函数
func appleLoginProviderAdapter(
	config types.ProviderConfig,
	redisClient *redis.Client,
) (types.LoginProvider, error) {
	provider := apple.NewAppleLoginProvider(config.PrivateKey, config.Kid, config.TeamID, redisClient)
	err := provider.Initialize(context.Background(), config)
	return provider, err
}

// 创建临时用户登录提供商的适配器函数
func tempLoginProviderAdapter(config types.ProviderConfig) (types.LoginProvider, error) {
	provider := temp.NewTempLoginProvider()
	err := provider.Initialize(context.Background(), config)
	return provider, err
}

// 创建邮箱验证码登录提供商的适配器函数
func emailVerifyLoginProviderAdapter(
	config types.ProviderConfig,
	userDao *dao.UserDao,
) (types.LoginProvider, error) {
	emailVerifyService, err := service.NewEmailVerifyService()
	if err != nil {
		return nil, err
	}
	provider := email.NewEmailVerifyProvider(userDao, emailVerifyService)
	err = provider.Initialize(context.Background(), config)
	return provider, err
}

// 创建邮箱密码登录提供商的适配器函数
func emailPasswordLoginProviderAdapter(
	config types.ProviderConfig,
	userDao *dao.UserDao,
) (types.LoginProvider, error) {
	provider := email.NewEmailPasswordProvider(userDao)
	err := provider.Initialize(context.Background(), config)
	return provider, err
}

// InitAuthService 初始化认证服务
func InitAuthService(ctx context.Context, userDao *dao.UserDao, smsService service.SmsVerifyService, redisClient *redis.Client) (*AuthService, error) {
	return InitAuthServiceWithConfig(ctx, userDao, smsService, redisClient, service.NewConfigService(), service.NewDeviceService(), nil)
}

// InitAuthServiceWithConfig 使用指定的配置服务初始化认证服务
func InitAuthServiceWithConfig(ctx context.Context, userDao *dao.UserDao, smsService service.SmsVerifyService, redisClient *redis.Client, configService *service.ConfigService, userDeviceService *service.UserDeviceService, dailyFreeCreditsService *service.DailyFreeCreditsService) (*AuthService, error) {
	var initErr error

	initOnce.Do(func() {
		// 创建提供商管理器
		providerManager := NewProviderManager()

		// 注册提供商工厂
		providerManager.RegisterFactory("default", func(config types.ProviderConfig) (types.LoginProvider, error) {
			return normalLoginProviderAdapter(config)
		})
		providerManager.RegisterFactory("apple", func(config types.ProviderConfig) (types.LoginProvider, error) {
			return appleLoginProviderAdapter(config, redisClient)
		})
		providerManager.RegisterFactory("google", func(config types.ProviderConfig) (types.LoginProvider, error) {
			return googleLoginProviderAdapter(config, configService, userDao, redisClient)
		})
		providerManager.RegisterFactory("temp", func(config types.ProviderConfig) (types.LoginProvider, error) {
			return tempLoginProviderAdapter(config)
		})
		providerManager.RegisterFactory("email", func(config types.ProviderConfig) (types.LoginProvider, error) {
			return emailVerifyLoginProviderAdapter(config, userDao)
		})
		providerManager.RegisterFactory("email_password", func(config types.ProviderConfig) (types.LoginProvider, error) {
			return emailPasswordLoginProviderAdapter(config, userDao)
		})

		// 微信凭证只注册到明确指定的项目，避免跨项目回退。
		providerManager.RegisterFactory(wechat.ProviderType, func(config types.ProviderConfig) (types.LoginProvider, error) {
			provider := wechat.NewProvider(redisClient)
			return provider, provider.Initialize(ctx, config)
		})
		seenProjects := make(map[string]bool)
		for _, config := range conf.GlobalConfig.WeChatMiniPrograms {
			projectID := config.ProjectID
			if strings.TrimSpace(projectID) == "" || projectID == "default" || projectID != strings.ToLower(strings.TrimSpace(projectID)) {
				initErr = fmt.Errorf("wechat requires an explicit lowercase project ID")
				return
			}
			if seenProjects[projectID] {
				initErr = fmt.Errorf("duplicate wechat project ID: %s", projectID)
				return
			}
			seenProjects[projectID] = true
			if err := providerManager.RegisterConfig(types.ProviderConfig{
				ProjectID: projectID, ProviderType: wechat.ProviderType,
				ClientID: config.AppID, ClientSecret: config.AppSecret,
			}); err != nil {
				initErr = err
				return
			}
		}

		// 注册默认配置
		// 普通登录
		err := providerManager.RegisterConfig(types.ProviderConfig{
			ProjectID:    "default",
			ProviderType: "default",
			ExtraConfig: map[string]interface{}{
				"user_dao":    userDao,
				"sms_service": smsService,
			},
		})
		if err != nil {
			initErr = err
			return
		}

		// Apple登录
		err = providerManager.RegisterConfig(types.ProviderConfig{
			ProjectID:    "default",
			ProviderType: "apple",
			PrivateKey:   conf.GlobalConfig.Apple.PrivateKey,
			TeamID:       conf.GlobalConfig.Apple.TeamID,
			Kid:          conf.GlobalConfig.Apple.KID,
			ExtraConfig: map[string]interface{}{
				"user_dao": userDao,
			},
		})
		if err != nil {
			initErr = err
			return
		}

		// Google登录
		err = providerManager.RegisterConfig(types.ProviderConfig{
			ProjectID:    "default",
			ProviderType: "google",
			// ClientID:     "your-google-client-id", // 替换为实际的Google Client ID
		})
		if err != nil {
			initErr = err
			return
		}

		// 临时用户登录
		err = providerManager.RegisterConfig(types.ProviderConfig{
			ProjectID:    "default",
			ProviderType: "temp",
			ExtraConfig: map[string]interface{}{
				"user_dao": userDao,
			},
		})
		if err != nil {
			initErr = err
			return
		}

		// 邮箱验证码登录
		err = providerManager.RegisterConfig(types.ProviderConfig{
			ProjectID:    "default",
			ProviderType: "email",
			ExtraConfig: map[string]interface{}{
				"user_dao": userDao,
			},
		})
		if err != nil {
			initErr = err
			return
		}

		// 邮箱密码登录
		err = providerManager.RegisterConfig(types.ProviderConfig{
			ProjectID:    "default",
			ProviderType: "email_password",
			ExtraConfig: map[string]interface{}{
				"user_dao": userDao,
			},
		})
		if err != nil {
			initErr = err
			return
		}

		// 创建认证服务
		authService := NewAuthService(userDao, providerManager, redisClient, userDeviceService, dailyFreeCreditsService)

		// 设置全局变量
		defaultProviderManager = providerManager
		defaultAuthService = authService
	})

	if initErr != nil {
		return nil, initErr
	}

	return defaultAuthService, nil
}

// SMSVerifyService 短信验证服务接口
// type SMSVerifyService interface {
// 	Verify(ctx context.Context, phoneNumber, verifyCode string) error
// }

// GetAuthService 获取认证服务
func GetAuthService() *AuthService {
	return defaultAuthService
}

// GetProviderManager 获取提供商管理器
func GetProviderManager() *ProviderManager {
	return defaultProviderManager
}
