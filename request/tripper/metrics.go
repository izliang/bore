package tripper

import (
	"net/http"
	"strconv"
	"time"
)

// ═══════════════════════════════════════════════════════════════════
// HTTPMetrics — 指标收集接口
// ═══════════════════════════════════════════════════════════════════

// HTTPMetrics 定义 HTTP 请求指标收集接口。
// 业务方可以实现此接口来接入自己的监控系统（Prometheus、Datadog 等）。
type HTTPMetrics interface {
	// IncRequest 增加请求计数
	IncRequest(method, host, status string)

	// ObserveRequestDuration 记录请求耗时
	ObserveRequestDuration(method, host, status string, duration time.Duration)

	// IncRetry 增加重试计数
	IncRetry(method, host string)

	// IncCircuitOpen 增加熔断触发计数
	IncCircuitOpen(method, host string)
}

// NoopMetrics 空实现，不收集任何指标。
type NoopMetrics struct{}

func (n *NoopMetrics) IncRequest(method, host, status string)                {}
func (n *NoopMetrics) ObserveRequestDuration(method, host, status string, d time.Duration) {}
func (n *NoopMetrics) IncRetry(method, host string)                          {}
func (n *NoopMetrics) IncCircuitOpen(method, host string)                    {}

// ═══════════════════════════════════════════════════════════════════
// MetricsRoundTripper — 指标收集中间件
// ═══════════════════════════════════════════════════════════════════

// MetricsRoundTripper 提供 HTTP 请求指标收集功能。
type MetricsRoundTripper struct {
	next    http.RoundTripper
	metrics HTTPMetrics
}

// NewMetricsRoundTripper 创建指标收集中间件装饰器。
func NewMetricsRoundTripper(metrics HTTPMetrics) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		if metrics == nil {
			metrics = &NoopMetrics{}
		}
		return &MetricsRoundTripper{
			next:    next,
			metrics: metrics,
		}
	}
}

// NewMetrics 是 NewMetricsRoundTripper 的便捷别名。
func NewMetrics(metrics HTTPMetrics) func(http.RoundTripper) http.RoundTripper {
	return NewMetricsRoundTripper(metrics)
}

// RoundTrip 实现 http.RoundTripper 接口。
func (m *MetricsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	host := req.URL.Host
	method := req.Method

	resp, err := m.next.RoundTrip(req)

	duration := time.Since(start)
	status := "0"
	if err == nil {
		status = strconv.Itoa(resp.StatusCode)
	}

	m.metrics.IncRequest(method, host, status)
	m.metrics.ObserveRequestDuration(method, host, status, duration)

	return resp, err
}
