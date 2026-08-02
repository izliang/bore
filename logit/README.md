# logit

Go 结构化**文本**日志库。提供 `Logger` 接口、两种内置实现（`SimpleLogger` / `ZapLogger`）、延迟求值的 `Field`/`FieldFunc` 构造器、调用者帧跳过机制，以及通过 `logit/lctx` 将键值对挂载到 `context.Context` 使其自动出现在每行日志中。

## 目录

- [快速开始](#快速开始)
- [日志级别](#日志级别)
- [Logger 接口](#logger-接口)
- [内置实现](#内置实现)
  - [SimpleLogger](#simplelogger)
  - [ZapLogger](#zaplogger)
- [配置](#配置)
  - [Config](#config)
  - [调用者帧跳过（CallerSkip）](#调用者帧跳过callerskip)
  - [调用者显示格式（CallerFormat）](#调用者显示格式callerformat)
  - [链路追踪（TraceID）](#链路追踪traceid)
  - [WithLevel：动态级别限定](#withlevel动态级别限定)
- [字段（Field / FieldFunc）](#字段field--fieldfunc)
  - [基础字段](#基础字段)
  - [泛型字段函数](#泛型字段函数)
  - [字符串截断字段](#字符串截断字段)
  - [JSON 截断字段](#json-截断字段)
  - [时长字段](#时长字段)
- [上下文键值对（lctx）](#上下文键值对lctx)
- [适配器](#适配器)
  - [GORM 适配器](#gorm-适配器)
  - [Gin 适配器](#gin-适配器)
- [日志行格式](#日志行格式)
- [导入路径](#导入路径)
- [测试](#测试)

## 快速开始

```go
w := os.Stdout // 或任意 io.Writer
log := logit.NewSimple(w)

log.Info(ctx, "你好",
    logit.StringField("key", "value"),
    logit.IntOf("n", 42),
)
```

`NewSimple(nil)` 和 `NewSimpleWithConfig(nil, cfg)` 使用 **`io.Discard`** 作为输出，不会 panic。

## 日志级别

| 常量 | 字符串值 | 严重等级 | 说明 |
|------|---------|---------|------|
| `Trace` | `"trace"` | 0 | 最低级别，精细的调试信息 |
| `Debug` | `"DEBUG"` | 10 | 开发调试 |
| `Info` | `"INFO"` | 20 | 常规运行信息（默认最低级别） |
| `Warn` | `"WARN"` | 50 | 警告，非阻断性问题 |
| `Error` | `"ERROR"` | 100 | 错误，阻断当前操作 |

配置中 `Level` 字段设置**最低输出级别**，低于此级别的日志被静默丢弃。

## Logger 接口

```go
type Logger interface {
    io.Writer
    Trace(ctx context.Context, msg string, fields ...FieldFunc)
    Info(ctx context.Context, msg string, fields ...FieldFunc)
    Debug(ctx context.Context, msg string, fields ...FieldFunc)
    Warn(ctx context.Context, msg string, fields ...FieldFunc)
    Error(ctx context.Context, err error, fields ...FieldFunc)
}
```

### Error 的特殊行为

- `err == nil`：不输出任何日志，直接返回。
- `err != nil` 且无自定义消息：`err.Error()` 自动成为日志消息；`err` 同时以字段 `error[err.Error()]` 的形式追加。
- `err != nil` 且有自定义字段：可额外传递 `FieldFunc` 补充上下文。

```go
log.Error(ctx, fmt.Errorf("连接超时"),
    logit.StringField("addr", "10.0.0.1:3306"),
)
// 输出: ... ERROR [...] msg[] error[连接超时] addr[10.0.0.1:3306]
```

## 内置实现

### SimpleLogger

默认实现，将格式化日志行写入单一 `io.Writer`。适用于控制台输出或简单文件写入。

```go
// 默认配置（最低级别 Info）
log := logit.NewSimple(os.Stdout)

// 自定义配置
log := logit.NewSimpleWithConfig(os.Stdout, &logit.Config{
    Level: logit.Debug,
})
```

### ZapLogger

支持**文件轮转**和**多输出模式**的实现。可同时输出到标准输出和文件，文件支持按大小自动轮转。

```go
log := logit.NewZapWithConfig(logit.ZapConfig{
    Config: logit.Config{
        Level: logit.Info,
        PodName: "my-service",
    },
    Mode:     logit.OutputStdFile,  // 同时输出到 stdout 和文件
    AppName:  "myapp",
    FileName: "app.log",
    MaxSizeMB:  100,  // 单文件最大 100MB
    MaxAgeDays: 30,   // 保留 30 天
    MaxBackups: 10,   // 最多 10 个备份
    Compress:   false,
})
```

输出模式：

| 常量 | 值 | 说明 |
|------|---|------|
| `OutputStd` | 1 | 仅标准输出 |
| `OutputFile` | 2 | 仅文件（路径为 `/opt/logs/{AppName}/{PodName}/{FileName}`） |
| `OutputStdFile` | 3 | 标准输出 + 文件 |

## 配置

### Config

```go
type Config struct {
    PodName    string            // 应用/Pod 名称，显示在日志行中，默认 "AppName"
    Level      Level             // 最低输出级别，默认 Info
    CallerSkip CallerSkipConfig  // 调用者帧跳过与显示格式配置
    TraceID    TraceID           // 链路追踪 ID 提取器
}
```

### 调用者帧跳过（CallerSkip）

当存在中间包装层（如自定义 logger wrapper、ORM 适配器）时，默认的调用者定位会指向包装层而非业务代码。`CallerSkip` 允许配置**跳过规则**，使日志中的 `文件:行号` 直接指向真正的调用点。

```go
type CallerSkipConfig struct {
    Format CallerFormat            // 调用者显示格式，默认 CallerShort
    List   []CallerSkipConfigItem  // 跳过规则列表
}

type CallerSkipConfigItem struct {
    Value string         // 匹配值：import 路径前缀或包名字串
    Mode  CallerSkipMode // 匹配模式
}
```

**匹配模式：**

| 常量 | 说明 | 示例 |
|------|------|------|
| `CallerSkipPrefix` | 边界安全前缀匹配 | `"a/b"` 匹配 `"a/b.Foo"` 和 `"a/b/sub.Pkg"`，**不**匹配 `"a/bextra.Foo"` |
| `CallerSkipContains` | 子串包含匹配 | `"logit"` 匹配任意函数名中包含 `"logit"` 的帧 |

**内置默认值：** logit 自身和 appctx 包的帧始终以 `CallerSkipPrefix` 模式被自动跳过，无需手动配置。用户配置的规则追加在内置规则之后。

```go
// 示例：跳过 mywrapper 包（子串匹配）和 gorm 包（前缀匹配）
log := logit.NewSimpleWithConfig(os.Stdout, &logit.Config{
    Level: logit.Info,
    CallerSkip: logit.CallerSkipConfig{
        List: []logit.CallerSkipConfigItem{
            {Value: "mywrapper", Mode: logit.CallerSkipContains},
            {Value: "gorm.io/gorm", Mode: logit.CallerSkipPrefix},
        },
    },
})
```

**匹配规则：**

- 逐帧遍历调用栈，对每一帧按 `List` 顺序匹配；首个命中的帧被跳过，继续检查下一帧。
- `CallerSkipPrefix` 要求 `Value` 必须是 Go import 路径前缀（以 `.` 或 `/` 为边界），避免误匹配相邻包。
- `CallerSkipContains` 仅做子串包含检查，更灵活但需注意不要让过短的值（如 `"a"`）错误命中无关帧。
- 匹配结果以 `(CallerSkip 指纹 + 调用栈 PC 序列)` 为键缓存，同一调用点不会重复遍历帧。

### 调用者显示格式（CallerFormat）

| 常量 | 输出示例                                            | 说明 |
|------|-------------------------------------------------|------|
| `CallerShort` | `logit/logit.go:212`                            | 最后两级路径（默认） |
| `CallerFull` | `/Users/.../logit/logit.go:212`                 | 完整绝对路径 |
| `CallerFunc` | `github.com/.../logit.(*baseLogger).output:212` | 完整函数限定名 |

```go
log := logit.NewSimpleWithConfig(os.Stdout, &logit.Config{
    CallerSkip: logit.CallerSkipConfig{
        Format: logit.CallerFull,
    },
})
```

### 链路追踪（TraceID）

通过 `TraceID` 接口从 `context.Context` 提取追踪 ID，显示在日志行中。

```go
// 写入 traceID
ctx = logit.WithDefaultTraceID(ctx, "trace-abc-123")

log.Info(ctx, "处理请求")
// 输出: ... [trace-abc-123] ... msg[处理请求]
```

可自定义实现 `TraceID` 接口：

```go
type TraceID interface {
    TraceID(ctx context.Context) string
}
```

### WithLevel：动态级别限定

在不修改原 logger 的前提下，创建一个限定在指定最低级别的派生 logger：

```go
base := logit.NewSimple(os.Stdout)           // 最低 Info
debugLog := logit.WithLevel(base, logit.Debug) // 最低 Debug

base.Debug(ctx, "不可见")  // 被过滤
debugLog.Debug(ctx, "可见") // 输出
```

## 字段（Field / FieldFunc）

`FieldFunc` 是**延迟求值**的字段构造器：仅在日志行实际输出时才执行，被过滤级别时跳过求值。

### 基础字段

| 函数 | Go 类型 | 输出格式 |
|------|--------|---------|
| `StringField(k, v)` | `string` | 原样，换行默认转义为 `\n` |
| `IntField(k, v)` | `int` | 十进制 |
| `Int64Field(k, v)` | `int64` | 十进制 |
| `UintField(k, v)` | `uint` | 十进制 |
| `Uint64Field(k, v)` | `uint64` | 十进制 |
| `Float32Field(k, v)` | `float32` | 最短浮点表示 |
| `Float64Field(k, v)` | `float64` | 最短浮点表示 |
| `BoolField(k, v)` | `bool` | `true` / `false` |
| `ErrorField(k, err)` | `error` | `err.Error()` 或 `<nil>` |
| `AnyField(k, v)` | `any` | `fmt.Sprintf("%v", v)` |

### 泛型字段函数

从基础字段函数中抽象出的泛型构造器，一个函数覆盖一类数据类型的所有变体，减少样板代码：

| 函数 | 类型约束 | 输出格式 | 说明 |
|------|---------|---------|------|
| `IntOf[T](k, v)` | `~int \| ~int8 \| ~int16 \| ~int32 \| ~int64` | 十进制 | 替代 `IntField` / `Int64Field` 等 |
| `UintOf[T](k, v)` | `~uint \| ~uint8 \| ~uint16 \| ~uint32 \| ~uint64` | 十进制 | 替代 `UintField` / `Uint64Field` 等 |
| `FloatOf[T](k, v)` | `~float32 \| ~float64` | 最短浮点表示 | 替代 `Float32Field` / `Float64Field` |
| `FieldOf[T](k, v, format)` | `any` | 自定义 | 泛型基础构造器，需提供 `func(T) string` 格式化函数 |

```go
// 编译器自动推断类型，同一个函数覆盖多种类型
logit.IntOf("count", 42)                // int
logit.IntOf("id", int64(123))           // int64
logit.IntOf("flag", int8(1))            // int8

logit.UintOf("size", uint(100))         // uint
logit.UintOf("code", uint16(200))       // uint16

logit.FloatOf("rate", 0.95)             // float64
logit.FloatOf("score", float32(0.5))    // float32

// 自定义格式化任意类型
logit.FieldOf("ip", net.IPv4(1,2,3,4), func(v net.IP) string { return v.String() })
```

> 旧的 `IntField` / `Int64Field` / `UintField` / `Uint64Field` / `Float32Field` / `Float64Field` 仍然可用，内部已委托给泛型实现，行为完全一致。

### 字符串截断字段

`StringAbsField` 用于记录可能非常长的字符串（如 HTML、JSON 原始文本），通过保留**头 + 中 + 尾**三段内容并附带 MD5 哈希来压缩输出。

```go
// 默认：head=200, middle=200, tail=200
log.Info(ctx, "响应体", logit.StringAbsField("body", hugeHTML))

// 自定义截断参数
log.Info(ctx, "响应体", logit.StringAbsField("body", hugeHTML,
    logit.WithStringAbsHead(100),
    logit.WithStringAbsMiddle(50),
    logit.WithStringAbsTail(100),
))
// 输出: body[<len=50000 md5=a1b2c3... preview=head...middle...tail>]
```

配置选项：

| 函数 | 说明 | 默认值 |
|------|------|--------|
| `WithStringAbsHead(n)` | 开头保留 rune 数 | 200 |
| `WithStringAbsMiddle(n)` | 中间保留 rune 数 | 200 |
| `WithStringAbsTail(n)` | 结尾保留 rune 数 | 200 |
| `WithAllowNewline(b)` | 是否保留原始换行 | `false` |
| `WithStringAbsFormatFunc(fn)` | 完全自定义格式化 | 无 |

### JSON 截断字段

`JSONAbsField` 将任意值序列化为 JSON 后，递归截断其中嵌套的长字符串。

```go
log.Info(ctx, "请求参数",
    logit.JSONAbsField("params", map[string]any{
        "query": veryLongSQL,
    }),
)
```

`JSONDetailAbsField` 是高性能变体，采用**流式编码**直接构建 JSON，避免中间拷贝和归一化开销，适合高频调用场景。

```go
log.Info(ctx, "详情",
    logit.JSONDetailAbsField("data", complexStruct,
        logit.WithJSONAbsHead(100),
        logit.WithJSONAbsTail(50),
    ),
)
```

JSON 字段配置选项：

| 函数 | 说明 | 默认值 |
|------|------|--------|
| `WithJSONAbsHead(n)` | 开头保留 rune 数 | 200 |
| `WithJSONAbsMiddle(n)` | 中间保留 rune 数 | 200 |
| `WithJSONAbsTail(n)` | 结尾保留 rune 数 | 200 |
| `WithJSONAbsAllowNewline(b)` | JSON 输出是否缩进 | `false` |
| `WithJSONAbsFormatFunc(fn)` | 完全自定义格式化 | 无 |

### 时长字段

`TimeDurationField` 将 `time.Duration` 渲染为紧凑的纯 ASCII 格式：

```go
log.Info(ctx, "耗时",
    logit.TimeDurationField("elapsed", 2*time.Second+150*time.Millisecond),
)
// ... elapsed[2.15s]
```

渲染规则：

| 范围 | 格式 | 示例 |
|------|------|------|
| `0` | `0ms` | `0ms` |
| `< 1s` | `X.Xms` | `350ms` / `1.5ms` |
| `< 1m` | `X.Xs` | `2.15s` / `45s` |
| `< 1h` | `XmYs` | `3m30s` |
| `>= 1h` | `XhYmZs` | `2h15m30s` |

## 上下文键值对（lctx）

`logit/lctx` 子包提供将键值对挂载到 `context.Context` 的能力，这些键值对会自动出现在使用该 context 的所有日志行中。

```go
// 实现 lctx.LogFormatter 接口
type myPairs []lctx.Pair
func (p myPairs) Values() []lctx.Pair { return p }

ctx = lctx.PushPair(ctx, myPairs{
    {Key: "request_id", Value: "req-001"},
    {Key: "user_id", Value: "u-42"},
})

log.Info(ctx, "处理中")
// ... request_id[req-001] user_id[u-42] msg[处理中]

// 可链式追加
ctx = lctx.PushPair(ctx, myPairs{{Key: "step", Value: "validate"}})
log.Info(ctx, "校验完成")
// ... request_id[req-001] user_id[u-42] step[validate] msg[校验完成]
```

每次 `PushPair` 生成新的 context，不修改原 context。`ListPair(ctx)` 返回所有已挂载的键值对。

## 适配器

### GORM 适配器

`GormAdapter` 将 GORM 的日志输出桥接到 logit，SQL 语句自动以结构化字段记录。

```go
import "gorm.io/gorm/logger"

db, _ := gorm.Open(mysql.Open(dsn), &gorm.Config{
    Logger: logit.NewGormAdapter(log),
})
```

SQL 跟踪行为：

| 场景 | 级别 | 字段 |
|------|------|------|
| SQL 执行错误 | Error | `sql`、`rows`、`elapsed`、`error` |
| 慢查询（>200ms） | Warn | `sql`、`rows`、`elapsed` |
| 常规 SQL 跟踪 | Debug | `sql`、`rows`、`elapsed` |

配置项：

| 字段 | 说明 | 默认值 |
|------|------|--------|
| `SlowThreshold` | 慢查询阈值（0 禁用） | `200ms` |
| `IgnoreRecordNotFoundError` | 忽略 `ErrRecordNotFound` | `false` |

### Gin 适配器

**AccessMiddleware：** 自动记录每个 HTTP 请求的访问日志。

```go
r := gin.New()
r.Use(logit.AccessMiddleware(log, nil))
```

默认输出字段：`method`、`path`、`status`、`latency`、`client_ip`、`body_size`。

默认严重级别映射：

| HTTP 状态码 | 级别 |
|------------|------|
| `>= 500` | Error |
| `>= 400` | Warn |
| 其他 | Info |

**ErrorMiddleware：** 记录 Gin 上下文中累积的错误（如 Bind/render 错误）。

```go
r.Use(logit.ErrorMiddleware(log))
```

## 日志行格式

每行日志的格式：

```text
时间戳 [应用名] [Thread] 级别 [调用者:行号] [traceID] 上下文字段... msg[消息] field[value]...
```

示例：

```text
2026-07-08 15:04:05.123 [myapp] [Thread] INFO handler/user.go:42 [trace-abc] request_id[req-001] msg[用户登录] user[alice] ip[10.0.0.1]
```

- `[Thread]` 当前为固定占位符，预留给未来的 goroutine/线程 ID。
- 时间戳精确到毫秒（`YYYY-MM-DD HH:MM:SS.mmm`），使用秒级缓存减少格式化开销。

## 测试

```bash
# 运行全部测试
go test -count=1 -cover ./logit/... ./logit/lctx/...

# 仅 logit 主包
go test -count=1 -cover ./logit/
```
