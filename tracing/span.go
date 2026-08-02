package tracing

import "github.com/izliang/bore/logit"

// Span 代表一次操作追踪。
type Span interface {
	// End 结束 Span。
	End()
	// RecordError 记录一个错误。
	RecordError(err error)
	// SetAttributes 设置业务属性。
	SetAttributes(attrs ...logit.FieldFunc)
}
