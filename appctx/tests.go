package appctx

import (
	"context"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/izliang/bore/logit"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// ============================================================
// MockAppContext —— AppContext 的测试替身
// ============================================================

// MockAppContext 是 AppContext 的可配置测试替身。
//
// 所有字段均可直接赋值，未设置的字段有安全默认值：
//   - Ctx 为 nil 时自动使用 context.Background()
//   - L 为 nil 时日志方法静默丢弃（noop），不会 panic
//   - DB / RC 为 nil 时 GORM() / Redis() 返回 nil，调用方自行判断
//
// 使用示例：
//
//	// 1. 最简构造
//	mock := appctx.NewMock()
//
//	// 2. 携带 sqlmock
//	mock := appctx.NewMock(appctx.WithSQLMock())
//	mock.SQLMock.ExpectQuery("SELECT").WillReturnRows(...)
//
//	// 3. 完全自定义
//	mock := appctx.NewMock(
//	    appctx.WithMockContext(ctx),
//	    appctx.WithMockLogger(logger),
//	    appctx.WithMockGORM(gormDB),
//	    appctx.WithMockRedis(redisClient),
//	)
type MockAppContext struct {
	// Ctx 内部 context，实现 context.Context 接口的方法均委托给它。
	// 为 nil 时自动回退为 context.Background()。
	Ctx context.Context

	// L 底层 logit.Logger。为 nil 时 Info/Debug/Warn/Error 均为 noop，不会 panic。
	L logit.Logger

	// DB GORM 数据库实例。GORM() 返回此值。
	DB *gorm.DB

	// RC Redis 客户端。Redis() 返回此值。
	RC *redis.Client

	// SQLMock 与 DB 关联的 sqlmock 实例。仅当通过 WithSQLMock() 或 NewSQLMock() 构造时填充。
	// 业务测试中通过此字段设置数据库预期行为：
	//
	//	mock.SQLMock.ExpectQuery("SELECT .* FROM users").WillReturnRows(rows)
	SQLMock sqlmock.Sqlmock
}

// 编译期检查：MockAppContext 实现了 AppContext
var _ AppContext = (*MockAppContext)(nil)

// 编译期检查：MockAppContext 实现了 Logger
var _ Logger = (*MockAppContext)(nil)

// ============================================================
// context.Context 接口实现（委托给 Ctx）
// ============================================================

// Deadline 实现 context.Context，当 Ctx 为 nil 时返回零值。
func (m *MockAppContext) Deadline() (deadline time.Time, ok bool) {
	if m.Ctx == nil {
		return time.Time{}, false
	}
	return m.Ctx.Deadline()
}

// Done 实现 context.Context，当 Ctx 为 nil 时返回 nil。
func (m *MockAppContext) Done() <-chan struct{} {
	if m.Ctx == nil {
		return nil
	}
	return m.Ctx.Done()
}

// Err 实现 context.Context，当 Ctx 为 nil 时返回 nil。
func (m *MockAppContext) Err() error {
	if m.Ctx == nil {
		return nil
	}
	return m.Ctx.Err()
}

// Value 实现 context.Context，当 Ctx 为 nil 时返回 nil。
func (m *MockAppContext) Value(key any) any {
	if m.Ctx == nil {
		return nil
	}
	return m.Ctx.Value(key)
}

// ============================================================
// AppContext 接口实现
// ============================================================

// Context 返回内部 context。若 Ctx 为 nil 则返回 context.Background()。
func (m *MockAppContext) Context() context.Context {
	if m.Ctx == nil {
		return context.Background()
	}
	return m.Ctx
}

// LogitLogger 返回底层 logit.Logger。
func (m *MockAppContext) LogitLogger() logit.Logger {
	return m.L
}

// Logger 返回 Logger 门面。MockAppContext 自身实现了 Logger 接口，
// 因此 Logger() 返回自身。
func (m *MockAppContext) Logger() Logger {
	return m
}

// GORM 返回配置的 GORM 实例，未注入时返回 nil。
func (m *MockAppContext) GORM() *gorm.DB {
	return m.DB
}

// Redis 返回配置的 Redis 客户端，未注入时返回 nil。
func (m *MockAppContext) Redis() *redis.Client {
	return m.RC
}

// With 返回新的 MockAppContext，包装给定的 ctx 并复用 logger、GORM、Redis 和 SQLMock。
// 若传入 nil context 会 panic。
func (m *MockAppContext) With(ctx context.Context) AppContext {
	if ctx == nil {
		panic("appctx: MockAppContext.With 传入 nil context")
	}
	return &MockAppContext{
		Ctx:     ctx,
		L:       m.L,
		DB:      m.DB,
		RC:      m.RC,
		SQLMock: m.SQLMock,
	}
}

// ============================================================
// Logger 接口实现（委托给 L，L 为 nil 时 noop）
// ============================================================

// Info 记录 INFO 级别日志。L 为 nil 时 noop。
func (m *MockAppContext) Info(msg string, fields ...logit.FieldFunc) {
	if m.L != nil {
		m.L.Info(m.Context(), msg, fields...)
	}
}

// Debug 记录 DEBUG 级别日志。L 为 nil 时 noop。
func (m *MockAppContext) Debug(msg string, fields ...logit.FieldFunc) {
	if m.L != nil {
		m.L.Debug(m.Context(), msg, fields...)
	}
}

// Warn 记录 WARN 级别日志。L 为 nil 时 noop。
func (m *MockAppContext) Warn(msg string, fields ...logit.FieldFunc) {
	if m.L != nil {
		m.L.Warn(m.Context(), msg, fields...)
	}
}

// Error 记录错误日志。L 为 nil 时 noop。
func (m *MockAppContext) Error(err error, fields ...logit.FieldFunc) {
	if m.L != nil {
		m.L.Error(m.Context(), err, fields...)
	}
}

// ============================================================
// MockOption —— 配置 MockAppContext
// ============================================================

// MockOption 是 NewMock() 的配置函数。
type MockOption func(*MockAppContext)

// WithMockContext 注入自定义 context。
func WithMockContext(ctx context.Context) MockOption {
	return func(m *MockAppContext) {
		m.Ctx = ctx
	}
}

// WithMockLogger 注入自定义 logit.Logger。
func WithMockLogger(l logit.Logger) MockOption {
	return func(m *MockAppContext) {
		m.L = l
	}
}

// WithMockGORM 注入自定义 *gorm.DB。
func WithMockGORM(db *gorm.DB) MockOption {
	return func(m *MockAppContext) {
		m.DB = db
	}
}

// WithMockRedis 注入自定义 *redis.Client。
// 可配合 miniredis 使用：
//
//	mr := miniredis.RunT(t)
//	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
//	mock := appctx.NewMock(appctx.WithMockRedis(rc))
func WithMockRedis(rc *redis.Client) MockOption {
	return func(m *MockAppContext) {
		m.RC = rc
	}
}

// ============================================================
// 构造器
// ============================================================

// NewMock 创建 MockAppContext。
//
// 默认值：
//   - Ctx = context.Background()
//   - L    = nil（日志 noop）
//   - DB   = nil
//   - RC   = nil
//
// 示例：
//
//	// 基础构造
//	mock := appctx.NewMock()
//
//	// 携带 sqlmock
//	mock := appctx.NewMock(appctx.WithSQLMock())
//	mock.SQLMock.ExpectQuery("SELECT").WillReturnRows(...)
func NewMock(opts ...MockOption) *MockAppContext {
	m := &MockAppContext{
		Ctx: context.Background(),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// ============================================================
// SQL Mock
// ============================================================

// NewSQLMock 创建带 sqlmock 的 GORM 实例（MySQL 方言）。
//
// 返回的 *gorm.DB 已禁用初始化版本查询，适合单元测试。
// sqlmock.Sqlmock 用于设置 SQL 预期行为。
//
// 示例：
//
//	gormDB, sqlMock, err := appctx.NewSQLMock()
//	if err != nil {
//	    t.Fatal(err)
//	}
//	defer func() { _ = appctx.CloseSQLMock(gormDB) }()
//
//	rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "alice")
//	sqlMock.ExpectQuery("SELECT .* FROM `users`").WillReturnRows(rows)
func NewSQLMock() (*gorm.DB, sqlmock.Sqlmock, error) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		return nil, nil, err
	}
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		return nil, nil, err
	}
	return gormDB, mock, nil
}

