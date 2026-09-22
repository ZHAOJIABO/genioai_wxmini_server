package rpc

import (
	"context"
	"errors"
	"net"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-redis/redis"
	"github.com/rs/xid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/db"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/prometheus"
	"va_visionai_server/internal/utils"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// func promethuesHook(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, resp interface{}, err error) {
//
//	defer func() {
//		if r := recover(); r != nil {
//			log.Printf("Recovered from panic in hook: %v", r)
//		}
//	}()
//
// }
// 拦截器相关的处理，统一参数检查会返回一个error 。其他 postprocessor 如果和统计相关的不会抛出error 避免影响正常的业务流程
func paramsCheck(ctx context.Context, m interface{}, reqHeader *vai.RequestHeader) error {
	defer func() {
		if r := recover(); r != nil {
			zlog.Logger.Error("Recovered from panic in hook: %v", zap.Any("err", r))
		}
	}()
	if msg, ok := m.(proto.Message); ok {
		// 免认证接口白名单
		_, isLoginReq := msg.(*vai.LoginRequest)
		_, isWeChatLoginReq := msg.(*vai.WeChatAuthRequest)
		_, isListImageModelsReq := msg.(*vai.ListImageGenerationModelsRequest)
		_, isAddUserCreditsReq := msg.(*vai.AddUserCreditsRequest)
		_, isGetUserTaskStatsReq := msg.(*vai.GetUserTaskStatsRequest)

		// 如果不在白名单中，则需要进行用户认证
		if !isLoginReq && !isWeChatLoginReq && !isListImageModelsReq && !isAddUserCreditsReq && !isGetUserTaskStatsReq {
			if err := checkReqHeader(ctx, reqHeader); err != nil {
				// zlog.LogWithContext(ctx).Error("checkReqHeader Error", zap.Error(err))
				// 移动端某些接口请求时没有带上userID 返回nil 避免影响正常的业务流程
				return nil
			}
		}
		err := validateMessage(ctx, msg)
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}

func extractReqHeader(msg interface{}) *vai.RequestHeader {
	defer func() {
		if r := recover(); r != nil {
			zlog.Logger.Error("Recovered from panic in hook: %v", zap.Any("err", r), zap.String("Func", "extractReqHeader"))
			return
		}
	}()
	t := reflect.TypeOf(msg)
	v := reflect.ValueOf(msg)

	if t.Kind() == reflect.Pointer {
		t = t.Elem()
		v = v.Elem()
	}

	if t.Kind() != reflect.Struct {
		return nil
	}

	fieldNum := t.NumField()
	for i := 0; i < fieldNum; i++ {
		fieldName := strings.ToUpper(t.Field(i).Name)
		if fieldName == "REQUESTHEADER" {
			fieldValue := v.Field(i)
			if fieldValue.Kind() == reflect.Ptr {
				if !fieldValue.IsNil() {
					reqHeader, ok := fieldValue.Interface().(*vai.RequestHeader)
					if ok {
						return reqHeader
					}
				}
			}
		}
	}
	return nil
}

func validateMessage(ctx context.Context, msg proto.Message) error {
	msgType := reflect.TypeOf(msg)
	switch msgType {
	case reflect.TypeOf(&vai.ChatMessageSendRequest{}):
		return validateChatMessageSendRequest(ctx, msg.(*vai.ChatMessageSendRequest))
	default:
		return nil
	}
}

func postProcessor(m interface{}, ctx context.Context) {
	msg, ok := m.(proto.Message)
	if !ok {
		return
	}
	msgType := reflect.TypeOf(msg)
	switch msgType {
	case reflect.TypeOf(&vai.ChatMessageSendRequest{}):
		chatMessageSendRequestPostProcessor(msg.(*vai.ChatMessageSendRequest), ctx)
	default:
		return
	}
}

func buildCtx(ctx context.Context, header *vai.RequestHeader) context.Context {
	newCtx := buildTraceIDCtx(ctx)
	newCtx = ctxSetUserID(newCtx, header.GetUserId())
	newCtx = buildDeviceCtx(newCtx, header)

	// 使用辅助函数获取 projectID
	projectID := strings.ToLower(getPackageName(header))
	newCtx = common.CtxSetStrValue(newCtx, constants.CtxProjectID, projectID)
	return newCtx
}

// isWebClient 判断是否为 Web 端客户端
func isWebClient(header *vai.RequestHeader) bool {
	return header.GetWebClient() != nil || header.GetBrowserInfo() != nil
}

// IsWebClient 判断是否为 Web 端客户端（导出版本）
func IsWebClient(header *vai.RequestHeader) bool {
	return isWebClient(header)
}

// getAppVersion 获取应用版本（兼容移动端和Web端）
func getAppVersion(header *vai.RequestHeader) string {
	if header == nil {
		return ""
	}
	if isWebClient(header) {
		if webClient := header.GetWebClient(); webClient != nil {
			return webClient.GetClientVersion()
		}
		return ""
	}
	if app := header.GetApp(); app != nil {
		return app.GetAppVersion()
	}
	return ""
}

// GetAppVersion 获取应用版本（兼容移动端和Web端，导出版本）
func GetAppVersion(header *vai.RequestHeader) string {
	return getAppVersion(header)
}

// getPlatformOS 获取操作系统/平台（兼容移动端和Web端）
func getPlatformOS(header *vai.RequestHeader) string {
	if header == nil {
		return "Unknown"
	}
	if isWebClient(header) {
		if browserInfo := header.GetBrowserInfo(); browserInfo != nil {
			platform := browserInfo.GetPlatform()
			if platform != "" {
				return platform
			}
		}
		return "Unknown"
	}
	if device := header.GetDevice(); device != nil {
		return constants.MappingOS(device.GetOs())
	}
	return "Unknown"
}

// GetPlatformOS 获取操作系统/平台（兼容移动端和Web端，导出版本）
func GetPlatformOS(header *vai.RequestHeader) string {
	return getPlatformOS(header)
}

// getLanguage 获取语言（兼容移动端和Web端）
func getLanguage(header *vai.RequestHeader) vai.Language {
	if header == nil {
		return vai.Language_ENGLISH
	}
	if isWebClient(header) {
		if browserInfo := header.GetBrowserInfo(); browserInfo != nil {
			return browserInfo.GetLanguage()
		}
		return vai.Language_ENGLISH
	}
	if device := header.GetDevice(); device != nil {
		return device.GetLanguage()
	}
	return vai.Language_ENGLISH
}

// GetLanguage 获取语言（兼容移动端和Web端，导出版本）
func GetLanguage(header *vai.RequestHeader) vai.Language {
	return getLanguage(header)
}

// getPackageName 获取包名/项目ID（兼容移动端和Web端）
func getPackageName(header *vai.RequestHeader) string {
	if header == nil {
		return constants.ProjectIdVisionAI
	}

	// 优先从 App 获取（移动端）
	if app := header.GetApp(); app != nil && app.GetPackageName() != "" {
		return app.GetPackageName()
	}

	// 从 WebClient 获取（Web端）
	if webClient := header.GetWebClient(); webClient != nil && webClient.GetPackageName() != "" {
		return webClient.GetPackageName()
	}

	// 如果都没有，返回默认值
	return constants.ProjectIdVisionAI
}

// GetPackageName 获取包名/项目ID（兼容移动端和Web端，导出版本）
func GetPackageName(header *vai.RequestHeader) string {
	return getPackageName(header)
}

func buildDeviceCtx(ctx context.Context, header *vai.RequestHeader) context.Context {
	var appStore, ip string

	// 使用辅助函数获取平台相关信息
	osName := getPlatformOS(header)
	appVersion := getAppVersion(header)
	lang := constants.LanguageMap(getLanguage(header))

	if isWebClient(header) {
		// Web 端
		appStore = "WEB"
		if browserInfo := header.GetBrowserInfo(); browserInfo != nil {
			ip = browserInfo.GetIp()
		}
		if ip == "" {
			ip = getPeer(ctx)
		}
	} else {
		// 移动端
		appStore = header.GetAppStore().String()
		ip = getPeer(ctx)
	}

	newCtx := context.WithValue(ctx, constants.CtxAppVersiopn, appVersion)
	newCtx = context.WithValue(newCtx, constants.CtxOSName, osName)
	newCtx = context.WithValue(newCtx, constants.CtxAppStore, appStore)
	newCtx = context.WithValue(newCtx, constants.CtxLang, lang)
	newCtx = context.WithValue(newCtx, constants.CtxIP, ip)
	return newCtx
}

func buildTraceIDCtx(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	var traceID string
	if ok {
		traceIDList := md.Get(constants.CtxTraceID)
		if len(traceIDList) > 0 {
			traceID = traceIDList[0]
		}
	}
	if traceID == "" {
		traceID = xid.New().String()
	}
	newMD := metadata.Join(md, metadata.Pairs("TraceID", traceID))
	newCtx := metadata.NewIncomingContext(ctx, newMD)

	return newCtx
}

func checkUserIDMapping(ctx context.Context, userID string) string {
	rdb := db.GetRedis()
	projectID := common.GetProjectID(ctx)
	userMapping, _ := rdb.HGet(constants.RedisKeyUserIDMapping+projectID, userID).Result()
	if userMapping != "" {
		zlog.LogWithContext(ctx).Info("CheckUserIDMapping UserID Mapping Found", zap.String("UserID", userID), zap.String("MappingUserID", userMapping))
		return userMapping
	}
	return userID
}

func ctxSetUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, constants.CtxUserID, userID)
}

