# Metrics

`metrics` 包提供 Prometheus collector 的组合、注册和 HTTP 暴露辅助。

它的定位是 **薄抽象**：

- 不持有全局 `prometheus.Registry`
- 不替 Prometheus 再封装 Counter/Gauge/Histogram 创建 API
- 不自动注入 `service`、`pod`、`instance` 等标签
- 只负责把 bore 内置组件的 metrics 统一组合到调用方传入的 `prometheus.Registerer` 中

业务方可以自由选择：

- `prometheus.DefaultRegisterer`
- `prometheus.NewRegistry()` 创建的独立 registry
- `prometheus.WrapRegistererWith(...)` 包装后的 registerer
- 其他兼容 `prometheus.Registerer` / `prometheus.Gatherer` 的实现

## 设计原则

### 1. SDK 不持有全局状态

公共 SDK 不应该假设业务的 metrics 生命周期，也不应该私自创建和保存全局 registry。

推荐业务显式创建 registry：

```go
reg := prometheus.NewRegistry()
```

然后把 registry 传给注册函数和 handler：

```go
metrics.MustRegisterCollectors(reg, metrics.DefaultCollectors()...)
metrics.MustRegister(reg, httpMetrics, dbMetrics)

router.GET("/metrics", metrics.GinHandler(reg))
```

### 2. SDK 不重复封装 Prometheus 原生 API

自定义业务指标直接使用 Prometheus 官方 API：

```go
requestTotal := prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: "order",
        Subsystem: "api",
        Name:      "requests_total",
        Help:      "Total number of API requests.",
    },
    []string{"method", "path", "status"},
)
```

`metrics` 包只提供注册辅助：

```go
metrics.MustRegisterCollectors(reg, requestTotal)
```

### 3. SDK 不自动注入 K8S 维度

在 K8S 环境中，Prometheus Operator、ServiceMonitor、PodMonitor 通常会从 target、pod label、service label 中补充以下维度：

- `pod`
- `namespace`
- `instance`
- `app`
- `service`

因此 SDK 不再从 `POD_NAME` 解析服务名或实例 ID，也不再自动添加 `name` / `instance` 标签。

如业务确实需要统一 const label，可以由业务显式使用 `WrapRegistererWith`：

```go
wrapped := prometheus.WrapRegistererWith(
    prometheus.Labels{"service": "order-api"},
    reg,
)
metrics.MustRegister(wrapped, httpMetrics, dbMetrics)
```

## 核心 API

### Provider

```go
type Provider interface {
    GetCollectors() []prometheus.Collector
}
```

bore 的组件 metrics 都应实现 `Provider`，调用方可以统一注册。

### DefaultCollectors

```go
func DefaultCollectors() []prometheus.Collector
```

返回基础运行时指标：

- Go runtime metrics
- Process metrics
- Build info metrics

使用示例：

```go
reg := prometheus.NewRegistry()
metrics.MustRegisterCollectors(reg, metrics.DefaultCollectors()...)
```

如果业务已经使用 `prometheus.DefaultRegisterer` 或其他框架自动注册了 Go/process 指标，可以不注册 `DefaultCollectors()`。

### Register / MustRegister

```go
func Register(registerer prometheus.Registerer, providers ...Provider) error
func MustRegister(registerer prometheus.Registerer, providers ...Provider)
```

用于注册实现了 `Provider` 的组件。

```go
httpMetrics := provider.NewHTTP()
dbMetrics := db.NewDBStats(gormDB, db.WithDBName("primary"))

if err := metrics.Register(reg, httpMetrics, dbMetrics); err != nil {
    return err
}
```

`MustRegister` 适合初始化阶段，注册失败时直接 panic：

```go
metrics.MustRegister(reg, httpMetrics, dbMetrics)
```

`registerer` 传 `nil` 时使用 `prometheus.DefaultRegisterer`。

### RegisterCollectors / MustRegisterCollectors

```go
func RegisterCollectors(registerer prometheus.Registerer, collectors ...prometheus.Collector) error
func MustRegisterCollectors(registerer prometheus.Registerer, collectors ...prometheus.Collector)
```

用于注册原生 Prometheus collectors。

```go
requestTotal := prometheus.NewCounterVec(...)
metrics.MustRegisterCollectors(reg, requestTotal)
```

也可以注册基础 collectors：

```go
metrics.MustRegisterCollectors(reg, metrics.DefaultCollectors()...)
```

`registerer` 传 `nil` 时使用 `prometheus.DefaultRegisterer`。

### Handler / GinHandler

