// Package logit 提供结构化文本日志：SimpleLogger（默认实现）、Logger 接口、
// 延迟求值的 Field/FieldFunc 构造器，以及通过 logit/lctx 将键值对挂载到 context.Context
// 使其自动出现在每行日志中。
package logit

import (
	"bytes"
	"context"
	"io"

	"github.com/izliang/bore/logit/lctx"
)

// Level 表示日志级别，用于过滤和行内渲染。
type Level string

// CallerFormat 控制调用者位置在日志行中的渲染格式。
type CallerFormat uint8

const (
	// CallerShort 输出 "dir/file.go:行号"（取最后两级路径），为默认值。
	CallerShort CallerFormat = iota
	// CallerFull 输出完整绝对路径 "/完整路径/dir/file.go:行号"。
	CallerFull
	// CallerFunc 输出完整函数名 "import/path.Func:行号"。
	CallerFunc
)

// CallerSkipMode 控制单个 CallerSkipConfigItem 与函数名的匹配方式。
type CallerSkipMode uint8

const (
	// CallerSkipPrefix 使用边界安全的前缀匹配："a/b" 匹配 "a/b.Foo" 和 "a/b/sub.Pkg"，
	// 但不匹配 "a/bextra.Foo"。
	CallerSkipPrefix CallerSkipMode = iota
	// CallerSkipContains 使用子串匹配："logit" 匹配任何函数全名中包含 "logit" 的帧。
	CallerSkipContains
)

const (
	Trace Level = "trace"
	Debug Level = "DEBUG"
	Info  Level = "INFO"
	Warn  Level = "WARN"
	Error Level = "ERROR"
)

const (
	// Separator 是日志行中各主要段落之间的分隔符。
	Separator = " "
	// NewLine 追加在每行日志末尾。
	NewLine = "\n"
)

// levelOrder 将每个日志级别映射到其数值严重等级。值越高表示级别越严重。
// 用于过滤：只有 rank >= minLevel 的日志才会输出。
var levelOrder = map[Level]int{
	Trace: 0,
	Debug: 10,
	Info:  20,
	Warn:  50,
	Error: 100,
}

// Logger 由 SimpleLogger 实现，也可由其他后端实现。
//
// Info、Debug、Warn 记录带可选结构化字段的消息。
// Error 在 err 非 nil 时记录；详见 SimpleLogger.Error 中 msg 的处理方式。
//
// 嵌入 io.Writer 允许向与结构化日志相同的输出端进行原始写入。
type Logger interface {
	io.Writer
	Trace(ctx context.Context, msg string, fields ...FieldFunc)
	Info(ctx context.Context, msg string, fields ...FieldFunc)
	Debug(ctx context.Context, msg string, fields ...FieldFunc)
	Warn(ctx context.Context, msg string, fields ...FieldFunc)
	Error(ctx context.Context, err error, fields ...FieldFunc)
}

type levelScopedLogger interface {
	withLevel(level Level) Logger
}

// PackageImportPath 是本包的 import 路径。它始终被合并到调用者跳过配置中，
// 以确保 logit 包（及其子包，如 logit/lctx）内的帧被跳过。
const PackageImportPath = "/bore.git/logit"

// AppCtxImportPath 是 appctx 包的 import 路径，默认也会被跳过。
const AppCtxImportPath = "/bore.git/appctx"

// WithLevel 返回一个限定在给定最低级别的 Logger，不修改原始 logger。
func WithLevel(logger Logger, level Level) Logger {
	if logger == nil {
		return nil
	}
	if scoped, ok := logger.(levelScopedLogger); ok {
		return scoped.withLevel(level)
	}
	return logger
}

// Config 控制 SimpleLogger 的最低级别和调用者注解行为。
type Config struct {
	PodName string
	// Level 是被输出的最低级别；低于此级别的条目将被丢弃。
	Level Level
	// CallerSkip 配置调用者帧的跳过规则和显示格式。
	// 内置默认值（logit、appctx，使用 CallerSkipPrefix 模式）始终被前置到 List 中。
	// 零值表示仅使用内置默认值，采用 CallerShort 格式。
	CallerSkip CallerSkipConfig

	// 内部字段：CallerSkip.List 的指纹，由 mergeConfig 设置。
	callerSkipFP uint64

	TraceID TraceID // 链路追踪 ID
}

