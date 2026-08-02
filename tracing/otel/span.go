package otel

import (
	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/tracing"
	"go.opentelemetry.io/otel/attribute"
	otelTrace "go.opentelemetry.io/otel/trace"
)

// span 是 tracing.Span 的 OpenTelemetry 实现，包装 otel trace.Span。
//
// 所有方法委托给内部的 trace.Span，SetAttributes 需做一次
// Field → attribute.KeyValue 转换。
type span struct {
	inner otelTrace.Span
}

var _ tracing.Span = (*span)(nil)

// End 结束 Span。
func (s *span) End() {
	s.inner.End()
}

// RecordError 记录错误。不自动设置 StatusCode，
// 仅将错误作为事件附加到 Span。
func (s *span) RecordError(err error) {
	s.inner.RecordError(err)
}

// SetAttributes 设置业务属性。逐个调用 Field.Value() 获取字符串键值，
// 映射为 attribute.String 后批量写入。
func (s *span) SetAttributes(attrs ...logit.FieldFunc) {
	for _, attr := range attrs {
		k, v := attr().Value()
		s.inner.SetAttributes(attribute.String(k, v))
	}
}
