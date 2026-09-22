package model

// CouponExchangeRequest PayLinker优惠券兑换请求
type CouponExchangeRequest struct {
	UserID     string `json:"user_id"`
	ProjectID  string `json:"project_id"`
	CouponCode string `json:"coupon_code"`
}

// CouponExchangeResponse PayLinker优惠券兑换响应
type CouponExchangeResponse struct {
	Success bool   `json:"success"`
	Msg     string `json:"msg"`
}
