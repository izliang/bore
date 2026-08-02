# bore

企业级 Go 微服务基础库。提供应用上下文、日志、配置、数据库、缓存、消息队列、链路追踪、
服务发现、Web 脚手架等通用能力，遵循 **Provider Pattern + 面向接口** 设计，各组件可独立使用也可组合。

## 目录

```
bore/
├── appctx/       # 应用上下文（核心）
├── logit/        # 结构化日志
├── config/       # 配置管理
├── db/           # 数据库连接（GORM）
├── ck/           # ClickHouse 连接与 DDL
├── cache/        # 缓存抽象
├── queue/        # 消息队列抽象
├── tracing/      # 链路追踪抽象 + OTel 适配器
├── metrics/      # Prometheus 指标
├── discovery/    # 服务发现（Consul）
├── web/          # Web 脚手架（Gin）
├── request/      # HTTP 客户端 + 中间件链
├── btask/        # 后台任务调度
├── i18n/         # 国际化
├── e/            # 错误处理
├── logprobe/     # 日志探针（AI Agent）
├── bmap/         # 并发安全 Map
├── cmd/          # 命令行执行
├── tu/           # 测试工具
├── cfgstore/     # 配置存储（数据库加载器）
├── grpc/         # gRPC Proto 定义
├── ds/           # 泛型数据结构 SDK（详见 ds/README.md）
└── mcp/          # MCP（预留）
```

## 包简介

### appctx — 应用上下文

Web 请求的核心载体，将 `context.Context`、`logit.Logger`、GORM、Redis 整合为单一不可变值。

```go
ctx := appctx.New(
    appctx.WithLogger(logger),
    appctx.WithGORM(gormDB),
    appctx.WithRedis(redisClient),
)
ctx.Info("请求处理", logit.StringField("user_id", "123"))
db := ctx.GORM().WithContext(ctx)
```

| 能力 | 说明 |
|---|---|
| 不可变传递 | `With()` 返回新实例，并发安全 |
| Log 门面 | `Info/Debug/Warn/Error` 直接使用当前 ctx |
| 组件注入 | GORM、Redis 通过 Option 绑定 |
| fail-fast | 构造期非法参数立即 panic |

### logit — 结构化日志

零外部依赖的结构化文本日志，提供 `SimpleLogger`（默认）和 `ZapLogger`（高性能）两种后端。

| 子包 | 用途 |
|---|---|
| `logit` | Logger 接口 + Field 构造 + SimpleLogger + ZapLogger |
| `logit/lctx` | 通过 context 自动传播日志字段（如 request_id、trace_id）|
| `logit/chlog` | ClickHouse 后端，批量异步写入 |

```go
logger := logit.NewSimple(os.Stdout)
logger.Info(ctx, "用户登录", logit.StringField("user_id", "123"))

// context 字段传播
ctx = lctx.PushPair(ctx, logit.StringField("request_id", rid))
logger.Info(ctx, "处理完成") // 自动携带 request_id
```

### config — 配置管理

类型安全的热重载配置容器，支持文件/环境变量等多数据源，模板渲染，原子读取。

