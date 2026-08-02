# logprobe — 设计文档与实现现状

> 最后更新：2026-06-23
> 文档目的：供 AI 持续开发参考，包含架构、接口契约、当前实现状态和待完成项。

---

## 1. 定位

`logprobe` 是一个**面向 AI Agent 的日志排查工具包**。抽象了 ES (ELK)、ClickHouse、远程文件三种后端，提供一组单意图原语供 AI 组合调用。不做 UI、不做告警、不做日志采集。

---

## 2. 架构总览

```
┌──────────────────────────────────────────────────────────────────┐
│                         AI Agent (LLM)                            │
│                                                                    │
│   discover → filter → stat → narrow → detail → context            │
│   Info()    Search() Stats() Search() Expand() Context()          │
└───────────────────────────┬──────────────────────────────────────┘
                            │
                   ┌────────┴────────┐
                   │  LogSource       │  ← 统一接口（5 原语 + Stream + Expand）
                   │  (logprobe.go)   │
                   └────────┬────────┘
                            │
           ┌────────────────┼────────────────┐
           ▼                ▼                ▼
   elasticsearch       kibanaproxy      ckadapter
    (直连 ES)         (Kibana Proxy)    (TODO)
           │                │
           └───────┬────────┘
                   ▼
               escommon (共享业务逻辑，纯函数)
```

**三层架构**：

| 层 | 包 | 职责 | 状态 |
|----|-----|------|------|
| 接口层 | `logprobe/` | LogSource 接口、所有类型定义、Registry、Tool 导出 | ✅ 完成 |
| 共享层 | `logprobe/adapters/escommon/` | ES 查询 DSL 构建、响应解析、字段发现 | ✅ 完成 |
| 适配器层 | `logprobe/adapters/elasticsearch/` | 直连 ES（go-elasticsearch/v8） | ✅ 完成 |
| 适配器层 | `logprobe/adapters/kibanaproxy/` | Kibana Console Proxy（net/http + cookie） | ✅ 完成 |
| 适配器层 | `logprobe/adapters/ckadapter/` | ClickHouse 适配器 | ⏳ 未开始 |
| 适配器层 | `logprobe/adapters/fileadapter/` | 远程文件适配器 | ⏳ 未开始 |

---

## 3. 包结构与文件职责

```
logprobe/
├── DESIGN.md                      本文档
│
├── logprobe.go                    包文档 + LogSource 接口 + Registry + TimeRange
├── types.go                       SearchRequest / StreamRequest / StatsRequest /
│                                  ContextRequest / TraceRequest /
│                                  SearchResult / StatsResult / TraceResult /
│                                  LogEntry / FieldValue / ExpandTarget / ExpandResult
├── cursor.go                      Cursor（可持久化游标）+ LogCursor（有状态迭代器）
├── info.go                        SourceInfo / FieldInfo / QueryError
├── tooldef.go                     OpenAITools() — 7 个 function-calling 工具定义
├── logprobe_test.go               接口契约测试 + stubSource
│
└── adapters/
    ├── escommon/                  共享层（纯函数，零外部依赖）
    │   ├── query.go               BuildSearchBody / BuildStatsBody / BuildContextBody
    │   ├── response.go            ParseSearchResult / ParseStatsResult / ParseDocResponse
    │   ├── fields.go              ExtractSourceInfo / ExtractSourceInfoFromFieldCaps
    │   └── escommon_test.go       23 个单元测试
    │
    ├── elasticsearch/             直连 ES 适配器
    │   ├── source.go              Config + Source + NewSource + Info() + HTTP esapi 传输
    │   ├── search.go              Search() + Stats() → 委托 escommon
    │   ├── context.go             Context() + Expand() → 委托 escommon
    │   └── stream.go              search_after + PIT 流式消费
    │
    └── kibanaproxy/               Kibana Console Proxy 适配器
        ├── source.go              Config + Source + NewSource + HTTP 中间件链 + doRequest
        ├── search.go              Search/Stats/Context/Expand/Info → 委托 escommon
        ├── stream.go              search_after + PIT（走 proxy 透传）
        └── kibanaproxy_test.go    集成测试（需要真实 Kibana session）
```

