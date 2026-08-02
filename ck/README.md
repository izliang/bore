# ck — ClickHouse Native Toolkit

DDL 类型安全生成 + 原生协议连接 + 游标流式消费。

```
ck/
├── ck.go       包文档
├── native.go    Conn（连接、查询、执行）
├── cursor.go    CursorStream（游标流）+ Poll（轮询）+ WatchOptions
├── schema.go    TableDef / ColumnDef / CreateTableSQL / CreateDatabaseSQL
└── ck_test.go
```

---

## 连接

```go
import "github.com/izliang/bore/ck"

conn, err := ck.Dial("127.0.0.1:9000", "default", "", "mydb")
if err != nil {
    panic(err)
}
defer conn.Close()
```

### Conn 方法

| 方法 | 说明 |
|------|------|
| `Dial(addr, user, password, database)` | 打开原生连接，默认 `max_block_size=100000` |
| `DialOptions(opts)` | 完整 `clickhouse.Options` 控制 |
| `Ping(ctx)` | 健康检查 |
| `Exec(ctx, query, args...)` | 执行 DDL / INSERT / DML |
| `NewCursorStream(query, cursorCol, batchSize)` | 创建游标流（见下文） |
| `Close()` | 关闭连接 |
| `RawConn()` | 暴露底层 `chdriver.Conn` 供高级用法 |

---

## DDL

从 `TableDef` 生成完整 `CREATE TABLE` 语句，自动处理 ORDER BY、bloom_filter 索引、TTL。

### ColumnDef

| 字段 | 类型 | 说明 |
|------|------|------|
| `Name` | `string` | 列名 |
| `Type` | `string` | ClickHouse 类型，如 `"String"`, `"DateTime64(3)"`, `"LowCardinality(String)"` |
| `Default` | `string` | SQL DEFAULT 表达式，如 `"now64(3)"`, `"0"` |
| `Searchable` | `bool` | 为 true 时自动加入 `ORDER BY` 并生成 `bloom_filter` 索引 |

### TableDef

| 字段 | 类型 | 说明 |
|------|------|------|
| `Database` | `string` | 数据库名 |
| `Table` | `string` | 表名 |
| `Columns` | `[]ColumnDef` | 列定义 |
| `Engine` | `string` | 默认为 `"MergeTree"` |
| `PartitionBy` | `string` | 分区表达式，如 `"toYYYYMMDD(ts)"` |
| `OrderBy` | `[]string` | 显式排序键；为空时从 `Searchable` 列自动生成 |
| `TTLDays` | `int` | `>0` 时在第一个 DateTime 列上生成 TTL 子句 |

### DDL 函数

| 函数 | 说明 |
|------|------|
| `CreateDatabaseSQL(name)` | 返回 `"CREATE DATABASE IF NOT EXISTS \`name\`"` |
| `CreateTableSQL(def)` | 从 `TableDef` 生成完整 DDL |
| `(d TableDef).Validate()` | 校验 Database / Table / Column 名称合法 |

### 示例

```go
def := ck.TableDef{
    Database: "mydb",
    Table:    "events",
    Columns: []ck.ColumnDef{
        {Name: "ts",        Type: "DateTime64(3)", Default: "now64(3)", Searchable: true},
        {Name: "user_id",   Type: "String",        Searchable: true},
        {Name: "event",     Type: "String",        Searchable: true},
        {Name: "duration",  Type: "Int64"},
        {Name: "payload",   Type: "String",        Default: "''"},
    },
    PartitionBy: "toYYYYMMDD(ts)",
    TTLDays:     90,
}

// 可选：先校验
if err := def.Validate(); err != nil {
    panic(err)
}

// 建库
conn.Exec(ctx, ck.CreateDatabaseSQL(def.Database))

// 建表
conn.Exec(ctx, ck.CreateTableSQL(def))

// 生成的 SQL 形如：
//
//   CREATE TABLE IF NOT EXISTS `mydb`.`events` (
//       `ts` DateTime64(3) DEFAULT now64(3),
//       `user_id` String,
//       `event` String,
//       `duration` Int64,
//       `payload` String DEFAULT '',
//       INDEX idx_user_id `user_id` TYPE bloom_filter GRANULARITY 1,
//       INDEX idx_event `event` TYPE bloom_filter GRANULARITY 1
//   ) ENGINE = MergeTree
//   PARTITION BY toYYYYMMDD(ts)
//   ORDER BY (ts, user_id, event)
//   TTL toDateTime(`ts`) + INTERVAL 90 DAY
```

