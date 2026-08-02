package logit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// GormAdapter 在 logit.Logger 之上实现 gorm.io/gorm/logger.Interface。
// 它将 GORM 的日志级别和 SQL 跟踪事件映射为结构化日志行（通过 FieldFunc 传递字段）。
//
// 配置：
//   - 设置 SlowThreshold 启用慢查询告警（语义与 gorm logger.Config 一致：SlowThreshold 为 0 时禁用慢查询检测）。
//   - 调整 IgnoreRecordNotFoundError 以匹配 gorm.Config.IgnoreRecordNotFoundError。
//   - LogMode 控制哪些方法输出日志，与官方 GORM logger 行为一致。
//
// NewGormAdapter 的默认值与 gorm/logger.Default 的级别和慢查询阈值一致，
// 直接放入 db.Config.Logger 即可获得预期行为。
type GormAdapter struct {
	log Logger
	// level 是当前生效的 GORM 日志级别（通过 LogMode 设置）。
	level logger.LogLevel
	// SlowThreshold 在 elapsed > SlowThreshold 且 SlowThreshold > 0 时启用慢 SQL 告警。
	SlowThreshold time.Duration
	// IgnoreRecordNotFoundError 为 true 时，Trace 不会将 ErrRecordNotFound 记录为错误
	// （与 gorm.Config.IgnoreRecordNotFoundError 含义相同）。
	IgnoreRecordNotFoundError bool
}

// NewGormAdapter 返回以 l 为后端的 GORM logger。
// 默认 LogLevel 为 Warn，SlowThreshold 为 200ms，与 gorm/logger.Default 配置一致。
func NewGormAdapter(l Logger) *GormAdapter {
	return &GormAdapter{
		log:           l,
		level:         logger.Warn,
		SlowThreshold: 200 * time.Millisecond,
	}
}

// LogMode 返回以给定级别配置的适配器副本（不可变，类似 GORM 的 logger）。
func (g *GormAdapter) LogMode(level logger.LogLevel) logger.Interface {
	newLogger := *g
	newLogger.level = level
	return &newLogger
}

// Info 在级别为 Info 或更高时记录 GORM 的普通信息消息。
func (g *GormAdapter) Info(ctx context.Context, msg string, args ...interface{}) {
	if g.level < logger.Info {
		return
	}
	g.log.Info(ctx, msg, JSONAbsField("args", args))
}

// Warn 在级别为 Warn 或更高时记录警告。
func (g *GormAdapter) Warn(ctx context.Context, msg string, args ...interface{}) {
	if g.level < logger.Warn {
		return
	}
	g.log.Warn(ctx, msg, JSONAbsField("args", args))
}

// Error 在级别为 Error 或更高时记录错误级别的 GORM 消息。
func (g *GormAdapter) Error(ctx context.Context, msg string, args ...interface{}) {
	if g.level < logger.Error {
		return
	}
	g.log.Error(ctx, fmt.Errorf("%s", msg), JSONAbsField("args", args))
}

// Trace 记录每次 SQL 执行，遵循 gorm/logger.logger.Trace 的分支逻辑：
// SQL 错误（可选排除 ErrRecordNotFound），然后是配置启用时的慢 SQL，
// 最后仅在级别为 Info 时记录常规 SQL 跟踪。
func (g *GormAdapter) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if g.level <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()
	fields := []FieldFunc{
		StringField("sql", sql),
		Int64Field("rows", rows),
		TimeDurationField("elapsed", elapsed),
	}

	switch {
	case err != nil && g.level >= logger.Error &&
		(!errors.Is(err, gorm.ErrRecordNotFound) || !g.IgnoreRecordNotFoundError):
		g.log.Error(ctx, err, fields...)
	case elapsed > g.SlowThreshold && g.SlowThreshold != 0 && g.level >= logger.Warn:
		g.log.Warn(ctx, "slow SQL", fields...)
	case g.level >= logger.Info:
		g.log.Debug(ctx, "SQL trace", fields...)
	}
}
