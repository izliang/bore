package kafka

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/queue"
	"github.com/segmentio/kafka-go"
)

const (
	// maxRetryCount 是默认的最大重试次数。
	maxRetryCount = 5

	// retryHeaderRetryCount 是记录当前重试次数的 header key。
	retryHeaderRetryCount = "x-retry-count"

	// retryHeaderOriginalTopic 是记录原始 topic 的 header key。
	retryHeaderOriginalTopic = "x-original-topic"

	// retryHeaderRetryAfter 是记录重试时间的 header key（Unix 毫秒时间戳）。
	retryHeaderRetryAfter = "x-retry-after"
)

// routeToRetry 将消息路由到重试 topic。
// 递增 x-retry-count，记录原始 topic 和重试时间。
func (q *kafkaQueue) routeToRetry(ctx context.Context, queueName string, msg kafka.Message) error {
	retryTopic := resolveRetryTopic(q.opts.Namespace, queueName, q.opts.RetryTopicSuffix)

	retryCount := getRetryCount(msg.Headers)
	retryCount++

	policy := queue.DefaultRetryPolicy()
	backoff := computeBackoff(retryCount, policy)
	retryAfter := time.Now().Add(backoff)

	headers := make([]kafka.Header, 0, len(msg.Headers)+3)
	headers = append(headers, msg.Headers...)
	headers = append(headers,
		kafka.Header{Key: retryHeaderRetryCount, Value: []byte(strconv.Itoa(retryCount))},
		kafka.Header{Key: retryHeaderOriginalTopic, Value: []byte(resolveTopic(q.opts.Namespace, queueName))},
		kafka.Header{Key: retryHeaderRetryAfter, Value: []byte(strconv.FormatInt(retryAfter.UnixMilli(), 10))},
	)

	writer := q.getOrCreateWriter(retryTopic)
	return writer.WriteMessages(ctx, kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: headers,
	})
}

// routeToDLQ 将消息路由到死信队列（DLQ）。
func (q *kafkaQueue) routeToDLQ(ctx context.Context, queueName string, msg kafka.Message) error {
	dlqTopic := resolveDLQTopic(q.opts.Namespace, queueName, q.opts.DLQTopicSuffix)

	headers := make([]kafka.Header, 0, len(msg.Headers)+1)
	headers = append(headers, msg.Headers...)
	headers = append(headers,
		kafka.Header{Key: retryHeaderOriginalTopic, Value: []byte(resolveTopic(q.opts.Namespace, queueName))},
	)

	writer := q.getOrCreateWriter(dlqTopic)
	return writer.WriteMessages(ctx, kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: headers,
	})
}

// handleRetry 处理 handler 返回的 Retry 决策。
//
// 检查重试次数是否超限：
//   - 未超限 → 路由到 Retry Topic
//   - 已超限 → 路由到 DLQ
func (q *kafkaQueue) handleRetry(ctx context.Context, queueName string, msg *kafkaMessage, handlerErr error) error {
	retryCount := getRetryCount(msg.msg.Headers)

	if retryCount >= maxRetryCount {
		if handlerErr != nil && q.opts.Logger != nil {
			q.opts.Logger.Error(ctx, handlerErr,
				logit.StringField("queue", queueName),
				logit.StringField("message_id", msg.MessageID()),
				logit.IntField("retry_count", retryCount),
			)
		}
		return q.routeToDLQ(ctx, queueName, msg.msg)
	}

	return q.routeToRetry(ctx, queueName, msg.msg)
}

// handleNack 处理 handler 返回的 Nack 决策。
//
// requeue=true  → 路由到 Retry Topic
// requeue=false → 路由到 DLQ
func (q *kafkaQueue) handleNack(ctx context.Context, queueName string, msg *kafkaMessage, result queue.HandleResult) {
	if result.Requeue {
		_ = q.routeToRetry(ctx, queueName, msg.msg)
	} else {
		_ = q.routeToDLQ(ctx, queueName, msg.msg)
	}
}

// getRetryCount 从消息 headers 中提取当前重试次数。
func getRetryCount(headers []kafka.Header) int {
	for _, h := range headers {
		if h.Key == retryHeaderRetryCount {
			if n, err := strconv.Atoi(string(h.Value)); err == nil {
				return n
			}
		}
	}
	return 0
}

// computeBackoff 计算指数退避时间。
//
// 公式：min(initial × 2^(retryCount-1), maxBackoff)
// 如果 policy 为 nil，使用默认重试策略。
func computeBackoff(retryCount int, policy *queue.RetryPolicy) time.Duration {
	if policy == nil {
		policy = queue.DefaultRetryPolicy()
	}
	initial := policy.Backoff
	if initial <= 0 {
		initial = 1 * time.Second
	}
	maxBackoff := policy.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 60 * time.Second
	}

	backoff := float64(initial) * math.Pow(2, float64(retryCount-1))
	if backoff > float64(maxBackoff) {
		backoff = float64(maxBackoff)
	}
	return time.Duration(backoff)
}
