# propagator — 链路传播器

`propagator` 在 HTTP、gRPC、MQ 调用链路中统一透传 request-scoped metadata。
设计参考 OpenTelemetry 的 `TextMapPropagator` 模式，通过 `Carrier` 抽象消除协议差异。

## 包结构

```
propagator/
├── types.go            # Carrier + Propagator 接口
├── carrier/            # 各协议的 Carrier 适配器
│   ├── http.go         # HTTPHeader —— 适配 http.Header
│   ├── grpc.go         # GRPCMetadata —— 适配 metadata.MD
│   └── mq.go           # MQHeaders —— 适配 map[string]string
├── sre/                # SRE 灰度/环境/版本传播
│   └── sre.go
└── trace/              # W3C TraceContext 传播
    └── trace.go
```

## 核心接口

```go
// Carrier 是 key-value 存储的抽象，每种协议提供自己的实现。
type Carrier interface {
    Get(key string) string
    Set(key string, value string)
    Keys() []string
}

// Propagator 在服务间透传 metadata。
type Propagator interface {
    Inject(ctx context.Context, carrier Carrier)
    Extract(ctx context.Context, carrier Carrier) context.Context
}
```

## 快速开始

### SRE 传播器

用于在服务间透传灰度、环境、版本等 SRE 上下文。

```go
import (
    "git.100credit.cn/.../propagator/sre"
    "git.100credit.cn/.../propagator/carrier"
)

// 创建传播器，声明需要额外透传的 header
sreProp := sre.New(
    sre.WithKey("x-request-id"),
    sre.WithMap("x-tenant-id", "default-tenant"),
)

// 设置传播数据
ctx := sre.Set(context.Background(), sre.SREPropagatorData{
    Env:     "prod",
    ApiCode: "api-risk-v1",
    Apps: []sre.SREPropagatorItem{
        {ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
    },
})

// 读取传播数据
data := sre.Get(ctx)
```

### Trace 传播器

基于 W3C TraceContext 标准，与 OpenTelemetry 互通。

```go
import "git.100credit.cn/.../propagator/trace"

traceProp := trace.New()
// 内部委托给 OTel 的 propagation.TraceContext
// 自动处理 traceparent / tracestate 的注入和提取
```

### 组合使用

所有 Propagator 可组合，按顺序依次执行：

```go
import (
    "git.100credit.cn/.../propagator/sre"
    "git.100credit.cn/.../propagator/trace"
)

sreProp := sre.New(sre.WithKey("x-request-id"))
traceProp := trace.New()
```

## 各协议接入点

| 链路 | Carrier | 自动接入点 |
| --- | --- | --- |
| HTTP 客户端 | `carrier.HTTPHeader` | `request/tripper.Propagation(...)` |
| HTTP 服务端 | `carrier.HTTPHeader` | `web/middleware.PropagatorMiddleware(...)` |
| gRPC Unary | `carrier.GRPCMetadata` | `request/interceptor.UnaryClientInterceptor(...)` / `UnaryServerInterceptor(...)` |
| gRPC Stream | `carrier.GRPCMetadata` | `request/interceptor.StreamClientInterceptor(...)` / `StreamServerInterceptor(...)` |
| RabbitMQ | `carrier.MQHeaders` | `rabbitmq.Options{Propagator: ...}` |

## HTTP 使用示例

### 服务端（Gin）

```go
sreProp := sre.New(sre.WithKey("x-request-id"))
traceProp := trace.New()

router := gin.New()
router.Use(middleware.PropagatorMiddleware(sreProp, traceProp))

router.GET("/orders", func(c *gin.Context) {
    data := sre.Get(c.Request.Context())
    // 使用 data...
})
```

### 客户端（tripper）

```go
client := tripper.New(
    tripper.Propagation(sreProp, traceProp),
    tripper.ClientTimeout(30*time.Second),
)
resp, _ := client.Get("https://api.example.com/data")
```

## gRPC 使用示例

```go
// 客户端
conn, _ := grpc.Dial(target,
    grpc.WithUnaryInterceptor(interceptor.UnaryClientInterceptor(sreProp, traceProp)),
)

// 服务端
s := grpc.NewServer(
    grpc.UnaryInterceptor(interceptor.UnaryServerInterceptor(sreProp, traceProp)),
)
```

## RabbitMQ 使用示例

```go
q, _ := rabbitmq.New(rabbitmq.Options{
    URL:        "amqp://guest:guest@localhost:5672/",
    Propagator: sreProp,
})

ctx := sre.Set(context.Background(), sre.SREPropagatorData{Env: "prod"})
msg := queue.NewMessage([]byte(`{"hello":"world"}`))
q.Publish(ctx, "order.queue", msg) // 自动注入 headers
```

## SRE 标准字段

| Header | 来源字段 | 说明 |
| --- | --- | --- |
| `x-sre-env` | `SREPropagatorData.Env` | 当前环境 |
| `brh-gray-{serverName}` | `SREPropagatorItem.BrhGrayVersion` | 指定服务的灰度版本 |
| `brh-subversion-{serverName}` | `SREPropagatorItem.BrhSubVersion` | 指定服务的子版本 |
| `brh-gray-apicode` | `SREPropagatorData.ApiCode` | API 灰度码 |

提取时 header key 不区分大小写。Apps 按 ServerName 字母排序。

## 自定义 Propagator

实现 `propagator.Propagator` 接口即可接入：

```go
type MyPropagator struct{}

func (p *MyPropagator) Inject(ctx context.Context, carrier propagator.Carrier) {
    // 从 ctx 读取数据，写入 carrier
    carrier.Set("x-my-key", "value")
}

func (p *MyPropagator) Extract(ctx context.Context, carrier propagator.Carrier) context.Context {
    // 从 carrier 读取数据，返回增强后的 ctx
    val := carrier.Get("x-my-key")
    return context.WithValue(ctx, myKey{}, val)
}
```

## 注意事项

- 空值不会被注入到 outgoing carrier
- 注入采用覆盖语义，同名 key 以 context 中的数据为准
- MQ Carrier 的 `Get` 执行大小写不敏感查找，与 HTTP 行为一致
- 不要手动调用 `Inject` / `Extract` —— 优先使用各协议的自动接入点
