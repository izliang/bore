package tripper

import (
	"net/http"

	"github.com/izliang/bore/propagator"
	"github.com/izliang/bore/propagator/carrier"
)

// ═══════════════════════════════════════════════════════════════════
// PropagatorRoundTripper — Header 透传中间件
// ═══════════════════════════════════════════════════════════════════

// PropagatorRoundTripper 提供 HTTP Header 透传功能。
// 从请求的 context 中提取值，并注入到 HTTP 请求头中。
type PropagatorRoundTripper struct {
	next        http.RoundTripper
	propagators []propagator.Propagator
}

// NewPropagatorRoundTripper 创建 Header 透传中间件装饰器。
func NewPropagatorRoundTripper(propagators ...propagator.Propagator) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		return &PropagatorRoundTripper{
			next:        next,
			propagators: propagators,
		}
	}
}

// NewPropagation 是 NewPropagatorRoundTripper 的便捷别名。
func NewPropagation(propagators ...propagator.Propagator) func(http.RoundTripper) http.RoundTripper {
	return NewPropagatorRoundTripper(propagators...)
}

// RoundTrip 实现 http.RoundTripper 接口。
func (p *PropagatorRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	carr := &carrier.HTTPHeader{Header: req.Header}
	for _, prop := range p.propagators {
		prop.Inject(req.Context(), carr)
	}
	return p.next.RoundTrip(req)
}