```go
func Handler(gatherer prometheus.Gatherer) http.Handler
func GinHandler(gatherer prometheus.Gatherer) gin.HandlerFunc
```

用于暴露 `/metrics` 端点。

原生 HTTP：

```go
http.Handle("/metrics", metrics.Handler(reg))
```

Gin：

```go
router.GET("/metrics", metrics.GinHandler(reg))
```

`gatherer` 传 `nil` 时使用 `prometheus.DefaultGatherer`。

## 推荐接入方式

### Gin 服务接入 HTTP Server + DB metrics

```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/prometheus/client_golang/prometheus"

    "github.com/izliang/bore/db"
    "github.com/izliang/bore/metrics"
    "github.com/izliang/bore/metrics/provider"
)

func main() {
    reg := prometheus.NewRegistry()
    metrics.MustRegisterCollectors(reg, metrics.DefaultCollectors()...)

    httpMetrics := provider.NewHTTP(
        provider.WithMetricNamespace("order"),
        provider.WithMetricSubsystem("api"),
    )

    gormDB, err := db.Open(db.Config{DSN: "..."})
    if err != nil {
        panic(err)
    }
    dbMetrics := db.NewDBStats(gormDB, db.WithDBName("primary"))

    metrics.MustRegister(reg, httpMetrics, dbMetrics)

    router := gin.Default()
    router.Use(httpMetrics.GinMiddleware())
    router.GET("/metrics", metrics.GinHandler(reg))

    _ = router.Run(":8080")
}
```

### 使用默认 Prometheus registry

如果业务没有自定义 registry，可以传 `nil`：

```go
httpMetrics := provider.NewHTTP()
metrics.MustRegister(nil, httpMetrics)

router.GET("/metrics", metrics.GinHandler(nil))
```

这种方式会使用：

- `prometheus.DefaultRegisterer`
- `prometheus.DefaultGatherer`

适合简单服务，但在 SDK、测试或多实例场景中更推荐显式传入 `prometheus.NewRegistry()`。

### 使用统一 const labels

```go
reg := prometheus.NewRegistry()
wrapped := prometheus.WrapRegistererWith(
    prometheus.Labels{
        "service": "order-api",
        "env":     "prod",
    },
    reg,
)

httpMetrics := provider.NewHTTP()
dbMetrics := db.NewDBStats(gormDB, db.WithDBName("primary"))

metrics.MustRegister(wrapped, httpMetrics, dbMetrics)
router.GET("/metrics", metrics.GinHandler(reg))
```

注意：不要把高基数字段放入 const labels，例如：

- request id
- user id
- order id
- trace id
- 原始 URL path

## 内置 Provider

### HTTP Server metrics

构造函数：

```go
provider.NewHTTP(opts ...provider.HTTPOption) *provider.HTTP
```

典型用法：

```go
httpMetrics := provider.NewHTTP(
    provider.WithMetricNamespace("order"),
    provider.WithMetricSubsystem("api"),
)
metrics.MustRegister(reg, httpMetrics)

router.Use(httpMetrics.GinMiddleware())
```

默认采集：

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `http_requests_total` | Counter | `method`, `path`, `status` | HTTP 请求总数 |
| `http_request_duration_seconds` | Histogram | `method`, `path` | 请求耗时 |
| `http_request_size_bytes` | Histogram | `method`, `path` | 请求大小 |
| `http_response_size_bytes` | Histogram | `method`, `path` | 响应大小 |
| `http_requests_in_flight` | Gauge | 无 | 当前处理中请求数 |

路径标签策略：

- 匹配到 Gin route 时使用 `gin.Context.FullPath()`，例如 `/users/:id`
- 未匹配 route 时使用清理后的 URL path
- 默认跳过 `/ping` 和 `/metrics`

常用配置：

```go
provider.WithSkipPathPrefixes("/health", "/metrics")
provider.WithSkipPathSuffixes(".html")
provider.WithPathLabeler(customPathLabeler)
provider.WithMethodLabeler(strings.ToUpper)
provider.WithStatusLabeler(func(code int) string { return strconv.Itoa(code) })
provider.WithDurationBuckets([]float64{0.05, 0.1, 0.2, 0.5, 1, 2, 5})
provider.WithSizeBuckets(prometheus.ExponentialBuckets(256, 4, 10))
```

### HTTP Client metrics

构造函数：

```go
provider.NewClientHTTP(opts ...provider.ClientHTTPOption) *provider.ClientHTTP
```

适配 `request/round_tripper.HTTPMetrics` 接口：