```go
type AppConfig struct {
    Service ServiceConfig `yaml:"service"`
}
cfg := config.New[AppConfig]()
cfg.SetTemplates(map[string]string{"ENV": os.Getenv("ENV")})
cfg.Load(source.Viper("conf/config.yaml", "APP"))
port := cfg.Get().Service.Port // 类型安全，IDE 友好

### db — 数据库连接

GORM 封装，提供建连重试（指数退避）、Context 超时传播、结构化日志和 Prometheus 指标。

```go
cfg := db.DefaultConfig()
cfg.DSN = "user:pass@tcp(host:3306)/mydb?charset=utf8mb4&parseTime=True"
gormDB, _ := db.OpenContext(ctx, cfg)
defer db.Close(gormDB)
```

### ck — ClickHouse

ClickHouse DDL 生成、原生连接、游标流式读取，内置搜索字段和 TTL 支持。

```go
def := ck.TableDef{
    Database: "mydb", Table: "events",
    Columns: []ck.ColumnDef{
        {Name: "ts", Type: "DateTime64(3)", Searchable: true},
        {Name: "user_id", Type: "String", Searchable: true},
    },
}
ddl := def.DDL()
```

### cache — 缓存抽象

`Cacher` 接口 + `Cache[V]` 泛型包装，编译期类型安全。内置 go-cache Memory/Redis/None provider。

```go
c := provider.NewMemory(5*time.Minute, 10*time.Minute)
userCache := cache.New[User](c, "user:")
userCache.Set(ctx, "123", &User{Name: "张三"}, time.Minute)
user, _ := userCache.Get(ctx, "123")
```

### queue — 消息队列

通用消息队列抽象，统一 Producer/Consumer/Message/Queue 接口，支持 RabbitMQ 等后端，内置指标和动态并发控制。

```go
q, _ := rabbitmq.New(rabbitmq.Options{Host: "localhost"})
q.Declare("tasks", queue.DefaultDeclareOptions())
q.Publish(ctx, "tasks", queue.NewMessage([]byte(`{"hello":"world"}`)))
q.Subscribe(ctx, "tasks", handler, &queue.SubscribeOptions{Concurrency: 4})
```

### tracing — 链路追踪

统一 Trace 抽象层。`Provider → Tracer → Span` 三层接口，业务代码零 OTel 依赖。

| 子包 | 用途 |
|---|---|
| `tracing` | Provider / Tracer / Span 接口 + Field |
| `tracing/otel` | OpenTelemetry 适配器，可注入任意 SpanExporter |

```go
exp, _ := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint("jaeger:4317"))
provider, _ := otel.NewProvider(
    otel.WithServiceName("my-service"),
    otel.WithExporter(exp),
)
tracer := provider.Tracer("http")

// 业务代码
ctx, span := tracer.Start(ctx, "HandleRequest")
defer span.End()
span.SetAttributes(tracing.StringField("user.id", "123"))
```

详见 `tracing/README.md`。

### metrics — Prometheus 指标

Provider 模式组合注册 Prometheus collectors，不持有全局 registry。

```go
httpMetrics := provider.NewHTTP(provider.WithMetricNamespace("myservice"))
dbMetrics := db.NewDBStats(gormDB, db.WithDBName("primary"))
metrics.MustRegister(reg, httpMetrics, dbMetrics)
r.GET("/metrics", metrics.GinHandler(reg))
```

### discovery — 服务发现

Consul 服务注册与健康检查。

```go
sd, _ := discovery.New(discovery.ConsulConfig{Address: "localhost:8500"})
sd.Register(ctx, discovery.Service{
    ID: "my-service-1", Name: "my-service", Address: "10.0.0.1", Port: 8080,
})
```

### web — Web 脚手架

Gin 框架桥接层，提供路由注册（SimpleRouter / HumaRouter）、中间件（CORS、Auth、RequestID、Propagator、Panic、Language）、统一响应信封。

```go
// 路由
web.POST("/api/user", handler)

// 中间件
r.Use(middleware.RequestIDMiddleware())
r.Use(middleware.PropagatorMiddleware(prop))
r.Use(middleware.CORSMiddleware())
```

| 子包 | 用途 |
|---|---|
| `web` | Gin 上下文键、路由注册 |
| `web/define` | Handler 签名 + 统一响应信封 |
| `web/middleware` | CORS / Auth / RequestID / Propagator / Panic / Language |
| `web/router/simple` | 简单路由注册 |
| `web/router/cyberhuma` | Huma 框架路由注册 |

### request — HTTP 客户端

统一的 HTTP 传输层 + 中间件链，一行构建完整 `http.Client`。内置 Logger、Metrics、Retry、Propagation 中间件。

```go
client := tripper.New(
    tripper.Proxy(proxyURL),
    tripper.Logger(logger),
    tripper.Retry(3),
    tripper.ClientTimeout(30*time.Second),
)
resp, _ := client.Get("https://api.example.com/data")
```

| 子包 | 用途 |
|---|---|
| `request/tripper` | HTTP 传输层 + 中间件链 |
| `request/propagator` | 请求传播（Header 透传）|
| `request/interceptor` | gRPC 拦截器 |

### btask — 后台任务调度

固定大小 worker pool + 多窗口路由，支持按名称分组的独立并发限制和超时，内置 Prometheus 指标。

```go
pool, _ := btask.NewPool(10)
pool.Submit(ctx, task)

