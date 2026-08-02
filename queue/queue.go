// Package queue 提供通用的、后端无关的消息队列抽象。
//
// 本包定义统一接口（Producer、Consumer、Message、Queue）及生命周期管理，
// 使应用程序可以在 RabbitMQ、Redis Streams、Kafka、进程内 channel 等不同后端之间
// 自由切换，而无需修改业务代码。
//
// # 设计原则
//
//   - 面向接口：调用方依赖 Producer / Consumer / Queue 接口，而非具体后端实现
//   - Context 传播：所有操作接受 context.Context，支持超时、取消与链路上下文传递
//   - 并发安全：所有实现必须安全支持并发调用
//   - 优雅关闭：所有后端必须支持 Shutdown，确保在途消息处理完毕
//   - 可观测性：内置 Prometheus 指标集成
//   - 动态并发：通过 ConcurrencyController 接口实现运行时 worker 扩缩容
//
// # 快速上手
//
//	q, _ := rabbitmq.New(rabbitmq.Options{
//	    Host: "localhost", Username: "guest", Password: "guest",
//	})
//	defer q.Shutdown(context.Background())
//	q.Declare("tasks", queue.DefaultDeclareOptions())
//	q.Publish(ctx, "tasks", queue.NewMessage([]byte(`{"hello":"world"}`)))
//	q.Subscribe(ctx, "tasks", handler, &queue.SubscribeOptions{Concurrency: 4})
package queue

import "context"

// Message 表示队列中的一条消息。
//
// 实现者必须保证 Body 和 Headers 的并发安全（因为可能被多个 goroutine 读取）。
// 调用方不应同时调用 Ack 和 Nack——框架通过 atomic 标记防止重复确认。
type Message interface {
	// Body 返回消息体字节，调用方可自由修改返回的副本。
	Body() []byte

	// Headers 返回消息头（键值对），用于携带元数据（如 trace-id、Content-Type）。
	Headers() map[string]string

	// Ack 确认消息已成功处理，broker 从队列中删除该消息。
	// 重复调用安全（第二次及之后为无操作）。
	Ack() error

	// Nack 否定确认，告知 broker 消息处理失败。
	// requeue=true   → broker 立即重新投递（注意可能造成死循环）
	// requeue=false  → broker 路由到 DLX 或丢弃
	// 重复调用安全。
	Nack(requeue bool) error

	// MessageID 返回消息的唯一标识符。
	MessageID() string
}

// Producer 向队列或 exchange 发送消息。
type Producer interface {
	// Publish 向默认 exchange 发送消息，routingKey 同时作为队列名和路由键。
	// 需要按 topic 路由时，请使用 PublishWithRouting。
	//
	// ctx 用于超时和取消控制；msg 不能为 nil。
	Publish(ctx context.Context, routingKey string, msg Message) error

	// PublishWithRouting 向指定 exchange 发送消息，使用显式的 routingKey。
	// 适用于 fanout、topic、direct 等非默认 exchange 的路由场景。
	PublishWithRouting(ctx context.Context, exchange, routingKey string, msg Message) error
}

// HandleAction 描述 handler 返回后，队列实现应采取的动作。
type HandleAction int

const (
	// HandleAck 确认消息并从队列中删除。
	HandleAck HandleAction = iota

	// HandleRetry 要求队列实现按 SubscribeOptions.Retry 策略进行重试。
	// 重试次数超限后自动转为 Nack(false)，路由到 DLX 或丢弃。
	HandleRetry

	// HandleNack 否定确认，不使用 RetryPolicy，直接传递 requeue 参数给 broker。
	HandleNack
)

// HandleResult 是消费者 handler 返回的显式处理决策。
//
// 零值（HandleResult{}）等价于 Ack，因此 handler 可以直接 return 不关心确认的场景。
// Err 字段用于日志和指标上下文，不影响确认行为。
// Requeue 字段仅当 Action 为 HandleNack 时生效。
type HandleResult struct {
	Action  HandleAction // 期望的处理动作
	Err     error        // 关联的错误（用于日志/指标，不驱动行为）
	Requeue bool         // Nack 时的 requeue 参数
}