func validateChatMessageSendRequest(ctx context.Context, msg *vai.ChatMessageSendRequest) error {
	chatID := msg.GetMessage().GetChatId()
	userID := msg.GetRequestHeader().GetUserId()

	if msg.GetRequestHeader() == nil || msg.GetRequestHeader().GetReqId() == "" || userID == "" ||
		msg.GetMessage() == nil || msg.GetMessage().GetChatId() == "" {
		return constants.ERR_INVALID_PARAM
	}

	db := db.GetDB()
	var chat model.Chat

	err := db.Model(&model.Chat{}).Where("chat_id = ?", chatID).First(&chat).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		zlog.Logger.Error("Find Chat Error", zap.String("chat_id", chatID))
		return constants.ERR_INVALID_REQUEST
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if chat.UserID != userID {
		zlog.Logger.Error("Chat UserID Not Matched", zap.String("chat_id", chatID), zap.String("user_id", userID))
		return constants.ERR_INVALID_PARAM
	}
	return nil
}

func chatMessageSendRequestPostProcessor(msg *vai.ChatMessageSendRequest, ctx context.Context) {
	header := msg.GetRequestHeader()
	if header == nil {
		return
	}

	// 使用辅助函数获取操作系统名称
	osName := getPlatformOS(header)
	counter := &prometheus.Counter{Label: []string{osName}}
	go counter.Inc()

	model := msg.GetMessage().GetModelId()
	inToken, outToken := utils.GetInOutTokenCount(ctx)
	if inToken > 0 && outToken > 0 {
		inputTokenCounter := &prometheus.UserTokenCounter{
			UserID:    header.GetUserId(),
			Token:     int32(inToken),
			TokenType: "input",
			ModelID:   model,
		}
		outputTokenCounter := &prometheus.UserTokenCounter{
			UserID:    header.GetUserId(),
			Token:     int32(outToken),
			TokenType: "output",
			ModelID:   model,
		}
		go inputTokenCounter.Inc()
		go outputTokenCounter.Inc()
	}
}

