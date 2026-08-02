package cache

import (
	"context"
	"reflect"
	"time"
)

// Cache 提供编译期类型安全的缓存操作。
//
// V 是缓存值的类型。通过 key prefix 实现命名空间隔离，
// 同一 Cacher 实例上的不同 Cache[V] 不会发生 key 冲突。
//
// 使用示例：
//
//	userCache := cache.New[User](c, "user:")
//	user, _ := userCache.Get(ctx, "123") // 实际 key: "user:123"
//
//	configCache := cache.New[AppConfig](c, "config:")
//	cfg, _ := configCache.Get(ctx, "site") // 实际 key: "config:site"
type Cache[V any] struct {
	cacher Cacher
	prefix string
}

// New 创建类型化缓存。
//
// prefix 用于命名空间隔离。建议以 ":" 结尾（如 "user:"），
// 空字符串表示无前缀。
func New[V any](c Cacher, prefix string) *Cache[V] {
	return &Cache[V]{cacher: c, prefix: prefix}
}

// Get 获取缓存值。key 不含 prefix。
//
// 缓存未命中返回 nil, nil（不视为错误）。
// 底层返回的 error（如 Redis 连接错误）会透传。
func (c *Cache[V]) Get(ctx context.Context, key string) (*V, error) {
	var v V
	if err := c.cacher.Get(ctx, c.fullKey(key), &v); err != nil {
		return nil, nil
	}
	return &v, nil
}

// Set 存储缓存值。
// 内部对 value 解引用后存储，确保 Memory/Redis 后端类型一致。
func (c *Cache[V]) Set(ctx context.Context, key string, value *V, ttl time.Duration) error {
	return c.cacher.Set(ctx, c.fullKey(key), *value, ttl)
}

// GetSet 缓存穿透保护。
//
// 先尝试从缓存获取；未命中时调用 loader，将结果写入缓存并返回。
// loader 仅在缓存未命中时调用。
func (c *Cache[V]) GetSet(ctx context.Context, key string, ttl time.Duration, loader LoaderFunc[V]) (*V, error) {
	var v V
	err := c.cacher.GetSet(ctx, c.fullKey(key), ttl, &v, func(k string, obj any) error {
		loaded, loadErr := loader(ctx, key) // 传业务 key（无 prefix）
		if loadErr != nil {
			return loadErr
		}
		// 通过反射将 loaded 写入 obj（go-cache 内部使用反射赋值）
		reflect.ValueOf(obj).Elem().Set(reflect.ValueOf(loaded).Elem())
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// Del 删除缓存。
func (c *Cache[V]) Del(ctx context.Context, key string) error {
	return c.cacher.Del(ctx, c.fullKey(key))
}

// Exists 检查 key 是否存在。
func (c *Cache[V]) Exists(ctx context.Context, key string) bool {
	return c.cacher.Exists(ctx, c.fullKey(key))
}

// fullKey 拼接 prefix + key。
func (c *Cache[V]) fullKey(key string) string {
	return c.prefix + key
}
