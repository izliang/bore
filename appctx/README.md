# appctx —— 通用 Web 应用上下文

将 `context.Context`、`logit.Logger` 和基础组件（GORM、Redis）整合为单一不可变值，作为调用链的第一个参数传递。设计出发点：一个 Web 请求最基本的需求（取消、超时、日志、DB、缓存）打包在一个参数里，减少函数签名复杂度。

## 快速开始

```go
import "github.com/izliang/bore/appctx"

// 默认构造
ctx := appctx.New()

// 完整构造
ctx := appctx.New(
    appctx.WithLogger(yourLogger),
    appctx.WithContext(baseCtx),
    appctx.WithGORM(gormDB),
    appctx.WithRedis(redisClient),
)
```

## 核心概念

### AppContext 接口

```go
type AppContext interface {
    context.Context                    // 可直接传入任何接受 context.Context 的函数

    Context()    context.Context       // 显式获取内部 context
    Logger()     Logger                // 日志门面，已绑定当前 context
    LogitLogger() logit.Logger         // 底层 logit.Logger，完整 API
    With(ctx context.Context) AppContext // 派生新上下文，复用 logger/GORM/Redis

    Info(msg string, fields ...logit.FieldFunc)
    Debug(msg string, fields ...logit.FieldFunc)
    Warn(msg string, fields ...logit.FieldFunc)
    Error(err error, fields ...logit.FieldFunc)

    GORM()  *gorm.DB       // 数据库句柄，未注入时返回 nil
    Redis() *redis.Client  // Redis 客户端，未注入时返回 nil
}
```

### 设计原则

- **不可变**：`With()` 返回新实例，不修改原值，多 goroutine 并发安全
- **fail-fast**：构造阶段传入 nil logger / nil context 立即 panic，不让错误推迟到远程调用点
- **运行时软降级**：日志方法在 logger 为 nil 时静默丢弃（noop），GORM/Redis 未注入时返回 nil 由调用方判断

## 用法指南

### 1. 构造与传递

```go
func main() {
    baseCtx, cancel := context.WithCancel(context.Background())
    defer cancel()

    ctx := appctx.New(
        appctx.WithLogger(logger),
        appctx.WithContext(baseCtx),
        appctx.WithGORM(gormDB),
        appctx.WithRedis(redisClient),
    )

    svc := NewUserService()
    svc.GetUser(ctx, "user-001")
}

// 业务方法签名：第一个参数固定为 appctx.AppContext
func (s *UserService) GetUser(ctx appctx.AppContext, id string) (*User, error) {
    ctx.Info("查询用户", logit.StringField("id", id))
    // ...
}
```

### 2. 直接作为 context.Context 使用

```go
// 传给 GORM
db := ctx.GORM().WithContext(ctx).Find(&users)

// 传给 HTTP client
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)

// 传给任何接受 context.Context 的函数
result, err := someLib.DoSomething(ctx, input)
```

### 3. 日志记录

```go
// 直接调用
ctx.Info("请求开始", logit.StringField("method", "GET"))
ctx.Debug("查询参数", logit.AnyField("params", params))
ctx.Error(err, logit.StringField("user_id", id))

// 通过 Logger 门面（向后兼容）
ctx.Logger().Info("请求开始")
```

### 4. 传播请求级字段

`lctx.PushPair` 将 request_id、trace_id 等字段挂到 context 上，`With()` 派生新 AppContext 后，后续所有日志自动携带这些字段。

```go
// 中间件中附加请求 ID
ctx = ctx.With(lctx.PushPair(ctx.Context(), requestFormatter))

// 后续日志自动包含 request_id
ctx.Info("处理完成") // 日志行：... request_id=abc-123 msg="处理完成"
```

### 5. GORM 和 Redis 的使用

```go
// 数据库查询
func (s *Repo) FindUser(ctx appctx.AppContext, id string) (*User, error) {
    db := ctx.GORM()
    if db == nil {
        return nil, errors.New("数据库未初始化")
    }
    var user User
    err := db.WithContext(ctx).First(&user, "id = ?", id).Error
    return &user, err
}

// Redis 缓存
func (s *Repo) GetCached(ctx appctx.AppContext, key string) (string, error) {
    rc := ctx.Redis()
    if rc == nil {
        return "", errors.New("Redis 未初始化")
    }
    return rc.Get(ctx, key).Result()
}
```