```go
clientMetrics := provider.NewClientHTTP(
    provider.WithClientHTTPNamespace("order"),
)
metrics.MustRegister(reg, clientMetrics)

transport := round_tripper.NewBuilder().
    Use(round_tripper.NewMetricsRoundTripper(clientMetrics)).
    Build()

client := &http.Client{Transport: transport}
```

默认采集：

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `http_client_requests_total` | Counter | `method`, `host`, `status` | HTTP client 请求总数 |
| `http_client_request_duration_seconds` | Histogram | `method`, `host`, `status` | 请求耗时 |
| `http_client_retries_total` | Counter | `method`, `host` | 重试次数 |
| `http_client_circuit_open_total` | Counter | `method`, `host` | 熔断打开次数 |

### DB metrics

构造函数：

```go
db.NewDBStats(gormDB, opts ...db.DBStatsOption) *db.DBStats
```

使用示例：

```go
dbMetrics := db.NewDBStats(gormDB, db.WithDBName("primary"))
metrics.MustRegister(reg, dbMetrics)
```

采集 `database/sql.DBStats` 中的连接池指标，例如：

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `db_connections_max_open` | Gauge | 最大打开连接数 |
| `db_connections_open` | Gauge | 当前打开连接数 |
| `db_connections_in_use` | Gauge | 使用中的连接数 |
| `db_connections_idle` | Gauge | 空闲连接数 |
| `db_wait_count_total` | Counter | 等待连接次数 |
| `db_wait_duration_seconds_total` | Counter | 等待连接总耗时 |
| `db_connections_max_idle_closed_total` | Counter | 因最大空闲连接数关闭的连接数 |
| `db_connections_max_lifetime_closed_total` | Counter | 因最大生命周期关闭的连接数 |

### Queue metrics

构造函数：

```go
queue.NewMetrics(subsystem string) *queue.Metrics
```

使用示例：

```go
queueMetrics := queue.NewMetrics("rabbitmq")
metrics.MustRegister(reg, queueMetrics)
```

RabbitMQ 使用示例：

```go
reg := prometheus.NewRegistry()

q, err := rabbitmq.New(rabbitmq.Options{
    URL:             "amqp://guest:guest@localhost:5672/",
    MetricsRegistry: reg,
    MetricsSubsystem: "rabbitmq",
})
if err != nil {
    panic(err)
}
_ = q
```

采集指标包括：

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `messages_published_total` | Counter | 发布消息数 |
| `messages_consumed_total` | Counter | 消费消息数 |
| `messages_acked_total` | Counter | ACK 消息数 |
| `messages_nacked_total` | Counter | NACK 消息数 |
| `messages_dead_lettered_total` | Counter | 死信消息数 |
| `handler_panics_total` | Counter | handler panic 次数 |
| `publish_duration_seconds` | Histogram | 发布耗时 |
| `handler_duration_seconds` | Histogram | handler 执行耗时 |
| `connection_status` | Gauge | 连接状态，1 表示已连接 |
| `reconnect_total` | Counter | 重连次数 |
| `consumer_resubscribe_total` | Counter | consumer 重新订阅次数 |

### btask metrics

构造函数：

```go
btask.NewPoolMetrics(name string, pool *btask.Pool, opts ...btask.MetricOption) *btask.Metrics
btask.NewMultiPoolMetrics(mp *btask.MultiPool, opts ...btask.MetricOption) *btask.Metrics
```

使用示例：

```go
poolMetrics := btask.NewPoolMetrics("default", pool)
metrics.MustRegister(reg, poolMetrics)
```

采集指标包括：

| 指标 | 类型 | 说明 |
| --- | --- | --- |
| `btask_pool_parallel` | Gauge | worker goroutine 配置数 |
| `btask_pool_queue_length` | Gauge | 当前队列长度 |
| `btask_pool_queue_capacity` | Gauge | 队列容量 |
| `btask_pool_running` | Gauge | 正在执行的任务数 |
| `btask_pool_pending` | Gauge | 未完成任务数 |
| `btask_pool_submitted_total` | Counter | 提交任务数 |
| `btask_pool_completed_total` | Counter | 完成任务数 |

## 自定义 Provider

业务可以实现 `metrics.Provider`，把多个自定义 collectors 组合成一个对象。

