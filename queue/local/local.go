// Package local 提供基于进程内 Go channel 的 queue.Queue 实现。
//
// 适用于本地开发、调试和单元测试场景，无需外部 broker（RabbitMQ、Redis 等）。
//
// 核心特性：
//   - 自包含：生产与消费在同一进程内完成
//   - 零外部依赖：仅依赖标准库和父级 queue 包
//   - 完整接口：实现了 Publish、Subscribe、Declare、Shutdown、IsHealthy
//   - 并发安全：多个 producer 和 consumer 可安全并发操作
//   - Ack/Nack 语义：支持 requeue 重入队
//   - 可配置缓冲区：通过 DefaultBufferSize 或 DeclareOptions.MaxLength 设置
//   - 动态并发：支持 ConcurrencyController 运行时扩缩容
//
// # 快速上手
//
//	q := local.New(local.Options{})
//	defer q.Shutdown(context.Background())
//
//	q.Declare("tasks", nil)
//	q.Publish(ctx, "tasks", queue.NewMessage([]byte("hello")))
//
//	q.Subscribe(ctx, "tasks", func(ctx context.Context, msg queue.Message) queue.HandleResult {
//	    fmt.Println(string(msg.Body()))
//	    return queue.Ack()
//	}, nil)
//
// # 与真实 broker 的关键差异
//
//   - 消息不持久化：进程退出后所有数据丢失
//   - 无网络延迟：投递是瞬时完成的（适合测试，不适合性能评估）
//   - 无死信交换：Nack(false) 直接丢弃消息
//   - 无 Exchange/Topic 路由：PublishWithRouting 始终投递到 routingKey 指定的队列
//   - Shutdown 取消所有活跃消费者并拒绝新发布请求
package local

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/izliang/bore/queue"
)

// Options 配置本地队列实例。
type Options struct {
	// DefaultBufferSize 是未通过 DeclareOptions.MaxLength 指定容量时，队列 channel 的默认缓冲区大小。
	// 默认值为 1024。
	DefaultBufferSize int

	// ConcurrencyController 提供全局的、按队列的动态并发度控制。
	//
	// 设置后，Subscribe 会 Watch 并发度变化并在运行时调整 worker 数量。
	// 为 nil 时使用 SubscribeOptions.Concurrency 的静态值。
	ConcurrencyController queue.ConcurrencyController
}

// localQueue 是基于进程内 Go channel 的 queue.Queue 实现。
//
// 每个队列对应一个带缓冲的 channel，发布和消费通过 channel 的 send/receive 完成。
// controller 字段控制 worker 数量的动态调整。
type localQueue struct {
	opts       Options
	mu         sync.RWMutex
	subMu      sync.Mutex
	queues     map[string]*localTopic
	closed     atomic.Bool
	ctx        context.Context
	cancel     context.CancelFunc
	doneWg     sync.WaitGroup
	controller queue.ConcurrencyController
}

// localTopic holds the channel and metadata for a single named queue.
type localTopic struct {
	ch      chan *localMessage
	declare *queue.DeclareOptions
}

