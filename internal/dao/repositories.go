package dao

import (
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// Repositories 集中管理所有 DAO 依赖
type Repositories struct {
	Model            *ModelDao
	Task             *PictureTaskDao
	PicForge         *PictureForgeDao
	Coupon           *CouponDao
	Upload           *UploadDao
	Prompt           *PromptDao
	Config           *ConfigDao
	Voice            *VoiceDao
	Event            *EventDao
	Tracking         *TrackingDao
	User             *UserDao
	UserPersonalInfo *ProfileDao
	UserPersonalChat *UserPersonalChatDao
	Attribution      *AttributionDao
	UserAmount       *UserAmountDao
	Chat             *ChatDao
	ChatParticipant  *ChatParticipantDao
	Profile          *ProfileDao
	Msg              *MsgDao
	// 邀请码相关DAO
	Invite *InviteDao
	// 弹窗提醒相关DAO
	PopupNotification *PopupNotificationDao
}

// NewRepositories 初始化所有 DAO 对象
func NewRepositories(db *gorm.DB) *Repositories {
	// 暂时将MongoDB客户端设为nil，应在实际使用前通过InitRepositoriesWithMongo初始化
	return NewRepositoriesWithMongo(db, nil)
}

// NewRepositoriesWithMongo 使用MongoDB客户端初始化所有DAO对象
func NewRepositoriesWithMongo(db *gorm.DB, mongoClient *mongo.Client) *Repositories {
	return &Repositories{
		Model:            NewModelDao(db),
		Task:             NewPictureTaskDao(db),
		PicForge:         NewPictureForgeDao(db),
		Coupon:           NewCouponDao(db),
		Upload:           NewUploadDao(db),
		Prompt:           NewPromptDao(db),
		Config:           NewConfigDao(),
		Voice:            NewVoiceDAO(db),
		Event:            NewEventDao(),
		Tracking:         NewTrackingDao(),
		User:             NewUserDao(db),
		UserPersonalInfo: NewProfileDao(db),
		UserPersonalChat: NewUserPersonalChatDao(db),
		Attribution:      AttributionDAO(),
		UserAmount:       NewUserAmountDao(),
		Chat:             NewChatDao(db),
		ChatParticipant:  NewChatParticipantDao(db),
		Profile:          NewProfileDao(db),
		Msg:              NewMsgDao(),
		// 邀请码相关DAO
		Invite: NewInviteDao(db),
		// 弹窗提醒相关DAO
		PopupNotification: NewPopupNotificationDao(db),
	}
}
