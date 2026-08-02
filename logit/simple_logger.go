package logit

import "io"

// SimpleLogger 是 Logger 的具体实现，将人类可读的结构化日志行写入底层 io.Writer。
// 它嵌入 baseLogger，后者提供所有核心的级别检查和格式化逻辑。
type SimpleLogger struct {
	baseLogger
}

// Write 实现 io.Writer，将原始字节直接转发到底层 writer。
// 这使得 logger 自身可作为 io.Writer 用于在同一输出端进行原始输出。
// 若底层 writer 为 nil，则不做任何操作并返回成功。
func (s *SimpleLogger) Write(p []byte) (n int, err error) {
	if s.w == nil {
		return 0, nil
	}
	return s.w.Write(p)
}

// NewSimple 使用默认配置创建一个新的 SimpleLogger：
//   - 最低级别：Info
//
// 若 w 为 nil，输出将被丢弃到 io.Discard。
func NewSimple(w io.Writer) Logger {
	return NewSimpleWithConfig(w, nil)
}

// NewSimpleWithConfig 使用提供的配置创建一个新的 SimpleLogger。
// 用户配置与默认值合并：非空/非零字段覆盖默认值。
// CallerSkip.List 与内置默认值（始终包含 logit 和 appctx）合并。
// 若 w 为 nil，输出将被丢弃到 io.Discard。
func NewSimpleWithConfig(w io.Writer, config *Config) Logger {
	if w == nil {
		w = io.Discard
	}
	s := &SimpleLogger{}
	s.w = w
	s.config = mergeConfig(config)
	return s
}

func (s *SimpleLogger) withLevel(level Level) Logger {
	cp := *s
	cfg := *s.config
	cfg.Level = level
	cp.config = &cfg
	return &cp
}
