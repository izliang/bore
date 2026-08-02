// Package appctx 通用 Web 应用上下文，将 context.Context、logit.Logger 和基础组件（GORM、Redis）
// 整合为单一不可变值，在调用链中传递。
//
// # 设计约定
//
//   - 不可变性：With() 返回新的 AppContext，绝不修改接收者。与 context.Context 一致。
//   - 并发安全：多 goroutine 同时使用安全。
//   - 日志始终使用当前 context：lctx.PushPair 添加的字段会自动出现在后续日志中。
//   - nil 安全：构造时若传入 nil ctx / nil logger 会立即 panic（fail-fast），
//     避免将错误推迟到遥远的调用点。
//
// # 常见用法
//
//  1. 在应用入口构造：
//
//     ctx := appctx.New(appctx.WithLogger(yourLogger), appctx.WithContext(baseCtx))
//
//  2. 作为所有 API 调用的第一个参数：
//
//     func (h *Handler) Handle(ctx appctx.AppContext, req *Request) (*Response, error)
//
//  3. 直接使用便捷日志方法：
//
//     ctx.Info("处理请求", logit.StringField("id", req.ID))
//
//  4. 为下游调用附加日志字段：
//
//     ctx = ctx.With(lctx.PushPair(ctx.Context(), requestFormatter))
//
//  5. 在需要 context.Context 的地方直接传入：
//
//     httpReq, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
//
// # 向后兼容
//
//   - ctx.Context() 仍然可用
//   - ctx.Logger().Info() 仍然可用
//   - appctx.ParamFunc 别名仍然保留
//   - 无破坏性变更
package appctx

