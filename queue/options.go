package queue

import "time"

// DeclareOptions 配置队列声明行为（创建或确保队列存在时的参数）。
//
// RabbitMQ 实现将这些选项映射到 AMQP queue.declare 的对应参数；
// local 实现中 MaxLength 用于设置 channel 缓冲区大小。
type DeclareOptions struct {
	// Durable 设置为 true 时队列在 broker 重启后依然存在。
	// 生产环境建议开启；local 实现无持久化，此参数被忽略。
	Durable bool

	// AutoDelete 设置为 true 时，最后一个消费者断开连接后队列自动删除。
	// 适用于临时队列场景。
	AutoDelete bool

	// Exclusive 设置为 true 时，队列只能被声明它的连接访问。
	// 连接断开后队列自动删除。
	Exclusive bool

	// NoWait 设置为 true 时，声明操作不等待 broker 确认即返回。
	// 仅在不需要保证队列已创建的场景下使用。
	NoWait bool

	// Arguments 是后端特定的额外参数（如 RabbitMQ 的 x-message-ttl 等）。
	// 优先使用下面的结构化字段，它们会自动转换为对应的 arguments。
	Arguments map[string]any

	// DeadLetterExchange 是死信队列绑定的 exchange。
	// 被拒绝或超时的消息会被路由到此 exchange。
	DeadLetterExchange string

	// DeadLetterRoutingKey 是死信消息的路由键。
	DeadLetterRoutingKey string

	// MessageTTL 是队列中消息的存活时间。
	// 超过此时间的消息会被自动死信或丢弃（取决于 broker 实现）。
	MessageTTL time.Duration

	// MaxLength 是队列中最大消息数。
	// 超过限制时，最旧的消息会被丢弃或死信。
	MaxLength int

	// MaxLengthBytes 是队列中消息体的最大总字节数。
	MaxLengthBytes int64
}

// DefaultDeclareOptions 返回合理的默认声明选项：持久化、非独占、非自动删除。
func DefaultDeclareOptions() *DeclareOptions {
	return &DeclareOptions{
		Durable:    true,
		AutoDelete: false,
		Exclusive:  false,
		NoWait:     false,
	}
}

// SubscribeOptions 配置消费者行为。
//
// 当 Queue 配置了 ConcurrencyController 时，Concurrency 字段作为回退值：
//   - Controller.Resolve(q) > 0 → 使用动态值
//   - 否则 → 使用 SubscribeOptions.Concurrency
//   - 两者均未设置 → 默认 1
type SubscribeOptions struct {
	// ConsumerTag 是消费者的唯一标识，broker 用它跟踪消费者状态。
	// 为空时自动生成（格式：<queue>-consumer-<timestamp>）。
	ConsumerTag string

	// AutoAck 为 true 时消息投递后立即自动确认，不等待 handler 返回值。
	// 生产环境建议设为 false（默认），由 handler 返回值驱动确认决策，
	// 以避免处理失败但消息已被确认丢失。
	AutoAck bool

	// Exclusive 为 true 时只有此消费者能访问队列。
	Exclusive bool

	// NoLocal 是 broker 特定参数：为 true 时，此连接发布的消息不会投递给此消费者。
	NoLocal bool

	// NoWait 为 true 时消费操作不等待 broker 确认。
	NoWait bool

	// Arguments 是后端特定的额外消费者参数。
	Arguments map[string]any

	// Concurrency 是并发处理消息的 goroutine 数量。
	//
	// 默认值为 1（串行处理）；< 1 的值会被自动修正为 1。
	//
	// 优先级：当 Queue 配置了 ConcurrencyController 且 Resolve 返回 > 0 时，
	// 此值被忽略，由 Controller 动态控制。
	Concurrency int

	// PrefetchCount 控制 RabbitMQ 的 QoS prefetch 数量（即客户端一次最多预取的消息数）。
	//
	// 默认值 = 0，表示自动计算：
	//   - 静态并发（无 Controller）：Concurrency × 2
	//   - 动态并发（有 Controller）：max(Concurrency × 4, 256)
	// 动态模式下使用较大的默认 prefetch 是为了避免扩容后受限于旧值。
	//
	// 手动设置此值可覆盖自动计算逻辑。
	PrefetchCount int

	// Retry 指定 handler 失败时的重试策略。
	//
	// 为 nil 时：handler 失败后执行一次 Nack(true)（重新入队），
	// 再次失败则 Nack(false)（丢弃或死信）。
	//
	// 建议生产环境显式配置 DefaultRetryPolicy 或自定义策略。
	Retry *RetryPolicy
}

// DefaultSubscribeOptions 返回合理的默认订阅选项。
func DefaultSubscribeOptions() *SubscribeOptions {
	return &SubscribeOptions{
		AutoAck:     false,
		Concurrency: 1,
	}
}

// RetryPolicy 描述 handler 失败时的重试行为。
type RetryPolicy struct {
	// MaxRetries 是重试次数上限。超过此次数后消息被死信或丢弃。
	// 设为 0 表示不重试，首次失败即进入死信。
	MaxRetries int

	// Backoff 是首次重试前的等待时间。后续每次重试间隔翻倍（指数退避）。
	Backoff time.Duration

	// MaxBackoff 是退避时间的上限，防止无限增长。
	MaxBackoff time.Duration

	// RetryQueue 是可选的独立重试队列名。
	// 为空时重试在原队列进行（通过 x-retry-count 头追踪次数）。
	// 不为空时重试消息发布到指定队列。
	RetryQueue string
}

// DefaultRetryPolicy 返回推荐的重试策略：最多重试 3 次，初始退避 1 秒，最大退避 60 秒。
// 每次退避附带 ±25% 的随机抖动以减少惊群效应。
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries: 3,
		Backoff:    1 * time.Second,
		MaxBackoff: 60 * time.Second,
	}
}