// New creates a new in-process Queue.
func New(opts Options) queue.Queue {
	if opts.DefaultBufferSize < 1 {
		opts.DefaultBufferSize = 1024
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &localQueue{
		opts:       opts,
		queues:     make(map[string]*localTopic),
		ctx:        ctx,
		cancel:     cancel,
		controller: opts.ConcurrencyController,
	}
}

// Publish sends a message to the named queue. The exchange parameter is ignored;
// messages are always delivered directly to the queue identified by routingKey.
func (q *localQueue) Publish(ctx context.Context, routingKey string, msg queue.Message) error {
	return q.PublishWithRouting(ctx, "", routingKey, msg)
}

// PublishWithRouting sends a message. The exchange parameter is ignored in the local
// implementation; messages go directly to the routingKey queue.
func (q *localQueue) PublishWithRouting(ctx context.Context, _ /*exchange*/, routingKey string, msg queue.Message) error {
	if msg == nil {
		return fmt.Errorf("%w: nil message", queue.ErrInvalidMessage)
	}
	if q.closed.Load() {
		return queue.ErrQueueClosed
	}

	q.mu.RLock()
	topic, ok := q.queues[routingKey]
	q.mu.RUnlock()

	if !ok {
		var err error
		topic, err = q.getOrCreateTopic(routingKey)
		if err != nil {
			return err
		}
	}

	lm := &localMessage{
		body:      make([]byte, len(msg.Body())),
		headers:   make(map[string]string, len(msg.Headers())),
		messageID: msg.MessageID(),
	}
	copy(lm.body, msg.Body())
	for k, v := range msg.Headers() {
		lm.headers[k] = v
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-q.ctx.Done():
		return queue.ErrQueueClosed
	case topic.ch <- lm:
		return nil
	}
}

// Subscribe starts consuming from the named queue. It blocks until ctx is cancelled
// or the queue is shut down.
func (q *localQueue) Subscribe(ctx context.Context, queueName string, handler queue.Handler, opts *queue.SubscribeOptions) error {
	if handler == nil {
		return fmt.Errorf("%w: nil handler", queue.ErrInvalidHandler)
	}
	q.subMu.Lock()
	if q.closed.Load() {
		q.subMu.Unlock()
		return queue.ErrQueueClosed
	}
	q.doneWg.Add(1)
	q.subMu.Unlock()
	defer q.doneWg.Done()

	if opts == nil {
		opts = queue.DefaultSubscribeOptions()
	}
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}

	concurrency := resolveLocalConcurrency(queueName, opts, q.controller)

	topic, err := q.getOrCreateTopic(queueName)
	if err != nil {
		return err
	}

	// Use a merged context: external ctx + internal shutdown ctx
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()

	// Monitor internal shutdown
	go func() {
		select {
		case <-q.ctx.Done():
			subCancel()
		case <-subCtx.Done():
		}
	}()

	if q.controller != nil {
		return q.runDynamicWorkers(subCtx, subCancel, queueName, topic.ch, handler, opts, concurrency)
	}
	return q.runStaticWorkers(subCtx, subCancel, queueName, topic.ch, handler, opts, concurrency)
}

// runStaticWorkers 启动固定数量的 worker goroutine，数量在 consumer 生命周期内不变。
func (q *localQueue) runStaticWorkers(ctx context.Context, cancel context.CancelFunc,
	queueName string, ch chan *localMessage, handler queue.Handler, opts *queue.SubscribeOptions, concurrency int) error {

	var wg sync.WaitGroup
	for i := range concurrency {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			q.worker(ctx, workerID, queueName, ch, handler, opts)
		}(i)
	}

	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

// runDynamicWorkers 管理由 ConcurrencyController 驱动的动态 worker 池。
//
// 通过 Watch channel 接收并发度变更通知，扩容时追加 worker，缩容时取消尾部 worker 的 context。
// worker 收到 context 取消后立即退出当前循环，实现无中断的平滑缩容。
func (q *localQueue) runDynamicWorkers(ctx context.Context, cancel context.CancelFunc,
	queueName string, ch chan *localMessage, handler queue.Handler, opts *queue.SubscribeOptions, initialConc int) error {

	watchCh, stopWatch := q.controller.Watch(queueName)
	defer stopWatch()

	var mu sync.Mutex
	var cancels []context.CancelFunc
	current := initialConc

	for i := 0; i < current; i++ {
		wCtx, wCancel := context.WithCancel(ctx)
		cancels = append(cancels, wCancel)
		id := len(cancels) - 1
		go q.worker(wCtx, id, queueName, ch, handler, opts)
	}

	cancelAll := func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range cancels {
			c()
		}
		cancels = nil
	}

	for {
		select {
		case <-ctx.Done():
			cancelAll()
			return ctx.Err()

		case newConc, ok := <-watchCh:
			if !ok {
				cancelAll()
				return fmt.Errorf("%w: concurrency controller closed", queue.ErrSubscribeFailed)
			}

			if newConc < 1 {
				newConc = 1
			}

			mu.Lock()
			delta := newConc - current
			if delta > 0 {
				for i := 0; i < delta; i++ {
					wCtx, wCancel := context.WithCancel(ctx)
					cancels = append(cancels, wCancel)
					id := len(cancels) - 1
					go q.worker(wCtx, id, queueName, ch, handler, opts)
				}
			} else if delta < 0 {
				for i := 0; i < -delta; i++ {
					last := len(cancels) - 1
					cancels[last]()
					cancels = cancels[:last]
				}
			}
			current = newConc
			mu.Unlock()
		}
	}
}

// resolveLocalConcurrency 解析本地队列的实际并发度。
//
// 优先级：ConcurrencyController.Resolve > SubscribeOptions.Concurrency > 1。
func resolveLocalConcurrency(queueName string, opts *queue.SubscribeOptions, ctrl queue.ConcurrencyController) int {
	if ctrl != nil {
		if c := ctrl.Resolve(queueName); c > 0 {
			return c
		}
	}
	if opts.Concurrency > 0 {
		return opts.Concurrency
	}
	return 1
}

