package tracing

import "context"

type startParam struct {
}

type StartOptions func(*startParam)

type Tracer interface {
	// Start 开启Trace链路
	Start(ctx context.Context, name string, opts ...StartOptions) (context.Context, Span)
}
