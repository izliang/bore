// Package cache 提供通用缓存抽象。
//
// 分层设计：
//   - Cacher 接口 — 底层缓存操作（与具体实现解耦）
//   - Cache[V] 泛型包装 — 编译期类型安全的缓存 API
//   - cache/provider — 基于 go-cache 的 Memory/Redis/None 实现
//
// 典型用法：
//
//	// 1. 创建底层实现
//	c := provider.NewMemory(5*time.Minute, 10*time.Minute)
//
//	// 2. 创建类型安全缓存
//	userCache := cache.New[User](c, "user:")
//
//	// 3. 使用
//	userCache.Set(ctx, "123", &User{Name: "张三"}, time.Minute)
//	user, _ := userCache.Get(ctx, "123")
package cache

import (
	"context"
	"time"
)

// LoaderFunc 缓存未命中时的单键加载回调。
//
// 用于 GetSet 的按需加载场景。
// key 是去掉 Cache[V] prefix 后的业务键。
type LoaderFunc[V any] func(ctx context.Context, key string) (*V, error)

// Cacher 底层缓存接口。
//
// 方法签名与 gsr.Cacher 兼容但独立定义，
// 避免对外暴露第三方依赖。
//
// 任何实现了此接口的类型都可以作为 Cache[V] 的后端。
// 内置实现见 cache/provider 包。
type Cacher interface {
	// Exists 检查 key 是否存在。
	Exists(ctx context.Context, key string) bool

	// Get 获取缓存值并写入 obj。obj 必须是非 nil 指针。
	// key 不存在时返回 error。
	Get(ctx context.Context, key string, obj any) error

	// Set 存储缓存值。ttl <= 0 表示永不过期。
	Set(ctx context.Context, key string, value any, ttl time.Duration) error

	// GetSet 缓存穿透保护：先 Get，未命中则调用 fn 加载并回填。
	// fn 接收 key 和 obj（与 Get 中的 obj 参数语义相同）。
	GetSet(ctx context.Context, key string, ttl time.Duration, obj any, fn func(key string, obj any) error) error

	// Del 删除缓存 key。
	Del(ctx context.Context, key string) error
}