### 生成规则

- `Searchable: true` 的列 → 自动进入 `ORDER BY` + 生成 `INDEX idx_xxx TYPE bloom_filter GRANULARITY 1`
- 显式设置 `OrderBy` → 覆盖自动生成
- `TTLDays > 0` → 选取第一个 `DateTime` 列（优先精确匹配 `DateTime`，其次 `DateTime64`）生成 TTL
- `DateTime` / `DateTime64` / `Date` 类型的列不生成 bloom_filter（ClickHouse 不支持时间类型的 data skipping index）
- 列名含特殊字符时自动用反引号包裹

---

## 游标流

游标流解决的核心问题：**给一条 SELECT，先同步拿全量历史数据，然后持续拿增量**。

### 创建

```go
// baseQuery = SELECT ... FROM ... WHERE ...
// 不要包含 ORDER BY 和 LIMIT，框架自动追加。
// cursorCol 通常是一个单调递增的 DateTime 列（如 "ts"）。

// 无 WHERE
stream := conn.NewCursorStream("SELECT * FROM events", "ts", 1000)

// 有 WHERE，带占位符
stream := conn.NewCursorStream("SELECT * FROM events WHERE type = ?", "ts", 500)
```

### 同步消费全量

```go
for {
    batch, err := stream.Next(ctx, "click")  // args 对应 query 中的 ? 占位符
    if err != nil {
        return err
    }
    if len(batch) == 0 {
        break  // 无更多数据
    }
    for _, row := range batch {
        fmt.Println(row["user_id"], row["event"])
    }
}
```

### 检查点（中断续传）

```go
// 保存
checkpoint := stream.Checkpoint()  // ck.Cursor{Value: "2026-06-06T15:04:05"}
saveToRedis("my_job_cursor", checkpoint)

// 恢复
var c ck.Cursor
loadFromRedis("my_job_cursor", &c)
stream.SetCheckpoint(c)           // 零值 Cursor 表示从头开始

// 恢复后继续调用 Next — 从 checkpoint 位置之后拿数据
for {
    batch, _ := stream.Next(ctx, "click")
    if len(batch) == 0 { break }
    ...
    saveToRedis("my_job_cursor", stream.Checkpoint())  // 每批后保存
}
```

### 持续轮询增量

```go
// 先同步消费历史（同上一节）
for { batch, _ := stream.Next(ctx, "click"); if len(batch) == 0 { break }; ... }

// 然后异步轮询新数据
go stream.Poll(ctx, ck.WatchOptions{}, func(batch []ck.Row) error {
    for _, row := range batch {
        publishEvent(row)
    }
    return nil
}, "click")
```

**Poll 算法**：有数据时连续消费（0 等待）；无数据时指数退避 `min(interval × BackoffFactor, MaxInterval)`；数据恢复时重置为 `MinInterval`。

### SQL 生成规则

这是理解游标流的关键：

```
baseQuery = "SELECT * FROM t"
────────────────────────────────────────────────────────
首次 Next:  SELECT * FROM t ORDER BY ts LIMIT 500
后续 Next:  SELECT * FROM t WHERE ts > ? ORDER BY ts LIMIT 500
────────────────────────────────────────────────────────

baseQuery = "SELECT * FROM t WHERE type = ?"
────────────────────────────────────────────────────────
首次 Next:  SELECT * FROM t WHERE type = ? ORDER BY ts LIMIT 500
后续 Next:  SELECT * FROM t WHERE type = ? AND ts > ? ORDER BY ts LIMIT 500
```

**要点**：
- 首次 `Next()` 时 cursor 为 nil → 不加 WHERE 条件，拿全量
- 之后每次 `Next()` 末尾自动推进 cursor 到该批最后一行的 cursorCol 值
- 带 WHERE 的查询自动追加 `AND`，不带 WHERE 自动追加 `WHERE`
- `Next()` 的 args 直接透传给 query 的 `?` 占位符，框架参数（cursor 值、batchSize）追加在末尾

### CursorStream 方法