// CallerSkipConfig 将调用者跳过设置归组。内置默认值由 mergeConfig 合并到 List 中。
type CallerSkipConfig struct {
	// Format 控制调用者位置在日志输出中的渲染格式。
	// CallerShort（默认）→ "dir/file.go:123"，CallerFull → 绝对路径，CallerFunc → 完整函数名。
	Format CallerFormat `json:"format"`
	// List 是调用者跳过规则列表。每个条目指定一个 Value 及其匹配方式（Mode）。
	// 内置默认值在合并时被前置；重复值将被移除（首次出现优先）。
	List []CallerSkipConfigItem `json:"list"`
}

// CallerSkipConfigItem 是单条调用者跳过规则。
type CallerSkipConfigItem struct {
	// Value 是要跳过的 import 路径前缀或包名字串。
	Value string `json:"value"`
	// Mode 控制 Value 的匹配方式：CallerSkipPrefix（边界安全）或 CallerSkipContains（子串）。
	Mode CallerSkipMode `json:"mode"`
}

func defaultConfig() *Config {
	return &Config{
		Level:   Info,
		PodName: "AppName",
		TraceID: &DefaultTraceID{},
	}
}

// builtinCallerSkipItems 返回内置的跳过条目（始终使用 CallerSkipPrefix 模式）。
func builtinCallerSkipItems() []CallerSkipConfigItem {
	return []CallerSkipConfigItem{
		{Value: PackageImportPath, Mode: CallerSkipContains},
		{Value: AppCtxImportPath, Mode: CallerSkipContains},
	}
}

// mergeCallerSkipItems 将内置条目与用户提供的条目合并，按 Value 去重。
// 内置条目在前，用户条目按原始顺序跟随。
func mergeCallerSkipItems(user []CallerSkipConfigItem) []CallerSkipConfigItem {
	builtins := builtinCallerSkipItems()
	seen := make(map[string]struct{}, len(builtins)+len(user))
	out := make([]CallerSkipConfigItem, 0, len(builtins)+len(user))
	for _, item := range builtins {
		if item.Value == "" {
			continue
		}
		if _, ok := seen[item.Value]; ok {
			continue
		}
		seen[item.Value] = struct{}{}
		out = append(out, item)
	}
	for _, item := range user {
		if item.Value == "" {
			continue
		}
		if _, ok := seen[item.Value]; ok {
			continue
		}
		seen[item.Value] = struct{}{}
		out = append(out, item)
	}
	return out
}

// mergeConfig 返回一个将默认设置与用户覆盖合并后的新 Config。
// 若 c 为 nil，返回默认配置。c 中的非零字段覆盖默认值。
// CallerSkip.List 与内置默认值（logit 和 appctx）合并。
func mergeConfig(c *Config) *Config {
	out := *defaultConfig()
	if c != nil {
		if c.Level != "" {
			out.Level = c.Level
		}
		if c.PodName != "" {
			out.PodName = c.PodName
		}
		if c.TraceID != nil {
			out.TraceID = c.TraceID
		}
		out.CallerSkip = c.CallerSkip
	}
	out.CallerSkip.List = mergeCallerSkipItems(out.CallerSkip.List)
	out.callerSkipFP = fingerprintCallerPrefixes(out.CallerSkip.List)
	return &out
}

// levelRank 返回给定日志级别的数值严重等级。布尔值表示该级别是否被识别。
func levelRank(l Level) (int, bool) {
	v, ok := levelOrder[l]
	return v, ok
}

// baseLogger 提供由具体 logger 实现共享的核心日志功能。
// 它负责级别检查、输出格式化和写入底层 io.Writer。
type baseLogger struct {
	w      io.Writer // 输出目标 writer
	config *Config   // logger 配置（级别、调用者设置）
}

// shouldLog 返回给定级别的消息是否应该被输出。
// 将消息级别与配置的最低级别进行比较。若任一级别无法识别，回退到安全默认值。
func (s *baseLogger) shouldLog(level Level) bool {
	lv, lok := levelRank(level)
	if !lok {
		lv = levelOrder[Info]
	}
	minLevel, mok := levelRank(s.config.Level)
	if !mok {
		minLevel = levelOrder[Debug]
	}
	return lv >= minLevel
}

// writeLine 在级别通过过滤时格式化并写入一行日志。
// 创建一个缓冲区，格式化输出后写入底层 writer。Write 返回的错误被静默忽略（日志不应因写入失败而中断）。
func (s *baseLogger) writeLine(level Level, msg string, ctx context.Context, data []FieldFunc) {
	if !s.shouldLog(level) {
		return
	}
	var sb bytes.Buffer
	s.output(&sb, level, msg, ctx, data...)
	_, _ = s.w.Write(sb.Bytes())
}