// worker 是单个消费者工作 goroutine 的主循环。
// 从 channel 读取消息并调用 processMessage 处理，ctx 取消时立即退出（缩容/关闭场景）。
func (q *localQueue) worker(ctx context.Context, workerID int, queueName string,
	ch chan *localMessage, handler queue.Handler, opts *queue.SubscribeOptions) {

	for {
		select {
		case <-ctx.Done():
			return
		case lm, ok := <-ch:
			if !ok {
				return
			}
			q.processMessage(ctx, workerID, queueName, lm, ch, handler, opts)
		}
	}
}

// processMessage 处理单条消息：调用 handler，根据 HandleResult 执行 Ack / Retry / Nack。
// handler panic 时自动 recover 并按 Retry 处理。
func (q *localQueue) processMessage(ctx context.Context, workerID int, queueName string,
	lm *localMessage, ch chan *localMessage, handler queue.Handler, opts *queue.SubscribeOptions) {

	result := queue.Ack()
	func() {
		defer func() {
			if r := recover(); r != nil {
				result = queue.Retry(fmt.Errorf("%w: %v", queue.ErrHandlerPanic, r))
			}
		}()
		result = handler(ctx, lm)
	}()

	if opts.AutoAck {
		return
	}

	switch result.Action {
	case queue.HandleAck:
		lm.ack()
	case queue.HandleRetry:
		q.retryMessage(ctx, lm, ch, opts)
	case queue.HandleNack:
		if result.Requeue {
			select {
			case <-ctx.Done():
			case <-q.ctx.Done():
			case ch <- lm:
			}
			return
		}
		lm.nack()
	default:
		q.retryMessage(ctx, lm, ch, opts)
	}
}

func (q *localQueue) retryMessage(ctx context.Context, lm *localMessage, ch chan *localMessage, opts *queue.SubscribeOptions) {
	retryCount := lm.retryCount
	policy := opts.Retry
	if policy == nil {
		policy = &queue.RetryPolicy{MaxRetries: 0}
	}

	lm.retryCount++

	if retryCount < policy.MaxRetries {
		backoff := computeBackoff(retryCount, policy)
		go func() {
			timer := time.NewTimer(backoff)
			defer timer.Stop()

			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}

			select {
			case <-ctx.Done():
			case <-q.ctx.Done():
			case ch <- lm:
			}
		}()
		return
	}

	lm.nack()
}

// Declare creates or ensures a queue exists with the given options.
// In the local implementation, Declare is idempotent and mainly sets the
// channel buffer size based on MaxLength.
func (q *localQueue) Declare(queueName string, opts *queue.DeclareOptions) error {
	if q.closed.Load() {
		return queue.ErrQueueClosed
	}
	_, err := q.getOrCreateTopicWithOpts(queueName, opts)
	return err
}

// Shutdown gracefully stops the queue and waits for active consumers to exit.
func (q *localQueue) Shutdown(ctx context.Context) error {
	if !q.closed.CompareAndSwap(false, true) {
		return nil
	}

	q.cancel()

	done := make(chan struct{})
	go func() {
		q.subMu.Lock()
		defer q.subMu.Unlock()
		q.doneWg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// IsHealthy always returns true for the local queue (no network dependency).
func (q *localQueue) IsHealthy() bool {
	return !q.closed.Load()
}

// getOrCreateTopic returns an existing topic or creates one with default buffer size.
func (q *localQueue) getOrCreateTopic(name string) (*localTopic, error) {
	return q.getOrCreateTopicWithOpts(name, nil)
}

// getOrCreateTopicWithOpts returns or creates a topic.
func (q *localQueue) getOrCreateTopicWithOpts(name string, opts *queue.DeclareOptions) (*localTopic, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed.Load() {
		return nil, queue.ErrQueueClosed
	}

	if topic, ok := q.queues[name]; ok {
		return topic, nil
	}

	bufSize := q.opts.DefaultBufferSize
	if opts != nil && opts.MaxLength > 0 {
		bufSize = opts.MaxLength
	}

	topic := &localTopic{
		ch:      make(chan *localMessage, bufSize),
		declare: opts,
	}
	q.queues[name] = topic
	return topic, nil
}

// computeBackoff mirrors the rabbitmq backoff logic for local retries.
// It does NOT mutate the policy.
func computeBackoff(retryCount int, policy *queue.RetryPolicy) time.Duration {
	initial := policy.Backoff
	if initial <= 0 {
		initial = 1 * time.Second
	}
	maxBackoff := policy.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 60 * time.Second
	}

	backoff := initial
	for range retryCount {
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
			break
		}
	}
	return backoff
}