// CloseSQLMock 关闭 sqlmock 的底层 *sql.DB。
// 应在测试 defer 中调用。
func CloseSQLMock(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// WithSQLMock 创建一个 MockOption，自动构造带 sqlmock 的 GORM 并注入到 MockAppContext。
//
// 创建成功后，可通过 MockAppContext.SQLMock 字段访问 sqlmock 实例来设置预期行为。
// 返回的 MockOption 在 sqlmock 创建失败时会 panic（因为是前置条件错误，fail-fast）。
//
// 示例：
//
//	mock := appctx.NewMock(appctx.WithSQLMock())
//	defer mock.Close()
//	mock.SQLMock.ExpectQuery("SELECT .* FROM `users`").WillReturnRows(rows)
func WithSQLMock() MockOption {
	return func(m *MockAppContext) {
		sqlDB, sqlMock, err := sqlmock.New()
		if err != nil {
			panic("appctx: WithSQLMock create sqlmock error: " + err.Error())
		}
		gormDB, err := gorm.Open(mysql.New(mysql.Config{
			Conn:                      sqlDB,
			SkipInitializeWithVersion: true,
		}), &gorm.Config{})
		if err != nil {
			sqlDB.Close()
			panic("appctx: WithSQLMock create GORM instance error: " + err.Error())
		}
		m.DB = gormDB
		m.SQLMock = sqlMock
	}
}

// ============================================================
// 资源清理
// ============================================================

// Close 关闭 MockAppContext 中的底层资源（*sql.DB）。
// 如果未注入 DB 或 DB 底层不可用，返回 nil。
//
// 建议在测试中使用 defer mock.Close()。
func (m *MockAppContext) Close() error {
	return CloseSQLMock(m.DB)
}

// ExpectationsWereMet 检查 sqlmock 的所有预期是否都已满足。
// 若未注入 SQLMock，返回 nil。
//
// 建议在每个测试末尾调用：
//
//	assert.NoError(t, mock.ExpectationsWereMet())
func (m *MockAppContext) ExpectationsWereMet() error {
	if m.SQLMock == nil {
		return nil
	}
	return m.SQLMock.ExpectationsWereMet()
}
