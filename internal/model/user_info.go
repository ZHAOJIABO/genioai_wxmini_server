package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	vai "va_visionai_server/internal/va_interface"
)

type UserProfile struct {
	gorm.Model
	//用户ID
	UserID string `gorm:"column:user_id;type:varchar(100)"`
	//项目ID
	ProjectID string `gorm:"column:project_id;type:varchar(100)"`
	// 档案唯一标识
	ProfileID string `gorm:"column:profile_id;type:varchar(64);index;not null"`
	//昵称
	NickName string `gorm:"column:nick_name;type:varchar(100)"`
	// 真名
	RealName string `gorm:"column:real_name;type:varchar(100)"`
	//头像
	AvatarUrl string `gorm:"column:avatar_url;type:varchar(255)"`
	//性别
	Sex int `gorm:"column:sex;type:tinyint(1)"`
	//生日
	// BirthTime string `gorm:"column:birthday;type:varchar(100)"`
	// 出生日期时间
	// BirthDatetime string `gorm:"column:birth_datetime;type:varchar(100)"`
	// 出生时间戳
	BirthTimestamp int64 `gorm:"column:birth_timestamp;type:bigint"`
	//城市
	City string `gorm:"column:city;type:varchar(100)"`
	//省份
	Province string `gorm:"column:province;type:varchar(100)"`
	//国家
	Country string `gorm:"column:country;type:varchar(100)"`
	//情感状态
	EmotionalState string `gorm:"column:emotional_state;type:varchar(64)"`
	// 星座
	Constellation string `gorm:"column:constellation;type:varchar(64)"`
	// 出生地经度
	BirthLongitude float32 `gorm:"column:birth_longitude;type:float"`
	// 出生地纬度
	BirthLatitude float32 `gorm:"column:birth_latitude;type:float"`
	// 出生地址
	BirthAddress string `gorm:"column:birth_address;type:varchar(255)"`
	// 出生省份
	BirthProvince string `gorm:"column:birth_province;type:varchar(100)"`
	// 出生城市
	BirthCity string `gorm:"column:birth_city;type:varchar(100)"`
	// 出生国家
	BirthCountry string `gorm:"column:birth_country;type:varchar(100)"`
}

type CustomProfile struct {
	gorm.Model
	// 档案唯一标识
	ProfileID string `gorm:"column:profile_id;type:varchar(64);index;not null"`
	//用户ID
	UserID string `gorm:"column:user_id;type:varchar(100);index"`
	//项目ID
	ProjectID string `gorm:"column:project_id;type:varchar(100);index"`
	//昵称
	NickName string `gorm:"column:nick_name;type:varchar(100)"`
	// 真名
	RealName string `gorm:"column:real_name;type:varchar(100)"`
	//头像
	AvatarUrl string `gorm:"column:avatar_url;type:varchar(255)"`
	//性别
	Sex int `gorm:"column:sex;type:tinyint(1)"`
	// 出生时间戳
	BirthTimestamp int64 `gorm:"column:birth_timestamp;type:bigint"`
	//城市
	City string `gorm:"column:city;type:varchar(100)"`
	//省份
	Province string `gorm:"column:province;type:varchar(100)"`
	//国家
	Country string `gorm:"column:country;type:varchar(100)"`
	//情感状态
	EmotionalState string `gorm:"column:emotional_state;type:varchar(64)"`
	// 星座
	Constellation string `gorm:"column:constellation;type:varchar(64)"`
	// 出生地经度
	BirthLongitude float32 `gorm:"column:birth_longitude;type:float"`
	// 出生地纬度
	BirthLatitude float32 `gorm:"column:birth_latitude;type:float"`
	// 出生地址
	BirthAddress string `gorm:"column:birth_address;type:varchar(255)"`
	// 出生省份
	BirthProvince string `gorm:"column:birth_province;type:varchar(100)"`
	// 出生城市
	BirthCity string `gorm:"column:birth_city;type:varchar(100)"`
	// 出生国家
	BirthCountry string `gorm:"column:birth_country;type:varchar(100)"`
	// 创建用户ID
	CreateBy string `gorm:"column:create_by;type:varchar(100)"`
	// 关系
	Relation string `gorm:"column:relation;type:varchar(100)"`
}

