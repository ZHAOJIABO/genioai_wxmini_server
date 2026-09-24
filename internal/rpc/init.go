package rpc

import (
	"context"
	"errors"
	"runtime/debug"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"

	"va_visionai_server/internal/common"
	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/geoip"
	"va_visionai_server/internal/prometheus"
	vai "va_visionai_server/internal/va_interface"
	"va_visionai_server/internal/zlog"
)

// UnaryInterceptor对象池，用于复用拦截器实例，减少内存分配
var unaryInterceptorPool = sync.Pool{
	New: func() interface{} {
		return &UnaryInterceptor{
			timing:     NewTimingManager(),
			validator:  NewValidationHandler(),
			ctxBuilder: NewContextBuilder(),
			processor:  NewRequestProcessor(),
			collector:  NewMetricsCollector(),
		}
	},
}

type TimingMetrics struct {
	TotalDuration      time.Duration
	PreprocessDuration time.Duration
	BusinessDuration   time.Duration
	StartTime          time.Time
}

type ValidationResult struct {
	RequestHeader *vai.RequestHeader
	MappedUserID  string
}

type TimingManager struct {
	mu                sync.RWMutex // 保护并发访问
	requestStartTime  time.Time
	preprocessTime    time.Time
	businessStartTime time.Time
	businessEndTime   time.Time
}

func NewTimingManager() *TimingManager {
	return &TimingManager{
		requestStartTime: time.Now(),
	}
}

func (tm *TimingManager) StartPreprocess() {
	tm.mu.Lock()
	tm.preprocessTime = time.Now()
	tm.mu.Unlock()
}

func (tm *TimingManager) StartBusiness() {
	tm.mu.Lock()
	tm.businessStartTime = time.Now()
	tm.mu.Unlock()
}

func (tm *TimingManager) EndBusiness() {
	tm.mu.Lock()
	tm.businessEndTime = time.Now()
	tm.mu.Unlock()
}

func (tm *TimingManager) GetMetrics() TimingMetrics {
	tm.mu.RLock()
	requestStartTime := tm.requestStartTime
	preprocessTime := tm.preprocessTime
	businessStartTime := tm.businessStartTime
	businessEndTime := tm.businessEndTime
	tm.mu.RUnlock()

	totalDuration := time.Since(requestStartTime)
	// 预处理耗时应表示从开始预处理到正式进入业务逻辑之间的时长
	// 原计算为 preprocessTime.Sub(requestStartTime)，仅表示请求开始到预处理打点的极短间隔，意义不大
	// 修正为 businessStartTime.Sub(preprocessTime)；若打点缺失或未进入业务阶段则为 0
	var preprocessDuration time.Duration
	if !preprocessTime.IsZero() && !businessStartTime.IsZero() {
		preprocessDuration = businessStartTime.Sub(preprocessTime)
	}
	var businessDuration time.Duration
	if !businessStartTime.IsZero() && !businessEndTime.IsZero() {
		businessDuration = businessEndTime.Sub(businessStartTime)
	}

	return TimingMetrics{
		TotalDuration:      totalDuration,
		PreprocessDuration: preprocessDuration,
		BusinessDuration:   businessDuration,
		StartTime:          requestStartTime,
	}
}

type ValidationHandler struct{}

func NewValidationHandler() *ValidationHandler {
	return &ValidationHandler{}
}

func (vh *ValidationHandler) ValidateAndExtract(ctx context.Context, req interface{}) (*ValidationResult, error) {
	result := &ValidationResult{}

	reqHeader := extractReqHeader(req)
	if reqHeader == nil {
		// 免认证接口允许 request_header 为空
		if isNoAuthRequest(req) {
			return result, nil
		}
		zlog.LogWithContext(ctx).Error("request header is nil")
		return nil, errors.New("invalid request header")
	}
	result.RequestHeader = reqHeader

	requestCtx := buildCtx(ctx, reqHeader)
	if err := paramsCheck(requestCtx, req, reqHeader); err != nil {
		return nil, err
	}

	result.MappedUserID = checkUserIDMapping(requestCtx, reqHeader.GetUserId())

	return result, nil
}

// isNoAuthRequest 判断请求是否为免认证接口
func isNoAuthRequest(req interface{}) bool {
	switch req.(type) {
	case *vai.AddUserCreditsRequest, *vai.GetUserTaskStatsRequest:
		return true
	}
	return false
}

type ContextBuilder struct{}

func NewContextBuilder() *ContextBuilder {
	return &ContextBuilder{}
}

func (cb *ContextBuilder) BuildContext(ctx context.Context, reqHeader *vai.RequestHeader, userID string) context.Context {
	newCtx := buildCtx(ctx, reqHeader)

	if reqHeader != nil && userID != "" {
		reqHeader.UserId = userID
	}

	return newCtx
}

type RequestProcessor struct{}

func NewRequestProcessor() *RequestProcessor {
	return &RequestProcessor{}
}

