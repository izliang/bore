package rabbitmq

import (
	"context"
	"fmt"
	"math/rand"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/propagator"
	"github.com/izliang/bore/propagator/carrier"
	"github.com/izliang/bore/queue"

	amqp "github.com/rabbitmq/amqp091-go"
)

// consumer 实现 queue.Consumer 接口，包含并发控制、重试策略和死信路由。
//
// 当 controller 字段非 nil 时，worker 数量由 ConcurrencyController 动态控制；
// 为 nil 时使用 SubscribeOptions.Concurrency 的静态值。
type consumer struct {
	cm         *connectionManager
	logger     logit.Logger
	metrics    *queue.Metrics
	vhost      string
	propagator propagator.Propagator
	controller queue.ConcurrencyController
}

// runConsumer 执行单次消费者生命周期。
//
// 内部流程：
//  1. 解析并发度（Controller 优先 → opts.Concurrency → 默认 1）
//  2. 声明队列（确保 broker 重启后队列依然存在）
//  3. 创建 AMQP channel 并设置 QoS prefetch
//  4. 调用 Consume 获取 delivery channel
//  5. 根据是否有 Controller 选择 runDynamicWorkers 或 runStaticWorkers
//
// 返回值：连接断开/channel 关闭时返回错误（Subscribe 层会重连后重试）；
// ctx 被取消时返回 ctx.Err()。
func (c *consumer) runConsumer(ctx context.Context, queueName string, handler queue.Handler, opts *queue.SubscribeOptions, declareFn func(string, *queue.DeclareOptions) error) error {
	if opts.Concurrency < 1 {
		opts.Concurrency = 1
	}

	// Resolve initial concurrency: controller takes priority.
	concurrency := resolveConcurrency(queueName, opts, c.controller)

	// Ensure queue exists before consuming (broker may have restarted)
	if err := declareFn(queueName, nil); err != nil {
		return fmt.Errorf("%w: declare queue: %v", queue.ErrSubscribeFailed, err)
	}

	ch, err := c.cm.channel()
	if err != nil {
		return fmt.Errorf("%w: %v", queue.ErrSubscribeFailed, err)
	}

	// Use a generous prefetch upper bound when controller is active, since
	// concurrency may increase at runtime and we don't want to bottleneck on
	// a stale prefetch.
	prefetch := opts.PrefetchCount
	if prefetch < 1 {
		if c.controller != nil {
			prefetch = max(concurrency*4, 256)
		} else {
			prefetch = concurrency * 2
		}
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		_ = ch.Close()
		return fmt.Errorf("%w: set qos: %v", queue.ErrSubscribeFailed, err)
	}

	// Unique consumer tag per attempt to avoid conflicts during resubscribe.
	consumerTag := opts.ConsumerTag
	if consumerTag == "" {
		consumerTag = fmt.Sprintf("%s-consumer-%d", queueName, time.Now().UnixNano())
	} else {
		consumerTag = fmt.Sprintf("%s-%d", consumerTag, time.Now().UnixNano())
	}

	// Prevent RabbitMQ 3.12+ consumer_timeout from closing the channel
	// when a handler takes longer than the server-side default (30 min).
	// Value 0 disables the timeout; callers may override via SubscribeOptions.Arguments.
	args := tableToAMQP(opts.Arguments)
	if args == nil {
		args = make(amqp.Table)
	}
	if _, exists := args["x-consumer-timeout"]; !exists {
		args["x-consumer-timeout"] = 0
	}

	deliveries, err := ch.Consume(
		queueName,
		consumerTag,
		opts.AutoAck,
		opts.Exclusive,
		opts.NoLocal,
		opts.NoWait,
		args,
	)
	if err != nil {
		_ = ch.Close()
		return fmt.Errorf("%w: amqp consume: %v", queue.ErrSubscribeFailed, err)
	}

	// Create a per-attempt cancel context so we can stop workers independently
	// without affecting the outer ctx (which is for the entire Subscribe call).
	workerCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()

	if c.controller != nil {
		return c.runDynamicWorkers(workerCtx, cancelWorkers, queueName, consumerTag, handler, opts, deliveries, concurrency, ch)
	}
	return c.runStaticWorkers(workerCtx, cancelWorkers, queueName, consumerTag, handler, opts, deliveries, concurrency, ch)
}

