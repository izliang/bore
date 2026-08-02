// Package kafka 提供基于 Apache Kafka 的 queue.Queue 接口实现。
//
// 核心能力：
//   - 命名空间隔离：通过 Topic 前缀（{namespace}.{queueName}）实现多租户隔离
//   - 自动 topic 创建：Declare 首次调用时自动通过 Admin API 创建 topic
//   - 动态并发：支持 ConcurrencyController 运行时扩缩容
//   - 重试与死信：内置 Retry Topic + DLQ Topic 模式
//   - 偏移量管理：滑动窗口提交，适配逐条 Ack/Nack 语义
//   - 可观测性：可选 Prometheus 指标集成
//   - 优雅关闭：Shutdown 等待在途消息处理完毕后关闭连接
//
// 快速上手：
//
//	q, err := kafka.New(kafka.Options{
//	    Brokers:   []string{"localhost:9092"},
//	    Namespace: "production",
//	})
//	if err != nil { ... }
//	defer q.Shutdown(context.Background())
//
//	q.Publish(ctx, "orders", queue.NewMessage([]byte("hello")))
//	q.Subscribe(ctx, "orders", handler, &queue.SubscribeOptions{Concurrency: 3})
package kafka

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/propagator"
	"github.com/izliang/bore/queue"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/segmentio/kafka-go"
)

// Options 配置 Kafka 队列实例的所有参数。
type Options struct {
	// Brokers 是 Kafka broker 地址列表（必填），如 []string{"localhost:9092"}。
	Brokers []string

	// Namespace 是当前命名空间，等价于 RabbitMQ 的 vhost。
	// 在 topic 命名时作为前缀：{namespace}.{queueName}。
	// 为空时使用 "default"。
	Namespace string

	// AutoCreateTopics 控制是否在 Declare 时自动创建不存在的 topic。
	// 默认为 true。
	AutoCreateTopics bool

	// TLS 配置 Kafka 连接的 TLS。为 nil 时使用明文连接。
	TLS *tls.Config

	// SASLMechanism 是 SASL 认证机制：PLAIN, SCRAM-SHA-256, SCRAM-SHA-512。
	// 为空时不启用 SASL。
	SASLMechanism string

	// SASLUsername 是 SASL 认证用户名。
	SASLUsername string

	// SASLPassword 是 SASL 认证密码。
	SASLPassword string

	// DefaultPartitions 是自动创建 topic 时的默认分区数，默认为 3。
	DefaultPartitions int

	// DefaultReplicationFactor 是自动创建 topic 时的默认副本因子，默认为 1。
	DefaultReplicationFactor int

	// DefaultRetention 是自动创建 topic 时的默认消息保留时间，默认为 7 天。
	DefaultRetention time.Duration

	// ConsumerGroupPrefix 是消费组名的前缀，默认为空。
	// 消费组 ID 格式：{ConsumerGroupPrefix}:{namespace}:{queueName}
	ConsumerGroupPrefix string

	// RetryTopicSuffix 是重试 topic 的后缀，默认为 ".retry"。
	RetryTopicSuffix string

	// DLQTopicSuffix 是死信 topic 的后缀，默认为 ".dlq"。
	DLQTopicSuffix string

	// RetryMaxDelay 是重试消息的最大延迟时间，默认为 10 分钟。
	RetryMaxDelay time.Duration

	// ProducerBatchSize 是生产者批量发送的消息数，默认为 100。
	ProducerBatchSize int

	// ProducerBatchTimeout 是生产者批量发送的超时时间，默认为 100ms。
	ProducerBatchTimeout time.Duration

	// ConsumerMinBytes 是消费者每次拉取的最小字节数，默认为 1。
	ConsumerMinBytes int

	// ConsumerMaxBytes 是消费者每次拉取的最大字节数，默认为 10MB。
	ConsumerMaxBytes int

	// MaxAttempts 是生产者发送消息的最大重试次数，默认为 3。
	MaxAttempts int

	// SessionTimeout 是 Consumer Group session 超时时间，默认为 30 秒。
	SessionTimeout time.Duration

	// RebalanceTimeout 是 Consumer Group rebalance 超时时间，默认为 60 秒。
	RebalanceTimeout time.Duration

	// HeartbeatInterval 是 Consumer Group 心跳间隔，默认为 3 秒。
	HeartbeatInterval time.Duration

	// CommitInterval 是自动提交 offset 的间隔。
	// 为 0 时禁用自动提交，由 SDK 的滑动窗口管理。默认为 0。
	CommitInterval time.Duration

	// Logger 是用于运维日志记录的 logit.Logger 实例。
	Logger logit.Logger

	// MetricsRegistry 是可选的 Prometheus 注册表，用于注册队列指标。
	MetricsRegistry *prometheus.Registry

	// MetricsSubsystem 是指标子系统标签，默认为 "kafka"。
	MetricsSubsystem string

	// ConcurrencyController 提供全局的、按队列的动态并发度控制。
	ConcurrencyController queue.ConcurrencyController

	// Propagator 是链路传播器，用于在 Kafka headers 中注入/提取链路上下文。
	Propagator propagator.Propagator

	// DialTimeout 是连接 Kafka broker 的超时时间，默认为 10 秒。
	DialTimeout time.Duration

	// ClientID 是 Kafka 客户端标识，显示在 broker 日志中。
	ClientID string

	// AllowAutoTopicCreation 允许 broker 自动创建 topic（服务端配置），默认为 false。
	AllowAutoTopicCreation bool

	// Resolver 用于自定义 broker 主机名到 IP 地址的解析。
	//
	// 在 Docker/K8s 等容器环境中，broker 的 advertised.listeners 可能是内部主机名
	// （如 kafka:9092），客户端无法通过标准 DNS 解析。设置此字段可提供自定义映射。
	//
	// 使用示例（将所有 broker 解析为 localhost）：
	//   Resolver: kafka.NewBrokerResolver(&net.Resolver{
	//       PreferGo: true,
	//       Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
	//           d := net.Dialer{}
	//           return d.DialContext(ctx, network, "127.0.0.1:53")
	//       },
	//   })
	//
	// 为 nil 时使用 net.DefaultResolver（标准 DNS 解析）。
	Resolver kafka.BrokerResolver

	// ManagementHTTPClient 可选的 HTTP 客户端，预留用于未来 Kafka REST API 集成。
	ManagementHTTPClient *http.Client
}

// DefaultOptions 返回带有合理默认值的 Options。
func DefaultOptions() Options {
	return Options{
		Namespace:                "default",
		AutoCreateTopics:         true,
		DefaultPartitions:        3,
		DefaultReplicationFactor: 1,
		DefaultRetention:         7 * 24 * time.Hour,
		RetryTopicSuffix:         ".retry",
		DLQTopicSuffix:           ".dlq",
		RetryMaxDelay:            10 * time.Minute,
		ProducerBatchSize:        100,
		ProducerBatchTimeout:     100 * time.Millisecond,
		ConsumerMinBytes:         1,
		ConsumerMaxBytes:         10e6,
		MaxAttempts:              3,
		SessionTimeout:           30 * time.Second,
		RebalanceTimeout:         60 * time.Second,
		HeartbeatInterval:        3 * time.Second,
		DialTimeout:              10 * time.Second,
		MetricsSubsystem:         "kafka",
	}
}