func (rp *RequestProcessor) Process(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	return handler(ctx, req)
}

type MetricsCollector struct{}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{}
}

func (mc *MetricsCollector) BuildLogFields(ctx context.Context, info *grpc.UnaryServerInfo, timing TimingMetrics, reqHeader *vai.RequestHeader) []zap.Field {
	clientIP := common.GetClientIP(ctx)
	countryCode := geoip.GetCountryCode(clientIP)
	fields := []zap.Field{
		zap.String("method", info.FullMethod),
		zap.String("server_start_time", timing.StartTime.Format(time.DateTime)),
		zap.Int64("server_total_duration_ms", timing.TotalDuration.Milliseconds()),
		zap.Int64("interceptor_overhead_ms", timing.PreprocessDuration.Milliseconds()),
		zap.String("country", countryCode),
	}

	if timing.BusinessDuration > 0 {
		fields = append(fields, zap.Int64("business_logic_duration_ms", timing.BusinessDuration.Milliseconds()))
	}

	if reqHeader != nil && reqHeader.GetRequestTimeMs() > 0 {
		appStartTime := time.Unix(0, reqHeader.GetRequestTimeMs()*int64(time.Millisecond))
		appDuration := time.Since(appStartTime)
		fields = append(fields,
			zap.String("app_request_time_ms", appStartTime.Format(time.DateTime)),
			zap.Int64("app_total_duration_ms", appDuration.Milliseconds()),
		)
	}

	return fields
}

func (mc *MetricsCollector) LogCompletion(ctx context.Context, fields []zap.Field) {
	zlog.LogWithContext(ctx).Info("Unary Request Completed", fields...)
}

func (mc *MetricsCollector) LogError(ctx context.Context, fields []zap.Field, err interface{}, stack []byte) {
	fields = append(fields,
		zap.Any("error", err),
		zap.ByteString("stack", stack),
	)
	zlog.LogWithContext(ctx).Error("Unary Interceptor Catch Panic", fields...)
}

type UnaryInterceptor struct {
	timing     *TimingManager
	validator  *ValidationHandler
	ctxBuilder *ContextBuilder
	processor  *RequestProcessor
	collector  *MetricsCollector
}

// Reset 重置UnaryInterceptor状态，用于对象池复用
func (ui *UnaryInterceptor) Reset() {
	// 重置TimingManager状态，使用写锁保护
	ui.timing.mu.Lock()
	ui.timing.requestStartTime = time.Now()
	ui.timing.preprocessTime = time.Time{}
	ui.timing.businessStartTime = time.Time{}
	ui.timing.businessEndTime = time.Time{}
	ui.timing.mu.Unlock()
}

// GetUnaryInterceptor 从对象池获取拦截器实例
func GetUnaryInterceptor() *UnaryInterceptor {
	interceptor := unaryInterceptorPool.Get().(*UnaryInterceptor)
	interceptor.Reset()
	return interceptor
}

// PutUnaryInterceptor 将拦截器实例放回对象池
func PutUnaryInterceptor(interceptor *UnaryInterceptor) {
	unaryInterceptorPool.Put(interceptor)
}

func NewUnaryInterceptor() *UnaryInterceptor {
	return &UnaryInterceptor{
		timing:     NewTimingManager(),
		validator:  NewValidationHandler(),
		ctxBuilder: NewContextBuilder(),
		processor:  NewRequestProcessor(),
		collector:  NewMetricsCollector(),
	}
}

func (ui *UnaryInterceptor) Process(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ui.timing.StartPreprocess()
	result, err := ui.validator.ValidateAndExtract(ctx, req)
	if err != nil {
		return common.BuildInterceptorErrorResponse("", "", err), nil
	}

	newCtx := ui.ctxBuilder.BuildContext(ctx, result.RequestHeader, result.MappedUserID)

	var resp interface{}

	defer func() {
		timing := ui.timing.GetMetrics()
		logFields := ui.collector.BuildLogFields(ctx, info, timing, result.RequestHeader)

		if r := recover(); r != nil {
			stack := debug.Stack()
			ui.collector.LogError(newCtx, logFields, r, stack)
		} else {
			ui.collector.LogCompletion(newCtx, logFields)
		}

		// 采集请求耗时指标到 Prometheus（传递context以获取IP）
		ui.collectDurationMetrics(newCtx, info, timing, result.RequestHeader)
	}()

	packageName := GetPackageName(result.RequestHeader)
	os := result.RequestHeader.GetDevice().GetOs()
	if common.IsLimited(packageName, constants.MappingOS(os)) {
		return nil, errors.New("Not Support")
	}
	ui.timing.StartBusiness()
	resp, err = ui.processor.Process(newCtx, req, info, handler)
	ui.timing.EndBusiness()

	return resp, err
}