// runStaticWorkers 启动固定数量的 worker goroutine，数量在 consumer 生命周期内不变。
// 当未配置 ConcurrencyController 时使用此路径，保持向后兼容。
func (c *consumer) runStaticWorkers(ctx context.Context, cancelWorkers context.CancelFunc,
	queueName, consumerTag string, handler queue.Handler, opts *queue.SubscribeOptions,
	deliveries <-chan amqp.Delivery, concurrency int, ch *amqp.Channel) error {

	var wg sync.WaitGroup
	for i := range concurrency {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			c.worker(ctx, workerID, queueName, handler, opts, deliveries)
		}(i)
	}
	workersDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(workersDone)
	}()

	c.logger.Info(ctx, "consumer started",
		logit.StringField("queue", queueName),
		logit.StringField("consumer_tag", consumerTag),
		logit.IntField("concurrency", concurrency),
	)

	waitWorkers := func(message string) {
		select {
		case <-workersDone:
		case <-time.After(30 * time.Second):
			c.logger.Warn(ctx, message)
		}
	}

	select {
	case <-ctx.Done():
		if err := ch.Cancel(consumerTag, false); err != nil {
			c.logger.Warn(ctx, "cancel consumer", logit.ErrorField("error", err))
		}
		_ = ch.Close()
		cancelWorkers()
		waitWorkers("consumer shutdown timed out waiting for workers")

		c.logger.Info(ctx, "consumer stopped", logit.StringField("queue", queueName))
		return ctx.Err()

	case <-workersDone:
		_ = ch.Close()
		cancelWorkers()
		if err := ctx.Err(); err != nil {
			return err
		}
		return fmt.Errorf("%w: delivery channel closed", queue.ErrSubscribeFailed)
	}
}

// runDynamicWorkers 管理由 ConcurrencyController 驱动的动态 worker 池。
//
// 工作方式：
//   - 通过 c.controller.Watch(queueName) 获取变更通知 channel
//   - 收到新并发度时：扩容 → 追加新 worker goroutine；缩容 → 取消尾部 worker 的 context
//   - 每个 worker 持有独立的 context.Context，缩容时 cancel() 使其优雅退出
//   - 使用 sync.Mutex 保护 worker 取消函数列表的并发访问
//
// 返回：
//   - ctx 被取消 → 停止所有 worker，返回 ctx.Err()
//   - Watch channel 关闭 → Controller 主动终止，返回错误
func (c *consumer) runDynamicWorkers(ctx context.Context, cancelWorkers context.CancelFunc,
	queueName, consumerTag string, handler queue.Handler, opts *queue.SubscribeOptions,
	deliveries <-chan amqp.Delivery, initialConc int, ch *amqp.Channel) error {

	watchCh, stopWatch := c.controller.Watch(queueName)
	defer stopWatch()

	var mu sync.Mutex
	var cancels []context.CancelFunc
	current := initialConc

	// Spawn initial workers.
	for i := 0; i < current; i++ {
		wCtx, wCancel := context.WithCancel(ctx)
		cancels = append(cancels, wCancel)
		id := len(cancels) - 1
		go c.worker(wCtx, id, queueName, handler, opts, deliveries)
	}

	c.logger.Info(ctx, "consumer started (dynamic)",
		logit.StringField("queue", queueName),
		logit.StringField("consumer_tag", consumerTag),
		logit.IntField("concurrency", current),
	)

	cancelAll := func() {
		mu.Lock()
		defer mu.Unlock()
		for _, cancel := range cancels {
			cancel()
		}
		cancels = nil
	}

	for {
		select {
		case <-ctx.Done():
			if err := ch.Cancel(consumerTag, false); err != nil {
				c.logger.Warn(ctx, "cancel consumer", logit.ErrorField("error", err))
			}
			_ = ch.Close()
			cancelWorkers()
			cancelAll()
			c.logger.Info(ctx, "consumer stopped", logit.StringField("queue", queueName))
			return ctx.Err()

		case newConc, ok := <-watchCh:
			if !ok {
				// Controller closed its side; treat as shutdown.
				_ = ch.Close()
				cancelWorkers()
				cancelAll()
				return fmt.Errorf("%w: concurrency controller closed", queue.ErrSubscribeFailed)
			}

			if newConc < 1 {
				newConc = 1
			}

			mu.Lock()
			delta := newConc - current
			if delta > 0 {
				// Scale up.
				for i := 0; i < delta; i++ {
					wCtx, wCancel := context.WithCancel(ctx)
					cancels = append(cancels, wCancel)
					id := len(cancels) - 1
					go c.worker(wCtx, id, queueName, handler, opts, deliveries)
				}
			} else if delta < 0 {
				// Scale down: cancel the last |delta| workers.
				for i := 0; i < -delta; i++ {
					last := len(cancels) - 1
					cancels[last]()
					cancels = cancels[:last]
				}
			}
			prev := current
			current = newConc
			mu.Unlock()

			c.logger.Info(ctx, "concurrency adjusted",
				logit.StringField("queue", queueName),
				logit.IntField("from", prev),
				logit.IntField("to", current),
			)
		}
	}
}

