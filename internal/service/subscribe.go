package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/conf"
	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service/cache"
	"va_visionai_server/internal/service/credit"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

const (
	UserSubscribeInfoCacheKeyPrefix  = "project:%s:user:%s:subscribe_info"
	UserSubscribeInfoCacheExpiration = 1 * time.Hour
)

type SubscribeService struct {
	dao               *dao.OrderDao
	productDao        *dao.ProductDao
	userDao           *dao.UserDao
	httpClient        *http.Client
	payLinkerURL      string
	userAmountDao     *dao.UserAmountDao
	membershipService credit.MembershipService
	creditService     credit.Service
	cacheService      *cache.CacheService
	pictureTaskDao    *dao.PictureTaskDao
}

func NewSubscribeService(
	userAmountDao *dao.UserAmountDao,
	userDao *dao.UserDao,
	membershipService credit.MembershipService,
	creditService credit.Service,
	cacheService *cache.CacheService,
	pictureTaskDao *dao.PictureTaskDao,
) *SubscribeService {
	orderDao := dao.NewOrderDao(db.GetDB())
	productDao := dao.NewProductDao()
	paylinkerUrl := conf.GlobalConfig.PayLinker.Addr
	return &SubscribeService{
		dao:        orderDao,
		productDao: productDao,
		userDao:    userDao,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   15 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:       100,
				IdleConnTimeout:    90 * time.Second,
				DisableCompression: true,
			},
		},
		payLinkerURL:      paylinkerUrl,
		userAmountDao:     userAmountDao,
		membershipService: membershipService,
		creditService:     creditService,
		cacheService:      cacheService,
		pictureTaskDao:    pictureTaskDao,
	}
}