---

## 4. 核心接口

### 4.1 LogSource

```go
type LogSource interface {
    // 发现
    ID() string
    Info(atx appctx.AppContext) (*SourceInfo, error)

    // 查询
    Search(atx appctx.AppContext, req SearchRequest) (*SearchResult, error)
    Stats(atx appctx.AppContext, req StatsRequest) (*StatsResult, error)
    Context(atx appctx.AppContext, req ContextRequest) (*SearchResult, error)

    // 流式
    Stream(atx appctx.AppContext, req StreamRequest) (*LogCursor, error)
    Cursor() Cursor
    SetCursor(Cursor)

    // 展开
    Expand(atx appctx.AppContext, targets []ExpandTarget) ([]ExpandResult, error)
}
```

**设计原则**：每个方法做一件事，AI 自行组合调用顺序。`atx appctx.AppContext` 集成 context.Context 和日志能力——适配器内部可通过 `atx.Logger().Debug(...)` 输出调试日志，调用方可通过 `atx.With(lctx.PushPair(...))` 注入请求级字段。

### 4.2 LogCursor（流式迭代器）

```go
type LogCursor struct {
    Next   func(ctx context.Context) ([]LogEntry, error)
    Cursor func() Cursor
    Close  func() error
}
```

- 首次 `Next()` → 获取已有数据
- 后续 `Next()` → 只获取新数据
- 空批次 + nil error = Follow 模式暂无新数据
- `ErrStreamDone` = Follow=false 模式所有数据消费完毕
- `Close()` → 释放后端资源（ES PIT、文件句柄等）

### 4.3 FieldValue（字段截断 + 懒加载）

```go
// 内存持有（CK、远程文件），Expand 零开销
fv := logprobe.NewFieldValue(full, 200)

// 惰性加载（ES 大字段），Expand 时发 GET /_doc/{id}
fv := logprobe.NewLazyFieldValue(prefix, fullLen, func(ctx) (string, error) { ... })
```

与 ES script_fields 的区别：script_fields 在 ES 侧计算并截断，结果是最终值，不可二次 Expand。本方案在后端返回全量数据、客户端侧截断，Expand 可获取完整内容。如需 ES 侧 script，通过 `escommon.WithScriptFields()` 或 `Config.ScriptFields` 注入。

---

## 5. 核心数据类型

### 5.1 请求类型

| 类型 | 关键字段 | 说明 |
|------|---------|------|
| `SearchRequest` | Query, Fields, FieldMaxLen, ExpandFields, TimeRange, Limit | 关键词搜索，FieldMaxLen=200 扫描 |
| `StreamRequest` | 嵌入 SearchRequest, Follow | Follow=true 实时监控 |
| `StatsRequest` | Field, TopN, Filter | 字段 Top N 分布 |
| `ContextRequest` | EntryID, Before, After, Scope, ScopeField | Scope: time/trace/host |
| `TraceRequest` | TraceID, Sources, MaxSpans | 跨数据源分布式追踪 |

### 5.2 响应类型

| 类型 | 关键字段 | 说明 |
|------|---------|------|
| `SearchResult` | Entries, TotalHits, IsExhaustive, ReturnedHits, TookMs | IsExhaustive=false 时 AI 应加过滤 |
| `StatsResult` | Field, Cardinality, Buckets | BucketItem{Value, Count} 降序 |
| `TraceResult` | TraceID, Spans | TraceSpan{Source, EntryID, Timestamp, Fields} |

### 5.3 元数据类型