// worker 是单个消费者工作 goroutine 的主循环。
// 从 deliveries channel 读取消息并调用 processMessage 处理。
// ctx 被取消时（缩容或关闭场景）立即退出。
func (c *consumer) worker(ctx context.Context, workerID int, queueName string,
	handler queue.Handler, opts *queue.SubscribeOptions, deliveries <-chan amqp.Delivery) {

	for {
		select {
		case <-ctx.Done():
			return
		case delivery, ok := <-deliveries:
			if !ok {
				return
			}
			if ctx.Err() != nil {
				return
			}
			c.processMessage(ctx, workerID, queueName, handler, opts, delivery)
		}
	}
}

// processMessage 处理单条消息：调用 handler，根据返回的 HandleResult 执行 Ack / Retry / Nack。
// handler 发生 panic 时自动 recover，记录日志并按 Requeue 处理。
func (c *consumer) processMessage(ctx context.Context, workerID int, queueName string,
	handler queue.Handler, opts *queue.SubscribeOptions, delivery amqp.Delivery) {

	msg := &amqpMessage{delivery: delivery}

	if c.metrics != nil {
		c.metrics.MessagesConsumed.WithLabelValues(c.vhost, queueName).Inc()
	}

	result := queue.Ack()

	func() {
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("%w: %v\n%s", queue.ErrHandlerPanic, r, string(debug.Stack()))
				result = queue.Requeue(err)
				if c.metrics != nil {
					c.metrics.HandlerPanics.WithLabelValues(c.vhost, queueName).Inc()
				}
				c.logger.Error(ctx, err,
					logit.StringField("vhost", c.vhost),
					logit.StringField("queue", queueName),
					logit.StringField("message_id", msg.MessageID()),
					logit.IntField("worker_id", workerID),
					logit.StringAbsField("message_body", string(msg.Body())),
				)
			}
		}()

		start := time.Now()
		if c.propagator != nil {
			ctx = c.propagator.Extract(ctx, &carrier.MQHeaders{Headers: msg.Headers()})
		}
		result = handler(ctx, msg)

		if c.metrics != nil {
			c.metrics.HandlerDuration.WithLabelValues(c.vhost, queueName).Observe(time.Since(start).Seconds())
		}
	}()

	if opts.AutoAck {
		return
	}

	switch result.Action {
	case queue.HandleAck:
		_ = msg.Ack() // nolint: errcheck
		if c.metrics != nil {
			c.metrics.MessagesAcked.WithLabelValues(c.vhost, queueName).Inc()
		}

	case queue.HandleRetry:
		c.handleRetry(ctx, queueName, msg, 0, delivery, opts)

	case queue.HandleNack:
		c.handleNack(ctx, queueName, msg, result)

	default:
		// Unknown action: treat as retry.
		c.handleRetry(ctx, queueName, msg, 0, delivery, opts)
	}
}

