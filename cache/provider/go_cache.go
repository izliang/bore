// Package provider 提供 cache.Cacher 接口的具体实现。
//
// 当前支持基于 go-cache 的三种后端：
//   - Memory — 进程内内存缓存
//   - Redis  — 基于 Redis 的分布式缓存
//   - None   — 空缓存（测试/禁用缓存场景）
//
// 所有实现均满足 cache.Cacher 接口，可无缝切换。
//
// 内部通过薄适配器桥接 gsr.CacheCallback 类型差异，
// 对调用方完全透明。
package provider

import (
	"context"
	"time"

	"github.com/izliang/bore/cache"
	gocache "github.com/muleiwu/go-cache"
	"github.com/muleiwu/gsr"
	"github.com/redis/go-redis/v9"
)

// ============================================================
// Memory 适配器
// ============================================================

// memoryAdapter 包装 *gocache.Memory，桥接 gsr.CacheCallback 类型差异。
type memoryAdapter struct {
	*gocache.Memory
}

// GetSet 重写以适配 cache.Cacher 的回调签名。
func (a *memoryAdapter) GetSet(ctx context.Context, key string, ttl time.Duration, obj any, fn func(string, any) error) error {
	return a.Memory.GetSet(ctx, key, ttl, obj, gsr.CacheCallback(fn))
}

// NewMemory 创建进程内内存缓存。
//
// 底层基于 patrickmn/go-cache (v2)，适合单机部署场景。
//
// 参数：
//
//	defaultExpiration — 条目默认过期时间
//	cleanupInterval  — 过期条目清理间隔
func NewMemory(defaultExpiration, cleanupInterval time.Duration) cache.Cacher {
	return &memoryAdapter{Memory: gocache.NewMemory(defaultExpiration, cleanupInterval)}
}

// ============================================================
// Redis 适配器
// ============================================================

// redisAdapter 包装 *gocache.Redis，桥接 gsr.CacheCallback 类型差异。
type redisAdapter struct {
	*gocache.Redis
}

// GetSet 重写以适配 cache.Cacher 的回调签名。
func (a *redisAdapter) GetSet(ctx context.Context, key string, ttl time.Duration, obj any, fn func(string, any) error) error {
	return a.Redis.GetSet(ctx, key, ttl, obj, gsr.CacheCallback(fn))
}

// NewRedis 创建基于 Redis 的分布式缓存。
//
// 底层使用 go-cache 的 Redis 实现，通过 Gob 序列化值。
// 适合多实例共享缓存场景。
//
// 参数：
//
//	client — 已初始化的 go-redis 客户端
func NewRedis(client *redis.Client) cache.Cacher {
	return &redisAdapter{Redis: gocache.NewRedis(client)}
}

// ============================================================
// None 适配器
// ============================================================

// noneAdapter 包装 *gocache.None，桥接 gsr.CacheCallback 类型差异。
type noneAdapter struct {
	*gocache.None
}

// GetSet 重写以适配 cache.Cacher 的回调签名。
func (a *noneAdapter) GetSet(ctx context.Context, key string, ttl time.Duration, obj any, fn func(string, any) error) error {
	return a.None.GetSet(ctx, key, ttl, obj, gsr.CacheCallback(fn))
}

// NewNone 创建空缓存。
//
// 所有操作均为空操作（Set 不存储，Get 始终未命中）。
// 适合测试环境或需要禁用缓存的场景。
func NewNone() cache.Cacher {
	return &noneAdapter{None: gocache.NewNone()}
}
