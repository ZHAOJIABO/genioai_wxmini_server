package model

import (
	"database/sql"

	"gorm.io/gorm"
)

type BaseModel struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt sql.NullTime
	UpdatedAt sql.NullTime
	DeletedAt sql.NullTime `gorm:"index"`
}

type UserRecord struct {
	// Id int64 `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	BaseModel
	// 项目ID
	ProjectID string `gorm:"column:project_id;type:varchar(100);"`
	// 用户昵称（英文无空格）
	UserName string
	// UUID  注册时随机生成
	UUID string `gorm:"column:uuid;type:varchar(100);uniqueIndex:idx_uuid"`
	// 用户统一ID，基于手机号或者mail生成且唯一
	UserId string `gorm:"column:user_id;type:varchar(100);"`
	// 手机号（含国家）
	PhoneNumber string `gorm:"type:varchar(20)"`
	// 邮箱
	Email string `gorm:"type:varchar(255);index:idx_email"`
	// 密码哈希（使用bcrypt加密）
	PasswordHash string `gorm:"column:password_hash;type:varchar(255)"`
	// 头像
	AvatarUrl string
	// 访问令牌
	AccessToken string
	// 访问令牌过期时间
	AccessTokenExpired int64
	// 刷新令牌
	RefreshToken string
	// 刷新令牌过期时间
	RefreshTokenExpired int64
	// 状态，0正常，1封禁
	Status int
	// 更新时间
	UpdateTime int64
	// 创建时间
	CreateTime int64
	// 用户类型
	UserType uint
	//注册IP
	RegisterIP string
	//最后登陆IP
	LastLoginIP string
	// apple auth identity token
	AppleAuthIdentityToken string
	// apple refresh token
	AppleRefreshToken string
	// 登录方式
	LoginType string `gorm:"column:login_type;type:varchar(50)"`
	// 登录提供商用户ID
	ProviderUserID string `gorm:"column:provider_user_id;type:varchar(100)"`
	// 用户国家代码 (ISO 3166-1 alpha-2，如 "US", "CN")
	Country string `gorm:"column:country;type:varchar(10)"`
	// 用户时区 (IANA 时区标识符，如 "Asia/Shanghai", "America/New_York")
	Timezone string `gorm:"column:timezone;type:varchar(50)"`
	// 注册时的客户端版本号
	AppVersion string `gorm:"column:app_version;type:varchar(50)"`
}

// UserAmountDetailRecord 用户签到明细表
// type UserAmountDetailRecord struct {
// 	Id int64
// 	// 用户ID
// 	UserId string
// 	// 日期(yyyymmdd)
// 	Date string
// 	// 来源 (签到：signin, 观看广告奖励：reward_ad, 充值：charge)
// 	Source string
// 	// 关联信息(观看的广告ID，充值的流水ID等)
// 	Ext string
// 	// 对应积分
// 	Amount int64
// 	// 创建时间
// 	CreateTime int64
// }

// // UserAmountRecord 用户积分表
// type UserAmountRecord struct {
// 	Id int64
// 	// 用户ID
// 	UserId string
// 	// 积分
// 	Amount int64
// 	// 创建时间
// 	CreateTime int64
// }

type UserMapping struct {
	gorm.Model
	ProjectID     string `gorm:"column:project_id;type:varchar(100);uniqueIndex:idx_project_id_user_id"`
	UserID        string `gorm:"column:user_id;type:varchar(100);uniqueIndex:idx_project_id_user_id"`
	MappingUserID string `gorm:"column:mapping_user_id;type:varchar(100)"`
	Reason        string `gorm:"column:reason;type:varchar(255)"`
}

// UserLoginRecord 用户登录记录表
type UserLoginRecord struct {
	gorm.Model
	// 用户ID
	UserID string `gorm:"column:user_id;type:varchar(100);index"`
	// 项目ID
	ProjectID string `gorm:"column:project_id;type:varchar(100);index"`
	// 登录类型
	LoginType string `gorm:"column:login_type;type:varchar(50)"`
	// 提供商用户ID
	ProviderUserID string `gorm:"column:provider_user_id;type:varchar(100)"`
	// 登录IP
	LoginIP string `gorm:"column:login_ip;type:varchar(50)"`
	// 登录设备信息
	DeviceInfo string `gorm:"column:device_info;type:text"`
	// 登录状态，0成功，1失败
	Status int `gorm:"column:status;type:tinyint(1)"`
	// 失败原因
	FailReason string `gorm:"column:fail_reason;type:varchar(255)"`
}