// BeforeCreate GORM Hook: 在创建记录前生成 UUID
func (p *CustomProfile) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ProfileID == "" {
		p.ProfileID = uuid.NewString()
	}
	return
}

func (p *UserProfile) ToCustomProfile() *CustomProfile {
	return &CustomProfile{
		ProfileID:      p.ProfileID,
		UserID:         p.UserID,
		ProjectID:      p.ProjectID,
		NickName:       p.NickName,
		RealName:       p.RealName,
		AvatarUrl:      p.AvatarUrl,
		Sex:            p.Sex,
		BirthTimestamp: p.BirthTimestamp,
		City:           p.City,
		Province:       p.Province,
		Country:        p.Country,
		EmotionalState: p.EmotionalState,
		Constellation:  p.Constellation,
		BirthLongitude: p.BirthLongitude,
		BirthLatitude:  p.BirthLatitude,
		BirthAddress:   p.BirthAddress,
		BirthProvince:  p.BirthProvince,
		BirthCity:      p.BirthCity,
		BirthCountry:   p.BirthCountry,
	}
}

// UpdateFromGRPC 从 GRPC CustomProfile 更新模型字段
func (p *CustomProfile) UpdateFromGRPC(projectID, userID string, profile *vai.CustomProfile) {
	userProfile := profile.GetProfile()
	p.CreateBy = userID
	p.Relation = profile.GetRelation().GetRelationIden()

	if userProfile != nil {
		p.ProjectID = projectID
		p.UserID = userProfile.GetUserId()
		p.NickName = userProfile.GetNickname()
		p.AvatarUrl = userProfile.GetAvatar()
		p.Sex = int(userProfile.GetGender())
		p.BirthTimestamp = userProfile.GetBirthTimestamp()
		p.EmotionalState = userProfile.GetEmotionalState()
		p.Constellation = userProfile.GetConstellation()
		p.BirthLongitude = userProfile.GetBirthLongitude()
		p.BirthLatitude = userProfile.GetBirthLatitude()
		p.BirthAddress = userProfile.GetBirthAddress()
		p.BirthProvince = userProfile.GetBirthProvince()
		p.BirthCity = userProfile.GetBirthCity()
		p.BirthCountry = userProfile.GetBirthCountry()
	}
}

// customProfileToGRPC 将自定义个人信息转换为 GRPC 格式
func (p *CustomProfile) CustomProfileToGRPC() *vai.CustomProfile {
	return &vai.CustomProfile{
		Profile: &vai.UserProfile{
			ProfileId:      p.ProfileID,
			UserId:         p.UserID,
			Avatar:         p.AvatarUrl,
			Nickname:       p.NickName,
			BirthTimestamp: p.BirthTimestamp,
			Gender:         vai.Gender(p.Sex),
			Constellation:  p.Constellation,
			EmotionalState: p.EmotionalState,
			BirthLongitude: p.BirthLongitude,
			BirthLatitude:  p.BirthLatitude,
			BirthAddress:   p.BirthAddress,
			BirthProvince:  p.BirthProvince,
			BirthCity:      p.BirthCity,
			BirthCountry:   p.BirthCountry,
		},
	}
}

func (p *UserProfile) ToGRPC() *vai.UserProfile {
	return &vai.UserProfile{
		ProfileId:      p.ProfileID,
		UserId:         p.UserID,
		Avatar:         p.AvatarUrl,
		Nickname:       p.NickName,
		BirthTimestamp: p.BirthTimestamp,
		Gender:         vai.Gender(p.Sex),
		Constellation:  p.Constellation,
		EmotionalState: p.EmotionalState,
		BirthLongitude: p.BirthLongitude,
		BirthLatitude:  p.BirthLatitude,
		BirthAddress:   p.BirthAddress,
		BirthProvince:  p.BirthProvince,
		BirthCity:      p.BirthCity,
		BirthCountry:   p.BirthCountry,
	}
}
