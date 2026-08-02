package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/izliang/bore/queue"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

// Publish 向默认 topic 发送消息。routingKey 即为队列名，自动添加命名空间前缀。
func (q *kafkaQueue) Publish(ctx context.Context, routingKey string, msg queue.Message) error {
	return q.PublishWithRouting(ctx, "", routingKey, msg)
}

// PublishWithRouting 向指定 topic 发送消息。
// Kafka 无 exchange 概念，exchange 参数被忽略。
// routingKey 即为队列名，会自动添加命名空间前缀。
func (q *kafkaQueue) PublishWithRouting(ctx context.Context, exchange, routingKey string, msg queue.Message) error {
	if msg == nil {
		return fmt.Errorf("%w: nil message", queue.ErrInvalidMessage)
	}

	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return queue.ErrQueueClosed
	}
	q.mu.RUnlock()

	topicName := resolveTopic(q.opts.Namespace, routingKey)
	writer := q.getOrCreateWriter(topicName)

	kmsg := kafka.Message{
		Topic:   topicName,
		Key:     []byte(msg.MessageID()),
		Value:   msg.Body(),
		Headers: toKafkaHeaders(msg.Headers()),
		Time:    time.Now(),
	}

	return writer.WriteMessages(ctx, kmsg)
}

// getOrCreateWriter 获取或创建指定 topic 的 Writer 实例。
func (q *kafkaQueue) getOrCreateWriter(topic string) *kafka.Writer {
	if w, ok := q.writers.Load(topic); ok {
		return w.(*kafka.Writer)
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(q.opts.Brokers...),
		Topic:        topic,
		Balancer:     &kafka.Hash{},
		BatchSize:    q.opts.ProducerBatchSize,
		BatchTimeout: q.opts.ProducerBatchTimeout,
		MaxAttempts:  q.opts.MaxAttempts,
		Async:        false,
		RequiredAcks: kafka.RequireOne,
		Transport:    buildTransportFromOpts(q.opts),
	}

	actual, _ := q.writers.LoadOrStore(topic, writer)
	return actual.(*kafka.Writer)
}

// buildTransportFromOpts 从 Options 构建 kafka.Transport。
// 不依赖 kafkaQueue 实例，供 New() 中的 admin Client 和 Writer 复用。
func buildTransportFromOpts(opts Options) *kafka.Transport {
	t := &kafka.Transport{
		DialTimeout: opts.DialTimeout,
	}
	if opts.Resolver != nil {
		t.Resolver = opts.Resolver
	}
	if opts.TLS != nil {
		t.TLS = opts.TLS
	}
	if opts.SASLMechanism != "" {
		switch opts.SASLMechanism {
		case "PLAIN":
			t.SASL = plain.Mechanism{
				Username: opts.SASLUsername,
				Password: opts.SASLPassword,
			}
		case "SCRAM-SHA-256":
			m, _ := scram.Mechanism(scram.SHA256, opts.SASLUsername, opts.SASLPassword)
			t.SASL = m
		case "SCRAM-SHA-512":
			m, _ := scram.Mechanism(scram.SHA512, opts.SASLUsername, opts.SASLPassword)
			t.SASL = m
		}
	}
	return t
}

// buildTransport 委托给 buildTransportFromOpts，供测试验证。
func (q *kafkaQueue) buildTransport() *kafka.Transport {
	return buildTransportFromOpts(q.opts)
}

// buildSASL 返回 SASL 认证机制，供测试验证。
func (q *kafkaQueue) buildSASL() sasl.Mechanism {
	return buildTransportFromOpts(q.opts).SASL
}
