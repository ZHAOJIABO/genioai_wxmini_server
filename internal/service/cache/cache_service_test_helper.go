package cache

// NewCacheServiceForTest 创建带有自定义 CacheUtil 的 CacheService（仅用于测试）
// 注意：此函数仅供测试使用，请勿在生产代码中调用
func NewCacheServiceForTest(util CacheUtil) *CacheService {
	return &CacheService{
		cache: util,
	}
}
