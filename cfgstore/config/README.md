# config

基于 `cfgstore` 动态配置的队列并发度控制器，实现 `queue.ConcurrencyController` 接口。

## 概述

从数据库 `params_config` 表读取 `QUEUE_CONCURRENCY_CONTROLLER_MAP` 配置项（JSON map），解析后提供队列级别的并发度查询与变更推送。

## 数据流

```
params_config 表变更
     │
     ▼
cfgstore.Config (定时轮询 / 增量刷新)
     │
     ▼  ChangeEvent
QueueConcurrencyController.run (全局唯一后台 goroutine)
     │
     ├─ 解析 JSON → diff → 更新内部 value map
     │
     ▼  fan-out
 ┌───┴───┬───┴───┐
 ▼       ▼       ▼
chA     chB     chC   (各队列的 Watch channel)
```

## 配置格式

`params_config` 表：

| name | value | is_delete | update_time |
|------|-------|-----------|-------------|
| `QUEUE_CONCURRENCY_CONTROLLER_MAP` | `{"queue_a":5,"queue_b":3}` | 0 | 2026-07-17 10:00:00 |

- `value` 为 JSON 对象，key 为队列名，value 为并发度（int）
- `is_delete=1` 会使所有队列并发度归零
- 新增 / 删除队列只需修改 JSON，无需变更表结构

## 快速开始

```go
import (
    "github.com/izliang/bore/cfgstore"
    "github.com/izliang/bore/cfgstore/config"
)

// 1. 创建 cfgstore.Config（全局唯一）
cfg, err := cfgstore.New(ctx, cfgstore.NewGormLoader(db), logger,
    cfgstore.WithRefreshInterval(30*time.Second),
    cfgstore.WithFullRefreshInterval(5*time.Minute),
)
defer cfg.Stop()

// 2. 创建控制器
qc, err := config.NewQueueConcurrencyController(cfg, logger)
defer qc.Close()

// 3. 获取初始并发度
n := qc.Resolve("queue_a") // → 5

// 4. 订阅变更
ch, cancel := qc.Watch("queue_a")
defer cancel()

go func() {
    for v := range ch {
        adjustWorkers(v) // v=0 表示队列被移除或配置被删除
    }
}()
```

## API

### NewQueueConcurrencyController

```go
func NewQueueConcurrencyController(config *cfgstore.Config, logger logit.Logger) (*QueueConcurrencyController, error)
```

创建控制器，立即从 `cfgstore.Config` 读取当前配置并订阅后续变更。

### Resolve

```go
func (q *QueueConcurrencyController) Resolve(queueName string) int
```

返回指定队列的并发度。不存在的队列返回 0。并发安全。

### Watch

```go
func (q *QueueConcurrencyController) Watch(queueName string) (<-chan int, func())
```

订阅队列并发度变更。返回带缓冲 channel（cap=1）和幂等的取消函数。同一队列允许多个 watcher。

### Close

```go
func (q *QueueConcurrencyController) Close()
```

取消 cfgstore 订阅并关闭所有 watcher channel。调用后不应再使用控制器。

## 关键行为

| 行为 | 说明 |
|------|------|
| 变更通知 | drain-and-replace 策略，channel buffer=1，保证最终一致性 |
| 多 watcher | 同一队列可注册多个 watcher，各自独立接收通知 |
| Cancel 幂等 | 基于 `sync.Once`，重复调用安全 |
| 变更日志 | 自动打印 INFO：`queue concurrency changed` / `queue concurrency removed` |
| 容错 | JSON 解析失败仅打 ERROR 日志，不 panic，保留旧值 |
| 关闭 | `Close()` 取消 cfgstore 订阅，关闭所有 watcher channel |
