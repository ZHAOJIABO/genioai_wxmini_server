package consumer

// Consumer 接口定义了从外部数据源消费事件的规范
// 任何实现了此接口的结构体，都可以作为一个事件源被注册到系统中
type Consumer interface {
	// Start 启动消费者，开始监听事件源
	// 它应该在一个非阻塞的 goroutine 中运行
	Start()

	// Stop 优雅地停止消费者
	Stop()

	// SourceName 返回消费者的名称，用于日志和管理
	SourceName() string
}

// Registry 消费者注册表
type Registry struct {
	consumers []Consumer
}

// NewRegistry 创建一个新的消费者注册表
func NewRegistry() *Registry {
	return &Registry{
		consumers: make([]Consumer, 0),
	}
}

// Register 注册一个或多个消费者
func (r *Registry) Register(consumers ...Consumer) {
	r.consumers = append(r.consumers, consumers...)
}

// StartAll 启动所有已注册的消费者
func (r *Registry) StartAll() {
	for _, c := range r.consumers {
		c.Start()
	}
}

// StopAll 停止所有已注册的消费者
func (r *Registry) StopAll() {
	for _, c := range r.consumers {
		c.Stop()
	}
}
