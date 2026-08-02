# queue — 通用消息队列抽象

`queue` 包是 `bore` 提供的消息队列统一抽象层，内置 RabbitMQ（AMQP 0-9-1）与进程内（local）两种实现。
业务代码应依赖 `queue.Queue` / `queue.Producer` / `queue.Consumer` / `queue.Message` 接口，而不是直接耦合底层
broker 的具体 API（如 `amqp.Channel`），从而在生产、测试、本地开发中自由切换后端。

相较于 `/cybergo/mq` 包，`queue` 不再使用全局单例和单个 AMQP channel，而是提供面向并发、可观测、
可关闭、可重连的队列能力，适合长期运行的服务和高并发生产消费场景。

---

## 核心能力

| 能力 | 说明 |
|------|------|
| 后端无关 | 业务层依赖统一接口，可在 RabbitMQ、local 或未来实现之间切换 |
| Context 传播 | 发布、消费、关闭都接受 `context.Context`，支持取消、超时和链路上下文注入 |
| 并发安全 | RabbitMQ 发布使用 channel pool 并发复用；local 使用 mutex + channel 保护并发访问 |
| 显式确认决策 | handler 返回 `HandleResult`，可选择 Ack、Retry（按策略重试）、Drop（死信/丢弃）或 Requeue（立即重新入队） |
| Panic 保护 | handler panic 会被 recover，按失败消息处理，不会打崩消费 goroutine |
| 自动重连 | RabbitMQ 连接断开后指数退避重连，消费者自动重新声明队列并重新订阅 |
| **动态并发控制** | 通过 `ConcurrencyController` 接口，外部系统可实时调整各队列的消费者并发度，无需重启 |
| 并发消费 | `SubscribeOptions.Concurrency` 控制 worker 数，`PrefetchCount` 可覆盖默认 RabbitMQ QoS |
| 重试与死信 | 支持指数退避、抖动、`x-retry-count` 头追踪、DLX / DLQ 配置 |
| Exchange 路由 | 支持 `PublishWithRouting`，RabbitMQ 实现还提供 `DeclareExchange` / `BindQueue` 便捷方法 |
| vhost 处理 | 支持根据环境变量改写 vhost，可在 vhost 缺失时通过 Management API 自动创建 |
| 可观测性 | 可选 Prometheus metrics，重复注册同名 collector 时自动复用，避免 panic |
| 安全关闭 | `Shutdown(ctx)` 拒绝新请求、取消消费者并关闭底层连接资源 |
| 本地实现 | `queue/local` 无外部依赖，适合单元测试、本地调试和原型验证 |

---

## 包结构

```text
queue/
├── queue.go             核心接口定义：Producer / Consumer / Queue / Handler / HandleResult
├── message.go           消息构造：NewMessage / NewJSONMessage / MessageBuilder / ParseJSON
├── options.go           配置结构：DeclareOptions / SubscribeOptions / RetryPolicy
├── concurrency.go       动态并发控制接口：ConcurrencyController
├── errors.go            标准哨兵错误
├── metrics.go           Prometheus 指标定义
├── controller/          内置并发控制器实现
│   └── controller.go    StaticController — 基于内存的可 Set 并发控制器
├── local/               进程内 channel 实现，适合测试和本地开发
│   ├── local.go
│   └── message.go
└── rabbitmq/            RabbitMQ AMQP 0-9-1 实现
    ├── rabbitmq.go
    ├── connection.go
    ├── consumer.go
    └── producer.go
```

---

## 快速开始：RabbitMQ

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/izliang/bore/queue"
    "github.com/izliang/bore/queue/rabbitmq"
)

