# tracing

统一 Trace 抽象层。业务代码只依赖本包接口，不直接依赖任何第三方 Trace SDK。

## 设计原则

- **Helper 非 Master**：本包无全局状态，不掌控生命周期。应用层创建 Provider、管理注入，SDK 只提供接口和适配器。
- **接口隔离**：`Provider` → `Tracer` → `Span` 三层接口，业务层只需接触 `Tracer` 和 `Span`。
- **适配器模式**：具体实现通过子包注入，切换 Trace 后端只需更换 Provider。

## 包结构

```
tracing/
├── span.go          # Span 接口
├── tracer.go        # Tracer 接口 + StartOptions
├── provider.go      # Provider 接口
├── field.go         # Field 类型（委托 logit.Field）
├── README.md
├── noop/            # 空实现（待实现）
├── logtracer/       # 日志追踪器（待实现）
└── otel/            # OpenTelemetry 适配器
    ├── provider.go  # Provider — 创建并持有 sdktrace.TracerProvider
    ├── tracer.go    # tracer — 包装 otel trace.Tracer
    ├── span.go      # span — 包装 otel trace.Span
    ├── traceid.go   # TraceID — 实现 logit.TraceID，提取 traceID-spanID
    └── convert.go   # Field → attribute.KeyValue 转换
```

## 核心接口

### Span

```go
type Span interface {
    End()                              // 结束 Span
    RecordError(err error)             // 记录错误
    SetAttributes(attrs ...Field)      // 设置业务属性
}
```

### Tracer

```go
type Tracer interface {
    Start(ctx context.Context, name string, opts ...StartOptions) (context.Context, Span)
}
```

### Provider

```go
type Scope string

type Provider interface {
    Tracer(scope Scope) Tracer
    Shutdown(ctx context.Context) error
}
```

## 使用

### 应用初始化（main.go）

```go
import (
    "os"

    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "github.com/izliang/bore/logit"
    "github.com/izliang/bore/tracing"
    "github.com/izliang/bore/tracing/otel"
)

func main() {
    ctx := context.Background()

    // 创建 Exporter（应用层做主）
    exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint("localhost:4317"))
    if err != nil {
        panic(err)
    }

    // 创建 Provider
    provider, err := otel.NewProvider(
        otel.WithServiceName("my-service"),
        otel.WithExporter(exp),
    )
    if err != nil {
        panic(err)
    }
    defer provider.Shutdown(ctx)

    // 创建 Logger，注入 TraceID，日志行自动携带 traceID-spanID
    logger := logit.NewSimpleWithConfig(os.Stdout, &logit.Config{
        TraceID: &otel.TraceID{},
    })

    // 获取 Tracer 并注入
    tracer := provider.Tracer("http")
    srv := &Server{tracer: tracer, logger: logger}
}
```

### 业务代码

```go
type Server struct {
    tracer tracing.Tracer
    logger logit.Logger
}

func (s *Server) Handle(ctx context.Context) error {
    ctx, span := s.tracer.Start(ctx, "HandleRequest")
    defer span.End()
    span.SetAttributes(tracing.StringField("user.id", "123")())

    s.logger.Info(ctx, "请求进入", logit.StringField("user_id", "123"))
    // 日志: ... [abc123-root] msg[请求进入] user_id[123]

    if err := s.queryDB(ctx); err != nil {
        span.RecordError(err)
        return err
    }
    return nil
}

func (s *Server) queryDB(ctx context.Context) error {
    ctx, span := s.tracer.Start(ctx, "QueryDB")
    defer span.End()
    span.SetAttributes(tracing.StringField("db.table", "users")())

    s.logger.Info(ctx, "查询数据库")
    // 日志: ... [abc123-db] msg[查询数据库]
    //               ↑ traceID 不变，spanID 更新为当前子 Span

    return nil
}
```

### 日志中的 Trace 信息

```go
// 1. 注入 otel.TraceID 到 logit.Config
logger := logit.NewSimpleWithConfig(os.Stdout, &logit.Config{
    TraceID: &otel.TraceID{},
})

// 2. ctx 中每层 Span 的 traceID-spanID 自动反映在日志行
ctx, span := tracer.Start(ctx, "parent")    // [abc-parentSpanID]
logger.Info(ctx, "进入父操作")

ctx2, child := tracer.Start(ctx, "child")   // [abc-childSpanID]
logger.Info(ctx2, "进入子操作")
```

日志输出效果：

```
... [abc-parentSpanID] msg[进入父操作]
... [abc-childSpanID]  msg[进入子操作]
... [abc-parentSpanID] msg[请求完成]       ← defer 恢复外层 ctx
```

`otel.TraceID` 从 context 中读取当前 OTel Span，组合 `traceID-spanID`。
无 Span 的 ctx 输出 `-`。

### 链路嵌套

```
HandleRequest ─── QueryDB
    ↑                 ↑
  Start(ctx, "HandleRequest")
     │
     └─ 返回的 ctx 传给 QueryDB
            │
          Start(ctx, "QueryDB")  ← 自动成为子 Span，继承 TraceID
```

## 扩展

### 新增 Trace 后端

1. 在 `tracing/` 下创建子包
2. 实现 `tracing.Provider`、`tracing.Tracer`、`tracing.Span` 接口

`tracing` 包接口无需变动。

### 切换导出器

OTel adapter 不绑定导出器，只需改 main.go 的 import：

```go
// OTLP HTTP
import "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
exp, _ := otlptracehttp.New(ctx)

// 控制台调试
import "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
exp, _ := stdouttrace.New(stdouttrace.WithWriter(os.Stdout))
```
