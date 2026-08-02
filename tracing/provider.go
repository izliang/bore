package tracing

import "context"

type Scope string

type Provider interface {
	// Tracer 获取一个Tracer实例
	Tracer(scope Scope) Tracer
	// Shutdown 停止trace实例
	Shutdown(ctx context.Context) error
}