// handleRetry 按 RetryPolicy 处理 handler 失败：计算退避时间 → 等待 → 重新发布到队列。
// 重试次数超限时转为 Nack(false)，路由到死信队列或丢弃。
func (c *consumer) handleRetry(ctx context.Context, queueName string, msg *amqpMessage, retryCount int, delivery amqp.Delivery, opts *queue.SubscribeOptions) {
	policy := opts.Retry
	if policy == nil {
		policy = queue.DefaultRetryPolicy()
	}

	handlerErr := fmt.Errorf("handler error (retry %d/%d)", retryCount+1, policy.MaxRetries)

	if retryCount < policy.MaxRetries {
		// Exponential backoff with jitter.
		backoff := computeBackoff(retryCount, policy)
		c.logger.Warn(ctx, "handler failed, retrying",
			logit.StringField("vhost", c.vhost),
			logit.StringField("queue", queueName),
			logit.StringField("message_id", msg.MessageID()),
			logit.IntField("retry_count", retryCount+1),
			logit.StringField("backoff", backoff.String()),
		)

		time.Sleep(backoff)

		// Republish with incremented retry count so the next attempt tracks properly.
		if err := c.republishForRetry(ctx, queueName, retryCount+1, delivery); err != nil {
			c.logger.Warn(ctx, "retry publish failed",
				logit.StringField("vhost", c.vhost),
				logit.StringField("queue", queueName),
				logit.StringField("message_id", msg.MessageID()),
				logit.IntField("retry_count", retryCount+1),
				logit.ErrorField("error", err),
			)
			_ = msg.Nack(true) // nolint: errcheck
			return
		}

		if c.metrics != nil {
			c.metrics.MessagesNacked.WithLabelValues(c.vhost, queueName).Inc()
		}
		_ = msg.Ack() // nolint: errcheck
		return
	}

	// Max retries exceeded: dead-letter or drop.
	c.logger.Error(ctx, fmt.Errorf("%w: %v", queue.ErrMaxRetriesExceeded, handlerErr),
		logit.StringField("vhost", c.vhost),
		logit.StringField("queue", queueName),
		logit.StringField("message_id", msg.MessageID()),
		logit.IntField("retry_count", retryCount),
		logit.IntField("max_retries", policy.MaxRetries),
	)

	if c.metrics != nil {
		c.metrics.MessagesDeadLettered.WithLabelValues(c.vhost, queueName).Inc()
	}
	_ = msg.Nack(false) // nolint: errcheck
}

// handleNack 处理 handler 返回的 Nack 决策，直接传递给 AMQP broker。
// requeue=true 时消息立即重新投递，requeue=false 时进入死信或丢弃。
func (c *consumer) handleNack(ctx context.Context, queueName string, msg *amqpMessage, result queue.HandleResult) {
	if result.Err != nil {
		c.logger.Warn(ctx, "handler requested nack",
			logit.StringField("vhost", c.vhost),
			logit.StringField("queue", queueName),
			logit.StringField("message_id", msg.MessageID()),
			logit.BoolField("requeue", result.Requeue),
			logit.ErrorField("error", result.Err),
		)
	}
	if c.metrics != nil {
		c.metrics.MessagesNacked.WithLabelValues(c.vhost, queueName).Inc()
	}
	_ = msg.Nack(result.Requeue) // nolint: errcheck
}

