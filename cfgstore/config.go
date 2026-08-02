package cfgstore

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/izliang/bore/logit"
)

// ChangeAction 变更动作类型
type ChangeAction string

const (
	ActionInit   ChangeAction = "init"   // 初始加载
	ActionAdd    ChangeAction = "add"    // 新增
	ActionUpdate ChangeAction = "update" // 更新
	ActionDelete ChangeAction = "delete" // 删除
)

// ChangeEvent 配置变更事件
type ChangeEvent struct {
	Action   ChangeAction
	Key      string
	OldValue string
	NewValue string
}

// Config 配置缓存
// 职责：内存缓存 + 定时刷新，不关心数据来源
type Config struct {
	cache               map[string]string
	mu                  sync.RWMutex
	loader              Loader
	stop                chan struct{}
	interval            time.Duration
	fullRefreshInterval time.Duration
	lastSince           time.Time // 增量刷新的时间锚点（数据库配置的最后更新时间）
	lastFullRefresh     time.Time // 上次全量刷新时间，用于控制兜底全量刷新的频率
	logger              logit.Logger
	subscribers         map[string][]chan ChangeEvent // key → 订阅者列表，"" 为通配
	subMu               sync.RWMutex
}

// Option 配置选项
type Option func(*Config)

// WithRefreshInterval 设置自动刷新间隔
// 默认 0 = 不自动刷新
func WithRefreshInterval(d time.Duration) Option {
	return func(c *Config) {
		c.interval = d
	}
}

// WithFullRefreshInterval 全量兜底刷新时间间隔
func WithFullRefreshInterval(d time.Duration) Option {
	return func(c *Config) {
		c.fullRefreshInterval = d
	}
}

// New 创建配置缓存
// 会立即调用一次 loader.LoadAll 完成初始化
func New(ctx context.Context, loader Loader, logger logit.Logger, opts ...Option) (*Config, error) {
	c := &Config{
		cache:       make(map[string]string),
		loader:      loader,
		stop:        make(chan struct{}),
		logger:      logger,
		subscribers: make(map[string][]chan ChangeEvent),
	}

	for _, opt := range opts {
		opt(c)
	}

	// 立即全量初始化
	if err := c.loadAll(ctx); err != nil {
		return nil, err
	}

	// 启动自动刷新
	if c.interval > 0 {
		go c.startRefreshLoop(ctx)
	}

	return c, nil
}

// Get 获取配置值
func (c *Config) Get(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cache[key]
}

// GetDefault 获取配置值，不存在返回默认值
func (c *Config) GetDefault(key, defaultValue string) string {
	if v := c.Get(key); v != "" {
		return v
	}
	return defaultValue
}

// GetAll 获取所有配置（拷贝）
func (c *Config) GetAll() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m := make(map[string]string, len(c.cache))
	for k, v := range c.cache {
		m[k] = v
	}
	return m
}

// Refresh 手动触发刷新
// 优先增量，距离上次全量刷新超过x时间自动降级为全量，交给用户配置
func (c *Config) Refresh(ctx context.Context) error {
	if c.lastSince.IsZero() || c.lastFullRefresh.IsZero() {
		return c.loadAll(ctx)
	}
	if c.fullRefreshInterval > 0 && time.Since(c.lastFullRefresh) > c.fullRefreshInterval {
		return c.loadAll(ctx)
	}
	return c.loadSince(ctx)
}