func main() {
    q, err := rabbitmq.New(rabbitmq.Options{
        Host:             "localhost",
        Port:             5672,
        Username:         "guest",
        Password:         "guest",
        Vhost:            "my-vhost",
        ProducerPoolSize: 10,
        ReconnectInitial: time.Second,
        ReconnectMax:     30 * time.Second,
    })
    if err != nil {
        log.Fatal(err)
    }

    defer func() {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        _ = q.Shutdown(ctx)
    }()

    if err := q.Declare("tasks", queue.DefaultDeclareOptions()); err != nil {
        log.Fatal(err)
    }

    msg, err := queue.NewJSONMessage(map[string]any{"id": 1, "action": "process"})
    if err != nil {
        log.Fatal(err)
    }

    publishCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    if err := q.Publish(publishCtx, "tasks", msg); err != nil {
        log.Fatal(err)
    }

    // 消费端：订阅 + 重试 + 并发控制
    err = q.Subscribe(context.Background(), "tasks", func(ctx context.Context, msg queue.Message) queue.HandleResult {
        var payload map[string]any
        if err := queue.ParseJSON(msg, &payload); err != nil {
            return queue.Drop(err) // 格式错误 → 直接死信
        }
        if err := process(ctx, payload); err != nil {
            return queue.Retry(err) // 临时失败 → 按策略重试
        }
        return queue.Ack()
    }, &queue.SubscribeOptions{
        ConsumerTag: "worker-1",
        Concurrency: 4,
        Retry:       queue.DefaultRetryPolicy(),
    })
    if err != nil {
        log.Fatal(err)
    }
}
```

---

## 快速开始：local

`local` 实现不依赖任何外部 broker，完全基于 Go channel，适合单元测试、本地调试和原型验证。

```go
q := local.New(local.Options{DefaultBufferSize: 1024})
defer q.Shutdown(context.Background())

_ = q.Declare("tasks", nil)
_ = q.Publish(context.Background(), "tasks", queue.NewMessage([]byte("hello")))

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

err := q.Subscribe(ctx, "tasks", func(ctx context.Context, msg queue.Message) queue.HandleResult {
    fmt.Println(string(msg.Body()))
    return queue.Ack()
}, nil)
```

**local 与真实 broker 的区别**：

| 特性 | local | RabbitMQ |
|------|-------|----------|
| 消息持久化 | 无，进程退出即丢失 | 支持 durable 队列和持久化消息 |
| 网络延迟 | 无，瞬时投递 | 有网络往返 |
| 死信队列 | Nack(false) 直接丢弃 | 支持 DLX 路由到死信队列 |
| Exchange 路由 | 仅按队列名投递 | 支持 exchange + routing key |
| 集群/HA | 不支持 | 支持镜像队列、Quorum Queue |

---

## 动态并发控制

`ConcurrencyController` 是 v1.1 引入的核心能力，允许外部系统（配置中心、Admin API、自动伸缩策略）
在运行时动态调整消费者并发度，无需重启服务或重新订阅。

### 接口定义

```go
// queue/concurrency.go
type ConcurrencyController interface {
    // 返回指定队列的期望并发数，返回 ≤ 0 表示不控制，回退到静态配置
    Resolve(queueName string) int

    // 订阅并发度变更通知，返回的 channel 在变更时收到新值
    // stop 函数用于取消订阅、释放资源
    Watch(queueName string) (<-chan int, func())
}
```

### 并发度解析优先级

```
1. ConcurrencyController.Resolve(q) > 0  → 动态控制
2. SubscribeOptions.Concurrency > 0      → 调用方静态指定
3. 默认值 1
```

### 使用示例

```go
import "github.com/izliang/bore/queue/controller"

// 创建控制器，设定初始并发度
ctrl := controller.NewStatic(map[string]int{
    "order.queue":  10,
    "notify.queue": 5,
})

q, _ := rabbitmq.New(rabbitmq.Options{
    ConcurrencyController: ctrl,
    // ... 其他配置
})

// 订阅时传入 nil opts 或仅指定 Retry，并发度由 Controller 接管
q.Subscribe(ctx, "order.queue", handler, &queue.SubscribeOptions{
    Retry: queue.DefaultRetryPolicy(),
})

