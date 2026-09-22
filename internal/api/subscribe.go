package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/service"
	credit "va_visionai_server/internal/service/credit"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

type SubscribeServer struct {
	vai.UnimplementedSubscribeServiceServer

	SubscribeService *service.SubscribeService
	ProductService   *service.ProductService
	creditService    credit.Service
	pictureTaskDao   *dao.PictureTaskDao
}

func NewSubscribeServer(subscribe *service.SubscribeService, product *service.ProductService, creditSvc credit.Service, taskDao *dao.PictureTaskDao) *SubscribeServer {
	return &SubscribeServer{
		SubscribeService: subscribe,
		ProductService:   product,
		creditService:    creditSvc,
		pictureTaskDao:   taskDao,
	}
}

func (s *SubscribeServer) CreateOrder(ctx context.Context, req *vai.CreateOrderRequest) (*vai.CreateOrderResponse, error) {
	productID := req.GetProductId()
	userID := req.GetRequestHeader().GetUserId()
	paymentWay := req.GetPaymentWay().String()
	projectID := common.GetProjectID(ctx)
	var err error
	var order *model.Order
	var rsp *vai.CreateOrderResponse

	defer func() {
		orderID := ""
		if order != nil {
			orderID = order.OrderID
		}
		zlog.LogWithContext(ctx).Info("CreateOrder",
			zap.String(constants.CtxOrderID, orderID),
			zap.String(constants.CtxUserID, userID),
			zap.String(constants.CtxProductID, productID),
			zap.String(constants.CtxProjectID, projectID),
			zap.String(constants.CtxPaymentWay, paymentWay),
			zap.Any(constants.ServiceEvent, constants.EventCreateOrder),
			zap.Error(err),
		)
	}()

	order, err = s.SubscribeService.CreateOrder(ctx, projectID, userID, productID, paymentWay)
	if err != nil {
		zlog.LogWithContext(ctx).Error("CreateOrder Error", zap.Error(err))
		rsp, err = BuildErrorResponse[vai.CreateOrderResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.CreateOrderResponse{
		Uuid:            order.OrderID,
		AlipayPayParams: order.AlipayPayParams,
	})
	return rsp, nil
}

func (s *SubscribeServer) GetProductInfoList(ctx context.Context, req *vai.ProductInfoListRequest) (*vai.ProductInfoListResponse, error) {
	groupID := int(req.GetProductGroup())
	appVersion := req.GetRequestHeader().GetApp().GetAppVersion()
	var err error
	var rsp *vai.ProductInfoListResponse
	fetchAll := req.GetFetchAll()

	defer func() {
		if rsp != nil && rsp.GetResponseHeader() != nil {
			zlog.LogWithContext(ctx).Info("GetProductInfoList",
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.Int("GroupID", groupID),
				zap.Error(err))
		} else {
			zlog.LogWithContext(ctx).Info("GetProductInfoList",
				zap.Int("GroupID", groupID),
				zap.Error(err))
		}
	}()

	// 获取用户订阅信息
	userID := req.GetRequestHeader().GetUserId()
	os := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())
	lang := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())

	var subscribeInfo *vai.SubscribeInfo
	if userID != "" {
		subscribeInfo, err = s.SubscribeService.GetSubscribeInfo(ctx, userID, os, lang, "")
		if err != nil {
			// 获取订阅信息失败时记录日志但不影响商品列表获取
			zlog.LogWithContext(ctx).Warn("failed to get subscribe info, proceeding without subscription filtering",
				zap.String("userID", userID), zap.Error(err))
			subscribeInfo = nil
		}
	}

	productInfoList, err := s.ProductService.ListProducts(ctx, userID, os, groupID, subscribeInfo, fetchAll, appVersion)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetProductInfoList Error", zap.Error(err))
		rsp, err = BuildErrorResponse[vai.ProductInfoListResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	projectID := common.GetProjectID(ctx)
	productInfos := s.ProductService.BuildProductInfos(ctx, productInfoList, projectID, userID)

	rsp, err = BuildSuccessResponse(&vai.ProductInfoListResponse{
		ProductInfo: productInfos,
	})
	return rsp, nil
}