// Subscribe 订阅配置变更事件
// key 为空字符串时订阅所有 key 的变更
// 返回事件 channel 和取消订阅函数
func (c *Config) Subscribe(key string) (<-chan ChangeEvent, func()) {
	ch := make(chan ChangeEvent, 32)
	c.subMu.Lock()
	c.subscribers[key] = append(c.subscribers[key], ch)
	c.subMu.Unlock()

	cancel := func() {
		c.subMu.Lock()
		defer c.subMu.Unlock()
		subs := c.subscribers[key]
		for i, sub := range subs {
			if sub == ch {
				c.subscribers[key] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		close(ch)
	}
	return ch, cancel
}

// notifyChanges 向所有订阅者推送变更事件（非阻塞）
func (c *Config) notifyChanges(changes []ChangeEvent) {
	if len(changes) == 0 {
		return
	}
	c.subMu.RLock()
	defer c.subMu.RUnlock()

	for _, event := range changes {
		// 通知 key 级订阅者
		if subs, ok := c.subscribers[event.Key]; ok {
			for _, ch := range subs {
				select {
				case ch <- event:
				default: // buffer 满则丢弃，不阻塞刷新循环
				}
			}
		}
		// 通知通配符订阅者（key == ""）
		if subs, ok := c.subscribers[""]; ok {
			for _, ch := range subs {
				select {
				case ch <- event:
				default:
				}
			}
		}
	}
}

// Stop 停止后台刷新
func (c *Config) Stop() {
	close(c.stop)
}

// loadAll 全量加载
func (c *Config) loadAll(ctx context.Context) error {
	result, err := c.loader.LoadAll(ctx, c.logger)
	if err != nil {
		return err
	}

	// 快照旧缓存用于 diff
	c.mu.RLock()
	oldCache := c.cache
	firstLoad := len(oldCache) == 0
	c.mu.RUnlock()

	// 计算变更事件
	changes := make([]ChangeEvent, 0, len(result.Data)+len(oldCache))
	for k, v := range result.Data {
		oldVal, existed := oldCache[k]
		if v == "" {
			if existed && oldVal != "" {
				changes = append(changes, ChangeEvent{Key: k, OldValue: oldVal, Action: ActionDelete})
			}
		} else if !existed {
			if firstLoad {
				changes = append(changes, ChangeEvent{Key: k, NewValue: v, Action: ActionInit})
			} else {
				changes = append(changes, ChangeEvent{Key: k, NewValue: v, Action: ActionAdd})
			}
		} else if oldVal != v {
			changes = append(changes, ChangeEvent{Key: k, OldValue: oldVal, NewValue: v, Action: ActionUpdate})
		}
	}
	for k, oldVal := range oldCache {
		if _, exists := result.Data[k]; !exists && oldVal != "" {
			changes = append(changes, ChangeEvent{Key: k, OldValue: oldVal, Action: ActionDelete})
		}
	}

	c.mu.Lock()
	c.cache = make(map[string]string, len(result.Data))
	for k, v := range result.Data {
		if v != "" {
			c.cache[k] = v
		}
	}
	c.lastSince = result.NextSince
	c.lastFullRefresh = time.Now()
	c.mu.Unlock()

	c.notifyChanges(changes)
	return nil
}

// loadSince 增量加载
func (c *Config) loadSince(ctx context.Context) error {
	result, err := c.loader.LoadSince(ctx, c.logger, c.lastSince)
	if err != nil {
		// 增量失败降级为全量
		return c.loadAll(ctx)
	}

	c.mu.Lock()
	changes := make([]ChangeEvent, 0, len(result.Data))
	for k, v := range result.Data {
		oldVal, existed := c.cache[k]
		if v == "" {
			if existed && oldVal != "" {
				changes = append(changes, ChangeEvent{Key: k, OldValue: oldVal, Action: ActionDelete})
			}
			delete(c.cache, k)
		} else {
			if !existed {
				changes = append(changes, ChangeEvent{Key: k, NewValue: v, Action: ActionAdd})
			} else if oldVal != v {
				changes = append(changes, ChangeEvent{Key: k, OldValue: oldVal, NewValue: v, Action: ActionUpdate})
			}
			c.cache[k] = v
		}
	}
	c.lastSince = result.NextSince
	c.mu.Unlock()

	c.notifyChanges(changes)
	return nil
}

func (c *Config) startRefreshLoop(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = c.Refresh(ctx)
		case <-c.stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

// GetJSON 解析JSON格式的配置值
func GetJSON[T any](c *Config, key string) (T, bool, error) {
	var zero T
	s := c.Get(key)
	if s == "" {
		return zero, false, nil
	}

	var t T
	err := json.Unmarshal([]byte(s), &t)
	return t, true, err
}

// GetWithParser 使用自定义解析器解析配置
func GetWithParser[T any](c *Config, key string, parser func(string) (T, error)) (T, bool, error) {
	var zero T
	s := c.Get(key)
	if s == "" {
		return zero, false, nil
	}
	t, e := parser(s)
	return t, true, e
}
