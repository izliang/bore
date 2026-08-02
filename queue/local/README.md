# local — 进程内消息队列

`local` 实现了 `queue.Queue` 接口，基于 Go channel 在进程内传递消息，**无需外部依赖**。

## 适用场景

| 场景 | 说明 |
|------|------|
| **本地开发** | 无需安装 RabbitMQ，`go run` 即可验证完整链路 |
| **单元测试** | 自包含，无网络依赖，快速启动/销毁 |
| **原型验证** | 快速验证 Producer/Consumer 逻辑的正确性 |

## 快速开始

```go
import "github.com/izliang/bore/queue/local"

q := local.New(local.Options{})
defer q.Shutdown(context.Background())

q.Declare("tasks", nil)

// 发布
q.Publish(ctx, "tasks", queue.NewMessage([]byte("hello")))

// 消费
q.Subscribe(ctx, "tasks", func(ctx context.Context, msg queue.Message) error {
    fmt.Println(string(msg.Body()))
    return nil
}, nil)
```

## 与真实 broker 的差异

| 特性 | local | RabbitMQ |
|------|-------|----------|
| 持久化 | ❌ 进程退出即丢失 | ✅ 磁盘持久化 |
| 网络延迟 | 0（进程内 channel） | 有 |
| Exchange/路由 | ❌ exchange 忽略，直接投递到队列 | ✅ 完整 exchange 路由 |
| 死信队列 | ❌ Nack(false) 直接丢弃 | ✅ DLX 自动路由 |
| 自动重连 | 不需要 | ✅ 指数退避 |
| 并发安全 | ✅ channel + mutex | ✅ channel pool |

## 切换回真实 broker

由于 `local.New()` 和 `rabbitmq.New()` 都返回 `queue.Queue` 接口，只需修改一行代码：

```go
// 本地调试
q := local.New(local.Options{})

// 生产环境
q, err := rabbitmq.New(rabbitmq.Options{URL: "amqp://..."})
```

其余代码（Publish / Subscribe / Handler）无需任何改动。