// 运行时动态调整——所有 consumer 毫秒级生效（扩缩容不中断消费）
ctrl.Set("order.queue", 20) // 扩容到 20 个 worker
ctrl.Set("order.queue", 5)  // 缩容到 5 个 worker
```

### 扩缩容机制

- **扩容**：直接追加新的 worker goroutine，共享同一个 delivery channel
- **缩容**：取消尾部 worker 的 context，goroutine 在完成当前消息后优雅退出
- **不阻塞**：`StaticController.Set` 使用 cap=1 的 buffered channel + drain-replace 策略，永不阻塞调用方
- **最终一致性**：channel 只保留最新值，多次快速变更时 consumer 只收到最终目标并发度

### 自定义 Controller

除了内置的 `StaticController`，可以实现 `ConcurrencyController` 接口对接任意外部系统：

```go
// 示例：从配置中心读取并 Watch 变更
type ConfigCenterController struct { ... }
func (c *ConfigCenterController) Resolve(queueName string) int { /* 查配置 */ }
func (c *ConfigCenterController) Watch(queueName string) (<-chan int, func()) { /* 订阅配置变更 */ }
```

---

## RabbitMQ Options 参考

```go
q, err := rabbitmq.New(rabbitmq.Options{
    // === 连接配置 ===
    Host:     "rabbitmq",
    Port:     5672,
    Username: "user",
    Password: "pass",
    Vhost:    "app-vhost",

    // === TLS ===
    TLSConfig:           tlsConfig,           // AMQPS 连接
    ManagementTLSConfig: mgmtTLSConfig,       // Management API

    // === 发布池 ===
    ProducerPoolSize: 10,                     // channel 池大小，高吞吐场景调大

    // === 重连 ===
    ReconnectInitial: time.Second,            // 首次重连等待
    ReconnectMax:     30 * time.Second,       // 最大退避上限

    // === 动态并发 ===
    ConcurrencyController: ctrl,             // 全局并发控制器，可选

    // === 可观测性 ===
    Logger:            logger,
    MetricsRegistry:   promReg,
    MetricsSubsystem:  "task_queue",         // 指标前缀

    // === 链路传播 ===
    Propagator:        propagator,

    // === vhost ===
    VhostHandle:       rabbitmq.VhostHandleEnv, // 按环境改写 vhost
    DeclareVhost:      true,                    // 自动创建 vhost

    // === 客户端标识 ===
    ClientProperties: map[string]any{
        "service": "order-worker",
        "version": "1.2.3",
    },
})
```

---

## SubscribeOptions 参考

```go
opts := &queue.SubscribeOptions{
    ConsumerTag: "worker-a",            // 消费者标签，空则自动生成
    AutoAck:     false,                 // 生产环境建议 false
    Exclusive:   false,                 // 是否独占消费者
    Concurrency: 4,                     // worker goroutine 数（Controller 优先级更高）
    PrefetchCount: 0,                   // 0 = 自动计算（静态: Concurrency*2; 动态: max(Concurrency*4, 256)）
    Retry:       queue.DefaultRetryPolicy(),
}
```

---

## Handler 返回值决策矩阵

| 场景 | 返回值 | 行为 |
|------|--------|------|
| 处理成功 | `queue.Ack()` | 确认并从队列删除 |
| 临时失败（可重试） | `queue.Retry(err)` | 按 `RetryPolicy` 指数退避重试，达上限后死信 |
| 永久失败（不可重试） | `queue.Drop(err)` | 立即 Nack(false)，路由到 DLX 或丢弃 |
| 需要立即重新投递 | `queue.Requeue(err)` | Nack(true)，broker 立即重新投递（注意死循环风险） |
| 自定义行为 | `queue.Nack(err, requeue)` | 底层 Nack，直接控制 requeue 参数 |

**Panic 默认行为**：handler 发生 panic 时，框架 recover 后自动执行 `Requeue`，确保消息不丢失。

---

## 消息构造

```go
// 方式 1：字节数组
msg := queue.NewMessage([]byte("hello"))

// 方式 2：JSON
msg, err := queue.NewJSONMessage(MyPayload{ID: 1})
// Content-Type 自动设为 application/json

// 方式 3：Builder 链式构造
msg := queue.NewMessageBuilder([]byte("body")).
    WithHeader("X-Trace-ID", traceID).
    WithContentType("text/plain").
    WithTimestamp(time.Now()).
    Build()
