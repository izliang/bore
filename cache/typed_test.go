package cache_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/izliang/bore/cache"
	"github.com/izliang/bore/cache/provider"
)

// test types
type testUser struct {
	ID   int
	Name string
}

type testConfig struct {
	Host string
	Port int
}

// ============================================================
// 辅助
// ============================================================

func newTestCache[V any](t *testing.T, prefix string) *cache.Cache[V] {
	t.Helper()
	c := provider.NewMemory(5*time.Minute, 10*time.Minute)
	return cache.New[V](c, prefix)
}

func ctx() context.Context { return context.Background() }

// ============================================================
// Get / Set
// ============================================================

func TestCache_GetSetBasic(t *testing.T) {
	c := newTestCache[testUser](t, "user:")

	t.Run("get non-existent returns nil", func(t *testing.T) {
		got, err := c.Get(ctx(), "nonexistent")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("set then get", func(t *testing.T) {
		key := "set_get"
		want := &testUser{ID: 1, Name: "张三"}

		if err := c.Set(ctx(), key, want, time.Minute); err != nil {
			t.Fatalf("Set: %v", err)
		}
		got, err := c.Get(ctx(), key)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got == nil || got.ID != want.ID || got.Name != want.Name {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("overwrite existing", func(t *testing.T) {
		key := "overwrite"
		c.Set(ctx(), key, &testUser{ID: 1, Name: "old"}, time.Minute)
		c.Set(ctx(), key, &testUser{ID: 2, Name: "new"}, time.Minute)

		got, _ := c.Get(ctx(), key)
		if got.Name != "new" {
			t.Errorf("got %q, want %q", got.Name, "new")
		}
	})

	t.Run("set with zero ttl means no expiry", func(t *testing.T) {
		key := "no_expiry"
		c.Set(ctx(), key, &testUser{ID: 99}, 0)
		got, _ := c.Get(ctx(), key)
		if got == nil {
			t.Error("expected value, got nil")
		}
	})
}

// ============================================================
// GetSet (缓存穿透保护)
// ============================================================

func TestCache_GetSetWithLoader(t *testing.T) {
	c := newTestCache[testUser](t, "user:")

	t.Run("hit returns cached value without calling loader", func(t *testing.T) {
		key := "getset_hit"
		c.Set(ctx(), key, &testUser{ID: 1, Name: "cached"}, time.Minute)

		var loaderCalled atomic.Bool
		got, err := c.GetSet(ctx(), key, time.Minute, func(ctx context.Context, key string) (*testUser, error) {
			loaderCalled.Store(true)
			return &testUser{ID: 999}, nil
		})
		if err != nil {
			t.Fatalf("GetSet: %v", err)
		}
		if got.Name != "cached" {
			t.Errorf("got %q, want %q", got.Name, "cached")
		}
		if loaderCalled.Load() {
			t.Error("loader should not be called on cache hit")
		}
	})

	t.Run("miss calls loader and stores result", func(t *testing.T) {
		key := "getset_miss"
		var loaderCalled atomic.Bool

		got, err := c.GetSet(ctx(), key, time.Minute, func(ctx context.Context, key string) (*testUser, error) {
			loaderCalled.Store(true)
			return &testUser{ID: 42, Name: "loaded"}, nil
		})
		if err != nil {
			t.Fatalf("GetSet: %v", err)
		}
		if !loaderCalled.Load() {
			t.Error("loader should be called on cache miss")
		}
		if got.Name != "loaded" {
			t.Errorf("got %q, want %q", got.Name, "loaded")
		}

		// 验证已写入缓存
		cached, _ := c.Get(ctx(), key)
		if cached == nil || cached.Name != "loaded" {
			t.Error("value should be cached after GetSet miss")
		}
	})

	t.Run("loader receives correct business key", func(t *testing.T) {
		key := "loader_key_test"
		var receivedKey string

		c.GetSet(ctx(), key, time.Minute, func(ctx context.Context, k string) (*testUser, error) {
			receivedKey = k
			return &testUser{ID: 1}, nil
		})
		if receivedKey != key {
			t.Errorf("loader received key %q, want %q", receivedKey, key)
		}
	})

	t.Run("loader error is propagated", func(t *testing.T) {
		_, err := c.GetSet(ctx(), "loader_err", time.Minute, func(ctx context.Context, key string) (*testUser, error) {
			return nil, context.DeadlineExceeded
		})
		if err == nil {
			t.Error("expected error from loader, got nil")
		}
	})
}

// ============================================================
// Del / Exists
// ============================================================

func TestCache_DelExists(t *testing.T) {
	c := newTestCache[testUser](t, "user:")

	t.Run("del removes key", func(t *testing.T) {
		key := "del_test"
		c.Set(ctx(), key, &testUser{ID: 1}, time.Minute)
		if !c.Exists(ctx(), key) {
			t.Fatal("expected key to exist before del")
		}
		c.Del(ctx(), key)
		if c.Exists(ctx(), key) {
			t.Error("expected key to not exist after del")
		}
	})

	t.Run("del non-existent is no-op", func(t *testing.T) {
		c.Del(ctx(), "never_set")
	})

	t.Run("exists returns false for never-set key", func(t *testing.T) {
		if c.Exists(ctx(), "never_set") {
			t.Error("expected false for non-existent key")
		}
	})
}

// ============================================================
// Prefix 隔离
// ============================================================

func TestCache_PrefixIsolation(t *testing.T) {
	base := provider.NewMemory(5*time.Minute, 10*time.Minute)
	userCache := cache.New[testUser](base, "user:")
	configCache := cache.New[testConfig](base, "config:")

	t.Run("different prefixes are isolated", func(t *testing.T) {
		userCache.Set(ctx(), "1", &testUser{ID: 1, Name: "u1"}, time.Minute)
		configCache.Set(ctx(), "1", &testConfig{Host: "localhost", Port: 8080}, time.Minute)

		u, _ := userCache.Get(ctx(), "1")
		c, _ := configCache.Get(ctx(), "1")

		if u == nil || u.Name != "u1" {
			t.Error("user cache corrupted")
		}
		if c == nil || c.Port != 8080 {
			t.Error("config cache corrupted")
		}
	})

	t.Run("same key with empty prefix", func(t *testing.T) {
		noPrefix := cache.New[testUser](base, "")
		noPrefix.Set(ctx(), "shared", &testUser{ID: 99}, time.Minute)

		got, _ := noPrefix.Get(ctx(), "shared")
		if got == nil || got.ID != 99 {
			t.Error("empty prefix cache failed")
		}
	})
}

// ============================================================
// 多类型共享一个 Cacher
// ============================================================

func TestCache_MultiType(t *testing.T) {
	c := provider.NewMemory(5*time.Minute, 10*time.Minute)

	users := cache.New[testUser](c, "u:")
	configs := cache.New[testConfig](c, "c:")

	users.Set(ctx(), "a", &testUser{ID: 1, Name: "Alice"}, time.Minute)
	configs.Set(ctx(), "a", &testConfig{Host: "example.com", Port: 443}, time.Minute)

	t.Run("users get correct type", func(t *testing.T) {
		u, _ := users.Get(ctx(), "a")
		if u.Name != "Alice" {
			t.Errorf("got %q, want Alice", u.Name)
		}
	})

	t.Run("configs get correct type", func(t *testing.T) {
		cfg, _ := configs.Get(ctx(), "a")
		if cfg.Port != 443 {
			t.Errorf("got %d, want 443", cfg.Port)
		}
	})
}

// ============================================================
// TTL 过期
// ============================================================

func TestCache_TTLExpiry(t *testing.T) {
	c := newTestCache[testUser](t, "user:")

	t.Run("value expires after short TTL", func(t *testing.T) {
		key := "expire_test"
		c.Set(ctx(), key, &testUser{ID: 1}, 50*time.Millisecond)

		got, _ := c.Get(ctx(), key)
		if got == nil {
			t.Fatal("expected value before expiry")
		}

		time.Sleep(100 * time.Millisecond)

		got, _ = c.Get(ctx(), key)
		if got != nil {
			t.Error("expected nil after TTL expiry")
		}
	})
}

// ============================================================
// 零值 / 空结构体
// ============================================================

func TestCache_ZeroValue(t *testing.T) {
	c := newTestCache[testUser](t, "user:")

	t.Run("zero value struct", func(t *testing.T) {
		key := "zero"
		c.Set(ctx(), key, &testUser{}, time.Minute)

		got, _ := c.Get(ctx(), key)
		if got == nil {
			t.Fatal("expected non-nil zero value")
		}
		if got.ID != 0 || got.Name != "" {
			t.Errorf("expected zero values, got %+v", got)
		}
	})
}

// ============================================================
// 并发安全
// ============================================================

func TestCache_Concurrency(t *testing.T) {
	c := newTestCache[testUser](t, "user:")

	const goroutines = 20
	const iterations = 50
	done := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			for j := 0; j < iterations; j++ {
				key := "key"
				c.Set(ctx(), key, &testUser{ID: id}, time.Minute)
				c.Get(ctx(), key)
				c.Exists(ctx(), key)
			}
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}
}