| 类型 | 说明 |
|------|------|
| `SourceInfo` | Name, Backend, Fields, TimeRange, TimeField, MessageField, Estimated |
| `FieldInfo` | Name, Type, Searchable, Roles, Example, DistinctN |
| `Cursor` | Backend + Position (json.RawMessage)，JSON 可序列化 |
| `QueryError` | Message, Retryable, Hint, Example（指导 AI 修正） |

---

## 6. escommon — 共享层（纯函数）

### 6.1 查询构建（query.go）

```go
BuildSearchBody(req, index, opts...) → map[string]any
//   Query → query_string (AND, lenient)
//   TimeRange → range filter on timeField
//   Fields → _source 过滤
//   Limit → size（上限 10000）

BuildStatsBody(req, index, opts...) → map[string]any
//   size:0 + terms aggregation on req.Field

BuildContextBody(req, refEntry, index, opts...) → map[string]any
//   Scope="time"  → range + sort
//   Scope="trace" → term on ScopeField
//   Scope="host"  → term + range (±1h)
```

**SearchOption**：`WithTimeField()`、`WithSearchFields()`、`WithScriptFields()`。

### 6.2 响应解析（response.go）

```go
ParseSearchResult(body, fieldMaxLen, expandFields, timeField) → (*SearchResult, error)
//   自动截断（expandFields 白名单不截断）
//   script_fields 结果合并到 Fields（不覆盖 _source 已有字段）
//   IsExhaustive = (total.relation == "eq")

ParseStatsResult(body, field) → (*StatsResult, error)
//   解析 terms aggregation → BucketItem 列表

ParseDocResponse(body, timeField) → (*LogEntry, error)
//   解析 GET /_doc/{id} 响应
```

### 6.3 字段发现（fields.go）

**双策略**：

```go
// 优先 _field_caps（权限宽松，直接提供 searchable/aggregatable）
ExtractSourceInfoFromFieldCaps(name, index, fieldCapsBody, estimated, timeField, msgField)

// 回退 _mapping
ExtractSourceInfo(name, index, mappingBody, estimated, timeField, msgField)
```

`_field_caps` 优势：不需要 monitor 权限，直接返回 searchable/aggregatable。对多类型字段（如 text + .keyword），优先选 keyword（精确匹配）。

---

## 7. elasticsearch 适配器

### 7.1 连接

```go
import "github.com/elastic/go-elasticsearch/v8"

client, _ := elasticsearch.NewClient(elasticsearch.Config{Addresses: []string{"http://localhost:9200"}})
src := elasticsearch.NewSource(client, elasticsearch.Config{Name: "prod-logs", Index: "applog-*"})
```

### 7.2 Stream — search_after + PIT

```
1. 创建 PIT: POST /{index}/_pit?keep_alive=5m
2. 首次查询: sort: [{timeField:"asc"},{_shard_doc:"asc"}], 无 search_after
3. 后续查询: 加入 search_after=[上次最后一条的 sort values]
4. 关闭: DELETE /_pit {id: pitID}
```

Sort 使用 `[timeField, _shard_doc]` 作为复合游标，解决同时间戳顺序问题。

---

## 8. kibanaproxy 适配器

### 8.1 协议

```
POST /api/console/proxy?path={url-encoded-es-path}&method={method}
Headers: Cookie: sid=xxx, kbn-xsrf: kibana, Content-Type: application/json
Body: 标准 ES 请求体（透传）
```

### 8.2 连接

```go
src := kibanaproxy.NewSource(kibanaproxy.Config{
    Name:          "prod-app-logs",
    KibanaBaseURL: "https://kibana-zw..cn",
    Index:         "applog-prod-internal-*",
    Cookies:       "sid=xxx; cas_cookie_key=xxx",
    Logger:        atx.LogitLogger(),  // HTTP 中间件日志
    MaxRetries:    2,                  // 网络错误自动重试
})
```

### 8.3 HTTP 中间件链

在 `NewSource` 时通过 `round_tripper.Builder` 一次性构建，之后不再修改（并发安全）。