```go
type OrderMetrics struct {
    ordersCreated *prometheus.CounterVec
    orderDuration *prometheus.HistogramVec
}

func NewOrderMetrics() *OrderMetrics {
    return &OrderMetrics{
        ordersCreated: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: "order",
                Name:      "created_total",
                Help:      "Total number of created orders.",
            },
            []string{"channel"},
        ),
        orderDuration: prometheus.NewHistogramVec(
            prometheus.HistogramOpts{
                Namespace: "order",
                Name:      "duration_seconds",
                Help:      "Order processing duration in seconds.",
                Buckets:   prometheus.DefBuckets,
            },
            []string{"channel"},
        ),
    }
}

func (m *OrderMetrics) GetCollectors() []prometheus.Collector {
    return []prometheus.Collector{
        m.ordersCreated,
        m.orderDuration,
    }
}

func (m *OrderMetrics) IncCreated(channel string) {
    m.ordersCreated.WithLabelValues(channel).Inc()
}
```

注册：

```go
orderMetrics := NewOrderMetrics()
metrics.MustRegister(reg, orderMetrics)
```

## 测试建议

### 每个测试使用独立 registry

不要在单元测试中直接使用 `prometheus.DefaultRegisterer`，否则不同测试之间容易因为重复注册同名指标而互相影响。

推荐：

```go
func TestMetrics(t *testing.T) {
    reg := prometheus.NewRegistry()

    m := provider.NewHTTP()
    metrics.MustRegister(reg, m)

    families, err := reg.Gather()
    if err != nil {
        t.Fatal(err)
    }
    _ = families
}
```

### 避免高基数标签

Prometheus 标签值应尽量低基数。常见反例：

- 原始 URL：`/users/10001/orders/90001`
- 用户 ID
- 订单 ID
- trace ID
- request ID
- 错误详情文本

HTTP server 默认使用 Gin 路由模板作为 `path` 标签，例如 `/users/:id`，可以有效降低基数。

## 迁移说明

### 从旧的 ServiceIdentity 迁移

旧方式要求业务实现 service name、instance id、labels 等身份信息，并由 SDK 自动注入标签。

新方式不再需要这些接口。

旧方式：

```go
type ServiceMetrics struct {
    identity.DefaultIdentity
    provider.Empty
}
metrics.MustInit(&ServiceMetrics{})
```

新方式：

```go
reg := prometheus.NewRegistry()
metrics.MustRegisterCollectors(reg, metrics.DefaultCollectors()...)
router.GET("/metrics", metrics.GinHandler(reg))
```

如果确实需要统一标签：

```go
wrapped := prometheus.WrapRegistererWith(
    prometheus.Labels{"service": "order-api"},
    reg,
)
metrics.MustRegister(wrapped, httpMetrics)
```

### 从直接注册 collectors 迁移

旧方式：

```go
for _, c := range httpMetrics.GetCollectors() {
    reg.MustRegister(c)
}
```

新方式：

```go
metrics.MustRegister(reg, httpMetrics)
```

### 从全局默认 registry 迁移

旧方式可能隐式依赖 `prometheus.DefaultRegisterer`。

新方式推荐显式 registry：

```go
reg := prometheus.NewRegistry()
metrics.MustRegister(reg, httpMetrics)
router.GET("/metrics", metrics.GinHandler(reg))
```

如要继续使用默认 registry，也可以传 `nil`：

```go
metrics.MustRegister(nil, httpMetrics)
router.GET("/metrics", metrics.GinHandler(nil))
```

## 常见问题

### 是否需要注册 DefaultCollectors？

视业务情况而定。

如果你创建了独立 registry，并希望暴露 Go runtime、process、build info 指标，需要注册：

```go
metrics.MustRegisterCollectors(reg, metrics.DefaultCollectors()...)
```

如果你的服务框架已经注册了这些指标，或者你不需要这些指标，可以不注册。

### 为什么不在 SDK 内部创建全局 registry？

因为 registry 的生命周期应该由业务控制。公共 SDK 如果持有全局 registry，会带来：

- 多服务、多实例场景难以隔离
- 单元测试容易出现重复注册冲突
- 无法按业务需要选择默认 registry、自定义 registry 或 wrapped registerer
- SDK 隐式行为过多，难以扩展

### 为什么不提供 Counter/Gauge/Histogram Builder？

Prometheus 官方 API 已经足够清晰，SDK 再封装一层收益很小，反而会增加学习成本和维护成本。

业务自定义指标应直接使用：

```go
prometheus.NewCounterVec(...)
prometheus.NewGaugeVec(...)
prometheus.NewHistogramVec(...)
```

bore 的 metrics 包只负责组件级组合和注册辅助。

### K8S 中还需要 service / instance 标签吗？

通常不需要。

K8S + Prometheus Operator 场景下，`namespace`、`pod`、`service`、`app`、`instance` 等维度一般由采集配置和 relabel 规则提供。SDK 层重复注入容易造成标签冗余或语义不一致。

如果业务确实需要特定标签，请显式使用 `WrapRegistererWith`。
