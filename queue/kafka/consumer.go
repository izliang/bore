package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/izliang/bore/queue"
	"github.com/segmentio/kafka-go"
)

// Subscribe 开始消费指定队列的消息，阻塞直到 ctx 被取消或发生致命错误。
//
// 内部行为：
//   - 根据 ConcurrencyController 或 SubscribeOptions.Concurrency 启动 worker 池
//   - 使用 Consumer Group 进行负载均衡
//   - 自动确保 topic 存在
//   - 返回 ctx.Err() 表示正常关闭
func (q *kafkaQueue) Subscribe(ctx context.Context, queueName string, handler queue.Handler, opts *queue.SubscribeOptions) error {
	if handler == nil {
		return fmt.Errorf("%w: nil handler", queue.ErrInvalidHandler)
	}

	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return queue.ErrQueueClosed
	}
	q.mu.RUnlock()

	topicName := resolveTopic(q.opts.Namespace, queueName)
	groupID := resolveConsumerGroup(q.opts.ConsumerGroupPrefix, q.opts.Namespace, queueName)

	if err := q.ensureTopic(ctx, topicName, &queue.DeclareOptions{}); err != nil {
		return fmt.Errorf("Subscribe 确保 topic 存在失败: %w", err)
	}

	reader := q.getOrCreateReader(topicName, groupID)
	defer func() {
		q.readers.Delete(topicName + ":" + groupID)
		_ = reader.Close()
	}()

	concurrency := resolveConcurrency(queueName, opts, q.opts.ConcurrencyController)

	var wg sync.WaitGroup

	// 启动 worker 池
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			q.consumeLoop(ctx, reader, handler, queueName)
		}(i)
	}

	wg.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// consumeLoop 消费消息的主循环。由单个 worker goroutine 执行。
func (q *kafkaQueue) consumeLoop(ctx context.Context, reader *kafka.Reader, handler queue.Handler, queueName string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		km, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// 拉取失败时的短暂等待，避免空转
			time.Sleep(100 * time.Millisecond)
			continue
		}

		msg := &kafkaMessage{
			msg:       km,
			messageID: string(km.Key),
			commitFn: func(m kafka.Message) error {
				return reader.CommitMessages(ctx, m)
			},
			nackFn: func(m kafka.Message, requeue bool) error {
				if requeue {
					return q.routeToRetry(ctx, queueName, m)
				}
				return q.routeToDLQ(ctx, queueName, m)
			},
		}

		result := handler(ctx, msg)

		switch result.Action {
		case queue.HandleAck:
			_ = msg.Ack()
		case queue.HandleRetry:
			_ = q.handleRetry(ctx, queueName, msg, result.Err)
		case queue.HandleNack:
			q.handleNack(ctx, queueName, msg, result)
		}
	}
}

// getOrCreateReader 获取或创建指定 Consumer Group 的 Reader 实例。
func (q *kafkaQueue) getOrCreateReader(topic, groupID string) *kafka.Reader {
	key := topic + ":" + groupID
	if r, ok := q.readers.Load(key); ok {
		return r.(*kafka.Reader)
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:           q.opts.Brokers,
		GroupID:           groupID,
		Topic:             topic,
		MinBytes:          q.opts.ConsumerMinBytes,
		MaxBytes:          q.opts.ConsumerMaxBytes,
		MaxWait:           1 * time.Second,
		SessionTimeout:    q.opts.SessionTimeout,
		RebalanceTimeout:  q.opts.RebalanceTimeout,
		HeartbeatInterval: q.opts.HeartbeatInterval,
		CommitInterval:    q.opts.CommitInterval,
		StartOffset:       kafka.LastOffset,
		Dialer: &kafka.Dialer{
			Timeout:   q.opts.DialTimeout,
			DualStack: true,
			TLS:       q.opts.TLS,
		},
	})

	actual, _ := q.readers.LoadOrStore(key, reader)
	return actual.(*kafka.Reader)
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
	if opts != nil && opts.Concurrency > 0 {
		return opts.Concurrency
	}
	return 1
}
