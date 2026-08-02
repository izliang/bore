// Package controller 提供 queue.ConcurrencyController 接口的内置实现。
//
// 当前包含 StaticController，适用于通过 Admin API、配置热更新、
// 自动伸缩策略等场景在运行时动态调整队列消费者并发度。
package controller

import (
	"sync"

	"github.com/izliang/bore/queue"
)

// StaticController 是基于内存的并发度控制器，值可通过 Set 方法在运行时动态更新。
//
// 它实现了 queue.ConcurrencyController 接口，线程安全，可被多个 goroutine 并发使用。
// 适用于以下场景：
//   - Admin API 动态调整并发度，调用 Set 即可实时生效
//   - 配置热更新：监听配置变更后调用 Set 同步
//   - 自动伸缩策略：根据队列深度或系统负载调用 Set 调整 worker 数
//
// # Channel 与阻塞策略
//
// Watch 返回的 channel 容量为 1（cap=1），采用 drain-and-replace 策略确保 Set 永不阻塞：
//   - 缓冲区为空时直接发送
//   - 缓冲区已满时，先排空旧值，再写入新值
//   - 调用方始终只收到最新的目标并发度，不关心中间状态
//
// 这保证了多 watcher 场景下，即使某个 consumer 处理较慢，Set 也不会被阻塞。
//
// # 典型用法
//
//	ctrl := controller.NewStatic(map[string]int{
//	    "order.queue": 10,
//	    "notify.queue": 5,
//	})
//	q, _ := rabbitmq.New(rabbitmq.Options{
//	    ConcurrencyController: ctrl,
//	})
//	// 运行时从 Admin API 动态调整
//	ctrl.Set("order.queue", 20) // 立即扩容
//	ctrl.Set("order.queue", 5)  // 立即缩容
type StaticController struct {
	mu   sync.RWMutex
	vals map[string]int        // queueName → 目标并发度
	subs map[string][]chan int // queueName → 活跃的 watcher channel 列表
}

// NewStatic 创建一个 StaticController 并初始化各队列的并发度。
// initial 可以为 nil，表示所有队列初始值均为 0（即不控制，回退到 SubscribeOptions.Concurrency）。
func NewStatic(initial map[string]int) *StaticController {
	vals := make(map[string]int, len(initial))
	for k, v := range initial {
		vals[k] = v
	}
	return &StaticController{
		vals: vals,
		subs: make(map[string][]chan int),
	}
}

// Set 更新指定队列的目标并发度，并实时通知所有活跃的 watcher。
//
// Set 永不阻塞：每个 watcher channel 容量为 1，采用 drain-and-replace 策略写入。
// 即使有 watcher 长时间不读取 channel，Set 也能立即返回。
//
// 并发安全：可在任意 goroutine 中调用，内部持锁保护。
func (c *StaticController) Set(queueName string, concurrency int) {
	c.mu.Lock()
	c.vals[queueName] = concurrency
	subs := c.subs[queueName]
	c.mu.Unlock()

	for _, ch := range subs {
		c.sendNonBlocking(ch, concurrency)
	}
}

// Resolve 返回指定队列的当前目标并发度。
// 返回 0 表示该队列未被显式设置，调用方应回退到 SubscribeOptions.Concurrency。
func (c *StaticController) Resolve(queueName string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.vals[queueName]
}

// Watch 订阅指定队列的并发度变更通知。
//
// 返回值：
//   - <-chan int：cap=1 的缓冲 channel，并发度变化时收到新值
//   - func()：停止订阅的回调，调用后 Controller 移除该 watcher 避免资源泄漏
//
// stop 函数是幂等的：重复调用不会 panic。调用 stop 后 channel 不再收到新值，
// consumer 不应再读取该 channel（但也不会被 Controller 关闭）。
func (c *StaticController) Watch(queueName string) (<-chan int, func()) {
	ch := make(chan int, 1) // cap=1：只保留最新值，支持非阻塞发送

	c.mu.Lock()
	c.subs[queueName] = append(c.subs[queueName], ch)
	c.mu.Unlock()

	stop := func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		subs := c.subs[queueName]
		for i, s := range subs {
			if s == ch {
				c.subs[queueName] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		// 清理空切片，避免内存泄漏。
		if len(c.subs[queueName]) == 0 {
			delete(c.subs, queueName)
		}
	}

	return ch, stop
}

// sendNonBlocking 以非阻塞方式向 ch 发送值 v。
//
// 策略：先尝试直接发送；若缓冲区已满（说明 consumer 尚未读取旧值），
// 则排空旧值再发送新值。这保证了：
//   - 发送方永不阻塞
//   - 接收方始终获得最新的目标并发度（最终一致性）
func (c *StaticController) sendNonBlocking(ch chan int, v int) {
	select {
	case ch <- v:
	default:
		// 缓冲区已满：排空旧值后发送最新值。
		select {
		case <-ch:
		default:
		}
		ch <- v
	}
}

// 编译期接口实现校验。
var _ queue.ConcurrencyController = (*StaticController)(nil)
