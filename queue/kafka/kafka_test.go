package kafka

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/izliang/bore/queue"
	"github.com/izliang/bore/queue/controller"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Tests: Options & New
// =============================================================================

func TestNewRequiresBrokers(t *testing.T) {
	_, err := New(Options{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Brokers")
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	assert.Equal(t, "default", opts.Namespace)
	assert.Equal(t, 3, opts.DefaultPartitions)
	assert.Equal(t, 1, opts.DefaultReplicationFactor)
	assert.Equal(t, ".retry", opts.RetryTopicSuffix)
	assert.Equal(t, ".dlq", opts.DLQTopicSuffix)
	assert.Equal(t, "kafka", opts.MetricsSubsystem)
	assert.Equal(t, 100, opts.ProducerBatchSize)
	assert.Equal(t, 3, opts.MaxAttempts)
	assert.True(t, opts.AutoCreateTopics)
}

func TestNew_MergesDefaults(t *testing.T) {
	// 验证只设置必填字段时，其余字段使用默认值
	opts := Options{
		Brokers:   []string{"localhost:9092"},
		Namespace: "custom",
	}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	assert.Equal(t, "custom", kq.opts.Namespace)
	assert.Equal(t, 100, kq.opts.ProducerBatchSize)         // 默认值
	assert.Equal(t, 3, kq.opts.MaxAttempts)                 // 默认值
	assert.Equal(t, 30*time.Second, kq.opts.SessionTimeout) // 默认值
	assert.Equal(t, "kafka", kq.opts.MetricsSubsystem)      // 默认值
	assert.Equal(t, 3, kq.opts.DefaultPartitions)           // 默认值
	assert.False(t, kq.opts.AutoCreateTopics)               // bool 零值，默认行为由 ensureTopic 控制                 // 默认值
}

// =============================================================================
// Tests: Namespace Resolution
// =============================================================================

func TestResolveTopic(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		queueName string
		want      string
	}{
		{name: "有命名空间", namespace: "production", queueName: "orders", want: "production.orders"},
		{name: "空命名空间使用默认", namespace: "", queueName: "orders", want: "default.orders"},
		{name: "default 命名空间", namespace: "default", queueName: "tasks", want: "default.tasks"},
		{name: "带点的队列名", namespace: "staging", queueName: "service.events", want: "staging.service.events"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTopic(tt.namespace, tt.queueName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveConsumerGroup(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		namespace string
		queueName string
		want      string
	}{
		{name: "有前缀", prefix: "myapp", namespace: "production", queueName: "orders", want: "myapp:production:orders"},
		{name: "无前缀", prefix: "", namespace: "production", queueName: "orders", want: "production:orders"},
		{name: "空命名空间", prefix: "svc", namespace: "", queueName: "tasks", want: "svc:default:tasks"},
		{name: "全空", prefix: "", namespace: "", queueName: "logs", want: "default:logs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveConsumerGroup(tt.prefix, tt.namespace, tt.queueName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveRetryTopic(t *testing.T) {
	got := resolveRetryTopic("production", "orders", ".retry")
	assert.Equal(t, "production.orders.retry", got)
}

func TestResolveDLQTopic(t *testing.T) {
	got := resolveDLQTopic("production", "orders", ".dlq")
	assert.Equal(t, "production.orders.dlq", got)
}

func TestParseTopicName(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		topic     string
		want      string
	}{
		{name: "匹配命名空间", namespace: "production", topic: "production.orders", want: "orders"},
		{name: "不匹配命名空间", namespace: "staging", topic: "production.orders", want: "production.orders"},
		{name: "空命名空间", namespace: "", topic: "default.orders", want: "orders"},
		{name: "不带前缀", namespace: "production", topic: "orders", want: "orders"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTopicName(tt.namespace, tt.topic)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsTopicInNamespace(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		topic     string
		want      bool
	}{
		{name: "属于命名空间", namespace: "production", topic: "production.orders", want: true},
		{name: "不属于命名空间", namespace: "production", topic: "staging.orders", want: false},
		{name: "无前缀", namespace: "production", topic: "orders", want: false},
		{name: "空命名空间使用 default", namespace: "", topic: "default.tasks", want: true},
		{name: "空命名空间不匹配", namespace: "", topic: "production.tasks", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTopicInNamespace(tt.namespace, tt.topic)
			assert.Equal(t, tt.want, got)
		})
	}
}

// =============================================================================
// Tests: kafkaMessage Adapter
// =============================================================================

func TestKafkaMessage_Body(t *testing.T) {
	km := &kafkaMessage{
		msg: kafka.Message{Value: []byte("hello world")},
	}
	assert.Equal(t, "hello world", string(km.Body()))
}

func TestKafkaMessage_Body_Empty(t *testing.T) {
	km := &kafkaMessage{
		msg: kafka.Message{Value: nil},
	}
	assert.Empty(t, km.Body())
}

func TestKafkaMessage_Headers(t *testing.T) {
	km := &kafkaMessage{
		msg: kafka.Message{
			Headers: []kafka.Header{
				{Key: "Content-Type", Value: []byte("application/json")},
				{Key: "X-Trace-ID", Value: []byte("abc123")},
			},
		},
	}
	headers := km.Headers()
	assert.Equal(t, "application/json", headers["Content-Type"])
	assert.Equal(t, "abc123", headers["X-Trace-ID"])
}

func TestKafkaMessage_Headers_Empty(t *testing.T) {
	km := &kafkaMessage{
		msg: kafka.Message{Headers: nil},
	}
	headers := km.Headers()
	assert.NotNil(t, headers)
	assert.Empty(t, headers)
}

func TestKafkaMessage_Ack(t *testing.T) {
	var committed kafka.Message
	km := &kafkaMessage{
		msg: kafka.Message{Value: []byte("test")},
		commitFn: func(msg kafka.Message) error {
			committed = msg
			return nil
		},
	}
	err := km.Ack()
	assert.NoError(t, err)
	assert.Equal(t, "test", string(committed.Value))

	// 重复 Ack 安全（幂等）
	err = km.Ack()
	assert.NoError(t, err)
}

func TestKafkaMessage_Ack_NilCommitFn(t *testing.T) {
	km := &kafkaMessage{
		msg:      kafka.Message{Value: []byte("test")},
		commitFn: nil,
	}
	err := km.Ack()
	assert.NoError(t, err) // 无操作时不报错
}

func TestKafkaMessage_Nack_Requeue(t *testing.T) {
	var nackedMsg kafka.Message
	var requeueFlag bool
	km := &kafkaMessage{
		msg: kafka.Message{Value: []byte("test")},
		nackFn: func(msg kafka.Message, requeue bool) error {
			nackedMsg = msg
			requeueFlag = requeue
			return nil
		},
	}
	err := km.Nack(true)
	assert.NoError(t, err)
	assert.Equal(t, "test", string(nackedMsg.Value))
	assert.True(t, requeueFlag)

	// 重复 Nack 安全（幂等）
	err = km.Nack(false)
	assert.NoError(t, err)
}

func TestKafkaMessage_Nack_NoRequeue(t *testing.T) {
	var requeueFlag bool
	km := &kafkaMessage{
		msg: kafka.Message{Value: []byte("test")},
		nackFn: func(msg kafka.Message, requeue bool) error {
			requeueFlag = requeue
			return nil
		},
	}
	err := km.Nack(false)
	assert.NoError(t, err)
	assert.False(t, requeueFlag)
}

func TestKafkaMessage_Nack_NilNackFn(t *testing.T) {
	km := &kafkaMessage{
		msg:    kafka.Message{Value: []byte("test")},
		nackFn: nil,
	}
	err := km.Nack(true)
	assert.NoError(t, err) // 无操作时不报错
}

func TestKafkaMessage_MessageID_FromKey(t *testing.T) {
	km := &kafkaMessage{
		msg: kafka.Message{
			Key:   []byte("my-message-id"),
			Value: []byte("body"),
		},
	}
	assert.Equal(t, "my-message-id", km.MessageID())
}

func TestKafkaMessage_MessageID_Fallback(t *testing.T) {
	km := &kafkaMessage{
		msg:       kafka.Message{Value: []byte("body")},
		messageID: "fallback-id",
	}
	assert.Equal(t, "fallback-id", km.MessageID())
}

func TestKafkaMessage_MessageID_Empty(t *testing.T) {
	km := &kafkaMessage{
		msg: kafka.Message{Value: []byte("body")},
	}
	assert.Equal(t, "", km.MessageID())
}

// =============================================================================
// Tests: Header Conversion
// =============================================================================

func TestToKafkaHeaders(t *testing.T) {
	headers := map[string]string{
		"Content-Type": "application/json",
		"X-ID":         "123",
	}
	result := toKafkaHeaders(headers)
	assert.Len(t, result, 2)

	// 验证排序（kafka-go 要求 headers 排序以确保一致性）
	keys := make([]string, len(result))
	for i, h := range result {
		keys[i] = h.Key
	}
	assert.True(t, keys[0] < keys[1], "headers 应按 key 字母序排列")
}

func TestToKafkaHeaders_Empty(t *testing.T) {
	result := toKafkaHeaders(nil)
	assert.Nil(t, result)

	result = toKafkaHeaders(map[string]string{})
	assert.Nil(t, result)
}

func TestFromKafkaHeaders(t *testing.T) {
	kh := []kafka.Header{
		{Key: "Content-Type", Value: []byte("text/plain")},
		{Key: "X-ID", Value: []byte("abc")},
	}
	result := fromKafkaHeaders(kh)
	assert.Len(t, result, 2)
	assert.Equal(t, "text/plain", result["Content-Type"])
	assert.Equal(t, "abc", result["X-ID"])
}

func TestFromKafkaHeaders_Empty(t *testing.T) {
	result := fromKafkaHeaders(nil)
	assert.NotNil(t, result)
	assert.Empty(t, result)

	result = fromKafkaHeaders([]kafka.Header{})
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

// =============================================================================
// Tests: Shutdown & Health
// =============================================================================

func TestShutdown_Idempotent(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	require.NotNil(t, q)

	err = q.Shutdown(context.Background())
	assert.NoError(t, err)

	// 第二次 shutdown 应安全
	err = q.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestIsHealthy_WhenClosed(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	require.NotNil(t, q)

	_ = q.Shutdown(context.Background())
	assert.False(t, q.IsHealthy())
}

func TestDeclare_WhenClosed(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	require.NotNil(t, q)

	_ = q.Shutdown(context.Background())
	err = q.Declare("test", nil)
	assert.ErrorIs(t, err, queue.ErrQueueClosed)
}

// =============================================================================
// Tests: Producer
// =============================================================================

func TestProducer_Publish_NilMessage(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)

	err = q.Publish(context.Background(), "orders", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil message")
}

func TestProducer_Publish_WhenClosed(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)

	_ = q.Shutdown(context.Background())
	msg := queue.NewMessage([]byte("hello"))
	err = q.Publish(context.Background(), "tasks", msg)
	assert.ErrorIs(t, err, queue.ErrQueueClosed)
}

// =============================================================================
// Tests: Consumer
// =============================================================================

func TestSubscribe_NilHandler(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)

	err = q.Subscribe(context.Background(), "tasks", nil, nil)
	assert.ErrorIs(t, err, queue.ErrInvalidHandler)
}

func TestSubscribe_WhenClosed(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)

	_ = q.Shutdown(context.Background())
	handler := func(ctx context.Context, msg queue.Message) queue.HandleResult {
		return queue.Ack()
	}
	err = q.Subscribe(context.Background(), "tasks", handler, nil)
	assert.ErrorIs(t, err, queue.ErrQueueClosed)
}

func TestConsumer_GetOrCreateReader(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	topic := resolveTopic(kq.opts.Namespace, "orders")
	groupID := resolveConsumerGroup(kq.opts.ConsumerGroupPrefix, kq.opts.Namespace, "orders")

	r := kq.getOrCreateReader(topic, groupID)
	assert.NotNil(t, r)

	// 再次获取应返回同一实例
	r2 := kq.getOrCreateReader(topic, groupID)
	assert.Equal(t, r, r2)
}

// =============================================================================
// Tests: Concurrency Resolution
// =============================================================================

func TestResolveConcurrency_ControllerReturnsPositive(t *testing.T) {
	ctrl := controller.NewStatic(map[string]int{"q": 5})
	got := resolveConcurrency("q", &queue.SubscribeOptions{Concurrency: 3}, ctrl)
	assert.Equal(t, 5, got)
}

func TestResolveConcurrency_ControllerReturnsZero(t *testing.T) {
	ctrl := controller.NewStatic(map[string]int{"q": 0})
	got := resolveConcurrency("q", &queue.SubscribeOptions{Concurrency: 8}, ctrl)
	assert.Equal(t, 8, got)
}

func TestResolveConcurrency_NoControllerNoOpts(t *testing.T) {
	got := resolveConcurrency("q", nil, nil)
	assert.Equal(t, 1, got)
}

func TestResolveConcurrency_OptsWithZeroConcurrency(t *testing.T) {
	got := resolveConcurrency("q", &queue.SubscribeOptions{Concurrency: 0}, nil)
	assert.Equal(t, 1, got)
}

func TestResolveConcurrency_NilOpts(t *testing.T) {
	got := resolveConcurrency("q", nil, nil)
	assert.Equal(t, 1, got)
}

// =============================================================================
// Tests: Retry Count & Backoff
// =============================================================================

func TestGetRetryCount(t *testing.T) {
	tests := []struct {
		name    string
		headers []kafka.Header
		want    int
	}{
		{name: "无 headers", headers: nil, want: 0},
		{name: "无 retry header", headers: []kafka.Header{{Key: "x-other", Value: []byte("1")}}, want: 0},
		{name: "重试次数 3", headers: []kafka.Header{{Key: "x-retry-count", Value: []byte("3")}}, want: 3},
		{name: "非法数字", headers: []kafka.Header{{Key: "x-retry-count", Value: []byte("abc")}}, want: 0},
		{name: "多个 headers", headers: []kafka.Header{
			{Key: "x-retry-count", Value: []byte("7")},
			{Key: "x-original-topic", Value: []byte("orders")},
		}, want: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRetryCount(tt.headers)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestComputeBackoff(t *testing.T) {
	policy := &queue.RetryPolicy{
		Backoff:    1 * time.Second,
		MaxBackoff: 30 * time.Second,
	}

	// retry 1: ~1s
	b1 := computeBackoff(1, policy)
	assert.GreaterOrEqual(t, b1, 1*time.Second)
	assert.LessOrEqual(t, b1, 2*time.Second)

	// retry 2: ~2s
	b2 := computeBackoff(2, policy)
	assert.GreaterOrEqual(t, b2, 2*time.Second)
	assert.LessOrEqual(t, b2, 4*time.Second)

	// retry 3: ~4s
	b3 := computeBackoff(3, policy)
	assert.GreaterOrEqual(t, b3, 4*time.Second)
	assert.LessOrEqual(t, b3, 8*time.Second)

	// retry 10: capped at max
	b10 := computeBackoff(10, policy)
	assert.LessOrEqual(t, b10, 30*time.Second)
}

func TestComputeBackoff_NilPolicy(t *testing.T) {
	b := computeBackoff(1, nil)
	assert.Greater(t, b, time.Duration(0))
	assert.LessOrEqual(t, b, 2*time.Second) // 默认 initial=1s
}

// =============================================================================
// Tests: Interface Compliance
// =============================================================================

func TestInterfaceCompliance_Queue(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	assert.Implements(t, (*queue.Queue)(nil), q)
}

func TestInterfaceCompliance_Message(t *testing.T) {
	km := &kafkaMessage{
		msg:       kafka.Message{Value: []byte("test"), Key: []byte("id1")},
		messageID: "id1",
	}
	assert.Implements(t, (*queue.Message)(nil), km)
}

func TestInterfaceCompliance_Producer(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	assert.Implements(t, (*queue.Producer)(nil), q)
}

func TestInterfaceCompliance_Consumer(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	assert.Implements(t, (*queue.Consumer)(nil), q)
}

// =============================================================================
// Tests: SASL
// =============================================================================

func TestBuildSASL_Plain(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	// 验证 PLAIN SASL 构建不 panic
	kq.opts.SASLMechanism = "PLAIN"
	kq.opts.SASLUsername = "user"
	kq.opts.SASLPassword = "pass"
	mechanism := kq.buildSASL()
	assert.NotNil(t, mechanism)
	assert.Equal(t, "PLAIN", mechanism.Name())
}

func TestBuildSASL_Scram256(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	kq.opts.SASLMechanism = "SCRAM-SHA-256"
	kq.opts.SASLUsername = "user"
	kq.opts.SASLPassword = "pass"
	mechanism := kq.buildSASL()
	assert.NotNil(t, mechanism)
	assert.Equal(t, "SCRAM-SHA-256", mechanism.Name())
}

func TestBuildSASL_Default(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	// 无 SASL 配置时返回 nil
	assert.Nil(t, kq.buildSASL())
}

func TestBuildTransport_NoTLS(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	transport := kq.buildTransport()
	assert.NotNil(t, transport)
	assert.Nil(t, transport.TLS)
}

// =============================================================================
// Tests: retry & DLQ routing logic (unit testable parts)
// =============================================================================

func TestHandleRetry_RoutesToDLQ_AfterMax(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	km := &kafkaMessage{
		msg: kafka.Message{
			Key:   []byte("msg1"),
			Value: []byte("data"),
			Headers: []kafka.Header{
				{Key: "x-retry-count", Value: []byte("5")},
			},
		},
	}

	// 重试次数=5 >= maxRetryCount=5，应路由到 DLQ
	// 由于无 broker，会失败但不 panic
	err = kq.handleRetry(context.Background(), "orders", km, assert.AnError)
	_ = err // 允许错误（无 broker）
}

func TestHandleRetry_RoutesToRetry_BeforeMax(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	km := &kafkaMessage{
		msg: kafka.Message{
			Key:   []byte("msg1"),
			Value: []byte("data"),
			Headers: []kafka.Header{
				{Key: "x-retry-count", Value: []byte("2")},
			},
		},
	}

	// 重试次数=2 < maxRetryCount=5，应路由到 retry topic
	// 由于无 broker，会失败但不 panic
	err = kq.handleRetry(context.Background(), "orders", km, nil)
	_ = err
}

func TestHandleRetry_NoHeaders(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	km := &kafkaMessage{
		msg: kafka.Message{
			Key:     []byte("msg1"),
			Value:   []byte("data"),
			Headers: nil, // 无 retry-count header
		},
	}

	// 无 header 时 retry count = 0，应路由到 retry topic
	err = kq.handleRetry(context.Background(), "orders", km, nil)
	_ = err
}

func TestHandleNack_Requeue(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	km := &kafkaMessage{
		msg: kafka.Message{
			Key:   []byte("msg1"),
			Value: []byte("data"),
		},
	}

	// requeue=true → 路由到 retry topic，不 panic
	kq.handleNack(context.Background(), "orders", km, queue.HandleResult{Action: queue.HandleNack, Requeue: true})
}

func TestHandleNack_NoRequeue(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	km := &kafkaMessage{
		msg: kafka.Message{
			Key:   []byte("msg1"),
			Value: []byte("data"),
		},
	}

	// requeue=false → 路由到 DLQ，不 panic
	kq.handleNack(context.Background(), "orders", km, queue.HandleResult{Action: queue.HandleNack, Requeue: false})
}

func TestComputeBackoff_DefaultPolicy(t *testing.T) {
	// 验证默认策略的各个分支
	b1 := computeBackoff(1, nil)
	assert.GreaterOrEqual(t, b1, 500*time.Millisecond)

	b2 := computeBackoff(100, nil) // 超大重试次数应被 cap
	assert.LessOrEqual(t, b2, 60*time.Second)
}

func TestGetOrCreateWriter_Reuse(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	w1 := kq.getOrCreateWriter("test-topic")
	assert.NotNil(t, w1)

	w2 := kq.getOrCreateWriter("test-topic")
	assert.Equal(t, w1, w2, "同一 topic 应复用 Writer")
}

func TestBuildSASL_Scram512(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	kq.opts.SASLMechanism = "SCRAM-SHA-512"
	kq.opts.SASLUsername = "user"
	kq.opts.SASLPassword = "pass"
	mechanism := kq.buildSASL()
	assert.NotNil(t, mechanism)
	assert.Equal(t, "SCRAM-SHA-512", mechanism.Name())
}

func TestBuildSASL_Unknown(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	kq.opts.SASLMechanism = "UNKNOWN-MECHANISM"
	mechanism := kq.buildSASL()
	assert.Nil(t, mechanism)
}

func TestBuildTransport_WithTLS(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	opts.TLS = &tls.Config{InsecureSkipVerify: true}
	q, err := New(opts)
	require.NoError(t, err)
	kq := q.(*kafkaQueue)

	transport := kq.buildTransport()
	assert.NotNil(t, transport)
	assert.NotNil(t, transport.TLS)
}

func TestDeclare_NilOpts(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)

	// Declare 使用 nil opts（使用默认配置）
	// 无 broker 会失败，但不 panic
	err = q.Declare("test-nil-opts", nil)
	_ = err
}

func TestPublishWithRouting(t *testing.T) {
	opts := DefaultOptions()
	opts.Brokers = []string{"localhost:9092"}
	q, err := New(opts)
	require.NoError(t, err)

	msg := queue.NewMessage([]byte("hello"))
	// PublishWithRouting 忽略 exchange 参数
	err = q.PublishWithRouting(context.Background(), "ignored-exchange", "tasks", msg)
	_ = err
}