```

---

## 与 cybergo/mq 的对比

bore/queue 是针对 cybergo/mq 的全新替代，解决的核心问题：

| 维度 | cybergo/mq | bore/queue |
|------|-----------|-----------------|
| 连接参数 | Host / Port / Username / Password / VirtualHost 分散字段 | Host / Port / Username / Password / Vhost 结构化字段，支持 `amqp` / `amqps` 和 TLS |
| vhost | 只使用传入的 `VirtualHost` | 支持环境改写 vhost，缺失时可自动声明 |
| 并发发布 | 单个 `amqp.Channel`，源码说明适合低并发 | channel pool，适合并发发布 |
| 消费模型 | `Consume` 内部启动 goroutine，handler 接收 `amqp.Delivery` | `Subscribe(ctx, queue, handler, opts)`，支持 context、并发 worker、自动确认 |
| Ack/Nack | 调用方手动操作 `amqp.Delivery` | handler 返回值驱动 Ack / Retry / Nack，框架统一处理 |
| 重试 | 无内置重试策略 | 内置 `RetryPolicy`、指数退避、`x-retry-count` 追踪 |
| 死信 | 可通过手写 `Arguments` 配置 | `DeclareOptions` 直接提供 DLX / DLQ 字段 |
| panic 保护 | 有 recover，仅记录日志 | 有 recover，自动 Requeue 消息，记录日志与 `handler_panics_total` 指标 |
| 自动重连 | 无 | 连接自动重连，消费者自动重新订阅 |
| Exchange / Bind | 发布可传 exchange，声明和绑定需直接用底层 channel | `PublishWithRouting`，并提供 `DeclareExchange` / `BindQueue` |
| 消息格式 | `Produce` 只接受 string，默认 `text/plain` | `Message` 抽象，支持 bytes、JSON、headers、messageID、timestamp |
| 链路传播 | 无 | 可配置 `Propagator` 注入和提取 MQ headers |
| 动态并发 | 无 | `ConcurrencyController` 运行时扩缩容 |
| 可观测性 | 日志为主 | 日志 + Prometheus metrics |
| 关闭 | `Close()` 关闭全局连接 | `Shutdown(ctx)` 支持超时与优雅退出 |
| 健康检查 | `Ping()` 检查连接是否关闭 | `IsHealthy()` 检查底层连接状态 |
| 测试替身 | 无 | `queue/local` 进程内无依赖实现 |

### 迁移示例

cybergo/mq：

```go
err := mq.InitMQ(&mq.MQOptions{
    Host:        "localhost",
    Port:        5672,
    Username:    "guest",
    Password:    "guest",
    VirtualHost: "app",
})
err = mq.InitQueue("tasks", true, nil)
err = mq.Produce("tasks", "", `{"id":1}`)
err = mq.Consume("tasks", "worker-a", false, func(d amqp.Delivery) {
    defer d.Ack(false)
    _ = process(d.Body)
})
```

bore/queue：

```go
q, _ := rabbitmq.New(rabbitmq.Options{
    Host: "localhost", Port: 5672, Username: "guest", Password: "guest", Vhost: "app",
    ProducerPoolSize: 10,
})
defer func() {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    _ = q.Shutdown(ctx)
}()

q.Declare("tasks", queue.DefaultDeclareOptions())

msg, _ := queue.NewJSONMessage(MyPayload{ID: 1})
q.Publish(ctx, "tasks", msg)

q.Subscribe(ctx, "tasks", func(ctx context.Context, msg queue.Message) queue.HandleResult {
    var payload MyPayload
    if err := queue.ParseJSON(msg, &payload); err != nil {
        return queue.Drop(err)
    }
    if err := process(ctx, payload); err != nil {
        return queue.Retry(err)
    }
    return queue.Ack()
}, &queue.SubscribeOptions{
    ConsumerTag: "worker-a",
    Concurrency: 4,
    Retry:       queue.DefaultRetryPolicy(),
})
```

迁移要点：

1. 用 `rabbitmq.New` 创建实例，替代全局 `InitMQ` / `GetMQ`
2. 用 `q.Declare` 替代 `InitQueue`，DLX / TTL / 长度限制优先使用 `DeclareOptions` 字段
3. 用 `queue.NewMessage` / `queue.NewJSONMessage` 构造消息，替代只传 string body
4. 用 `q.Publish` / `q.PublishWithRouting` 替代 `Produce`
5. handler 返回 `queue.HandleResult`，显式表达确认/重试/死信决策
6. 用 `Shutdown(ctx)` 替代 `Close()`，并为发布和关闭设置超时 context

---

## 最佳实践

1. **发布和关闭必须使用带超时的 context**，避免永久阻塞。
2. **handler 必须尊重 `ctx.Done()`**，不要无限阻塞，收到取消信号应尽快返回。
3. **不要传 nil message 或 nil handler**，包会返回标准错误，但这是调用方 bug，应在测试阶段暴露。
4. **业务 handler 只返回 `HandleResult`**，不要手动调用 `msg.Ack()` / `msg.Nack()`（框架已处理重复确认）。
5. **`AutoAck` 生产环境保持 false**，避免处理失败但消息已被确认丢失。
6. **重要队列必须配置 `RetryPolicy` 和 DLX**；默认 `Retry=nil` 不重试，且未配置 DLX 时最终失败消息会被 broker 静默丢弃。
7. **高并发发布时根据吞吐调大 `ProducerPoolSize`**，不要共享单个 AMQP channel。
8. **local 只用于测试和本地开发**，不要用它验证持久化、死信、路由或真实网络场景。
9. **需要链路追踪时传入 `Propagator`**，框架自动注入/提取 MQ headers。
10. **如果运行环境不允许服务创建 vhost**，显式设置 `DeclareVhost: true`。
11. **使用 `ConcurrencyController` 实现弹性伸缩**，避免硬编码并发度；缩容不会中断正在处理的消息。
