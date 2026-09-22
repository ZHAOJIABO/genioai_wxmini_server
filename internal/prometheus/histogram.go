package prometheus

// RequestDurationMetric 记录请求耗时指标
type RequestDurationMetric struct {
	Method         string  // gRPC 方法名，如 /VisionAiService/Chat
	OS             string  // 操作系统，如 ios, android, web
	Country        string  // 国家代码，如 US, CN, SG
	DurationE2E    float64 // 端到端耗时（毫秒），从客户端发起到服务端响应完成
	DurationServer float64 // 服务端处理耗时（毫秒），服务端收到请求到返回响应
}

// Observe 记录耗时观测值到 Prometheus Histogram
func (m *RequestDurationMetric) Observe() {
	if m.Method == "" || m.OS == "" {
		return
	}

	// 默认country为unknown
	country := m.Country
	if country == "" {
		country = "unknown"
	}

	// 记录端到端耗时（如果客户端提供了时间戳）
	if m.DurationE2E > 0 {
		requestDurationE2EHistogram.WithLabelValues(m.Method, m.OS, country).Observe(m.DurationE2E)
	}

	// 记录服务端处理耗时
	if m.DurationServer > 0 {
		requestDurationServerHistogram.WithLabelValues(m.Method, m.OS, country).Observe(m.DurationServer)
	}
}