// republishForRetry 将消息重新发布到原队列，携带递增的 x-retry-count 头。
func (c *consumer) republishForRetry(ctx context.Context, queueName string, retryCount int, delivery amqp.Delivery) error {
	ch, err := c.cm.channel()
	if err != nil {
		return err
	}
	defer func() {
		_ = ch.Close()
	}()

	headers := amqp.Table{}
	for k, v := range delivery.Headers {
		headers[k] = v
	}
	headers["x-retry-count"] = retryCount

	return ch.PublishWithContext(ctx, "", queueName, false, false, amqp.Publishing{
		Headers:         headers,
		ContentType:     delivery.ContentType,
		ContentEncoding: delivery.ContentEncoding,
		DeliveryMode:    delivery.DeliveryMode,
		Priority:        delivery.Priority,
		CorrelationId:   delivery.CorrelationId,
		ReplyTo:         delivery.ReplyTo,
		Expiration:      delivery.Expiration,
		MessageId:       delivery.MessageId,
		Timestamp:       delivery.Timestamp,
		Type:            delivery.Type,
		UserId:          delivery.UserId,
		AppId:           delivery.AppId,
		Body:            delivery.Body,
	})
}

// getRetryCount 从消息头中提取当前重试次数。
// 优先读取 x-retry-count 头，回退到 x-death 数组长度（broker 元数据）。
func (c *consumer) getRetryCount(delivery amqp.Delivery) int {
	if val, ok := delivery.Headers["x-retry-count"]; ok {
		switch v := val.(type) {
		case int:
			return v
		case int32:
			return int(v)
		case int64:
			return int(v)
		case float64:
			return int(v)
		}
	}
	if deaths, ok := delivery.Headers["x-death"]; ok {
		switch deathList := deaths.(type) {
		case []amqp.Table:
			return len(deathList)
		case []any:
			return len(deathList)
		}
	}
	return 0
}

// computeBackoff 计算指数退避时间，附带 ±25% 的随机抖动以减少惊群效应。
// 公式：min(initial × 2^retryCount, maxBackoff) + jitter
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

	jitterRange := float64(backoff) * 0.25
	jitter := time.Duration(rand.Float64()*2*jitterRange - jitterRange)
	return backoff + jitter
}

// resolveConcurrency 解析订阅的实际并发度。
//
// 优先级：
//  1. ConcurrencyController.Resolve(queueName) → 返回值 > 0 时采用
//  2. SubscribeOptions.Concurrency → Controller 未设置或返回 ≤ 0 时回退
//  3. 默认值 1
func resolveConcurrency(queueName string, opts *queue.SubscribeOptions, ctrl queue.ConcurrencyController) int {
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

// amqpMessage adapts amqp.Delivery to queue.Message.
type amqpMessage struct {
	delivery amqp.Delivery
	acked    atomic.Bool
}

func (m *amqpMessage) Body() []byte {
	return m.delivery.Body
}

func (m *amqpMessage) Headers() map[string]string {
	h := make(map[string]string, len(m.delivery.Headers))
	for k, v := range m.delivery.Headers {
		h[k] = fmt.Sprintf("%v", v)
	}
	if m.delivery.ContentType != "" {
		h["Content-Type"] = m.delivery.ContentType
	}
	if m.delivery.ContentEncoding != "" {
		h["Content-Encoding"] = m.delivery.ContentEncoding
	}
	return h
}

func (m *amqpMessage) Ack() error {
	if !m.acked.CompareAndSwap(false, true) {
		return nil
	}
	return m.delivery.Ack(false)
}

func (m *amqpMessage) Nack(requeue bool) error {
	if !m.acked.CompareAndSwap(false, true) {
		return nil
	}
	return m.delivery.Nack(false, requeue)
}

func (m *amqpMessage) MessageID() string {
	return m.delivery.MessageId
}
