package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/trace"
)

// TraceID 实现 logit.TraceID 接口，从 context 的 OTel Span 中提取 "traceID-spanID"。
//
// 使用方式：创建 logit Logger 时注入 Config.TraceID 字段。
//
//	logger := logit.NewSimpleWithConfig(os.Stdout, &logit.Config{
//	    TraceID: &otel.TraceID{},
//	})
//
// 日志行中将渲染为：
//
//	2026-07-27 10:00:00 [myservice] [Thread] INFO [caller] [abc123-def456] msg[...]
type TraceID struct{}

// TraceID 从 ctx 中提取 OTel Span 的 TraceID 和 SpanID，组合为 "traceID-spanID"。
// ctx 中无有效 Span 时返回 "-"。
func (t *TraceID) TraceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return "-"
	}
	return fmt.Sprintf("%s-%s", sc.TraceID().String(), sc.SpanID().String())
}
