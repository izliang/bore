// Package queue 提供消息队列统一抽象，本文件定义动态并发控制接口。
package queue

// ConcurrencyController 是全局消费者并发度控制器的标准协议。
//
// 外部系统（配置中心、Admin API、自动伸缩策略、运维平台等）通过实现此接口，
// 即可在运行时动态调整各队列的消费者并发度，无需重启服务或重新调用 Subscribe。
//
// 一个 Controller 实例可被多个 Queue 实例共享，实现跨服务的统一并发管控。
//
// # 并发度解析优先级（在 Subscribe 内部生效）
//
//  1. ConcurrencyController.Resolve(queueName) — 返回值 > 0 时优先采用（动态控制）
//  2. SubscribeOptions.Concurrency — Controller 未设置或返回 ≤ 0 时的静态回退值
//  3. 默认值 1 — 以上两者均未设置时的兜底
//
// # 实现要求
//
//   - Resolve 和 Watch 必须并发安全（会被多个 goroutine 并发调用）
//   - Watch 返回的 channel 关闭时，表示 Controller 主动终止了对该队列的控制
//   - stop 函数必须幂等（重复调用安全），用于取消订阅并释放 Controller 内部资源
//   - Watch 可被同一个队列多次调用（多个 consumer 各自订阅），Controller 需支持多 watcher
type ConcurrencyController interface {
	// Resolve 返回指定队列的期望并发数（即期望的 worker goroutine 数量）。
	//
	// 返回值语义：
	//   > 0  — 使用该值作为并发度
	//   = 0  — 表示不控制该队列，回退到 SubscribeOptions.Concurrency
	//   < 0  — 同 = 0，回退到 SubscribeOptions.Concurrency
	//
	// 每次 Subscribe 启动时调用一次以获取初始并发度。
	Resolve(queueName string) int

	// Watch 订阅指定队列的并发度变更通知。
	//
	// 返回值：
	//   <-chan int — 当并发度发生变化时，channel 会收到新的目标值。
	//                 消费方（consumer）读取后立即调整 worker 数量。
	//   func()     — 停止订阅的回调。调用后 Controller 释放与该 watcher 关联的资源，
	//                 channel 不再接收新值，consumer 不应再读取该 channel。
	//
	// Channel 契约：
	//   - 推荐使用 cap=1 的带缓冲 channel
	//   - 发送方应采用非阻塞发送（drain-and-replace），确保不阻塞 Set 调用方
	//   - 只保证最终一致性：快速连续变更时，consumer 收到的是最新的目标值
	//   - channel 由 Controller 持有，Controller 关闭时 channel 也会被关闭
	Watch(queueName string) (<-chan int, func())
}
