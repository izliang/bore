package logit

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// OutputMode 定义日志写入的目标。
type OutputMode int8

const (
	OutputStd     OutputMode = 1 // 仅输出到标准输出
	OutputFile    OutputMode = 2 // 仅输出到文件
	OutputStdFile OutputMode = 3 // 同时输出到标准输出和文件
)

// ZapLogger 实现 Logger 接口，支持文件轮转和多种输出模式。
// 它直接使用 formatLogLine 以保证所有 logger 的格式一致。
type ZapLogger struct {
	writers          []io.Writer
	level            Level
	appName          string
	TraceID          TraceID                // 链路追踪 ID
	callerSkipItems  []CallerSkipConfigItem // 合并后的跳过条目（内置 + 用户）
	callerSkipFP     uint64                 // callerSkipItems 的指纹
	callerSkipFormat CallerFormat           // 调用者显示格式
}

// ZapConfig 控制输出模式和文件轮转设置。嵌入 logit.Config 以共享配置字段。
type ZapConfig struct {
	Config

	// Mode 控制日志写入目标（stdout、文件或两者）
	Mode OutputMode

	AppName string
	// FileName 是日志文件的完整路径（文件输出时必填）
	FileName string

	// 轮转设置（仅在启用文件输出时使用）
	MaxSizeMB  int  // 轮转前的最大文件大小（MB），默认 100
	MaxAgeDays int  // 保留旧文件的最大天数，默认 30
	MaxBackups int  // 最大备份文件数，默认 10
	Compress   bool // 是否压缩旧日志文件，默认 false
}

// NewZapWithConfig 使用提供的配置创建一个 ZapLogger。
// 支持 stdout、文件或组合输出，并自动进行日志轮转。
func NewZapWithConfig(cfg ZapConfig) Logger {
	var writers []io.Writer

	// 按需添加 stdout writer
	if cfg.Mode == OutputStd || cfg.Mode == OutputStdFile {
		writers = append(writers, os.Stdout)
	}

	// 按需添加文件 writer
	if (cfg.Mode == OutputFile || cfg.Mode == OutputStdFile) && cfg.AppName != "" && cfg.FileName != "" && cfg.PodName != "" {
		filePath := filepath.Join("/opt", "logs", cfg.AppName, cfg.PodName, cfg.FileName)
		if err := ensureLogFile(filePath); err != nil {
			_, _ = os.Stderr.WriteString("logit: 创建日志文件失败: " + err.Error() + "\n")
		} else {
			fw := &lumberjack.Logger{
				Filename:   filePath,
				MaxSize:    defaultIfZero(cfg.MaxSizeMB, 100),
				MaxAge:     defaultIfZero(cfg.MaxAgeDays, 30),
				MaxBackups: defaultIfZero(cfg.MaxBackups, 10),
				Compress:   cfg.Compress,
				LocalTime:  true,
			}
			writers = append(writers, fw)
		}
	}

	// 未配置任何 writer 时回退到 stdout
	if len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}

	mergedCfg := mergeConfig(&cfg.Config)

	return &ZapLogger{
		writers:          writers,
		level:            mergedCfg.Level,
		appName:          mergedCfg.PodName,
		TraceID:          mergedCfg.TraceID,
		callerSkipItems:  mergedCfg.CallerSkip.List,
		callerSkipFP:     mergedCfg.callerSkipFP,
		callerSkipFormat: mergedCfg.CallerSkip.Format,
	}
}

func (z *ZapLogger) withLevel(level Level) Logger {
	cp := *z
	cp.level = level
	return &cp
}

// shouldLog 返回给定级别的消息是否应该被输出。
func (z *ZapLogger) shouldLog(level Level) bool {
	lv, lok := levelRank(level)
	if !lok {
		lv = levelOrder[Info]
	}
	minLevel, mok := levelRank(z.level)
	if !mok {
		minLevel = levelOrder[Debug]
	}
	return lv >= minLevel
}

// Write 实现 io.Writer，将原始字节写入所有输出 writer。
func (z *ZapLogger) Write(p []byte) (n int, err error) {
	for _, w := range z.writers {
		_, _ = w.Write(p)
	}
	return len(p), nil
}

// log 以给定级别写入一条日志消息。
func (z *ZapLogger) log(ctx context.Context, level Level, msg string, fields []FieldFunc) {
	if !z.shouldLog(level) {
		return
	}

	var buf bytes.Buffer
	formatLogLine(&buf, z.appName, string(level), getRequestIDByCtx(ctx, &Config{TraceID: z.TraceID}), msg, ctx, fields, func() {
		appendCaller(&buf, 4, z.callerSkipItems, z.callerSkipFP, z.callerSkipFormat)
	})

	logBytes := buf.Bytes()
	for _, w := range z.writers {
		_, _ = w.Write(logBytes)
	}
}

// Trace 以 Trace 级别记录日志。
func (z *ZapLogger) Trace(ctx context.Context, msg string, fields ...FieldFunc) {
	z.log(ctx, Trace, msg, fields)
}

// Info 以 Info 级别记录日志。
func (z *ZapLogger) Info(ctx context.Context, msg string, fields ...FieldFunc) {
	z.log(ctx, Info, msg, fields)
}

// Debug 以 Debug 级别记录日志。
func (z *ZapLogger) Debug(ctx context.Context, msg string, fields ...FieldFunc) {
	z.log(ctx, Debug, msg, fields)
}

// Warn 以 Warn 级别记录日志。
func (z *ZapLogger) Warn(ctx context.Context, msg string, fields ...FieldFunc) {
	z.log(ctx, Warn, msg, fields)
}

// Error 记录错误。若 err 为 nil，则不记录任何内容。
func (z *ZapLogger) Error(ctx context.Context, err error, fields ...FieldFunc) {
	if err == nil || !z.shouldLog(Error) {
		return
	}
	allFields := append([]FieldFunc{ErrorField("error", err)}, fields...)
	z.log(ctx, Error, "", allFields)
}

func ensureLogFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

func defaultIfZero(val, def int) int {
	if val == 0 {
		return def
	}
	return val
}