mw := btask.NewMultiPool(btask.MultiPoolConfig{...})
mw.Submit(ctx, "llm", task)
```

### i18n — 国际化

零外部依赖，兼容 go-i18n v2 的三种 JSON 文件格式，支持 text/template 模板语法。

```go
bundle, _ := i18n.Load("./locales")
translator := bundle.Translator("zh-CN")
msg := translator.T("welcome", i18n.TplData{"Name": "张三"})
```

### e — 错误处理

错误码 + 消息 + 详情的结构化错误体系，支持错误注册表和链式构建。

```go
err := e.NewBuilder().WithCode("USER_NOT_FOUND").WithMessage("user.notfound").Build()
code := e.GetCode(err)       // "USER_NOT_FOUND"
msg  := e.GetMessage(err)    // "user.notfound"
```

### logprobe — 日志探针

面向 AI Agent 的日志排查工具，统一 Elasticsearch / ClickHouse / 远程文件的查询接口。

### bmap — 并发安全 Map

`sync.Map` 的泛型包装 + 有序 Map 实现。

### cmd — 命令行执行

带超时、输出缓冲、进程组控制的命令执行工具。

### tu — 测试工具

Error 比较、验证、断言辅助，支持 `errors.Is` 语义匹配。

```go
tu.CompareErr(t, err, "expected message")
tu.ValidateErr(t, err, true) // 预期有错误
```

### cfgstore — 配置存储

从数据库加载配置的 Loader 接口，支持全量加载和增量刷新。内置 GORM loader。

### grpc — gRPC Proto

gRPC 服务的 protobuf 定义与生成代码。


### ds — 泛型数据结构 SDK

企业级 Go 泛型数据结构库。核心理念：算法与数据结构彻底解耦 —— 算法只依赖 Core 接口，任何新结构只需实现接口即可复用所有算法。

```
依赖关系：Core（定义接口） ← Algorithm（仅依赖 Core） → Tree / Graph / Trie / Workflow / ...（实现 Core）
```

| 能力接口 | 说明 |
|---|---|
| `Collection[T]` | 根接口，所有结构都实现 |
| `Iterator[T]` | 逐个访问元素 |
| `Iterable[T]` | 能生成迭代器 |
| `Traversable[T]` | **算法唯一依赖** —— BFS / DFS / 拓扑排序等 |
| `Searchable[T]` | 判断是否包含某元素 |
| `Mutable[T]` | 增、删、清空 |
| `Cloneable[T]` | 深拷贝 |
| `Comparable[T]` | 集合相等性比较 |
| `Weighted` | 边或元素携带权重 |

> 📖 详见 [ds/README.md](ds/README.md)
 
### mcp — MCP

预留目录，MCP 服务端集成（待实现）。

## 设计原则

| 原则 | 说明 |
|---|---|
| **Helper 非 Master** | 所有包无全局状态，不掌控生命周期。应用层创建、管理、注入。 |
| **面向接口** | 核心能力通过接口暴露，具体实现通过子包注入，切换后端不改业务代码。 |
| **Provider Pattern** | 接口定义在顶层包，实现在 `provider/` 子包（如 `cache/provider`、`metrics/provider`）。 |
| **显式依赖** | 通过 `appctx.AppContext` 或 struct DI 传递依赖，不依赖包级全局变量。 |
| **零/少外部依赖** | 核心包（logit、i18n、e、bmap）零外部依赖；adaptor 包隔离第三方依赖。 |

## 快速开始

```go
package main

import (
    "context"
    "os"

    "github.com/izliang/bore/appctx"
    "github.com/izliang/bore/logit"
)

func main() {
    logger := logit.NewSimple(os.Stdout)
    ctx := appctx.New(appctx.WithLogger(logger))

    ctx.Info("服务启动")
    // ...
}
```
