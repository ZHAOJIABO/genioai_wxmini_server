package picture_generate

import "sync"

var (
	executors []TaskExecutor
	mu        sync.RWMutex
)

// RegisterExecutor 注册一个执行器
// 这个函数应该在服务初始化时被调用
func RegisterExecutor(e TaskExecutor) {
	mu.Lock()
	defer mu.Unlock()
	executors = append(executors, e)
}

// GetExecutor 根据参数选择合适的执行器
func GetExecutor(params map[string]string) TaskExecutor {
	mu.RLock()
	defer mu.RUnlock()
	for _, e := range executors {
		if e.Match(params) {
			return e
		}
	}
	return nil
}
