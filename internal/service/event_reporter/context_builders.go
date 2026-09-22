package event_reporter

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/types/known/structpb"

	eventsv1 "va_visionai_server/pkg/event_sink/v1"
)

// ============ ServerContext 构建器 ============

// ServerContextBuilder 服务端上下文构建器
type ServerContextBuilder struct {
	ctx *eventsv1.ServerContext
}

// NewServerContext 创建服务端上下文构建器
func NewServerContext() *ServerContextBuilder {
	return &ServerContextBuilder{
		ctx: &eventsv1.ServerContext{},
	}
}

// IP 设置客户端IP
func (b *ServerContextBuilder) IP(ip string) *ServerContextBuilder {
	b.ctx.Ip = ip
	return b
}

// UserAgent 设置 User-Agent
func (b *ServerContextBuilder) UserAgent(ua string) *ServerContextBuilder {
	b.ctx.UserAgent = ua
	return b
}

// Geo 设置地理位置
func (b *ServerContextBuilder) Geo(country, region, city string) *ServerContextBuilder {
	b.ctx.GeoCountry = country
	b.ctx.GeoRegion = region
	b.ctx.GeoCity = city
	return b
}

// ServerNode 设置服务节点标识
func (b *ServerContextBuilder) ServerNode(node string) *ServerContextBuilder {
	b.ctx.ServerNode = node
	return b
}

// ProcessLatency 设置处理延迟
func (b *ServerContextBuilder) ProcessLatency(ms int64) *ServerContextBuilder {
	b.ctx.ProcessLatencyMs = ms
	return b
}

// ProcessLatencyDuration 设置处理延迟（通过 time.Duration）
func (b *ServerContextBuilder) ProcessLatencyDuration(d time.Duration) *ServerContextBuilder {
	b.ctx.ProcessLatencyMs = d.Milliseconds()
	return b
}

// FromHTTPRequest 从 HTTP 请求中提取（IP、User-Agent）
func (b *ServerContextBuilder) FromHTTPRequest(r *http.Request) *ServerContextBuilder {
	// 提取 IP，优先使用代理头
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		// X-Forwarded-For 可能包含多个 IP，取第一个
		ips := strings.Split(ip, ",")
		ip = strings.TrimSpace(ips[0])
	}
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		// 从 RemoteAddr 提取
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			ip = host
		} else {
			ip = r.RemoteAddr
		}
	}
	b.ctx.Ip = ip

	// 提取 User-Agent
	b.ctx.UserAgent = r.UserAgent()

	return b
}

// FromGRPCPeer 从 gRPC peer 中提取 IP
func (b *ServerContextBuilder) FromGRPCPeer(ctx context.Context) *ServerContextBuilder {
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		addr := p.Addr.String()
		host, _, err := net.SplitHostPort(addr)
		if err == nil {
			b.ctx.Ip = host
		} else {
			b.ctx.Ip = addr
		}
	}
	return b
}

// Build 构建
func (b *ServerContextBuilder) Build() *eventsv1.ServerContext {
	return b.ctx
}

// ============ ActionContext 构建器 ============

// ActionContextBuilder 动作上下文构建器
type ActionContextBuilder struct {
	ctx *eventsv1.ActionContext
}

// NewActionContext 创建动作上下文构建器
func NewActionContext() *ActionContextBuilder {
	return &ActionContextBuilder{
		ctx: &eventsv1.ActionContext{},
	}
}

// Domain 设置动作领域（internal/external）
func (b *ActionContextBuilder) Domain(domain string) *ActionContextBuilder {
	b.ctx.Domain = domain
	return b
}

// Type 设置动作类型（function/api/database/cache/queue/rpc）
func (b *ActionContextBuilder) Type(actionType string) *ActionContextBuilder {
	b.ctx.Type = actionType
	return b
}

// Operation 设置动作操作（execute/call/query/command/publish/consume）
func (b *ActionContextBuilder) Operation(op string) *ActionContextBuilder {
	b.ctx.Operation = op
	return b
}

// Target 设置动作目标
func (b *ActionContextBuilder) Target(target string) *ActionContextBuilder {
	b.ctx.Target = target
	return b
}

// Success 设置成功状态
func (b *ActionContextBuilder) Success(duration time.Duration) *ActionContextBuilder {
	b.ctx.Success = true
	b.ctx.DurationMs = duration.Milliseconds()
	return b
}

// Failed 设置失败状态
func (b *ActionContextBuilder) Failed(duration time.Duration, err error) *ActionContextBuilder {
	b.ctx.Success = false
	b.ctx.DurationMs = duration.Milliseconds()
	if err != nil {
		b.ctx.Error = err.Error()
	}
	return b
}

// Metadata 设置元数据
func (b *ActionContextBuilder) Metadata(m map[string]any) *ActionContextBuilder {
	if s, err := structpb.NewStruct(m); err == nil {
		b.ctx.Metadata = s
	}
	return b
}

// Build 构建
func (b *ActionContextBuilder) Build() *eventsv1.ActionContext {
	return b.ctx
}

// ============ 预定义 ActionContext 工厂函数 ============

// APICallAction 创建 API 调用类型的 ActionContext
func APICallAction(method, endpoint string, statusCode int, duration time.Duration) *eventsv1.ActionContext {
	metadata, _ := structpb.NewStruct(map[string]any{
		"method":      method,
		"status_code": statusCode,
	})
	return &eventsv1.ActionContext{
		Domain:     DomainExternal,
		Type:       TypeAPI,
		Operation:  OpCall,
		Target:     endpoint,
		Success:    statusCode >= 200 && statusCode < 400,
		DurationMs: duration.Milliseconds(),
		Metadata:   metadata,
	}
}

