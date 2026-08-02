# Kafka Queue 实现

基于 `segmentio/kafka-go` 的 `queue.Queue` 接口实现，提供命名空间隔离、自动 Topic 创建和完整的消息生命周期管理。

## 快速上手

```go
import (
    "github.com/izliang/bore/queue"
    "github.com/izliang/bore/queue/kafka"
)

// 1. 创建 Queue 实例（连接到 Kafka 集群）
q, err := kafka.New(kafka.Options{
    Brokers:   []string{"localhost:9092"},
    Namespace: "production", // 命名空间隔离（等价于 RabbitMQ vhost）
})
if err != nil {
    panic(err)
}
defer q.Shutdown(context.Background())

// 2. 声明队列（自动创建 Topic）
q.Declare("orders", &queue.DeclareOptions{})

// 3. 发布消息
msg, _ := queue.NewJSONMessage(map[string]string{"order_id": "12345"})
q.Publish(ctx, "orders", msg)

// 4. 订阅消费（Consumer Group 负载均衡）
q.Subscribe(ctx, "orders", func(ctx context.Context, msg queue.Message) queue.HandleResult {
    var payload map[string]string
    if err := queue.ParseJSON(msg, &payload); err != nil {
        return queue.Drop(err) // 格式错误，直接丢弃
    }
    // 处理业务逻辑...
    return queue.Ack()
}, &queue.SubscribeOptions{
    Concurrency: 4, // 启动 4 个 worker goroutine
    Retry:       queue.DefaultRetryPolicy(),
})
```

## 命名空间隔离

Kafka 通过 **Topic 前缀** 实现命名空间隔离，等价于 RabbitMQ 的 vhost：

| 命名空间 | 队列名 | Kafka Topic 名 |
|----------|--------|----------------|
| `production` | `orders` | `production.orders` |
| `staging` | `orders` | `staging.orders` |
| `default` | `tasks` | `default.tasks` |

**Consumer Group ID** 格式：
- 有前缀：`{ConsumerGroupPrefix}:{namespace}:{queueName}`
- 无前缀：`{namespace}:{queueName}`

## Topic 命名规范

| Topic 类型 | 命名格式 | 示例 |
|-----------|---------|------|
| 主 Topic | `{namespace}.{queueName}` | `production.orders` |
| 重试 Topic | `{namespace}.{queueName}.retry` | `production.orders.retry` |
| 死信 Topic | `{namespace}.{queueName}.dlq` | `production.orders.dlq` |

## 重试与死信机制

当 Handler 返回 `queue.Retry(err)` 时：

1. 消息被发布到 **Retry Topic**（如 `production.orders.retry`）
2. 携带 `x-retry-count` header（递增计数）
3. 指数退避延迟（公式：`min(initial × 2^(n-1), maxBackoff)`）
4. 超过最大重试次数（默认 5 次）后，消息路由到 **DLQ Topic**（如 `production.orders.dlq`）

当 Handler 返回 `queue.Drop(err)` / `queue.Nack(err, false)` 时，消息直接进入 DLQ。

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `Brokers` | 必填 | Kafka broker 地址列表 |
| `Namespace` | `"default"` | 命名空间（Topic 前缀） |
| `AutoCreateTopics` | `true` | 是否自动创建 Topic |
| `DefaultPartitions` | `3` | 默认分区数 |
| `DefaultReplicationFactor` | `1` | 默认副本因子 |
| `DefaultRetention` | 7 天 | 默认消息保留时间 |
| `ProducerBatchSize` | `100` | 生产者批量大小 |
| `ProducerBatchTimeout` | `100ms` | 生产者批量超时 |
| `MaxAttempts` | `3` | 发送消息最大重试次数 |
| `SessionTimeout` | `30s` | Consumer Group session 超时 |
| `RebalanceTimeout` | `60s` | Consumer Group rebalance 超时 |
| `HeartbeatInterval` | `3s` | Consumer Group 心跳间隔 |
| `ConcurrencyController` | `nil` | 动态并发度控制器 |
| `SASLMechanism` | `""` | SASL 认证：`PLAIN`, `SCRAM-SHA-256`, `SCRAM-SHA-512` |
| `TLS` | `nil` | TLS 配置 |
| `Logger` | `nil` | 日志记录器 |
| `MetricsRegistry` | `nil` | Prometheus 注册表 |

## 并发控制

### 静态并发

```go
q.Subscribe(ctx, "orders", handler, &queue.SubscribeOptions{
    Concurrency: 8, // 固定 8 个 worker
})
```

### 动态并发（通过 ConcurrencyController）

```go
ctrl := controller.NewStatic(map[string]int{"orders": 4})
q, _ := kafka.New(kafka.Options{
    Brokers:                []string{"localhost:9092"},
    ConcurrencyController:  ctrl,
})

// 运行时动态调整
ctrl.Set("orders", 10) // 扩容到 10
ctrl.Set("orders", 2)  // 缩容到 2
```

**注意：** 实际并行度受 Topic 分区数限制。当 Concurrency > Partitions 时，多余 worker 会闲置。

## SASL 认证

```go
// PLAIN
q, _ := kafka.New(kafka.Options{
    Brokers:       []string{"broker:9092"},
    SASLMechanism: "PLAIN",
    SASLUsername:  "user",
    SASLPassword:  "pass",
})

// SCRAM-SHA-256
q, _ := kafka.New(kafka.Options{
    Brokers:       []string{"broker:9092"},
    SASLMechanism: "SCRAM-SHA-256",
    SASLUsername:  "user",
    SASLPassword:  "pass",
})
```

## TLS 连接

```go
q, _ := kafka.New(kafka.Options{
    Brokers: []string{"broker:9093"},
    TLS:     &tls.Config{},
})
```

## 异常处理

| 返回值 | 语义 | Kafka 行为 |
|--------|------|-----------|
| `queue.Ack()` | 处理成功 | 提交 offset |
| `queue.Retry(err)` | 指数退避重试 | 发布到 Retry Topic，递增 `x-retry-count` |
| `queue.Drop(err)` | 永久丢弃 | 发布到 DLQ Topic |
| `queue.Requeue(err)` | 立即重试 | 发布到 Retry Topic |
| `queue.Nack(err, false)` | 不重试 | 发布到 DLQ Topic |
| `queue.Nack(err, true)` | 立即重试 | 发布到 Retry Topic |

## 与 RabbitMQ 实现的差异

| 特性 | RabbitMQ | Kafka |
|------|----------|-------|
| 命名空间 | AMQP vhost（原生） | Topic 前缀（应用层实现） |
| 消息确认 | 逐条 Ack/Nack | Offset 提交（滑动窗口） |
| 死信机制 | DLX（原生） | DLQ Topic（应用层实现） |
| 重试实现 | Republish + header | Retry Topic 模式 |
| 并发上限 | 无硬限制 | 受 partition 数限制 |
| 消息顺序 | 无强保证 | 分区内严格有序 |
| Exchange 路由 | 支持多种 exchange | 无（忽略 exchange 参数） |

## 运行测试

```bash
# 纯单元测试（无需 Kafka broker）
go test ./queue/kafka/ -short -v

# 集成测试（需要 Kafka broker）
go test ./queue/kafka/ -v
```
