package kafka

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/izliang/bore/queue"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 端到端测试辅助函数
// =============================================================================

// kafkaBrokers 从环境变量 KAFKA_BROKERS 读取 broker 地址列表。
func kafkaBrokers() []string {
	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		return []string{v}
	}
	return []string{"localhost:9092"}
}

// testNamespace 返回用于测试的命名空间。
var testNamespace = fmt.Sprintf("e2e-test-%d", time.Now().UnixNano())

// k8sExternalResolver 将所有 broker 主机名解析为固定的外部 IP。
// 用于 K8s 部署场景：broker 的 advertised.listeners 是集群内部 Service 名，
// 但客户端通过 NodePort/LB 外部 IP 直连。
type k8sExternalResolver struct {
	externalIP string
}

func (r *k8sExternalResolver) LookupBrokerIPAddr(ctx context.Context, broker kafka.Broker) ([]net.IPAddr, error) {
	ip := net.ParseIP(r.externalIP)
	if ip == nil {
		return nil, &net.DNSError{
			Err:  "invalid external IP in KAFKA_EXTERNAL_IP",
			Name: r.externalIP,
		}
	}
	return []net.IPAddr{{IP: ip}}, nil
}

// newTestResolver 根据环境变量 KAFKA_EXTERNAL_IP 构建自定义 Resolver。
// 设置此环境变量后，所有 broker 主机名被强制解析为该 IP。
func newTestResolver() kafka.BrokerResolver {
	externalIP := os.Getenv("KAFKA_EXTERNAL_IP")
	if externalIP == "" {
		return nil
	}
	return &k8sExternalResolver{externalIP: externalIP}
}

// skipIfNoKafka 在短测试模式或无法连接 Kafka 时跳过测试。
func skipIfNoKafka(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("跳过需要 Kafka broker 的端到端测试（-short 模式）")
	}
	opts := DefaultOptions()
	opts.Brokers = kafkaBrokers()
	opts.DialTimeout = 2 * time.Second
	opts.Resolver = newTestResolver()
	q, err := New(opts)
	if err != nil {
		t.Skipf("跳过：无法连接 Kafka broker %v: %v", opts.Brokers, err)
	}
	_ = q.Shutdown(context.Background())
}

// newTestQueue 创建一个测试专用的 Queue 实例。
func newTestQueue(t *testing.T) queue.Queue {
	t.Helper()
	opts := DefaultOptions()
	opts.Brokers = kafkaBrokers()
	opts.Namespace = testNamespace
	opts.RetryMaxDelay = 30 * time.Second
	opts.DialTimeout = 5 * time.Second
	opts.SessionTimeout = 15 * time.Second
	opts.Resolver = newTestResolver()
	q, err := New(opts)
	require.NoError(t, err, "创建 Kafka Queue 实例失败")
	t.Cleanup(func() {
		_ = q.Shutdown(context.Background())
	})
	return q
}

// uniqueQueueName 生成唯一的队列名。
func uniqueQueueName(base string) string {
	return fmt.Sprintf("%s-%d", base, time.Now().UnixNano())
}

// =============================================================================
// 测试 1: 单一生产者 → 单一消费者
// =============================================================================

func TestE2E_ProducerToConsumer(t *testing.T) {
	skipIfNoKafka(t)
	q := newTestQueue(t)
	queueName := uniqueQueueName("basic")

	err := q.Declare(queueName, &queue.DeclareOptions{})
	require.NoError(t, err, "Declare 失败")

	msgCount := 10
	for i := 0; i < msgCount; i++ {
		body := fmt.Sprintf("message-%d", i)
		err := q.Publish(context.Background(), queueName, queue.NewMessage([]byte(body)))
		require.NoError(t, err, "Publish 消息 %d 失败", i)
	}
	t.Logf("成功发布 %d 条消息", msgCount)

	var received atomic.Int32
	var mu sync.Mutex
	receivedBodies := make(map[string]bool)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		_ = q.Subscribe(ctx, queueName, func(ctx context.Context, msg queue.Message) queue.HandleResult {
			received.Add(1)
			mu.Lock()
			receivedBodies[string(msg.Body())] = true
			mu.Unlock()
			return queue.Ack()
		}, &queue.SubscribeOptions{Concurrency: 2})
	}()

	assert.Eventually(t, func() bool {
		return received.Load() == int32(msgCount)
	}, 20*time.Second, 200*time.Millisecond, "应在超时前收到全部 %d 条消息", msgCount)

	mu.Lock()
	for i := 0; i < msgCount; i++ {
		expected := fmt.Sprintf("message-%d", i)
		assert.True(t, receivedBodies[expected], "缺少消息: %s", expected)
	}
	mu.Unlock()
	t.Logf("消费者确认收到全部 %d 条消息", msgCount)
}

// =============================================================================
// 测试 2: 独立生产者
// =============================================================================