// collectDurationMetrics 采集请求耗时指标到 Prometheus
func (ui *UnaryInterceptor) collectDurationMetrics(ctx context.Context, info *grpc.UnaryServerInfo, timing TimingMetrics, reqHeader *vai.RequestHeader) {
	if reqHeader == nil || reqHeader.GetDevice() == nil {
		return
	}

	// 获取操作系统标签
	osName := constants.MappingOS(reqHeader.GetDevice().GetOs())

	// 计算服务端处理耗时（毫秒）
	serverDuration := timing.TotalDuration.Milliseconds()

	// 计算端到端耗时（如果客户端提供了时间戳）
	var e2eDuration int64
	if reqHeader.GetRequestTimeMs() > 0 {
		clientStartTime := time.Unix(0, reqHeader.GetRequestTimeMs()*int64(time.Millisecond))
		e2eDuration = time.Since(clientStartTime).Milliseconds()
		zlog.Logger.Debug("e2eDuration", zap.Int64("e2eDuration", e2eDuration))
	}

	// 获取客户端IP并解析国家代码
	clientIP := common.GetClientIP(ctx)
	countryCode := geoip.GetCountryCode(clientIP)

	// 异步记录到 Prometheus
	durationMetric := &prometheus.RequestDurationMetric{
		Method:         info.FullMethod,
		OS:             osName,
		Country:        countryCode,
		DurationE2E:    float64(e2eDuration),
		DurationServer: float64(serverDuration),
	}
	go durationMetric.Observe()
}

func GetOpts(m *StatusCodeMapper) []grpc.ServerOption {
	opts := make([]grpc.ServerOption, 0, 4)
	maxRecvMsgSize := grpc.MaxRecvMsgSize(15 * 1024 * 1024)
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionIdle:     time.Minute * 10,
		MaxConnectionAge:      time.Minute * 60,
		MaxConnectionAgeGrace: time.Minute * 5,
		Time:                  time.Hour,
		Timeout:               time.Second * 20,
	}),
		grpc.ChainUnaryInterceptor(m.UnaryServerInterceptor(), unaryInterceptor),
		grpc.ChainStreamInterceptor(m.StreamServerInterceptor(), streamInterceptor),
		maxRecvMsgSize,
	)
	return opts
}

type WrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
	msg interface{}
}

func (w *WrappedServerStream) Context() context.Context {
	return w.ctx
}

func (w *WrappedServerStream) SendMsg(m any) error {
	return w.ServerStream.SendMsg(m)
}

func (w *WrappedServerStream) Send(m *vai.ChatMessageStreamResponse) error {
	return w.ServerStream.SendMsg(m)
}

func (w *WrappedServerStream) SetContext(ctx context.Context) {
	w.ctx = ctx
}

func (w *WrappedServerStream) SetTrailer(md metadata.MD) {
	w.ServerStream.SetTrailer(md)
}

func (w *WrappedServerStream) SendHeader(md metadata.MD) error {
	return w.ServerStream.SendHeader(md)
}

func (w *WrappedServerStream) SetHeader(md metadata.MD) error {
	return w.ServerStream.SetHeader(md)
}

func (w *WrappedServerStream) RecvMsg(m interface{}) error {
	err := w.ServerStream.RecvMsg(m)
	if err != nil {
		zlog.Logger.Error("GRPC Server Stream RecvMsg Error", zap.Error(err))
		msg := BuildChatStreamMessage(vai.StatusCode_INVALID_REQUEST, "", "", true)
		ChatStreamResponse(w.ServerStream, msg)
		return err
	}
	w.msg = m
	reqHeader := extractReqHeader(m)
	newCtx := buildCtx(w.ctx, reqHeader)
	err = paramsCheck(newCtx, m, reqHeader)
	if err != nil {
		zlog.Logger.Error("GRPC Server Stream ParamsCheck Error", zap.Error(err))
		msg := BuildChatStreamMessage(vai.StatusCode_INVALID_REQUEST, "", "", true)
		ChatStreamResponse(w.ServerStream, msg)
		return err
	}

	userID := checkUserIDMapping(newCtx, reqHeader.GetUserId())
	if reqHeader != nil {
		reqHeader.UserId = userID
	}
	w.SetContext(newCtx)
	return nil
}

func unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// 从对象池获取拦截器实例
	interceptor := GetUnaryInterceptor()
	defer PutUnaryInterceptor(interceptor) // 使用完毕后放回对象池

	return interceptor.Process(ctx, req, info, handler)
}

func streamInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	wss := &WrappedServerStream{ServerStream: ss, ctx: ss.Context()}
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			zlog.LogWithContext(wss.ctx).Error("Stream Interceptor Catch Panic",
				zap.Any("error", r),
				zap.ByteString("stack", stack))
			msg := BuildChatStreamMessage(vai.StatusCode_INVALID_REQUEST, "", "", true)
			ChatStreamResponse(wss, msg)
		}
	}()
	err := handler(srv, wss)
	if err != nil {
		zlog.LogWithContext(wss.Context()).Error("StreamInterceptor Catch Error", zap.Error(err))
	}
	postProcessor(wss.msg, wss.Context())
	return nil
}
