package logit

import "context"

// TraceID 定义从 context 提取链路追踪 ID 的接口。
type TraceID interface {
	TraceID(ctx context.Context) string
}

// DefaultTraceID 是 TraceID 的默认实现，从 context 中以 defaultTraceID 为键提取字符串值。
type DefaultTraceID struct{}

var defaultTraceID = DefaultTraceID{}

func (*DefaultTraceID) TraceID(ctx context.Context) string {
	t, ok := ctx.Value(defaultTraceID).(string)
	if ok {
		return t
	}
	return "-"
}

// WithDefaultTraceID 将 traceID 写入 context，使用 DefaultTraceID 的默认键。
func WithDefaultTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, defaultTraceID, traceID)
}