import (
	"context"
	"os"
	"time"

	"github.com/izliang/bore/logit"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// AppContext 通用 Web 应用上下文，整合 context.Context、logit.Logger 以及基础组件（GORM、Redis）。
//
// AppContext 直接实现了 context.Context，可传给任何接受 context.Context 的函数。
//
// 不可变：With() 返回新实例，保留 logger 和基础组件。调用 lctx.PushPair 后使用
// With() 更新上下文，后续日志会自动携带新增字段。
//
// # nil 安全约定
//
// 构造阶段（fail-fast —— 立即 panic，避免将错误推迟到远程调用点）：
//
//   - WithContext(nil) → panic("appctx: WithContext 传入 nil context")
//   - With(nil)        → panic("appctx: With 传入 nil context")
//   - WithLogger(nil)  → panic("appctx: WithLogger 传入 nil logger")
//
// 运行时（软降级 —— 由调用方负责检查）：
//
//   - GORM() 在未调用 WithGORM 时返回 nil，调用方负责 nil 检查
//   - Redis() 在未调用 WithRedis 时返回 nil，调用方负责 nil 检查
//   - 日志方法（Info/Debug/Warn/Error）在 logger 为 nil 时静默丢弃（noop）
type AppContext interface {
	// Context context.Context：Deadline、Done、Err、Value 委托给内部的 context。
	// 可直接将 AppContext 传给任何接受 context.Context 的函数。
	context.Context

	// Context 返回底层 context.Context。
	// 推荐直接传入 AppContext 本身；此方法为兼容旧代码保留。
	Context() context.Context

	// Logger 返回绑定到此 AppContext 的 Logger 门面。
	// 每次日志调用都使用当前包装的 context，因此 lctx.PushPair 添加的字段
	// 会自动出现在后续日志中。
	Logger() Logger

	// LogitLogger 返回底层 logit.Logger（完整 API，如 writer 或 helper）。
	LogitLogger() logit.Logger

	// With 返回新的 AppContext，包装给定的 ctx，同时复用接收者的 logger、GORM 和 Redis 句柄。
	// 用于附加 deadline、cancellation、请求域值或 lctx 字段栈。
	//
	// 若传入 nil context 会立即 panic。
	//
	// 示例：
	//
	//	ctx = ctx.With(lctx.PushPair(ctx.Context(), myLogFields))
	With(ctx context.Context) AppContext

	// Info 使用当前 context 记录 INFO 级别日志。
	Info(msg string, fields ...logit.FieldFunc)
	// Debug 使用当前 context 记录 DEBUG 级别日志。
	Debug(msg string, fields ...logit.FieldFunc)
	// Warn 使用当前 context 记录 WARN 级别日志。
	Warn(msg string, fields ...logit.FieldFunc)
	// Error 使用当前 context 记录错误，附带可选的结构化字段。
	Error(err error, fields ...logit.FieldFunc)

	// GORM 返回构造时绑定的 GORM 数据库句柄，未调用 WithGORM 时返回 nil。
	GORM() *gorm.DB
	// Redis 返回构造时绑定的 Redis 客户端，未调用 WithRedis 时返回 nil。
	Redis() *redis.Client
}

// Logger 是 AppContext.Logger() 返回的日志门面。
// 方法省略了 context 参数，因为 logger 已绑定到 AppContext 的 context。
type Logger interface {
	Info(msg string, fields ...logit.FieldFunc)
	Debug(msg string, fields ...logit.FieldFunc)
	Warn(msg string, fields ...logit.FieldFunc)
	Error(err error, fields ...logit.FieldFunc)
}

// Option 是 New() 用于配置 AppContext 的函数。
type Option func(*impl)

// ParamFunc 是 Option 的别名，向后兼容旧代码。
//
// Deprecated: 使用 Option 代替。此别名仅为保持旧代码编译通过而保留。
type ParamFunc = Option

// WithLogger 设置底层 logit.Logger。
// 传入 nil 会立即 panic。
//
// 示例：
//
//	logger := logit.NewSimpleWithConfig(os.Stdout, &logit.Config{Level: logit.Debug})
//	ctx := appctx.New(appctx.WithLogger(logger))
func WithLogger(logger logit.Logger) Option {
	return func(r *impl) {
		if logger == nil {
			panic("appctx: WithLogger nil logger")
		}
		r.logger = logger
	}
}

// WithContext 设置基础 context。
// 传入 nil 会立即 panic。
//
// 示例：
//
//	baseCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	ctx := appctx.New(appctx.WithContext(baseCtx))
func WithContext(ctx context.Context) Option {
	return func(r *impl) {
		if ctx == nil {
			panic("appctx: WithContext nil context")
		}
		r.ctx = ctx
	}
}

// WithGORM 绑定 *gorm.DB 到 AppContext。
// GORM() 将返回同一实例；请在传入前完成方言和连接池的配置。
func WithGORM(db *gorm.DB) Option {
	return func(r *impl) {
		r.gorm = db
	}
}

// WithRedis 绑定 go-redis Client 到 AppContext。
func WithRedis(redis *redis.Client) Option {
	return func(r *impl) {
		r.redis = redis
	}
}

// New 创建新的 AppContext。
//
// 未指定时的默认值：
//   - Context = context.Background()
//   - Logger  = logit.NewSimple(os.Stdout)（输出到 stdout，INFO 级别）
//
// 示例：
//
//	// 默认 AppContext
//	ctx := appctx.New()
//
//	// 指定 logger
//	logger := logit.NewSimple(os.Stdout)
//	ctx := appctx.New(appctx.WithLogger(logger))
//
//	// 同时指定 logger 和带取消的 context
//	baseCtx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	ctx := appctx.New(appctx.WithLogger(logger), appctx.WithContext(baseCtx))
func New(opts ...Option) AppContext {
	r := &impl{
		ctx:    context.Background(),
		logger: logit.NewSimple(os.Stdout),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// impl 是 AppContext 的具体实现。保持 context 和 logger 同步，
// 确保结构化日志与 context API 始终观察相同的取消和值。
type impl struct {
	ctx    context.Context
	logger logit.Logger
	gorm   *gorm.DB
	redis  *redis.Client
}

// Deadline 实现 context.Context。
func (r *impl) Deadline() (deadline time.Time, ok bool) {
	return r.ctx.Deadline()
}

// Done 实现 context.Context。
func (r *impl) Done() <-chan struct{} {
	return r.ctx.Done()
}

// Err 实现 context.Context。
func (r *impl) Err() error {
	return r.ctx.Err()
}

// Value 实现 context.Context。
func (r *impl) Value(key any) any {
	return r.ctx.Value(key)
}

// Context 返回底层 context。
func (r *impl) Context() context.Context {
	return r.ctx
}

// LogitLogger 返回底层 logit.Logger。
func (r *impl) LogitLogger() logit.Logger {
	return r.logger
}

// Logger 返回 Logger 门面，始终使用此 AppContext 的当前 context。
func (r *impl) Logger() Logger {
	return r
}

// GORM 返回配置的数据库句柄，未调用 WithGORM 时返回 nil。
func (r *impl) GORM() *gorm.DB {
	return r.gorm
}

// Redis 返回配置的 Redis 客户端，未调用 WithRedis 时返回 nil。
func (r *impl) Redis() *redis.Client {
	return r.redis
}

// With 返回新的 AppContext，包装给定的 ctx，同时复用接收者的 logger、GORM 和 Redis 句柄。
// 传入 nil context 会立即 panic。
func (r *impl) With(ctx context.Context) AppContext {
	if ctx == nil {
		panic("appctx: With nil context")
	}
	return &impl{
		ctx:    ctx,
		logger: r.logger,
		gorm:   r.gorm,
		redis:  r.redis,
	}
}

// Info 使用当前 context 记录 INFO 级别日志。
// logger 为 nil 时静默丢弃（noop），不会 panic。
func (r *impl) Info(msg string, fields ...logit.FieldFunc) {
	if r.logger == nil {
		return
	}
	r.logger.Info(r.ctx, msg, fields...)
}

// Debug 使用当前 context 记录 DEBUG 级别日志。
// logger 为 nil 时静默丢弃（noop），不会 panic。
func (r *impl) Debug(msg string, fields ...logit.FieldFunc) {
	if r.logger == nil {
		return
	}
	r.logger.Debug(r.ctx, msg, fields...)
}

// Warn 使用当前 context 记录 WARN 级别日志。
// logger 为 nil 时静默丢弃（noop），不会 panic。
func (r *impl) Warn(msg string, fields ...logit.FieldFunc) {
	if r.logger == nil {
		return
	}
	r.logger.Warn(r.ctx, msg, fields...)
}

// Error 使用当前 context 记录错误日志。
// logger 为 nil 时静默丢弃（noop），不会 panic。
func (r *impl) Error(err error, fields ...logit.FieldFunc) {
	if r.logger == nil {
		return
	}
	r.logger.Error(r.ctx, err, fields...)
}
