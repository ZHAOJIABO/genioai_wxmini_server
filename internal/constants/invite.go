package constants

// 邀请码相关常量
const (
	// InviteCodeLength 邀请码长度
	InviteCodeLength = 10

	// InviteCodeCharset 邀请码字符集（排除易混淆字符 0OoIl1）
	InviteCodeCharset = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

	// InviteCreditAmount 邀请奖励积分数量
	InviteCreditAmount int64 = 300
)

// 邀请相关错误消息
const (
	ErrMsgInviteCodeNotFound    = "Invalid Invite Code"
	ErrMsgCannotInviteSelf      = "Cannot Invite Yourself"
	ErrMsgAlreadyUsedInviteCode = "Already Used Invite Code"
)

// 邀请相关配置键
const (
	// ConfigKeyInviteBaseUrl 邀请链接基础URL配置键
	ConfigKeyInviteBaseUrl = "invite_base_url"
	// ConfigKeyInviteCreditAmount 每次邀请奖励积分配置键
	ConfigKeyInviteCreditAmount = "invite_credit_amount"
	// ConfigKeyInviteMaxCredits 邀请积分封顶配置键（0表示无上限）
	ConfigKeyInviteMaxCredits = "invite_max_credits"
)
