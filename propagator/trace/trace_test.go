package trace

import (
	"context"
	"net/http"
	"testing"

	"github.com/izliang/bore/propagator"
	"github.com/izliang/bore/propagator/carrier"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// ──────────────────────────── 接口合规 ──────────────────────────────

func TestTracePropagator_ImplementsInterface(t *testing.T) {
	// 编译期断言：TracePropagator 实现 propagator.Propagator
	var _ propagator.Propagator = New()
}

// ──────────────────────────── 注入 ──────────────────────────────────

func TestTracePropagator_Inject(t *testing.T) {
	tests := []struct {
		name     string
		setupCtx func() context.Context
		wantKey  string // header 中应出现的 key
	}{
		{
			name: "有活动 Span 时注入 traceparent",
			setupCtx: func() context.Context {
				tp := sdktrace.NewTracerProvider()
				ctx, span := tp.Tracer("test").Start(context.Background(), "op")
				defer span.End()
				return ctx
			},
			wantKey: "traceparent",
		},
		{
			name: "无活动 Span 时不注入 traceparent",
			setupCtx: func() context.Context {
				return context.Background()
			},
			wantKey: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			ctx := tt.setupCtx()
			header := http.Header{}
			p.Inject(ctx, &carrier.HTTPHeader{Header: header})

			if tt.wantKey == "" {
				if got := header.Get("traceparent"); got != "" {
					t.Errorf("无 Span 时不应注入 traceparent, got %q", got)
				}
			} else {
				if got := header.Get(tt.wantKey); got == "" {
					t.Error("有 Span 时应注入 traceparent")
				}
			}
		})
	}
}

// ──────────────────────────── 提取 ──────────────────────────────────

func TestTracePropagator_Extract(t *testing.T) {
	tests := []struct {
		name        string
		traceparent string
		wantValid   bool
	}{
		{
			name:        "有效 traceparent 解析成功",
			traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			wantValid:   true,
		},
		{
			name:        "空 traceparent 不产生 SpanContext",
			traceparent: "",
			wantValid:   false,
		},
		{
			name:        "无效 traceparent 格式",
			traceparent: "invalid",
			wantValid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			header := http.Header{}
			if tt.traceparent != "" {
				header.Set("traceparent", tt.traceparent)
			}

			ctx := p.Extract(context.Background(), &carrier.HTTPHeader{Header: header})
			sc := trace.SpanContextFromContext(ctx)

			if tt.wantValid && !sc.IsValid() {
				t.Error("期望得到有效 SpanContext，实际无效")
			}
			if !tt.wantValid && sc.IsValid() {
				t.Error("期望 SpanContext 无效，实际有效")
			}
		})
	}
}

// ──────────────────────────── 往返 ──────────────────────────────────

func TestTracePropagator_RoundTrip(t *testing.T) {
	// 创建带有 trace 的 context
	tp := sdktrace.NewTracerProvider()
	ctx, span := tp.Tracer("test").Start(context.Background(), "roundtrip")
	traceID := span.SpanContext().TraceID().String()
	span.End()

	// 注入
	p := New()
	outHeader := http.Header{}
	p.Inject(ctx, &carrier.HTTPHeader{Header: outHeader})

	// 提取
	newCtx := p.Extract(context.Background(), &carrier.HTTPHeader{Header: outHeader})
	sc := trace.SpanContextFromContext(newCtx)

	if !sc.IsValid() {
		t.Fatal("往返后 SpanContext 无效")
	}
	if sc.TraceID().String() != traceID {
		t.Errorf("往返后 TraceID = %s, want %s", sc.TraceID().String(), traceID)
	}
}
