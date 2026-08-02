package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/izliang/bore/queue"
	"github.com/segmentio/kafka-go"
)

// kafkaQueue 是基于 Apache Kafka 的 queue.Queue 接口实现。
//
// 核心特性：
//   - 命名空间隔离：Topic 前缀 {namespace}.{queueName}
//   - 自动 Topic 创建：Declare 首次调用时通过 Admin API 创建
//   - 动态并发：支持 ConcurrencyController 运行时扩缩容
//   - 重试与死信：Retry Topic + DLQ Topic 模式
//   - Writer/Reader 复用：同一 topic 共享 Writer 实例，同一 consumer group 共享 Reader 实例
type kafkaQueue struct {
	opts    Options
	admin   *kafka.Client
	writers sync.Map // map[string]*kafka.Writer
	readers sync.Map // map[string]*kafka.Reader
	closed  bool
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// New 创建一个新的 Kafka Queue 实例并连接到 Kafka broker。
//
// 仅建立元数据管理连接，实际的 Producer/Consumer 连接在首次操作时延迟建立。
// 如果 AutoCreateTopics 为 true，Declare 会自动创建不存在的 topic。
func New(opts Options) (queue.Queue, error) {
	if len(opts.Brokers) == 0 {
		return nil, fmt.Errorf("kafka: Brokers 不能为空")
	}

	// 使用 DefaultOptions 填充缺失的默认值
	def := DefaultOptions()
	if opts.Namespace == "" {
		opts.Namespace = def.Namespace
	}
	if opts.DefaultPartitions <= 0 {
		opts.DefaultPartitions = def.DefaultPartitions
	}
	if opts.DefaultReplicationFactor <= 0 {
		opts.DefaultReplicationFactor = def.DefaultReplicationFactor
	}
	if opts.DefaultRetention <= 0 {
		opts.DefaultRetention = def.DefaultRetention
	}
	if opts.RetryTopicSuffix == "" {
		opts.RetryTopicSuffix = def.RetryTopicSuffix
	}
	if opts.DLQTopicSuffix == "" {
		opts.DLQTopicSuffix = def.DLQTopicSuffix
	}
	if opts.RetryMaxDelay <= 0 {
		opts.RetryMaxDelay = def.RetryMaxDelay
	}
	if opts.ProducerBatchSize <= 0 {
		opts.ProducerBatchSize = def.ProducerBatchSize
	}
	if opts.ProducerBatchTimeout <= 0 {
		opts.ProducerBatchTimeout = def.ProducerBatchTimeout
	}
	if opts.ConsumerMinBytes <= 0 {
		opts.ConsumerMinBytes = def.ConsumerMinBytes
	}
	if opts.ConsumerMaxBytes <= 0 {
		opts.ConsumerMaxBytes = def.ConsumerMaxBytes
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = def.MaxAttempts
	}
	if opts.SessionTimeout <= 0 {
		opts.SessionTimeout = def.SessionTimeout
	}
	if opts.RebalanceTimeout <= 0 {
		opts.RebalanceTimeout = def.RebalanceTimeout
	}
	if opts.HeartbeatInterval <= 0 {
		opts.HeartbeatInterval = def.HeartbeatInterval
	}
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = def.DialTimeout
	}

	if opts.MetricsSubsystem == "" {
		opts.MetricsSubsystem = def.MetricsSubsystem
	}

	ctx, cancel := context.WithCancel(context.Background())

	transport := buildTransportFromOpts(opts)

	admin := &kafka.Client{
		Addr:      kafka.TCP(opts.Brokers...),
		Timeout:   opts.DialTimeout,
		Transport: transport,
	}

	q := &kafkaQueue{
		opts:   opts,
		admin:  admin,
		ctx:    ctx,
		cancel: cancel,
	}

	return q, nil
}

// Declare 创建或确保指定队列（Kafka topic）存在。
//
// 如果 opts 为 nil，使用默认配置（分区数、副本因子、保留时间）。
// 如果 AutoCreateTopics 为 true 且 topic 不存在，自动创建。
// 如果 topic 已存在，该操作为无操作（幂等）。
func (q *kafkaQueue) Declare(queueName string, opts *queue.DeclareOptions) error {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return queue.ErrQueueClosed
	}
	q.mu.RUnlock()

	topicName := resolveTopic(q.opts.Namespace, queueName)
	return q.ensureTopic(context.Background(), topicName, opts)
}

// Shutdown 优雅关闭队列：停止所有消费者，关闭所有生产者，断开 broker 连接。
//
// 支持重复调用（幂等）。
func (q *kafkaQueue) Shutdown(ctx context.Context) error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil
	}
	q.closed = true
	q.mu.Unlock()

	q.cancel()

	// 关闭所有 writer
	q.writers.Range(func(key, value interface{}) bool {
		if w, ok := value.(*kafka.Writer); ok {
			_ = w.Close()
		}
		return true
	})

	// 关闭所有 reader
	q.readers.Range(func(key, value interface{}) bool {
		if r, ok := value.(*kafka.Reader); ok {
			_ = r.Close()
		}
		return true
	})

	return nil
}

// IsHealthy 返回与 Kafka broker 的连接是否存活。
//
// 通过查询 broker 元数据来验证连接状态。closed 状态下始终返回 false。
func (q *kafkaQueue) IsHealthy() bool {
	q.mu.RLock()
	if q.closed {
		q.mu.RUnlock()
		return false
	}
	q.mu.RUnlock()

	ctx, cancel := context.WithTimeout(q.ctx, 2*time.Second)
	defer cancel()

	_, err := q.admin.Metadata(ctx, &kafka.MetadataRequest{})
	return err == nil
}

// 编译期接口实现校验。
var _ queue.Queue = (*kafkaQueue)(nil)