```
请求 → LogitRoundTripper → RetryRoundTripper → http.DefaultTransport → Kibana
响应 ← LogitRoundTripper ← RetryRoundTripper ← http.DefaultTransport ← Kibana
```

**LogitRoundTripper**：记录每个请求的 method、URL、status、duration、body（默认 4KB 截断）。

**RetryRoundTripper**：网络错误自动重试，指数退避 100ms→200ms→400ms + ±25% 抖动，上限 5s。401/403 不触发传输层重试（在应用层返回 QueryError）。

---

## 9. AI 工具定义

`OpenAITools()` 返回 7 个 function-calling tool 定义：

| 工具名 | 对应方法 | 描述 |
|--------|---------|------|
| `logprobe_describe` | Info() | 发现可用数据源及字段结构 |
| `logprobe_search` | Search() | 关键词搜索 |
| `logprobe_stats` | Stats() | 字段 Top N 分布 |
| `logprobe_context` | Context() | 获取日志上下文 |
| `logprobe_stream` | Stream() | 游标流式消费 |
| `logprobe_expand` | Expand() | 展开截断字段 |
| `logprobe_trace` | Trace() | 跨数据源分布式追踪 |

---

## 10. 测试现状

| 包 | 测试数 | 类型 |
|----|--------|------|
| `logprobe` | 12 | 单元测试 |
| `escommon` | 23 | 单元测试 |
| `elasticsearch` | 0 | 待补充 |
| `kibanaproxy` | 1 | 集成测试（需真实 Kibana） |

---

## 11. 当前状态

**已完成**：
- [x] LogSource 接口 + 全部类型定义
- [x] Cursor 游标 + LogCursor 流式迭代器
- [x] FieldValue 截断（内存持有 / 惰性加载双模式）
- [x] Registry 多数据源管理
- [x] OpenAI Tools（7 个 function-calling）
- [x] escommon 共享层（查询 DSL、响应解析、字段发现双策略）
- [x] elasticsearch 适配器（直连 ES，search_after+PIT）
- [x] kibanaproxy 适配器（Kibana Proxy，中间件链，cookie 认证）
- [x] HTTP 中间件链（日志 + 重试）
- [x] _field_caps?fields=* 字段发现（优先，已实际跑通 75 字段/124亿文档）
- [x] _mapping 字段发现（回退）
- [x] LogSource 方法签名统一为 appctx.AppContext
- [x] 全部中文注释

**待完成**：
- [ ] CK 适配器（`logprobe/adapters/ckadapter/`）
- [ ] 远程文件适配器（`logprobe/adapters/fileadapter/`）
- [ ] Registry.Trace() 跨数据源追踪实现
- [ ] elasticsearch 适配器单元测试
- [ ] 性能测试

---

## 12. 关键设计决策（FAQ）

**为什么 FieldValue 在客户端截断而不是用 ES script_fields？**
ES script_fields 的值是最终值，不可二次 Expand。客户端截断 + 闭包持有让 AI 可以先扫描（短前缀），发现关键条目后再 Expand 获取完整内容。

**为什么 LogCursor.Next 用 context.Context 而不是 appctx.AppContext？**
LogCursor 是底层迭代器，Next 只需要取消/超时。AppContext 实现 context.Context，传入 atx 即可。

**为什么 escommon 参数用 context.Context？**
escommon 是纯函数层，AppContext 实现 context.Context 接口，适配器直接传入 atx 无需转换。保持共享层零依赖。

**Kibana proxy 重试和 Cookie 过期如何不冲突？**
RetryRoundTripper 在传输层重试网络错误。401/403 在 `doRequest` 应用层检查，返回 QueryError 不触发重试——session 过期重试无意义。

**_field_caps 返回 keyword + text 两个类型，选哪个？**
优先 keyword（精确匹配、可聚合），其次 searchable=true 的类型，最后取第一个。