func (s *SubscribeServer) PaymentResultCallback(ctx context.Context, req *vai.PaymentResultCallbackRequest) (*vai.PaymentResultCallbackResponse, error) {
	receipt := req.GetTransaction()
	userID := req.GetRequestHeader().GetUserId()
	orderID := req.GetOrderId()
	os := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())
	lang := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	projectID := common.GetProjectID(ctx)
	var err error
	var RspSubscriptionID string
	var rsp *vai.PaymentResultCallbackResponse
	var transactionInfo *model.UserSubscription
	var productID string

	defer func() {
		zlog.LogWithContext(ctx).Info("PaymentResultCallback",
			zap.String("Receipt", receipt),
			zap.String("OrderID", orderID),
			zap.Any(constants.ServiceEvent, constants.EventPaymentResultCallback),
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
			zap.Error(err),
		)
	}()

	paymentStatus := vai.PaymentStatus_PaymentStatusPending

	purchaseType := vai.PurchaseType_PURCHASE_TYPE_UNKNOWN
	var paymentWay vai.PaymentWay
	if order, err := s.SubscribeService.GetOrder(ctx, orderID); err != nil {
		zlog.LogWithContext(ctx).Error("PaymentCallback GetOrder Error", zap.Error(err))
		if os != constants.IOS {
			rsp, _ = BuildErrorResponse[vai.PaymentResultCallbackResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, "GetPaymentWay Error")
			return rsp, nil
		}

	} else {
		paymentWay = vai.PaymentWay(vai.PaymentWay_value[order.PaymentWay])
		product, err := s.ProductService.GetProduct(ctx, order.ProductID, lang, os, projectID)
		if err != nil {
			zlog.LogWithContext(ctx).Error("PaymentCallback GetProduct Error", zap.Error(err))
		}
		switch product.ProductType {
		case constants.ProductTypeSubscribe:
			purchaseType = vai.PurchaseType_PURCHASE_TYPE_SUBSCRIPTION
		case constants.ProductTypeOneTimeCharge:
			purchaseType = vai.PurchaseType_PURCHASE_TYPE_IN_APP
		default:
			purchaseType = vai.PurchaseType_PURCHASE_TYPE_UNKNOWN
		}
	}
	if os == constants.IOS {
		paymentWay = vai.PaymentWay_PaymentApple
	}

	var success bool

	switch paymentWay {
	case vai.PaymentWay_PaymentApple:
		success, transactionInfo, err = s.SubscribeService.ApplePaymentCallBack(ctx, projectID, userID, orderID, receipt, lang, vai.PaymentStatus_PaymentStatusSuccess)
		if transactionInfo != nil && orderID == "" {
			productID = transactionInfo.ProductID
		}
	case vai.PaymentWay_PaymentWayAlipay:
		success, RspSubscriptionID, err = s.SubscribeService.AlipayPaymentCallBack(ctx, projectID, userID, orderID, receipt, lang, vai.PaymentStatus_PaymentStatusSuccess)
	case vai.PaymentWay_PaymentWayGoogle:
		success, err = s.SubscribeService.GooglePaymentCallBack(ctx, projectID, userID, orderID, receipt, lang, vai.PaymentStatus_PaymentStatusSuccess)
	}

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		zlog.LogWithContext(ctx).Error("PaymentCallback Error", zap.Error(err))
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		zlog.LogWithContext(ctx).Error("PaymentCallback Order Waiting for Payment")
	}

	if success {
		paymentStatus = vai.PaymentStatus_PaymentStatusSuccess
		err = s.SubscribeService.CheckUserSubscribeRestore(ctx, userID, orderID, receipt)
		if err != nil {
			zlog.LogWithContext(ctx).Error("PaymentCallback CheckUserSubscribeRestore Error", zap.Error(err))
		}

		zlog.LogWithContext(ctx).Info("PaymentSuccess",
			zap.String("Receipt", receipt),
			zap.String("AliReceipt", RspSubscriptionID),
			zap.String("OrderID", orderID),
			zap.String(constants.ServiceEvent, constants.EventPaymentSuccess),
			zap.String(constants.CtxIdfv, req.GetRequestHeader().GetDevice().GetIdfv()),
			zap.String(constants.CtxAndroidId, req.GetRequestHeader().GetDevice().GetAndroidId()),
			zap.String(constants.CtxPackageName, req.GetRequestHeader().GetApp().GetPackageName()),
		)
	}

	//判断是否是第一次支付成功
	if s.SubscribeService.FirstPay(receipt, RspSubscriptionID, os, req) {
		attributionService := service.NewattributionService()
		//记录初始基本数据
		id, err := attributionService.RecordEventInfo(req.GetRequestHeader(), constants.PaymentFirst)
		if err != nil {
			zlog.LogWithContext(ctx).Error("Record EventInfo Error", zap.Error(err))
		}
		//记录支付金额等信息
		err = s.SubscribeService.GetAndInsertPrice(req, id)
		if err != nil {
			zlog.LogWithContext(ctx).Error("PaymentCallback GetAndInsertPrice Error", zap.Error(err))
		}
	}

	subscribeInfo, err := s.SubscribeService.GetSubscribeInfo(ctx, userID, os, lang, receipt)
	if err != nil {
		zlog.LogWithContext(ctx).Error("PaymentCallback GetSubscribeInfo Error", zap.Error(err))
		rsp, err = BuildErrorResponse[vai.PaymentResultCallbackResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	var product *model.Product
	if orderID != "" {
		product, err = s.SubscribeService.GetProductInfoByOrderID(ctx, orderID)
	} else if productID != "" {
		product, err = s.ProductService.GetProduct(ctx, productID, lang, os, projectID)
	}
	if err != nil {
		zlog.LogWithContext(ctx).Error("PaymentCallback GetProductInfoByOrderID Error", zap.Error(err))
	}

	rsp, err = BuildSuccessResponse(&vai.PaymentResultCallbackResponse{
		PaymentStatus: paymentStatus,
		SubscribeInfo: subscribeInfo,
		PurchaseType:  purchaseType,
	})
	if product != nil {
		rsp.ProductInfo = s.ProductService.BuildProductInfo(ctx, product, projectID, userID)
	}
	return rsp, nil
}

func (s *SubscribeServer) GetUserAmount(ctx context.Context, req *vai.GetUserAmountRequest) (*vai.GetUserAmountResponse, error) {
	userID := req.GetRequestHeader().GetUserId()
	projectID := common.GetProjectID(ctx)
	var err error
	var rsp *vai.GetUserAmountResponse

	defer func() {
		zlog.LogWithContext(ctx).Info("GetUserAmount",
			zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
			zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
			zap.Error(err))
	}()

	userAmountRsp, err := s.SubscribeService.GetUserAmountByProjectID(ctx, projectID, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetUserAmount Error", zap.Error(err))
		rsp, err = BuildErrorResponse[vai.GetUserAmountResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	rsp = userAmountRsp
	rsp, err = BuildSuccessResponse(rsp)
	return rsp, nil
}

func (s *SubscribeServer) GetGenioUserAmount(ctx context.Context, req *vai.GetGenioUserAmountRequest) (*vai.GetGenioUserAmountResponse, error) {
	userID := req.GetRequestHeader().GetUserId()
	var err error
	var rsp *vai.GetGenioUserAmountResponse

	defer func() {
		if rsp != nil && rsp.GetResponseHeader() != nil {
			zlog.LogWithContext(ctx).Info("GetGenioUserAmount",
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.Error(err))
		} else {
			zlog.LogWithContext(ctx).Info("GetGenioUserAmount", zap.Error(err))
		}
	}()

	userAmountRsp, err := s.SubscribeService.GetGenioUserAmount(ctx, userID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetGenioUserAmount Error", zap.Error(err))
		rsp, err = BuildErrorResponse[vai.GetGenioUserAmountResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	rsp = userAmountRsp
	rsp, err = BuildSuccessResponse(rsp)
	return rsp, nil
}

func (s *SubscribeServer) GetCreditDetail(ctx context.Context, req *vai.CreditDetailRequest) (*vai.CreditDetailResponse, error) {
	userID := req.GetRequestHeader().GetUserId()
	projectID := common.GetProjectID(ctx)
	page := int(req.GetPageCommon().GetPage())
	pageSize := int(req.GetPageCommon().GetPageSize())
	creditChangeType := req.GetCreditChangeType()
	var err error
	var rsp *vai.CreditDetailResponse

	defer func() {
		if rsp != nil && rsp.GetResponseHeader() != nil {
			zlog.LogWithContext(ctx).Info("GetCreditDetail",
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.Error(err))
		} else {
			zlog.LogWithContext(ctx).Info("GetCreditDetail", zap.Error(err))
		}
	}()

	transactions, total, err := s.creditService.GetUserCreditTransactions(ctx, projectID, userID, pageSize, (page-1)*pageSize, creditChangeType)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.CreditDetailResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	balance, err := s.creditService.GetUserCreditBalance(ctx, projectID, userID)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.CreditDetailResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	// 收集扣费/退款交易关联的 task_id（存储在 SourceID 字段中）
	taskIDSet := make(map[string]struct{})
	for _, tx := range transactions {
		txType := constants.CreditTransactionType(tx.TransactionType)
		if txType == constants.TransactionTypeUsageDeduction || txType == constants.TransactionTypeCreationFailedRefund || txType == constants.TransactionTypeTaskRetryDeduction {
			if strings.HasPrefix(tx.SourceID, "task_") {
				taskIDSet[tx.SourceID] = struct{}{}
			}
		}
	}

	// 批量查询关联的 PictureTask
	taskMap := make(map[string]*dao.PictureTaskBrief)
	if len(taskIDSet) > 0 && s.pictureTaskDao != nil {
		taskIDs := make([]string, 0, len(taskIDSet))
		for id := range taskIDSet {
			taskIDs = append(taskIDs, id)
		}
		tasks, queryErr := s.pictureTaskDao.GetTasksByTaskIDs(ctx, taskIDs)
		if queryErr != nil {
			zlog.LogWithContext(ctx).Warn("failed to query picture tasks for credit detail",
				zap.Error(queryErr))
		} else {
			for _, t := range tasks {
				brief := &dao.PictureTaskBrief{
					TaskID:     t.TaskID,
					UserPrompt: t.UserPrompt,
				}
				// 从 ResultJSON 解析结果 URL
				if t.ResultJSON != "" {
					var result model.PictureTaskResult
					if err := json.Unmarshal([]byte(t.ResultJSON), &result); err == nil && result.ResultURL != "" {
						brief.ResultURLs = []string{result.ResultURL}
					}
				}
				taskMap[t.TaskID] = brief
			}
		}
	}

	details := make([]*vai.CreditDetail, 0, len(transactions))
	for _, tx := range transactions {
		detail := &vai.CreditDetail{
			CreditChangeAmount: int32(tx.AmountChange),
			CreditChangeTime:   tx.CreatedAt.Format(time.DateTime),
			CreditChangeReason: getTransactionReason(constants.CreditTransactionType(tx.TransactionType)),
		}
		// 填充关联的生图任务信息
		if strings.HasPrefix(tx.SourceID, "task_") {
			detail.TaskId = tx.SourceID
			if brief, ok := taskMap[tx.SourceID]; ok {
				detail.UserPrompt = brief.UserPrompt
				detail.ResultUrls = brief.ResultURLs
			}
		}
		details = append(details, detail)
	}

	rsp, err = BuildSuccessResponse(&vai.CreditDetailResponse{
		CreditDetail:                  details,
		RemainingTotalCredit:          int32(balance.TotalCredits),
		RemainingSubscribeCreditPoint: int32(balance.SubscriptionCredits),
		RemainingBuyCreditPoint:       int32(balance.PurchasedCredits),
		RemainingSystemGrantCredit:    int32(balance.SystemGrantCredits),
		PageCommon: &vai.PageCommon{
			Page:     int32(page),
			PageSize: int32(pageSize),
			Total:    int32(total),
		},
	})
	return rsp, nil
}

// getTransactionReason translates transaction types into human-readable strings.
func getTransactionReason(txType constants.CreditTransactionType) string {
	switch txType {
	case constants.TransactionTypeUsageDeduction:
		return "Usage"
	case constants.TransactionTypeCreationFailedRefund:
		return "Creation Failed Refund"
	case constants.TransactionTypePurchase, constants.TransactionTypeOnetimePurchase:
		return "Purchase"
	case constants.TransactionTypeSubscriptionGrant, constants.TransactionTypeSubscriptionCreated, constants.TransactionTypeSubscriptionRenewed, constants.TransactionTypeSubscriptionUpgrade:
		return "Subscription Gift"
	case constants.TransactionTypeSubscriptionExpiry, constants.TransactionTypeSubscriptionExpiredClear:
		return "Subscription Expiry"
	case constants.TransactionTypeAdminAdjustment, constants.TransactionTypeSystemGrant, constants.TransactionTypeDailyGrant:
		return "System Gift"
	case constants.TransactionTypeTaskRetryDeduction:
		return "Retry Usage"
	case constants.TransactionTypeEventGrant:
		return "Active Grant"
	case constants.TransactionTypeInviteReward:
		return "Invitation Reward"
	default:
		return "UNKNOWN"
	}
}

func (s *SubscribeServer) CouponRedeem(ctx context.Context, req *vai.CouponRedeemRequest) (*vai.CouponRedeemResponse, error) {
	userID := req.GetRequestHeader().GetUserId()
	couponCode := req.GetCouponCode()
	projectID := common.GetProjectID(ctx)
	os := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())
	lang := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())

	var err error
	var rsp *vai.CouponRedeemResponse

	defer func() {
		code := vai.StatusCode_SUCCESS
		msg := "兑换成功"
		if rsp != nil && rsp.GetResponseHeader() != nil {
			code = rsp.GetResponseHeader().GetCode()
			msg = rsp.GetResponseHeader().GetMsg()
		}

		zlog.LogWithContext(ctx).Info("ExchangeCoupon",
			zap.String("user_id", userID),
			zap.String("coupon_code", couponCode),
			zap.String("project_id", projectID),
			zap.String("code", code.String()),
			zap.String("msg", msg),
			zap.Error(err))
	}()

	// 参数验证
	if userID == "" {
		rsp, err = BuildErrorResponse[vai.CouponRedeemResponse](ctx, vai.StatusCode_INVALID_PARAM,
			errors.New("用户ID不能为空"), "用户ID不能为空")
		return rsp, nil
	}

	if couponCode == "" {
		rsp, err = BuildErrorResponse[vai.CouponRedeemResponse](ctx, vai.StatusCode_INVALID_PARAM,
			errors.New("优惠券代码不能为空"), "优惠券代码不能为空")
		return rsp, nil
	}

	// 调用SubscribeService进行优惠券兑换
	redeemResult, err := s.SubscribeService.ExchangeCoupon(ctx, projectID, userID, couponCode)
	if err != nil {
		zlog.LogWithContext(ctx).Error("优惠券兑换失败", zap.Error(err))
		rsp, err = BuildErrorResponse[vai.CouponRedeemResponse](ctx, vai.StatusCode_REQUEST_FAILED,
			err, err.Error())
		return rsp, nil
	}

	if !redeemResult.Success {
		rsp, err = BuildErrorResponse[vai.CouponRedeemResponse](ctx, vai.StatusCode_INVALID_COUPON,
			errors.New("invalid coupon"), "invalid coupon")
		return rsp, nil
	}

	subscribeInfo, err := s.SubscribeService.GetSubscribeInfo(ctx, userID, os, lang, "")
	if err != nil {
		zlog.LogWithContext(ctx).Error("ExchangeCoupon GetSubscribeInfo Error", zap.Error(err))
		rsp, err = BuildErrorResponse[vai.CouponRedeemResponse](ctx, vai.StatusCode_REQUEST_FAILED, err, constants.ErrMsgRequestFailed)
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.CouponRedeemResponse{
		RedeemResult:  redeemResult.Success,
		SubscribeInfo: subscribeInfo,
	})

	return rsp, nil
}

// GetSubscriptionExitConfig 获取用户订阅退出流程配置
// 基于A/B测试框架，为不同用户组返回不同的退出流程配置
func (s *SubscribeServer) GetSubscriptionExitConfig(ctx context.Context, req *vai.GetSubscriptionExitConfigRequest) (*vai.GetSubscriptionExitConfigResponse, error) {
	userID := req.GetRequestHeader().GetUserId()
	projectID := common.GetProjectID(ctx)
	os := constants.MappingOS(req.GetRequestHeader().GetDevice().GetOs())
	lang := constants.LanguageMap(req.GetRequestHeader().GetDevice().GetLanguage())
	appVersion := req.GetRequestHeader().GetApp().GetAppVersion()
	var err error
	var rsp *vai.GetSubscriptionExitConfigResponse

	defer func() {
		if rsp != nil && rsp.GetResponseHeader() != nil {
			zlog.LogWithContext(ctx).Info("GetSubscriptionExitConfig",
				zap.Any(constants.ServiceEvent, constants.EventGetSubscriptionExitConfigComplete),
				zap.String("user_id", userID),
				zap.String("project_id", projectID),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.Error(err))
		} else {
			zlog.LogWithContext(ctx).Info("GetSubscriptionExitConfig",
				zap.Any(constants.ServiceEvent, constants.EventGetSubscriptionExitConfigComplete),
				zap.String("user_id", userID),
				zap.String("project_id", projectID),
				zap.Error(err))
		}
	}()

	// 参数验证
	if userID == "" {
		rsp, err = BuildErrorResponse[vai.GetSubscriptionExitConfigResponse](ctx, vai.StatusCode_INVALID_PARAM,
			errors.New("用户ID不能为空"), "用户ID不能为空")
		return rsp, nil
	}
	//goCal项目 且版本号等于正在审核的版本号，则直接返回返回默认退出流程
	if projectID == constants.ProjectIdDietAI && s.ProductService.IsGoCalReviewingVersion(ctx, appVersion) {
		zlog.LogWithContext(ctx).Info("appVersion is reviewing version", zap.String("appVersion", appVersion),
			zap.String("userID", userID),
			zap.String("ExitFlowType", vai.SubscriptionExitFlowType_EXIT_FLOW_DEFAULT.String()))
		rsp, err = BuildSuccessResponse(&vai.GetSubscriptionExitConfigResponse{
			ExitFlowType:  vai.SubscriptionExitFlowType_EXIT_FLOW_DEFAULT,
			ConfigDetails: "",
		})
		return rsp, nil
	}
	// 首先判断用户是否为会员
	subscribeInfo, err := s.SubscribeService.GetSubscribeInfo(ctx, userID, os, lang, "")
	if err != nil {
		zlog.LogWithContext(ctx).Error("GetSubscribeInfo Error", zap.Error(err))
		// 如果判断失败，继续执行原有逻辑
	} else if subscribeInfo != nil && subscribeInfo.GetStatus() == vai.SubscribeStatus_SubscribeActivate {
		// 如果是会员，直接返回默认退出流程
		zlog.LogWithContext(ctx).Info("user is subscriber",
			zap.String("userID", userID),
			zap.String("ExitFlowType", vai.SubscriptionExitFlowType_EXIT_FLOW_DEFAULT.String()))
		rsp, err = BuildSuccessResponse(&vai.GetSubscriptionExitConfigResponse{
			ExitFlowType:  vai.SubscriptionExitFlowType_EXIT_FLOW_DEFAULT,
			ConfigDetails: "",
		})
		return rsp, nil
	}

	// 如果不是会员，执行原有的A/B测试逻辑
	// 调用ProductService获取退出流程配置
	exitConfig, err := s.ProductService.GetSubscriptionExitConfig(ctx, userID, projectID)
	if err != nil {
		zlog.LogWithContext(ctx).Error("获取订阅退出配置失败", zap.Error(err))
		rsp, err = BuildErrorResponse[vai.GetSubscriptionExitConfigResponse](ctx, vai.StatusCode_REQUEST_FAILED,
			err, "获取配置失败")
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.GetSubscriptionExitConfigResponse{
		ExitFlowType:  exitConfig.ExitFlowType,
		ConfigDetails: exitConfig.ConfigDetails,
	})

	return rsp, nil
}

// AddUserCredits 给用户增加积分
func (s *SubscribeServer) AddUserCredits(ctx context.Context, req *vai.AddUserCreditsRequest) (*vai.AddUserCreditsResponse, error) {
	email := req.GetEmail()
	amount := req.GetAmount()
	projectID := common.GetProjectID(ctx)
	var err error
	var rsp *vai.AddUserCreditsResponse

	defer func() {
		if rsp != nil && rsp.GetResponseHeader() != nil {
			zlog.LogWithContext(ctx).Info("AddUserCredits",
				zap.String("email", email),
				zap.Int64("amount", amount),
				zap.String("project_id", projectID),
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.Error(err))
		} else {
			zlog.LogWithContext(ctx).Info("AddUserCredits",
				zap.String("email", email),
				zap.Int64("amount", amount),
				zap.String("project_id", projectID),
				zap.Error(err))
		}
	}()

	// 参数验证
	if email == "" {
		rsp, err = BuildErrorResponse[vai.AddUserCreditsResponse](ctx, vai.StatusCode_INVALID_PARAM,
			errors.New("邮箱不能为空"), "邮箱不能为空")
		return rsp, nil
	}
	if amount <= 0 {
		rsp, err = BuildErrorResponse[vai.AddUserCreditsResponse](ctx, vai.StatusCode_INVALID_PARAM,
			errors.New("积分数量必须大于0"), "积分数量必须大于0")
		return rsp, nil
	}

	// 调用 Service 层添加积分
	totalCredits, err := s.SubscribeService.AddUserCreditsByEmail(ctx, projectID, email, amount)
	if err != nil {
		zlog.LogWithContext(ctx).Error("AddUserCredits failed", zap.Error(err))
		rsp, err = BuildErrorResponse[vai.AddUserCreditsResponse](ctx, vai.StatusCode_REQUEST_FAILED,
			err, "增加积分失败")
		return rsp, nil
	}

	rsp, err = BuildSuccessResponse(&vai.AddUserCreditsResponse{
		Success:      true,
		TotalCredits: totalCredits,
	})

	return rsp, nil
}

// GetUserTaskStats 获取用户生图统计
func (s *SubscribeServer) GetUserTaskStats(ctx context.Context, req *vai.GetUserTaskStatsRequest) (*vai.GetUserTaskStatsResponse, error) {
	var err error
	var rsp *vai.GetUserTaskStatsResponse

	defer func() {
		if rsp != nil && rsp.GetResponseHeader() != nil {
			zlog.LogWithContext(ctx).Info("GetUserTaskStats",
				zap.String("Code", rsp.GetResponseHeader().GetCode().String()),
				zap.String("Msg", rsp.GetResponseHeader().GetMsg()),
				zap.Error(err))
		} else {
			zlog.LogWithContext(ctx).Info("GetUserTaskStats", zap.Error(err))
		}
	}()

	statsRsp, err := s.SubscribeService.GetUserTaskStats(ctx)
	if err != nil {
		rsp, err = BuildErrorResponse[vai.GetUserTaskStatsResponse](ctx, vai.StatusCode_REQUEST_FAILED,
			err, "获取用户生图统计失败")
		return rsp, nil
	}

	rsp = statsRsp
	rsp, err = BuildSuccessResponse(rsp)
	return rsp, nil
}