func getRequestIDByCtx(ctx context.Context, config *Config) string {
	if ctx == nil || config == nil || config.TraceID == nil {
		return "-"
	}
	return config.TraceID.TraceID(ctx)
}

// output 将完整的日志行渲染到提供的缓冲区中。
// 格式（受 log4j 启发）：时间戳 [应用] [Thread] 级别 [调用者:行号] 上下文字段 msg[消息] key[value]...
// lctx 中的所有上下文字段自动包含在消息之前。
func (s *baseLogger) output(bf *bytes.Buffer, level Level, msg string, ctx context.Context, data ...FieldFunc) {
	formatLogLine(bf, s.config.PodName, string(level), getRequestIDByCtx(ctx, s.config), msg, ctx, data, func() {
		appendCaller(bf, 4, s.config.CallerSkip.List, s.config.callerSkipFP, s.config.CallerSkip.Format)
	})
}

// formatLogLine 将完整的标准化日志行写入缓冲区。
// 这是所有 logger 日志格式的唯一来源。
func formatLogLine(bf *bytes.Buffer, appName, level, traceID, msg string, ctx context.Context, fields []FieldFunc, appendCaller func()) {
	timeCache.appendFormattedTimestamp(bf)
	bf.WriteString(Separator)

	bf.WriteByte('[')
	bf.WriteString(appName)
	bf.WriteByte(']')
	bf.WriteString(Separator)

	bf.WriteString("[Thread]")
	bf.WriteString(Separator)

	bf.WriteString(level)
	bf.WriteString(Separator)

	bf.WriteByte('[')
	appendCaller()
	bf.WriteString("]")
	bf.WriteString(Separator)

	bf.WriteByte('[')
	bf.WriteString(traceID)
	bf.WriteByte(']')
	bf.WriteString(Separator)

	// 上下文字段
	beforeCtxLen := bf.Len()
	outputCtx(bf, ctx)
	if bf.Len() > beforeCtxLen {
		bf.WriteString(Separator)
	}

	bf.WriteString("msg[")
	bf.WriteString(msg)
	bf.WriteByte(']')

	for _, field := range fields {
		if field == nil {
			continue
		}
		k, v := field().Value()
		bf.WriteString(Separator)
		bf.WriteString(k)
		bf.WriteByte('[')
		bf.WriteString(v)
		bf.WriteByte(']')
	}

	bf.WriteString(NewLine)
}

// outputCtx 将提供的 context 中的所有结构化字段追加到缓冲区。
// 通过 lctx.ListPair 提取键值对，并以相同的 key[value] 格式进行格式化。
func outputCtx(bf *bytes.Buffer, ctx context.Context) {
	if ctx == nil {
		return
	}
	ps := lctx.ListPair(ctx)
	if len(ps) == 0 {
		return
	}
	for _, pair := range ps {
		bf.WriteByte(' ')
		bf.WriteString(pair.Key)
		bf.WriteByte('[')
		bf.WriteString(pair.Value)
		bf.WriteByte(']')
	}
}

// Trace 以 Trace 级别记录消息，可附带可选的结构化字段。
func (s *baseLogger) Trace(ctx context.Context, msg string, data ...FieldFunc) {
	s.writeLine(Trace, msg, ctx, data)
}

// Info 以 Info 级别记录消息，可附带可选的结构化字段。
func (s *baseLogger) Info(ctx context.Context, msg string, data ...FieldFunc) {
	s.writeLine(Info, msg, ctx, data)
}

// Debug 以 Debug 级别记录消息，可附带可选的结构化字段。
func (s *baseLogger) Debug(ctx context.Context, msg string, data ...FieldFunc) {
	s.writeLine(Debug, msg, ctx, data)
}

// Warn 以 Warn 级别记录消息，可附带可选的结构化字段。
func (s *baseLogger) Warn(ctx context.Context, msg string, data ...FieldFunc) {
	s.writeLine(Warn, msg, ctx, data)
}

// Error 以 Error 级别记录错误。若 err 为 nil，则不记录任何内容。
// 错误会自动以结构化字段 "error" 的形式添加到日志中。
func (s *baseLogger) Error(ctx context.Context, err error, data ...FieldFunc) {
	if err == nil || !s.shouldLog(Error) {
		return
	}
	fields := append([]FieldFunc{ErrorField("error", err)}, data...)
	s.writeLine(Error, "", ctx, fields)
}
