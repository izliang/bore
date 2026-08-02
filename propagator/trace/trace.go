package trace

import (
	"context"

	"github.com/izliang/bore/propagator"
	"go.opentelemetry.io/otel/propagation"
)

var (
	_ propagator.Propagator = (*TracePropagator)(nil)
)

// otelCarrier 将 propagator.Carrier 适配为 OTel 的 propagation.TextMapCarrier。
type otelCarrier struct{ c propagator.Carrier }

func (o *otelCarrier) Get(key string) string        { return o.c.Get(key) }
func (o *otelCarrier) Set(key string, value string) { o.c.Set(key, value) }
func (o *otelCarrier) Keys() []string               { return o.c.Keys() }

// TracePropagator 基于 W3C TraceContext 标准在服务间传播 trace 上下文。
// 内部委托给 OpenTelemetry 的 propagation.TraceContext。
type TracePropagator struct {
	inner propagation.TextMapPropagator
}

// New 创建一个 TracePropagator。
// 默认使用 W3C TraceContext 格式（traceparent / tracestate）。
func New() *TracePropagator {
	return &TracePropagator{
		inner: propagation.TraceContext{},
	}
}

// Inject 实现 propagator.Propagator。
// 从 context 中提取当前 SpanContext，序列化为 traceparent 写入 carrier。
func (t *TracePropagator) Inject(ctx context.Context, carrier propagator.Carrier) {
	t.inner.Inject(ctx, &otelCarrier{c: carrier})
}

// Extract 实现 propagator.Propagator。
// 从 carrier 解析 traceparent，将远程 SpanContext 写入 context。
// 后续通过 tracing.Tracer.Start() 创建 Span 时会自动将其作为父 Span。
func (t *TracePropagator) Extract(ctx context.Context, carrier propagator.Carrier) context.Context {
	return t.inner.Extract(ctx, &otelCarrier{c: carrier})
}