// DatabaseAction 创建数据库操作类型的 ActionContext
func DatabaseAction(queryType, table string, rowsAffected int, duration time.Duration) *eventsv1.ActionContext {
	metadata, _ := structpb.NewStruct(map[string]any{
		"query_type":    queryType,
		"rows_affected": rowsAffected,
	})
	return &eventsv1.ActionContext{
		Domain:     DomainExternal,
		Type:       TypeDatabase,
		Operation:  OpQuery,
		Target:     table,
		Success:    true,
		DurationMs: duration.Milliseconds(),
		Metadata:   metadata,
	}
}

// FunctionAction 创建函数执行类型的 ActionContext
func FunctionAction(funcName string, success bool, duration time.Duration) *eventsv1.ActionContext {
	return &eventsv1.ActionContext{
		Domain:     DomainInternal,
		Type:       TypeFunction,
		Operation:  OpExecute,
		Target:     funcName,
		Success:    success,
		DurationMs: duration.Milliseconds(),
	}
}

// CacheAction 创建缓存操作类型的 ActionContext
func CacheAction(operation, key string, hit bool, duration time.Duration) *eventsv1.ActionContext {
	metadata, _ := structpb.NewStruct(map[string]any{
		"hit": hit,
	})
	return &eventsv1.ActionContext{
		Domain:     DomainExternal,
		Type:       TypeCache,
		Operation:  operation,
		Target:     key,
		Success:    true,
		DurationMs: duration.Milliseconds(),
		Metadata:   metadata,
	}
}

// RPCAction 创建 RPC 调用类型的 ActionContext
func RPCAction(service, method string, success bool, duration time.Duration) *eventsv1.ActionContext {
	return &eventsv1.ActionContext{
		Domain:     DomainExternal,
		Type:       TypeRPC,
		Operation:  OpCall,
		Target:     service + "/" + method,
		Success:    success,
		DurationMs: duration.Milliseconds(),
	}
}

// ============ ClientContext 构建器 ============

// ClientContextBuilder 客户端上下文构建器
type ClientContextBuilder struct {
	ctx *eventsv1.ClientContext
}

// NewClientContext 创建客户端上下文构建器
func NewClientContext() *ClientContextBuilder {
	return &ClientContextBuilder{
		ctx: &eventsv1.ClientContext{},
	}
}

// DeviceID 设置设备ID
func (b *ClientContextBuilder) DeviceID(id string) *ClientContextBuilder {
	b.ctx.DeviceId = id
	return b
}

// OS 设置操作系统信息
func (b *ClientContextBuilder) OS(name, version string) *ClientContextBuilder {
	b.ctx.OsName = name
	b.ctx.OsVersion = version
	return b
}

// Device 设置设备信息
func (b *ClientContextBuilder) Device(brand, model string) *ClientContextBuilder {
	b.ctx.DeviceBrand = brand
	b.ctx.DeviceModel = model
	return b
}

// App 设置应用信息
func (b *ClientContextBuilder) App(id, version string) *ClientContextBuilder {
	b.ctx.AppId = id
	b.ctx.AppVersion = version
	return b
}

// Network 设置网络类型
func (b *ClientContextBuilder) Network(networkType string) *ClientContextBuilder {
	b.ctx.NetworkType = networkType
	return b
}

// Carrier 设置运营商
func (b *ClientContextBuilder) Carrier(carrier string) *ClientContextBuilder {
	b.ctx.Carrier = carrier
	return b
}

// Locale 设置语言区域
func (b *ClientContextBuilder) Locale(locale, timezone string) *ClientContextBuilder {
	b.ctx.Locale = locale
	b.ctx.TimeZone = timezone
	return b
}

// CountryCode 设置国家代码
func (b *ClientContextBuilder) CountryCode(code string) *ClientContextBuilder {
	b.ctx.CountryCode = code
	return b
}

// Screen 设置屏幕信息
func (b *ClientContextBuilder) Screen(resolution string, dpi int) *ClientContextBuilder {
	b.ctx.ScreenResolution = resolution
	b.ctx.ScreenDpi = int32(dpi)
	return b
}

// Build 构建
func (b *ClientContextBuilder) Build() *eventsv1.ClientContext {
	return b.ctx
}

// ============ Payload 构建器 ============

// PayloadBuilder Payload 构建器
type PayloadBuilder struct {
	data map[string]any
}

// NewPayload 创建 Payload 构建器
func NewPayload() *PayloadBuilder {
	return &PayloadBuilder{
		data: make(map[string]any),
	}
}

// Set 设置键值对
func (b *PayloadBuilder) Set(key string, value any) *PayloadBuilder {
	b.data[key] = value
	return b
}

// Merge 合并 map
func (b *PayloadBuilder) Merge(m map[string]any) *PayloadBuilder {
	for k, v := range m {
		b.data[k] = v
	}
	return b
}

// Build 构建为 map
func (b *PayloadBuilder) Build() map[string]any {
	return b.data
}

// JSON 构建为 JSON bytes
func (b *PayloadBuilder) JSON() []byte {
	data, _ := structpb.NewStruct(b.data)
	if data == nil {
		return nil
	}
	// 使用 json 序列化
	result, _ := data.MarshalJSON()
	return result
}
