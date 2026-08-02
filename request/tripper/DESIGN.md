# tripper — 统一的 HTTP 传输 + 中间件包

## 理念

一个包解决全部 HTTP 客户端需求。传输层配置（代理、TLS、超时、连接池）和请求拦截（日志、重试、指标、Header 透传）不再是两个正交概念——所有 Option 平铺在一个 config 里，构建阶段内部自动分层。

调用者不用思考"这是传输层问题还是中间件问题"。

## 与旧 round_tripper 的对比

| 维度 | 旧 round_tripper | tripper（方案 C） |
|------|-----------------|-------------------|
| 包数量 | 1（混乱） | 1 |
| 传输层配置入口 | `NewProxyRoundTripper` 伪装成中间件 | `Proxy()` 一等 Option |
| 构造方式 | `NewBuilder → Add → Add → Build` | `tripper.New(Logger, Retry, Proxy)` 一步到位 |
| 输出 | `http.RoundTripper` + 手动组装 Client | `*http.Client` 直接可用（或 `Build` 返回 `RT`） |
| 代理配置出错风险 | 高（放错位置静默失效） | 无（Proxy 进 Transport，不走中间件链） |
| 中间件顺序 | 靠注释 + Add 逆序心智模型 | 内置推荐顺序，Option 顺序无关 |

## 三个构造函数

```go
// Build — 构建 http.RoundTripper，用于自定义 http.Client
func Build(opts ...Option) http.RoundTripper

// New — 一键构建完整 *http.Client（含 Timeout、CheckRedirect）
func New(opts ...Option) *http.Client

// TransportOnly — 只构建 *http.Transport（无中间件）
func TransportOnly(opts ...Option) *http.Transport
```

## Option 一览

### 传输层

| 函数 | 说明 |
|------|------|
| `Proxy(u *url.URL)` | 设置 HTTP 代理 |
| `ProxyFromEnv()` | 从 `USE_PROXY`/`OPENAI_PROXY_HTTP` 读代理 |
| `TLSInsecure()` | 跳过 TLS 证书验证 |
| `ConnectTimeout(d)` | TCP 连接超时 |
| `TLSHandshakeTimeout(d)` | TLS 握手超时 |
| `MaxIdleConns(n)` | 最大空闲连接数 |
| `IdleConnTimeout(d)` | 空闲连接保活时间 |
| `BaseTransport(rt)` | 传入预配置的 Transport |

### Client 层

| 函数 | 说明 |
|------|------|
| `ClientTimeout(d)` | http.Client 整体超时 |
| `NoRedirect()` | 禁止跟随重定向 |

### 中间件层

| 函数 | 说明 |
|------|------|
| `Logger(logger)` | 请求/响应日志（body 默认 100KB 截断） |
| `LoggerOpts(logger, opts...)` | 同上，自定义截断大小 |
| `Retry(maxRetries)` | 重试（指数退避 100ms~5s） |
| `RetryOpts(opts...)` | 同上，自定义策略/方法/退避 |
| `Metrics(m)` | Prometheus 指标收集 |
| `Propagation(ps...)` | 追踪 Header 透传 |
| `Custom(ds...)` | 自定义中间件（最外层） |

## 中间件内置顺序

```
请求 → Custom（最外层）
     → Logger
     → Metrics
     → Retry
     → Propagation
     → Transport（含 Proxy/TLS/连接池）
```

Option 传入顺序无关，Build/New 内部按此顺序组装。

## 使用示例

### 一行构建完整 Client

```go
client := tripper.New(
    tripper.Proxy(proxyURL),
    tripper.Logger(logger),
    tripper.Retry(3),
    tripper.ClientTimeout(30*time.Second),
)
resp, _ := client.Get("https://api.example.com/data")
```

### 只构建 RoundTripper

```go
rt := tripper.Build(
    tripper.Logger(logger),
    tripper.Retry(3),
)
client := &http.Client{Transport: rt}
```

### 只需代理，无中间件

```go
transport := tripper.TransportOnly(
    tripper.Proxy(proxyURL),
    tripper.TLSInsecure(),
)
```

### 自定义中间件顺序

```go
var rt http.RoundTripper = myTransport
rt = tripper.NewRetry(3)(rt)       // 手动包裹 Retry
rt = tripper.NewLogger(logger)(rt) // 再包裹 Logger
```

### FromConfig — 从配置文件

```go
client := tripper.FromConfig(tripper.Config{
    ProxyURL:   "http://proxy:8080",
    Logger:     logger,
    MaxRetries: 3,
    ClientTimeout: 30*time.Second,
})
```

## 独立装饰器

需要完全自定义中间件顺序时，使用独立装饰器：

| 函数 | 说明 |
|------|------|
| `NewLogger(logger, opts...)` | 创建日志装饰器 |
| `NewRetry(maxRetries, opts...)` | 创建重试装饰器 |
| `NewMetrics(m)` | 创建指标装饰器 |
| `NewPropagation(ps...)` | 创建 Header 透传装饰器 |

## 内部实现

```go
type Option func(*config)

type config struct {
    // 传输层
    proxyURL, insecureSkipVerify, connectTimeout, ...
    // Client 层
    clientTimeout, noRedirect
    // 中间件层
    logger, maxRetries, metrics, propagators, customMW
}

func New(opts ...Option) *http.Client {
    var c config
    for _, opt := range opts { opt(&c) }    // 收集，顺序无关
    transport := c.buildTransport()          // 阶段1：构建 *http.Transport
    rt := c.wrapMiddleware(transport)        // 阶段2：包裹中间件
    return &http.Client{Transport: rt, Timeout: c.clientTimeout}
}
```

## 文件结构

```
request/tripper/
├── DESIGN.md          设计文档
├── tripper.go         Config / Build / New / TransportOnly / FromConfig
├── options.go         全部 Option 函数
├── middleware.go       LogitRoundTripper + RetryRoundTripper
├── metrics.go         HTTPMetrics + MetricsRoundTripper
├── propagation.go     PropagatorRoundTripper
├── mock.go            MockRoundTripper
└── tripper_test.go    34 个测试 + Benchmark + 编译期验证
```

## 测试覆盖

- `Build`: 零选项、Logger、Retry、顺序保证、全部选项、Custom、RetryOpts
- `New`: 完整 Client、NoRedirect、零选项
- `TransportOnly`: 基础、Proxy、TLS、MaxIdleConns、IdleConnTimeout、ConnectTimeout、TLSHandshakeTimeout、BaseTransport
- `FromConfig`: 完整配置
- 独立装饰器: Logger、Retry、Metrics、Propagation、手动链式
- 中间件单元: Logger error、Logger largeBody、Retry context cancel、Retry non-idempotent、Retry customMethods、Metrics nil、readAndTruncateBody（4 个子测试）
- MockRoundTripper: 5 个测试
- Propagator 集成
- 全链路: Build 全中间件、New 全栈
- 编译期验证: Option 类型、HTTPMetrics 实现、http.RoundTripper 实现
