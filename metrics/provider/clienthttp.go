package provider

import (
	"maps"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// ClientHTTP 提供 HTTP client metrics，适配 request/tripper.HTTPMetrics 接口。
//
// 使用示例：
//
//	reg := prometheus.NewRegistry()
//	m := provider.NewClientHTTP(provider.WithClientHTTPNamespace("myservice"))
//	metrics.MustRegister(reg, m)
//
//	client := tripper.New(
//	    tripper.Metrics(m),
//	)
type ClientHTTP struct {
	requestsTotal   *prometheus.CounterVec
	durationSeconds *prometheus.HistogramVec
	retriesTotal    *prometheus.CounterVec
	circuitOpen     *prometheus.CounterVec
}

type clientHTTPConfig struct {
	namespace   string
	subsystem   string
	constLabels prometheus.Labels
	buckets     []float64
}

// ClientHTTPOption customizes ClientHTTP metrics.
type ClientHTTPOption func(*clientHTTPConfig)

// WithClientHTTPNamespace sets Prometheus metric namespace.
func WithClientHTTPNamespace(namespace string) ClientHTTPOption {
	return func(c *clientHTTPConfig) {
		c.namespace = strings.TrimSpace(namespace)
	}
}

// WithClientHTTPSubsystem sets Prometheus metric subsystem.
func WithClientHTTPSubsystem(subsystem string) ClientHTTPOption {
	return func(c *clientHTTPConfig) {
		c.subsystem = strings.TrimSpace(subsystem)
	}
}

// WithClientHTTPConstLabels sets const labels on all metrics.
func WithClientHTTPConstLabels(labels prometheus.Labels) ClientHTTPOption {
	return func(c *clientHTTPConfig) {
		if len(labels) == 0 {
			return
		}
		cl := make(prometheus.Labels, len(labels))
		maps.Copy(cl, labels)
		c.constLabels = cl
	}
}

// WithClientHTTPDurationBuckets overrides latency histogram buckets (seconds).
func WithClientHTTPDurationBuckets(buckets []float64) ClientHTTPOption {
	return func(c *clientHTTPConfig) {
		if len(buckets) > 0 {
			c.buckets = append([]float64(nil), buckets...)
		}
	}
}

// NewClientHTTP creates a default HTTP client metrics set.
// It records request count, latency, retry count, and circuit-open count.
func NewClientHTTP(opts ...ClientHTTPOption) *ClientHTTP {
	cfg := clientHTTPConfig{
		subsystem: "http_client",
		buckets:   prometheus.DefBuckets,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return &ClientHTTP{
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   cfg.namespace,
				Subsystem:   cfg.subsystem,
				Name:        "requests_total",
				Help:        "Total number of HTTP client requests.",
				ConstLabels: cfg.constLabels,
			},
			[]string{"method", "host", "status"},
		),
		durationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace:   cfg.namespace,
				Subsystem:   cfg.subsystem,
				Name:        "request_duration_seconds",
				Help:        "HTTP client request duration in seconds.",
				Buckets:     cfg.buckets,
				ConstLabels: cfg.constLabels,
			},
			[]string{"method", "host", "status"},
		),
		retriesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   cfg.namespace,
				Subsystem:   cfg.subsystem,
				Name:        "retries_total",
				Help:        "Total number of HTTP client retries.",
				ConstLabels: cfg.constLabels,
			},
			[]string{"method", "host"},
		),
		circuitOpen: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   cfg.namespace,
				Subsystem:   cfg.subsystem,
				Name:        "circuit_open_total",
				Help:        "Total number of HTTP client circuit-open events.",
				ConstLabels: cfg.constLabels,
			},
			[]string{"method", "host"},
		),
	}
}

// GetCollectors implements metrics.Provider.
func (m *ClientHTTP) GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.requestsTotal,
		m.durationSeconds,
		m.retriesTotal,
		m.circuitOpen,
	}
}

// IncRequest increments HTTP client request count.
func (m *ClientHTTP) IncRequest(method, host, status string) {
	m.requestsTotal.WithLabelValues(method, host, status).Inc()
}

// ObserveRequestDuration records HTTP client request duration.
func (m *ClientHTTP) ObserveRequestDuration(method, host, status string, duration time.Duration) {
	m.durationSeconds.WithLabelValues(method, host, status).Observe(duration.Seconds())
}

// IncRetry increments HTTP client retry count.
func (m *ClientHTTP) IncRetry(method, host string) {
	m.retriesTotal.WithLabelValues(method, host).Inc()
}

// IncCircuitOpen increments HTTP client circuit-open count.
func (m *ClientHTTP) IncCircuitOpen(method, host string) {
	m.circuitOpen.WithLabelValues(method, host).Inc()
}