func TestE2E_Producer(t *testing.T) {
	skipIfNoKafka(t)
	q := newTestQueue(t)
	queueName := uniqueQueueName("producer-test")

	err := q.Declare(queueName, nil)
	require.NoError(t, err)

	msg := queue.NewMessage([]byte("hello-from-producer"))
	err = q.Publish(context.Background(), queueName, msg)
	require.NoError(t, err)
	t.Logf("生产普通消息成功: %s", msg.MessageID())

	jsonMsg, err := queue.NewJSONMessage(map[string]any{
		"user_id": 12345,
		"action":  "order.created",
		"amount":  99.99,
	})
	require.NoError(t, err)
	err = q.Publish(context.Background(), queueName, jsonMsg)
	require.NoError(t, err)
	t.Logf("生产 JSON 消息成功: %s", jsonMsg.MessageID())

	err = q.PublishWithRouting(context.Background(), "ignored", queueName,
		queue.NewMessage([]byte("via-routing")))
	require.NoError(t, err)

	batchSize := 50
	for i := 0; i < batchSize; i++ {
		body := fmt.Sprintf("batch-msg-%04d", i)
		err := q.Publish(context.Background(), queueName, queue.NewMessage([]byte(body)))
		if err != nil {
			t.Fatalf("批量生产第 %d 条失败: %v", i, err)
		}
	}
	t.Logf("批量生产 %d 条消息完成", batchSize)
}

// =============================================================================
// 测试 3: 独立消费者
// =============================================================================

func TestE2E_Consumer(t *testing.T) {
	skipIfNoKafka(t)
	q := newTestQueue(t)
	queueName := uniqueQueueName("consumer-test")

	err := q.Declare(queueName, nil)
	require.NoError(t, err)

	preloadCount := 20
	for i := 0; i < preloadCount; i++ {
		body := fmt.Sprintf("consumer-test-msg-%02d", i)
		err := q.Publish(context.Background(), queueName, queue.NewMessage([]byte(body)))
		require.NoError(t, err)
	}
	t.Logf("预加载 %d 条消息", preloadCount)

	var acked, nacked, retried, dropped atomic.Int32

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		_ = q.Subscribe(ctx, queueName, func(ctx context.Context, msg queue.Message) queue.HandleResult {
			body := string(msg.Body())
			switch {
			case body == "consumer-test-msg-00":
				dropped.Add(1)
				return queue.Drop(fmt.Errorf("业务拒绝"))
			case body == "consumer-test-msg-01":
				nacked.Add(1)
				return queue.Requeue(fmt.Errorf("需要重新处理"))
			case body == "consumer-test-msg-02" && retried.Load() == 0:
				retried.Add(1)
				return queue.Retry(fmt.Errorf("临时错误"))
			default:
				acked.Add(1)
				return queue.Ack()
			}
		}, &queue.SubscribeOptions{Concurrency: 2})
	}()

	assert.Eventually(t, func() bool {
		return acked.Load() > 0
	}, 15*time.Second, 200*time.Millisecond, "应在超时前收到消息")

	t.Logf("消费统计: ack=%d, retry=%d, nack=%d, drop=%d",
		acked.Load(), retried.Load(), nacked.Load(), dropped.Load())
}

// =============================================================================
// 测试 4: JSON 消息往返
// =============================================================================

func TestE2E_JSONMessageRoundTrip(t *testing.T) {
	skipIfNoKafka(t)
	q := newTestQueue(t)
	queueName := uniqueQueueName("json")

	err := q.Declare(queueName, nil)
	require.NoError(t, err)

	type OrderPayload struct {
		OrderID  string  `json:"order_id"`
		UserID   int64   `json:"user_id"`
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	}
	original := OrderPayload{OrderID: "ORD-001", UserID: 10086, Amount: 299.99, Currency: "CNY"}

	msg, err := queue.NewJSONMessage(original)
	require.NoError(t, err)
	require.NoError(t, q.Publish(context.Background(), queueName, msg))

	var decoded OrderPayload
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = q.Subscribe(ctx, queueName, func(ctx context.Context, msg queue.Message) queue.HandleResult {
			if err := queue.ParseJSON(msg, &decoded); err != nil {
				t.Errorf("JSON 反序列化失败: %v", err)
				return queue.Drop(err)
			}
			return queue.Ack()
		}, nil)
	}()
	wg.Wait()

	assert.Equal(t, original.OrderID, decoded.OrderID)
	assert.Equal(t, original.UserID, decoded.UserID)
	assert.Equal(t, original.Amount, decoded.Amount)
	t.Logf("JSON 往返成功: %+v", decoded)
}

// =============================================================================
// 测试 5: 声明周期
// =============================================================================

func TestE2E_Lifecycle(t *testing.T) {
	skipIfNoKafka(t)

	opts := DefaultOptions()
	opts.Brokers = kafkaBrokers()
	opts.Namespace = testNamespace + "-lifecycle"
	opts.DialTimeout = 5 * time.Second
	opts.Resolver = newTestResolver()

	q, err := New(opts)
	require.NoError(t, err)

	assert.True(t, q.IsHealthy())
	queueName := uniqueQueueName("lifecycle")
	err = q.Declare(queueName, nil)
	require.NoError(t, err)

	msg := queue.NewMessage([]byte("lifecycle-test"))
	err = q.Publish(context.Background(), queueName, msg)
	require.NoError(t, err)
	assert.True(t, q.IsHealthy())

	err = q.Shutdown(context.Background())
	require.NoError(t, err)
	assert.False(t, q.IsHealthy())

	err = q.Publish(context.Background(), queueName, msg)
	assert.ErrorIs(t, err, queue.ErrQueueClosed)
	t.Log("声明周期测试通过")
}