func (s *SubscribeService) CreateOrder(ctx context.Context, projectID, userID, productID, paymentWay string) (*model.Order, error) {
	if userID == "" || productID == "" || paymentWay == "" {
		return nil, errors.New("invalid parameters: userID, productID and paymentWay are required")
	}

	orderCreateReq := struct {
		ProjectID  string `json:"project_id"`
		ProductID  string `json:"product_id"`
		UserID     string `json:"user_id"`
		PaymentWay string `json:"payment_way"`
	}{
		ProjectID:  projectID,
		ProductID:  productID,
		UserID:     userID,
		PaymentWay: paymentWay,
	}

	jsonBody, err := json.Marshal(orderCreateReq)
	if err != nil {
		zlog.LogWithContext(ctx).Error("CreateOrder Marshal Error", zap.Error(err))
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.payLinkerURL+"/order/create", bytes.NewBuffer(jsonBody))
	if err != nil {
		zlog.LogWithContext(ctx).Error("CreateOrder Create Request Error", zap.Error(err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		zlog.LogWithContext(ctx).Error("CreateOrder Request Error", zap.Error(err))
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		zlog.LogWithContext(ctx).Error("CreateOrder Response StatusCode Error",
			zap.Int("code", resp.StatusCode))
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		zlog.LogWithContext(ctx).Error("CreateOrder Read Body Error", zap.Error(err))
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var rsp struct {
		Data *model.Order `json:"data"`
		Code int32        `json:"code"`
		Msg  string       `json:"msg"`
	}

	if err := json.Unmarshal(body, &rsp); err != nil {
		zlog.LogWithContext(ctx).Error("CreateOrder Unmarshal Error", zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if rsp.Code != http.StatusOK {
		zlog.LogWithContext(ctx).Error("CreateOrder Response Error",
			zap.Int32("code", rsp.Code),
			zap.String("msg", rsp.Msg))
		return nil, fmt.Errorf("service error: %s", rsp.Msg)
	}

	return rsp.Data, nil
}

func (s *SubscribeService) GetSubscribeInfo(ctx context.Context, userID, os, lang, subscriptionID string) (*vai.SubscribeInfo, error) {

	result := vai.SubscribeInfo{}
	projectID := common.GetProjectID(ctx)
	fetchSubscribeInfoAddr := fmt.Sprintf("%s/subscription/info?project_id=%s&user_id=%s&subscription_id=%s", conf.GlobalConfig.PayLinker.Addr, projectID, userID, subscriptionID)
	//nolint:bodyclose
	_, body, err := utils.Get(ctx, fetchSubscribeInfoAddr, nil)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetSubscribeInfo GetSubscribeInfoFromPayLinker Error", zap.Error(err))
		return nil, err
	}

	var userSubscriptionVO struct {
		model.UserSubscription
		ProductLevel int `json:"product_level"`
	}
	err = json.Unmarshal(body, &userSubscriptionVO)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetSubscribeInfo Unmarshal Error", zap.Error(err))
		return nil, err
	}
	subscribeStatus := vai.SubscribeStatus(vai.SubscribeStatus_value[userSubscriptionVO.SubscriptionStatus])
	result = vai.SubscribeInfo{
		ProductId:            userSubscriptionVO.ProductID,
		Status:               subscribeStatus,
		SubscribeLevel:       int32(userSubscriptionVO.ProductLevel),
		SubscribeExpiredTime: time.UnixMilli(userSubscriptionVO.ExpireDate).Format(time.DateOnly),
	}

	return &result, nil
}

func (s *SubscribeService) SetUserSubscribeInfoCache(ctx context.Context, projectID, userID string, subscribeInfo *vai.SubscribeInfo) error {
	key := fmt.Sprintf(UserSubscribeInfoCacheKeyPrefix, projectID, userID)
	return s.cacheService.Set(ctx, key, subscribeInfo, UserSubscribeInfoCacheExpiration)
}

func (s *SubscribeService) GetUserSubscribeInfoCache(ctx context.Context, projectID, userID, os, lang, subscriptionID string) (*vai.SubscribeInfo, error) {
	key := fmt.Sprintf(UserSubscribeInfoCacheKeyPrefix, projectID, userID)

	loader := func() (interface{}, error) {
		return s.GetSubscribeInfo(ctx, userID, os, lang, subscriptionID)
	}

	var result vai.SubscribeInfo
	err := s.cacheService.GetOrSet(ctx, key, &result, UserSubscribeInfoCacheExpiration, loader)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (s *SubscribeService) IsUserSubscriber(ctx context.Context, projectID, userID, os, lang, subscriptionID string) (bool, error) {
	subscribeInfo, err := s.GetUserSubscribeInfoCache(ctx, projectID, userID, os, lang, subscriptionID)
	if err != nil {
		return false, err
	}

	if subscribeInfo != nil && subscribeInfo.GetStatus() == vai.SubscribeStatus_SubscribeActivate {
		return true, nil
	}

	return false, nil
}

func (s *SubscribeService) AlipayPaymentCallBack(ctx context.Context, projectID, userID, orderID, receipt, lang string, paymentStatus vai.PaymentStatus) (bool, string, error) {
	var userSubscription *model.UserSubscription
	var err error
	for i := 0; i < 30; i++ {
		err = s.dao.DB.Table("pay_user_subscription").Where("project_id = ? and user_id = ?  and order_id = ?", projectID, userID, orderID).
			Order("id desc").First(&userSubscription).Error
		if err != nil {
			zlog.LogWithContext(ctx).Error("PaymentCallBack GetUserSubscription Error", zap.Error(err))
			time.Sleep(1 * time.Second)
			continue
		} else {
			break
		}
	}
	if err != nil {
		return false, userSubscription.SubscriptionID, err
	}
	if userSubscription.ExpireDate > time.Now().UnixMilli() {
		if err := s.userAmountDao.SetUserAmount(ctx, projectID, userID, true); err != nil {
			zlog.LogWithContext(ctx).Error("PaymentCallBack SetUserAmount Error", zap.Error(err))
			return false, userSubscription.SubscriptionID, err
		}

	}

	return true, userSubscription.SubscriptionID, nil
}

func (s *SubscribeService) GooglePaymentCallBack(ctx context.Context, projectID, userID, orderID, receipt, lang string, paymentStatus vai.PaymentStatus) (bool, error) {
	var err error

	var verifyRequest struct {
		TransactionID string `json:"transaction_id"`
		UserID        string `json:"user_id"`
		ProjectID     string `json:"project_id"`
		OrderID       string `json:"order_id"`
	}
	verifyRequest.TransactionID = receipt
	verifyRequest.UserID = userID
	verifyRequest.ProjectID = projectID
	verifyRequest.OrderID = orderID
	jsonBody, err := json.Marshal(verifyRequest)
	if err != nil {
		zlog.LogWithContext(ctx).Error("PaymentCallBack GooglePaymentCallBackFailed Marshal VerifyRequest Error", zap.Error(err))
		return false, err
	}
	resp, err := s.httpClient.Post(conf.GlobalConfig.PayLinker.Addr+"/verify/googleplay", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		zlog.LogWithContext(ctx).Error("PaymentCallBack GooglePaymentCallBackFailed",
			zap.String("Receipt", receipt),
			zap.String("OrderID", orderID),
			zap.Any(constants.ServiceEvent, constants.EventPaymentResultCallback),
		)
		return false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		zlog.LogWithContext(ctx).Error("PaymentCallBack GooglePaymentCallBackFailed",
			zap.String("Receipt", receipt),
			zap.String("OrderID", orderID),
			zap.Any(constants.ServiceEvent, constants.EventPaymentResultCallback),
		)
		return false, err
	}
	var rsp struct {
		Data *model.UserSubscription `json:"data"`
		Code int32                   `json:"code"`
		Msg  string                  `json:"msg"`
	}
	err = json.Unmarshal(body, &rsp)
	if err != nil {
		zlog.LogWithContext(ctx).Error("PaymentCallBack GooglePaymentCallBackFailed",
			zap.String("Receipt", receipt),
			zap.String("OrderID", orderID),
			zap.Any(constants.ServiceEvent, constants.EventPaymentResultCallback),
		)
		return false, err
	}
	if rsp.Code != http.StatusOK {
		zlog.LogWithContext(ctx).Error("PaymentCallBack GooglePaymentCallBackFailed",
			zap.String("Receipt", receipt),
			zap.String("OrderID", orderID),
			zap.Any(constants.ServiceEvent, constants.EventPaymentResultCallback),
		)
		return false, errors.New(rsp.Msg)
	}
	if time.Now().UnixMilli() < rsp.Data.ExpireDate {
		if err := s.userAmountDao.SetUserAmount(ctx, projectID, userID, true); err != nil {
			zlog.LogWithContext(ctx).Error("PaymentCallBack ApplePaymentCallBackSuccess SetUserAmount Error", zap.Error(err))
		}
	} else {
		zlog.LogWithContext(ctx).Error("PaymentCallBack ApplePaymentCallBackSuccess SetUserAmount Error:ExpireDate has expired")
	}
	if err := s.userAmountDao.SetUserAmount(ctx, projectID, userID, true); err != nil {
		zlog.LogWithContext(ctx).Error("PaymentCallBack GooglePaymentCallBackSuccess SetUserAmount Error", zap.Error(err))
	}

	return true, err
}

func (s *SubscribeService) CheckUserSubscribeRestore(ctx context.Context, userID, orderID, receipt string) error {
	var userSubscription *model.UserSubscription
	projectID := common.GetProjectID(ctx)
	err := s.dao.DB.Table("pay_user_subscription").Where("project_id = ? and user_id = ? and subscription_id = ?", projectID, userID, receipt).
		Order("id desc").First(&userSubscription).Error
	if err != nil {
		zlog.LogWithContext(ctx).Error("CheckUserSubscribeRestore GetUserSubscription Error", zap.Error(err))
		return err
	}
	if userSubscription == nil {
		zlog.LogWithContext(ctx).Error("CheckUserSubscribeRestore GetUserSubscription Error,userSubscription is Nil")
		return errors.New("userSubscription is Nil")
	}
	if userSubscription.UserID != userID {
		zlog.LogWithContext(ctx).Error("CheckUserSubscribeRestore UserID Not Match,Will Restore",
			zap.String("UserID", userID),
			zap.String("MappingUserID", userSubscription.UserID),
		)
		err := s.userDao.CreateUserMapping(userID, userSubscription.UserID, "SubscribeRestore")
		if err != nil {
			zlog.LogWithContext(ctx).Error("CheckUserSubscribeRestore CreateUserMapping Error", zap.Error(err))
			return err
		}
		rdb := db.GetRedis()
		rdb.HSet(constants.RedisKeyUserIDMapping+projectID, userID, userSubscription.UserID)
	}

	return nil
}

func (s *SubscribeService) ApplePaymentCallBack(ctx context.Context, projectID, userID, orderID, receipt, lang string, paymentStatus vai.PaymentStatus) (bool, *model.UserSubscription, error) {
	var err error

	var verifyRequest struct {
		TransactionID string `json:"transaction_id"`
		UserID        string `json:"user_id"`
		ProjectID     string `json:"project_id"`
	}
	verifyRequest.TransactionID = receipt
	verifyRequest.UserID = userID
	verifyRequest.ProjectID = projectID
	jsonBody, err := json.Marshal(verifyRequest)
	if err != nil {
		zlog.LogWithContext(ctx).Error("PaymentCallBack ApplePaymentCallBackFailed Marshal VerifyRequest Error", zap.Error(err))
		return false, nil, err
	}
	resp, err := s.httpClient.Post(conf.GlobalConfig.PayLinker.Addr+"/verify/apple", "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		zlog.LogWithContext(ctx).Error("PaymentCallBack ApplePaymentCallBackFailed",
			zap.String("Receipt", receipt),
			zap.String("OrderID", orderID),
			zap.Any(constants.ServiceEvent, constants.EventPaymentResultCallback),
		)
		return false, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		zlog.LogWithContext(ctx).Error("PaymentCallBack ApplePaymentCallBackFailed",
			zap.String("Receipt", receipt),
			zap.String("OrderID", orderID),
			zap.Any(constants.ServiceEvent, constants.EventPaymentResultCallback),
		)
		return false, nil, err
	}
	var rsp struct {
		Data *model.UserSubscription `json:"data"`
		Code int32                   `json:"code"`
		Msg  string                  `json:"msg"`
	}
	err = json.Unmarshal(body, &rsp)
	if err != nil {
		zlog.LogWithContext(ctx).Error("PaymentCallBack ApplePaymentCallBackFailed",
			zap.String("Receipt", receipt),
			zap.String("OrderID", orderID),
			zap.Any(constants.ServiceEvent, constants.EventPaymentResultCallback),
		)
		return false, nil, err
	}
	if rsp.Code != http.StatusOK {
		zlog.LogWithContext(ctx).Error("PaymentCallBack ApplePaymentCallBackFailed",
			zap.String("Receipt", receipt),
			zap.String("OrderID", orderID),
			zap.Any(constants.ServiceEvent, constants.EventPaymentResultCallback),
		)
		return false, nil, errors.New(rsp.Msg)
	}
	if rsp.Data != nil {
		if time.Now().UnixMilli() < rsp.Data.ExpireDate {
			if err := s.userAmountDao.SetUserAmount(ctx, projectID, userID, true); err != nil {
				zlog.LogWithContext(ctx).Error("PaymentCallBack ApplePaymentCallBackSuccess SetUserAmount Error", zap.Error(err))
			}
		} else {
			zlog.LogWithContext(ctx).Error("PaymentCallBack ApplePaymentCallBackSuccess SetUserAmount Error:ExpireDate has expired")
		}
	} else {
		zlog.LogWithContext(ctx).Error("PaymentCallBack ApplePaymentCallBackFailed: rsp.Data is nil",
			zap.String("Receipt", receipt),
			zap.String("OrderID", orderID),
			zap.Any(constants.ServiceEvent, constants.EventPaymentResultCallback),
		)
		return false, nil, errors.New("verify service returned nil data")
	}

	return true, rsp.Data, nil
}

type UserAmount struct {
	TextAmount  int32
	ImageAmount int32
}

func (s *SubscribeService) GetUserAmountByProjectID(ctx context.Context, projectID, userID string) (*vai.GetUserAmountResponse, error) {
	switch projectID {
	case constants.ProjectIdVisionAI, constants.ProjectIdVisualAI:
		return s.GetUserAmount(ctx, userID, "", "")
	case constants.ProjectIdPicFlow, constants.ProjectIdPicLib:
		return s.GetPicLibUserAmount(ctx, userID)
	default:
		return s.GetUserAmount(ctx, userID, "", "")
	}
	//return nil, errors.New("projectID not found")
}

func (s *SubscribeService) GetPicLibUserAmount(ctx context.Context, userID string) (*vai.GetUserAmountResponse, error) {
	projectID := common.GetProjectID(ctx)
	totalAmount, membershipAmount, purchasedAmount, systemGrantAmount, err := s.creditService.GetUserCreditOverview(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	userAmount := &vai.GetUserAmountResponse{}
	userAmount.PicLibAmount = &vai.PicLibUserAmount{
		TotalAmount:       int32(totalAmount),
		MemberAmount:      int32(membershipAmount),
		PurchaseAmount:    int32(purchasedAmount),
		SystemGrantAmount: int32(systemGrantAmount),
	}
	return userAmount, nil
}

func (s *SubscribeService) GetGenioUserAmount(ctx context.Context, userID string) (*vai.GetGenioUserAmountResponse, error) {
	projectID := common.GetProjectID(ctx)
	totalAmount, membershipAmount, purchasedAmount, systemGrantAmount, err := s.creditService.GetUserCreditOverview(ctx, projectID, userID)
	if err != nil {
		return nil, err
	}
	userAmount := &vai.GetGenioUserAmountResponse{}
	userAmount.GenioAmount = &vai.GenioUserAmount{
		TotalAmount:       int32(totalAmount),
		MemberAmount:      int32(membershipAmount),
		PurchaseAmount:    int32(purchasedAmount),
		SystemGrantAmount: int32(systemGrantAmount),
	}
	return userAmount, nil
}

// GetUserCreditAmount 直接查询 va_user_amount 表汇总用户积分
// 返回: totalAmount, memberAmount, purchaseAmount, systemGrantAmount, error
func (s *SubscribeService) GetUserCreditAmount(ctx context.Context, projectID, userID string) (int64, int64, int64, int64, error) {
	return s.creditService.GetUserCreditOverview(ctx, projectID, userID)
}

func (s *SubscribeService) GetUserAmount(ctx context.Context, userID, os, lang string) (*vai.GetUserAmountResponse, error) {
	projectID := common.GetProjectID(ctx)
	textAmount, imageAmount, err := s.userAmountDao.GetUserAmount(ctx, projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取用户额度失败", zap.Error(err), zap.String("projectID", projectID))
	}
	userAmount := &vai.GetUserAmountResponse{}
	userAmount.TextChatAmount = int32(textAmount)
	userAmount.ImageChatAmount = int32(imageAmount)
	return userAmount, nil
}

func (s *SubscribeService) CheckAndDeductUserAmount(ctx context.Context, userID string, msgType vai.MessageType) bool {
	projectID := common.GetProjectID(ctx)
	_, _, err := s.userAmountDao.GetUserAmount(ctx, projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("CheckAndDeductUserAmount GetUserAmount Error", zap.Error(err))
	}
	err = s.userAmountDao.DecrUserAmount(ctx, projectID, userID, msgType == vai.MessageType_MT_TEXT)
	return !errors.Is(err, constants.ERR_DAILY_TOLEN_LIMIT)
}

func (s *SubscribeService) FirstPay(receipt string, RspSubscriptionID string, os string, req *vai.PaymentResultCallbackRequest) bool {
	var TranscationID string
	if os == constants.IOS {
		TranscationID = receipt
	} else if os == constants.ANDROID {
		TranscationID = RspSubscriptionID
	}

	if TranscationID == "" {
		return false
	}
	var firstPay string
	db.GetDB().Table("pay_user_subscription").
		Where("user_id = ?", req.GetRequestHeader().GetUserId()).
		Select("subscription_id").
		Order("created_at asc").
		Limit(1).
		Find(&firstPay)

	if firstPay == TranscationID {
		return true
	} else {
		return false
	}
}

func (s *SubscribeService) GetAndInsertPrice(req *vai.PaymentResultCallbackRequest, id uint) (err error) {
	var apple_price int
	var price string
	if req.GetRequestHeader().GetDevice().GetOs() == 2 {
		db.GetDB().Table("pay_apple_transaction").
			Select("price").
			Where("user_id = ? and order_id = ?", req.GetRequestHeader().GetUserId(), req.GetOrderId()).
			Find(&apple_price)
		price = strconv.Itoa(apple_price)
	} else if req.GetRequestHeader().GetDevice().GetOs() == 1 {
		db.GetDB().Table("pay_alipay_notify").
			Select("receipt_amount").
			Where("user_id = ? and order_id = ?", req.GetRequestHeader().GetUserId(), req.GetOrderId()).
			Find(&price)
	}
	err = db.GetDB().Table("va_attribution_event").Where("id = ?", id).
		Update("price", price).
		Error
	if err != nil {
		return err
	}
	return err
}

func (s *SubscribeService) GetOrder(ctx context.Context, orderID string) (*model.Order, error) {
	var order *model.Order
	err := s.dao.DB.Table("pay_order").Where("order_id = ?", orderID).First(&order).Error
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (s *SubscribeService) GetProductInfoByOrderID(ctx context.Context, orderID string) (*model.Product, error) {
	var product *model.Product
	order, err := s.dao.GetOrderByID(orderID)
	if err != nil {
		return nil, err
	}

	product, err = s.productDao.GetByProductID(order.ProductID, "", "", order.ProjectID)
	if err != nil {
		return nil, err
	}

	return product, nil
}

// ExchangeCoupon 调用PayLinker服务兑换优惠券
func (s *SubscribeService) ExchangeCoupon(ctx context.Context, projectID, userID, couponCode string) (*model.CouponExchangeResponse, error) {
	// 构造请求
	exchangeReq := model.CouponExchangeRequest{
		UserID:     userID,
		ProjectID:  projectID,
		CouponCode: couponCode,
	}

	jsonBody, err := json.Marshal(exchangeReq)
	if err != nil {
		zlog.LogWithContext(ctx).Error("ExchangeCoupon Marshal Error", zap.Error(err))
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiEndpoint := conf.GlobalConfig.PayLinker.Addr + "/coupon/redeem"
	// 创建HTTP请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		zlog.LogWithContext(ctx).Error("ExchangeCoupon Create Request Error", zap.Error(err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := s.httpClient.Do(req)
	if err != nil {
		zlog.LogWithContext(ctx).Error("ExchangeCoupon Request Error", zap.Error(err))
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		zlog.LogWithContext(ctx).Error("ExchangeCoupon Response StatusCode Error",
			zap.Int("code", resp.StatusCode))
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		zlog.LogWithContext(ctx).Error("ExchangeCoupon Read Body Error", zap.Error(err))
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var response struct {
		Data *model.CouponExchangeResponse `json:"data"`
		Code int32                         `json:"code"`
		Msg  string                        `json:"msg"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		zlog.LogWithContext(ctx).Error("ExchangeCoupon Unmarshal Error", zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if response.Code != http.StatusOK {
		zlog.LogWithContext(ctx).Error("ExchangeCoupon Failed",
			zap.Int32("code", response.Code),
			zap.String("msg", response.Msg))
		return nil, fmt.Errorf("exchange failed: %s", response.Msg)
	}

	return response.Data, nil
}

// AddUserCredits 给用户增加积分（系统赠送）
func (s *SubscribeService) AddUserCredits(ctx context.Context, projectID, userID string, amount int64) (int64, error) {
	// 生成唯一的 sourceID
	//daily_free_com.web.genioai_6632b23de9e927fc10f4cca925ed0497_2026-02-11
	sourceID := "daily_free_com.web.genioai_" + userID + "_Byapi"

	// 设置积分永不过期（100年后）
	expiredAt := time.Now().AddDate(100, 0, 0)

	// 调用 creditService 添加积分
	err := s.creditService.AddCredits(
		ctx,
		"com.web.genioai",
		userID,
		sourceID,
		constants.CreditTypeDailyFree,        // 使用活动赠送类型
		constants.TransactionTypeSystemGrant, // 系统赠送交易类型
		amount,
		expiredAt,
		"系统赠送积分",
		0, // subscribeLevel: 0 表示非会员相关
	)
	if err != nil {
		zlog.LogWithContext(ctx).Error("AddUserCredits failed",
			zap.String("userID", userID),
			zap.String("projectID", projectID),
			zap.Int64("amount", amount),
			zap.Error(err))
		return 0, err
	}

	// 获取用户当前积分总额
	totalAmount, _, _, _, err := s.creditService.GetUserCreditOverview(ctx, projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetUserCreditOverview failed after adding credits",
			zap.String("userID", userID),
			zap.Error(err))
		return 0, err
	}

	return totalAmount, nil
}

// AddUserCreditsByUUID 通过 UUID 给用户增加积分（系统赠送）
func (s *SubscribeService) AddUserCreditsByUUID(ctx context.Context, projectID, uuid string, amount int64) (int64, error) {
	// 通过 UUID 查询用户获取真正的 user_id
	user, err := s.userDao.GetUserByUUID(projectID, uuid)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetUserByUUID failed",
			zap.String("uuid", uuid),
			zap.String("projectID", projectID),
			zap.Error(err))
		return 0, errors.New("用户不存在")
	}

	// 使用查询到的 user_id 添加积分
	return s.AddUserCredits(ctx, projectID, user.UserId, amount)
}

// AddUserCreditsByEmail 通过邮箱给用户增加积分（系统赠送）
func (s *SubscribeService) AddUserCreditsByEmail(ctx context.Context, projectID, email string, amount int64) (int64, error) {
	// 通过邮箱查询用户获取真正的 user_id
	user, err := s.userDao.GetUserByEmail(projectID, email)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetUserByEmail failed",
			zap.String("email", email),
			zap.String("projectID", projectID),
			zap.Error(err))
		return 0, errors.New("用户不存在")
	}

	// 使用查询到的 user_id 添加积分
	return s.AddUserCredits(ctx, projectID, user.UserId, amount)
}

// GetUserByEmail 通过邮箱查询用户
func (s *SubscribeService) GetUserByEmail(projectID, email string) (*model.UserRecord, error) {
	return s.userDao.GetUserByEmail(projectID, email)
}

// GetUserTaskStats 获取所有用户的生图统计
func (s *SubscribeService) GetUserTaskStats(ctx context.Context) (*vai.GetUserTaskStatsResponse, error) {
	// 查询所有用户的任务统计
	stats, err := s.pictureTaskDao.GetAllUserTaskStats(ctx)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetAllUserTaskStats failed", zap.Error(err))
		return nil, err
	}

	// 收集所有 user_id
	userIDs := make([]string, 0, len(stats))
	for _, stat := range stats {
		userIDs = append(userIDs, stat.UserID)
	}

	// 批量查询用户信息
	userMap := make(map[string]*model.UserRecord)
	if len(userIDs) > 0 {
		users, err := s.userDao.GetUsersByUserIDs(userIDs)
		if err != nil {
			zlog.LogWithContext(ctx).Error("GetUsersByUserIDs failed", zap.Error(err))
			// 用户信息查询失败不影响主流程，继续返回统计数据
		} else {
			for i := range users {
				userMap[users[i].UserId] = &users[i]
			}
		}
	}

	// 组装响应
	var userStats []*vai.UserTaskStatsInfo
	for _, stat := range stats {
		info := &vai.UserTaskStatsInfo{
			UserId:              stat.UserID,
			TotalTasks:          stat.TotalTasks,
			CompletedTasks:      stat.CompletedTasks,
			FailedTasks:         stat.FailedTasks,
			ProcessingTasks:     stat.ProcessingTasks,
			TotalCreditsConsumed: stat.TotalCreditsUsed,
		}
		if stat.LastTaskTime != nil {
			info.LastTaskTime = stat.LastTaskTime.Format("2006-01-02 15:04:05")
		}
		if user, ok := userMap[stat.UserID]; ok {
			info.Uuid = user.UUID
			info.Email = user.Email
		}
		userStats = append(userStats, info)
	}

	return &vai.GetUserTaskStatsResponse{
		UserStats: userStats,
	}, nil
}