func ChatStreamResponse(s grpc.ServerStream, msg *vai.ChatMessageStreamResponse) {
	if err := s.SendMsg(msg); err != nil {
		zlog.Logger.Error("send Stream message failed", zap.Any("err", err))
	}
}

func getPeer(ctx context.Context) string {
	// 1. 先尝试从 peer 获取 IP
	if p, ok := peer.FromContext(ctx); ok {
		ipStr := p.Addr.String()
		if tcpAddr, ok := p.Addr.(*net.TCPAddr); ok {
			if ipv4 := tcpAddr.IP.To4(); ipv4 != nil {
				ipStr = ipv4.String()
			}
		}

		// 如果不是回环地址，直接返回
		if ipStr != "127.0.0.1" && ipStr != "::1" {
			return ipStr
		}
		zlog.LogWithContext(ctx).Info("Got peer IP is localhost, trying headers", zap.String("peer_ip", ipStr))
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		headers := []string{"x-forwarded-for", "x-real-ip", "x-original-forwarded-for"}
		for _, header := range headers {
			if ips := md.Get(header); len(ips) > 0 {
				if header == "x-forwarded-for" || header == "x-original-forwarded-for" {
					if firstIP := strings.TrimSpace(strings.Split(ips[0], ",")[0]); firstIP != "" {
						if firstIP != "127.0.0.1" && firstIP != "::1" {
							return firstIP
						}
						zlog.LogWithContext(ctx).Info("Got localhost IP from header",
							zap.String("header", header),
							zap.String("ip", firstIP))
					}
				} else if ips[0] != "127.0.0.1" && ips[0] != "::1" {
					return ips[0]
				} else {
					zlog.LogWithContext(ctx).Info("Got localhost IP from header",
						zap.String("header", header),
						zap.String("ip", ips[0]))
				}
			}
		}
	}

	zlog.LogWithContext(ctx).Warn("Failed to get valid IP, all attempts returned localhost or failed")

	return ""
}

