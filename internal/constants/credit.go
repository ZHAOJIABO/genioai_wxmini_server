package constants

// CreditType defines the type of credit.
type CreditType string

const (
	// CreditTypePurchased represents credits bought by the user, which do not expire.
	CreditTypePurchased CreditType = "credits_purchase"
	// CreditTypeMembershipGrant represents credits granted as part of a membership plan.
	CreditTypeMembershipGrant CreditType = "membership_grant"
	// CreditTypeDailyFree represents daily free credits for non-members.
	CreditTypeDailyFree CreditType = "daily_free"
	// CreditTypeEventGrant represents credits granted through promotional events.
	CreditTypeEventGrant CreditType = "event_grant"
)

// CreditTransactionType defines the type of a credit transaction.
type CreditTransactionType string

const (
	TransactionTypeUsageDeduction           CreditTransactionType = "usage_deduction"            // 使用扣减
	TransactionTypeCreationFailedRefund     CreditTransactionType = "creation_failed_refund"     // 创作失败补增
	TransactionTypePurchase                 CreditTransactionType = "purchase"                   // 购买 (旧)
	TransactionTypeOnetimePurchase          CreditTransactionType = "onetime_purchase"           // 一次性购买 (新)
	TransactionTypeSubscriptionGrant        CreditTransactionType = "subscription_grant"         // 会员周期赠送
	TransactionTypeSubscriptionCreated      CreditTransactionType = "subscription_created"       // 会员首次订阅
	TransactionTypeSubscriptionRenewed      CreditTransactionType = "subscription_renewed"       // 会员续费
	TransactionTypeSubscriptionUpgrade      CreditTransactionType = "subscription_upgrade"       // 会员升级赠送
	TransactionTypeSubscriptionExpiry       CreditTransactionType = "subscription_expiry"        // 会员到期扣减（统计用）
	TransactionTypeSubscriptionExpiredClear CreditTransactionType = "subscription_expired_clear" // 会员到期清零
	TransactionTypeAdminAdjustment          CreditTransactionType = "admin_adjustment"           // 管理员调整 @deprecated
	TransactionTypeSystemGrant              CreditTransactionType = "system_grant"               // 系统赠送
	TransactionTypeTaskRetryDeduction       CreditTransactionType = "task_retry_deduction"       // 任务重试扣款
	TransactionTypeDailyGrant               CreditTransactionType = "daily_grant"                // 每日积分发放
	TransactionTypeEventGrant               CreditTransactionType = "event_grant"                // 活动赠送积分
	TransactionTypeInviteReward             CreditTransactionType = "invite_reward"              // 邀请奖励
	TransactionTypeRegisterBonus            CreditTransactionType = "register_bonus"             // 新用户注册奖励
)

const (
	ConfigKeyDailyFreeCreditsEnabled = "daily_free_credits.enabled"
	ConfigKeyDailyFreeCreditsAmount  = "daily_free_credits.amount"
)