| 方法 | 说明 |
|------|------|
| `Next(ctx, args...)` | 取下一批。首次全量，后续增量。返回空切片表示无新数据 |
| `Checkpoint()` | 获取当前游标位置 `Cursor{Value}`，可 JSON 序列化 |
| `SetCheckpoint(c)` | 恢复游标，零值 `Cursor{}` 重置 |
| `SQL(args...)` | 调试用，返回下次 `Next()` 会执行的参数化 SQL |
| `Poll(ctx, opts, fn, args...)` | 自适应轮询，直到 ctx 取消 |

### WatchOptions

| 字段 | 默认值 | 说明 |
|------|--------|------|
| `MinInterval` | `100ms` | 有数据时的最小轮询间隔 |
| `MaxInterval` | `5s` | 空闲时最大轮询间隔 |
| `BackoffFactor` | `2.0` | 退避乘数 |
| `NoJitter` | `false` | `true` 时禁用 ±25% 随机抖动 |

零值 `WatchOptions{}` 即使用全部默认值。

---

## Row 类型

`Row` 是 `map[string]any`，所有列值转为字符串：

| ClickHouse 类型 | Go `any` 实际类型 | 格式化结果示例 |
|-----------------|-------------------|----------------|
| `String`, `FixedString` | `string` | `"alice"` |
| `Int8` ~ `Int64` | `string` | `"123"` |
| `UInt8` ~ `UInt64` | `string` | `"456"` |
| `Float32`, `Float64` | `string` | `"1.500000"` |
| `Bool` | `string` | `"true"` / `"false"` |
| `DateTime`, `DateTime64` | `string` | `"2026-01-02 15:04:05.000"` |
| `Date` | `string` | `"2026-01-02"`（time.Time fallback） |
| 其他 | `string` | `fmt.Sprint(v)` |

---

## 常用模式

### 模式 1：全量导出 + 断点续传

```go
stream := conn.NewCursorStream("SELECT * FROM events", "ts", 5000)

// 从外部存储恢复
if c, ok := loadCheckpoint(); ok {
    stream.SetCheckpoint(c)
}

for {
    batch, err := stream.Next(ctx)
    if err != nil {
        return err
    }
    if len(batch) == 0 {
        break
    }
    export(batch)
    saveCheckpoint(stream.Checkpoint())  // 每批后保存
}
```

### 模式 2：实时事件管道

```go
stream := conn.NewCursorStream(
    "SELECT * FROM events WHERE status = ?", "ts", 500,
)

// Phase 1: 同步回放历史
for {
    batch, _ := stream.Next(ctx, "pending")
    if len(batch) == 0 { break }
    replayHistory(batch)
}

// Phase 2: 持续消费新事件
go stream.Poll(ctx, ck.WatchOptions{
    MinInterval: 50 * time.Millisecond,  // 低延迟
    MaxInterval: 1 * time.Second,
}, func(batch []ck.Row) error {
    for _, row := range batch {
        publish(row)
    }
    return nil
}, "pending")
```

### 模式 3：多实例竞争消费（Consul 锁 + 游标）

```go
// 获取分布式锁
lock, _ := consulLock.Acquire(ctx, "events_consumer")
defer lock.Release()

stream := conn.NewCursorStream("SELECT * FROM events", "ts", 1000)
stream.SetCheckpoint(loadCheckpoint())

for {
    batch, err := stream.Next(ctx)
    if err != nil { return err }
    if len(batch) == 0 { break }
    process(batch)
    saveCheckpoint(stream.Checkpoint())
}
```

### 模式 4：调试 SQL

```go
stream := conn.NewCursorStream("SELECT * FROM t WHERE type = ?", "ts", 100)
stream.SetCheckpoint(ck.Cursor{Value: "2026-01-01"})
fmt.Println(stream.SQL("click"))
// → SELECT * FROM t WHERE type = ? AND ts > ? ORDER BY ts LIMIT ? [click 2026-01-01 100]
```

---

## 与 chlog 集成

`logit/chlog` 的日志表 DDL 直接使用 `ck.TableDef` 和 `ck.ColumnDef`：

```go
// chlog 配置中
Schema: &ck.TableDef{
    Columns: []ck.ColumnDef{
        {Name: "ts",         Type: "DateTime64(3)"},
        {Name: "raw",        Type: "String"},
        {Name: "request_id", Type: "String", Searchable: true},
    },
    PartitionBy: "toYYYYMMDD(ts)",
    TTLDays:     30,
},
```

两者共享相同的 DDL 类型，chlog 创建的日志表可以用 `CursorStream` 消费。