// isAlphanumeric 检查字符串是否只包含字母和数字
func isAlphanumeric(s string) bool {
	re := regexp.MustCompile("^[a-zA-Z0-9]+$")
	return re.MatchString(s)
}

// extractResponseHeader 使用反射从消息中提取 ResponseHeader
func (m *StatusCodeMapper) extractResponseHeader(msg proto.Message) *vai.ResponseHeader {
	val := reflect.ValueOf(msg)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	// 检查是否是结构体
	if val.Kind() != reflect.Struct {
		return nil
	}

	// 查找 ResponseHeader 字段
	field := val.FieldByName("ResponseHeader")
	if !field.IsValid() {
		return nil
	}

	// 检查字段类型和值
	if field.Type().String() == "*va_interface.ResponseHeader" {
		if !field.IsNil() {
			if header, ok := field.Interface().(*vai.ResponseHeader); ok {
				return header
			}
		}
	}

	return nil
}

// checkReqHeader 检查请求头中的用户信息
func checkReqHeader(ctx context.Context, header *vai.RequestHeader) error {
	if header == nil {
		return constants.ERR_INVALID_PARAM
	}
	userID := header.GetUserId()
	projectID := common.GetProjectID(ctx)
	db := db.GetDB()
	udao := dao.NewUserDao(db)
	var user *model.UserRecord
	var err error

	if userID == "" {
		return constants.ERR_INVALID_USER_ID
	}

	user, err = udao.GetUserByIdOrPhone(projectID, userID)
	if err != nil {
		return constants.ERR_USER_INVALID
	}
	if user == nil {
		return constants.ERR_USER_INVALID
	}

	if user.UserType == uint(vai.UserType_USER_TYPE_TEMP) {
		return nil
	}

	if user.Status == 1 {
		return constants.ERR_USER_BLOCKED
	}

	if errors.Is(err, redis.Nil) {
		return constants.ERR_TOKEN_EXPIRED
	}

	return nil
}
