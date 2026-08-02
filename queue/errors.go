package queue

import "errors"

// 队列操作返回的标准哨兵错误。
// 调用方可使用 errors.Is 进行语义判断，避免依赖错误消息字符串。
var (
	// ErrQueueClosed 表示队列已关闭，无法执行任何操作。
	// 触发场景：Shutdown 之后调用 Publish / Subscribe / Declare 等。
	ErrQueueClosed = errors.New("queue: closed")

	// ErrNotInitialized 表示操作需要的前置初始化未完成。
	ErrNotInitialized = errors.New("queue: not initialized")

	// ErrPublishFailed 表示消息发布失败。
	// 可能原因：broker 不可达、连接断开、exchange 不存在等。
	ErrPublishFailed = errors.New("queue: publish failed")

	// ErrSubscribeFailed 表示消费者订阅失败。
	// 可能原因：broker 不可达、队列不存在且未自动声明、channel 异常等。
	ErrSubscribeFailed = errors.New("queue: subscribe failed")

	// ErrDeclareFailed 表示队列声明失败。
	// 可能原因：broker 不可达、参数不兼容（如修改了已存在队列的 durable 属性）等。
	ErrDeclareFailed = errors.New("queue: declare failed")

	// ErrConnectionFailed 表示无法建立与 broker 的连接。
	// 可能原因：网络不通、认证失败、vhost 不存在等。
	ErrConnectionFailed = errors.New("queue: connection failed")

	// ErrHandlerPanic 表示消息处理 handler 发生了 panic。
	// 框架已 recover，此错误用于日志和指标记录。
	ErrHandlerPanic = errors.New("queue: handler panicked")

	// ErrMaxRetriesExceeded 表示消息重试次数已达 RetryPolicy.MaxRetries 上限。
	// 此时消息被路由到 DLX 或丢弃，不再重试。
	ErrMaxRetriesExceeded = errors.New("queue: max retries exceeded")

	// ErrInvalidMessage 表示传入了 nil 或格式错误的消息。
	ErrInvalidMessage = errors.New("queue: invalid message")

	// ErrInvalidHandler 表示传入了 nil handler。
	ErrInvalidHandler = errors.New("queue: invalid handler")
)
