package otel

import (
	"context"

	"github.com/izliang/bore/tracing"
	otelTrace "go.opentelemetry.io/otel/trace"
)

// tracer 是 tracing.Tracer 的 OpenTelemetry 实现，包装 otel trace.Tracer。
//
// Start 委托给 otel trace.Tracer，后者自动完成：
//  1. 从 ctx 提取父 SpanContext
//  2. 创建子 Span，继承 TraceID
//  3. 将新 Span 注入返回的 ctx
//
// 业务代码只需传递 ctx 即可自动建立父子链路。
type tracer struct {
	inner otelTrace.Tracer
}

var _ tracing.Tracer = (*tracer)(nil)

// Start 创建新 Span。
//
// ctx 中存在父 Span 时，新 Span 自动成为子节点，继承相同 TraceID。
// 返回的 ctx 携带当前 Span，传递给下一个 Start 即形成嵌套链路。
//
// opts 暂未使用，留作后续扩展。
func (t *tracer) Start(ctx context.Context, name string, _ ...tracing.StartOptions) (context.Context, tracing.Span) {
	ctx, s := t.inner.Start(ctx, name)
	return ctx, &span{inner: s}
}