// Ack 返回一个表示"处理成功"的决策，消息从队列中删除。
func Ack() HandleResult {
	return HandleResult{Action: HandleAck}
}

// Retry 返回一个表示"需要按重试策略重试"的决策。
// 框架会根据 SubscribeOptions.Retry 进行指数退避重试，
// 重试次数超限后自动转为 Nack(false) 进入死信或丢弃。
func Retry(err error) HandleResult {
	return HandleResult{Action: HandleRetry, Err: err}
}

// Nack 返回一个否定确认决策，直接控制 requeue 参数。
// requeue=true  → broker 立即重新投递（注意可能无限循环）
// requeue=false → broker 路由到 DLX（若已配置）或丢弃
func Nack(err error, requeue bool) HandleResult {
	return HandleResult{Action: HandleNack, Err: err, Requeue: requeue}
}

// Drop 返回一个"丢弃/死信"决策，等价于 Nack(err, false)。
// 适用于消息格式错误、业务逻辑判定不可重试等永久失败场景。
func Drop(err error) HandleResult {
	return Nack(err, false)
}

// Requeue 返回一个"立即重新入队"的决策，等价于 Nack(err, true)。
//
// ⚠️ 谨慎使用：如果消息本身有逻辑问题导致持续失败，会造成无限循环。
// 仅在明确知道重新投递后能成功的场景下使用。
func Requeue(err error) HandleResult {
	return Nack(err, true)
}

// Handler 是消费者收到消息时的回调函数。
//
// 返回值决定消息的最终命运：
//   - Ack()              → 确认成功，消息从队列删除
//   - Retry(err)         → 按 RetryPolicy 指数退避重试
//   - Drop(err)          → 立即 Nack(false)，进入 DLX 或丢弃
//   - Requeue(err)       → 立即 Nack(true)，broker 重新投递
//   - Nack(err, requeue) → 直接控制 requeue 参数
//
// ctx 在消费者关闭时会被取消，handler 必须尊重 ctx.Done() 并尽快返回，
// 避免阻塞 shutdown 流程。
type Handler func(ctx context.Context, msg Message) HandleResult

// Consumer 从队列接收消息并分发给 Handler。
type Consumer interface {
	// Subscribe 开始消费指定队列的消息，阻塞直到 ctx 被取消或发生致命错误。
	//
	// 内部行为：
	//   - 根据 ConcurrencyController（若有）或 SubscribeOptions.Concurrency 启动 worker 池
	//   - 若有 ConcurrencyController，worker 数量会随 Watch 通知动态调整
	//   - 连接断开时自动重连并重新订阅（RabbitMQ 实现）
	//   - 返回 ctx.Err() 表示正常关闭，其他错误表示需要关注
	Subscribe(ctx context.Context, queueName string, handler Handler, opts *SubscribeOptions) error
}

// Queue 组合了 Producer 和 Consumer，并提供队列生命周期管理。
//
// 这是应用层最常使用的接口。通过 rabbitmq.New 或 local.New 创建实例后，
// 即可进行发布、消费、声明、健康检查和优雅关闭。
type Queue interface {
	Producer
	Consumer

	// Declare 创建或确保指定队列存在，使用给定的声明选项。
	// opts 为 nil 时使用默认值（持久化、非独占、非自动删除）。
	Declare(queueName string, opts *DeclareOptions) error

	// Shutdown 优雅关闭队列：停止接受新请求，取消所有消费者，关闭 broker 连接。
	// ctx 用于设置关闭超时，超时后强制终止。
	Shutdown(ctx context.Context) error

	// IsHealthy 返回底层连接是否存活（local 实现始终为 true）。
	IsHealthy() bool
}