## nil 安全约定

| 场景 | 行为 | 设计理由 |
|------|------|---------|
| `WithContext(nil)` | **panic** | 非法参数，构造期立即暴露 |
| `With(nil)` | **panic** | 同上 |
| `WithLogger(nil)` | **panic** | 同上 |
| `Info/Debug/Warn/Error`（logger 为 nil） | **noop**，静默丢弃 | 极端场景下的 safety net |
| `GORM()` 未调用 `WithGORM` | 返回 nil | 部分服务不需要数据库 |
| `Redis()` 未调用 `WithRedis` | 返回 nil | 部分服务不需要缓存 |

## 测试支持

`MockAppContext` 提供完整的 AppContext 测试替身，内置 sqlmock 支持。

### 基础用法

```go
import "github.com/izliang/bore/appctx"

func TestUserService_GetUser(t *testing.T) {
    // 一行创建带 sqlmock 的 MockAppContext
    mock := appctx.NewMock(appctx.WithSQLMock())
    defer mock.Close()

    // 设置数据库预期
    rows := sqlmock.NewRows([]string{"id", "name"}).
        AddRow(1, "alice")
    mock.SQLMock.ExpectQuery("SELECT .* FROM `users`").
        WillReturnRows(rows)

    // 执行业务逻辑
    svc := NewUserService()
    user, err := svc.GetUser(mock, "1")

    assert.NoError(t, err)
    assert.Equal(t, "alice", user.Name)

    // 断言所有数据库预期均已满足
    assert.NoError(t, mock.ExpectationsWereMet())
}
```

### 完整自定义

```go
func TestWithCustomDeps(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    gormDB, sqlMock, _ := appctx.NewSQLMock()
    defer appctx.CloseSQLMock(gormDB)

    // 注入 miniredis（需自行引入 github.com/alicebob/miniredis/v2）
    mr := miniredis.RunT(t)
    rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})

    mock := appctx.NewMock(
        appctx.WithMockContext(ctx),
        appctx.WithMockGORM(gormDB),
        appctx.WithMockRedis(rc),
    )

    // mock.SQLMock 指向 sqlMock
    // mock.Redis() 返回 miniredis 客户端
}
```

### MockOption 一览

| Option | 作用 |
|--------|------|
| `WithMockContext(ctx)` | 注入自定义 `context.Context` |
| `WithMockLogger(l)` | 注入自定义 `logit.Logger` |
| `WithMockGORM(db)` | 注入自定义 `*gorm.DB` |
| `WithMockRedis(rc)` | 注入自定义 `*redis.Client` |
| `WithSQLMock()` | 自动创建带 sqlmock 的 GORM 并注入 |

### 独立 SQL Mock 函数

```go
// 创建
gormDB, sqlMock, err := appctx.NewSQLMock()
defer appctx.CloseSQLMock(gormDB)

// 设置预期
sqlMock.ExpectExec("INSERT INTO `users`").
    WillReturnResult(sqlmock.NewResult(1, 1))

// 验证
assert.NoError(t, sqlMock.ExpectationsWereMet())
```

## 注意事项

- **GORM 和 Redis 是连接池单例**：通过 `WithGORM` / `WithRedis` 注入的是指针，整个进程复用同一实例。连接池、方言等配置应在传入前完成
- **`With()` 不改变 GORM/Redis**：`With()` 只替换 context，logger、GORM、Redis 保持原值不变。需要独立实例时应构造新的 AppContext
- **`Logger()` 返回的是门面**：`ctx.Logger().Info(...)` 和 `ctx.Info(...)` 行为完全一致，推荐直接使用 `ctx.Info(...)`
- **MockAppContext 的 `L` 字段为 nil 时日志 noop**：测试中如果不关心日志输出，无需注入 mock logger
