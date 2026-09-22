package prometheus

import (
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"va_visionai_server/conf"
)

const (
	ChatReceive  = "chat_request"
	TokenReceive = "token_receive"

	// DefaultNamespace 是 Prometheus metrics 的默认命名空间
	DefaultNamespace = "aix_visionai_server"
)

var (
	chatCounter                    *prometheus.CounterVec
	tokenCounter                   *prometheus.CounterVec
	requestDurationE2EHistogram    *prometheus.HistogramVec
	requestDurationServerHistogram *prometheus.HistogramVec
)

// initMetrics 使用指定的 namespace 初始化所有 metrics
func initMetrics(namespace string) {
	if namespace == "" {
		namespace = DefaultNamespace
	}

	chatCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      ChatReceive,
		Help:      "request counter",
	}, []string{"os"})

	tokenCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      TokenReceive,
		Help:      "request counter",
	}, []string{"user_id", "model", "token_type"})

	// requestDurationE2EHistogram 记录端到端请求耗时（从客户端发起到服务端响应完成）
	requestDurationE2EHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "request_duration_e2e_milliseconds",
			Help:      "End-to-end request duration from client to server response in milliseconds",
			Buckets:   []float64{10, 50, 100, 200, 500, 1000, 2000, 5000, 10000},
		},
		[]string{"method", "os", "country"},
	)

	// requestDurationServerHistogram 记录服务端处理耗时（服务端收到请求到返回响应）
	requestDurationServerHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "request_duration_server_milliseconds",
			Help:      "Server-side request processing duration in milliseconds",
			Buckets:   []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2000},
		},
		[]string{"method", "os", "country"},
	)
}

// initDefaultLabels 初始化默认 label 组合，避免 metrics endpoint 返回空数据
func initDefaultLabels() {
	// 常见的 gRPC 方法
	commonMethods := []string{
		"/VisionAiService/Chat",
		"/PromptService/EnhancePrompt",
		"/PictureForgeService/GeneratePicture",
		"/UserService/GetUserInfo",
		"/ChatService/CreateChat",
	}

	// 常见的操作系统
	commonOS := []string{"ios", "android", "web", "unknown"}

	// 常见的国家（只初始化unknown，其他国家在实际请求时动态创建）
	commonCountries := []string{"unknown"}

	// 为 chatCounter 初始化
	for _, os := range commonOS {
		chatCounter.WithLabelValues(os).Add(0)
	}

	// 为 tokenCounter 初始化一个示例（避免空数据）
	tokenCounter.WithLabelValues("_init", "UNKNOWN", "input").Add(0)

	// 为 histogram 初始化常见的 label 组合
	for _, method := range commonMethods {
		for _, os := range commonOS {
			for _, country := range commonCountries {
				requestDurationE2EHistogram.WithLabelValues(method, os, country)
				requestDurationServerHistogram.WithLabelValues(method, os, country)
			}
		}
	}
}

func RegisterPrometheus(addr, namespace string) (*http.Server, error) {
	// 初始化 metrics（必须在注册之前）
	initMetrics(namespace)

	r := prometheus.NewRegistry()
	r.MustRegister(
		chatCounter,
		tokenCounter,
		requestDurationE2EHistogram,
		requestDurationServerHistogram,
	)

	// 初始化默认 label 组合，确保 metrics 立即可见
	initDefaultLabels()

	// register slot reconciler metrics (temporarily disabled per refactor plan)
	// RegisterSlotReconcilerMetrics(r)
	// register queue slot reconciler metrics (temporarily disabled per refactor plan)
	// RegisterQueueSlotReconcilerMetrics(r)
	httpHandle := promhttp.HandlerFor(r, promhttp.HandlerOpts{})

	route := http.NewServeMux()

	route.Handle(conf.GlobalConfig.Metrics.Path, httpHandle)
	srv := &http.Server{
		Addr:    addr,
		Handler: route,
	}
	go func() {
		log.Println("prometheus start as", addr)
		if err := srv.ListenAndServe(); err != nil {
			if err == http.ErrServerClosed {
				log.Println("Shutting down prometheus serve")
				return
			}
			log.Fatalf("prometheus server got err %v", err)
		}
	}()
	return srv, nil
}
