package model

import "time"

type AttributionEvent struct {
	ID uint `gorm:"primarykey"`
	//日期和具体小时
	Date string `gorm:"type:varchar(255);not null"`
	Hour string `gorm:"type:varchar(255);not null"`
	//平台信息
	Platform string `gorm:"platform;type:varchar(255);not null"`
	//iOS用户设备标识
	Idfv string `gorm:"idfv;type:varchar(255)"`
	Idfa string `gorm:"idfa;type:varchar(255)"`
	Oaid string `gorm:"oaid;type:varchar(255)"`
	//安卓用户设备标识
	AndroidId string `gorm:"android_id;type:varchar(255)"`
	Userid    string `gorm:"user_id;type:varchar(255)"`
	// 0 首次打开app 1 首次支付 2 首次传图片 3 首次发起会话 4 完成手机号注册（仅限华为、荣耀渠道用户）
	EventType string `gorm:"event_type;type:varchar(255)"`
	//支付金额
	Price string `gorm:"price;type:varchar(255)"`
	//支付币种
	PriceUnit string `gorm:"price_unit;type:varchar(255)"`
	//-------------------------------------------
	CreatedAt time.Time
}

// asa asa归因记录
type AsaEvent struct {
	ID             uint   `gorm:"primarykey"`
	Date           string `gorm:"type:varchar(255);not null"`
	Hour           string `gorm:"type:varchar(255);not null"`
	Userid         string `gorm:"user_id;type:varchar(255)"`
	Attribution    bool   `gorm:"attribution;type:bool"`
	OrgId          int64  `gorm:"org_id;type:varchar(255)"`
	CampaignId     int64  `gorm:"campaign_id;type:varchar(255)"`
	ConversionType string `gorm:"conversion_type;type:varchar(255)"`
	ClickDate      string `gorm:"click_date;type:varchar(255)"`
	ClaimType      string `gorm:"claim_type;type:varchar(255)"`
	AdGroupId      int64  `gorm:"ad_group_id;type:varchar(255)"`
	CountyOrRegion string `gorm:"county_or_region;type:varchar(255)"`
	KeywordId      int64  `gorm:"keyword_id;type:varchar(255)"`
	AdId           int64  `gorm:"ad_id;type:varchar(255)"`
	CreatedAt      time.Time
}
