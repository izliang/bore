package btask

import (
	"maps"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

type MetricOption func(*metricConfig)

type metricConfig struct {
	namespace   string
	subsystem   string
	constLabels prometheus.Labels
}

// Metrics exposes Pool and MultiPool state as Prometheus metrics.
type Metrics struct {
	source func() []poolMetricSample
	descs  poolMetricDescs
}

type poolMetricSample struct {
	name  string
	stats PoolStats
}

type poolMetricDescs struct {
	parallel      *prometheus.Desc
	queueLength   *prometheus.Desc
	queueCapacity *prometheus.Desc
	running       *prometheus.Desc
	pending       *prometheus.Desc
	submitted     *prometheus.Desc
	completed     *prometheus.Desc
}

// WithMetricNamespace sets the Prometheus metric namespace prefix.
func WithMetricNamespace(namespace string) MetricOption {
	return func(c *metricConfig) {
		c.namespace = strings.TrimSpace(namespace)
	}
}

// WithMetricSubsystem sets the Prometheus metric subsystem prefix.
func WithMetricSubsystem(subsystem string) MetricOption {
	return func(c *metricConfig) {
		c.subsystem = strings.TrimSpace(subsystem)
	}
}

// WithMetricConstLabels sets constant labels applied to every metric.
func WithMetricConstLabels(labels prometheus.Labels) MetricOption {
	return func(c *metricConfig) {
		cl := make(prometheus.Labels, len(labels))
		maps.Copy(cl, labels)
		c.constLabels = cl
	}
}

// NewPoolMetrics creates a Prometheus collector for a single Pool.
func NewPoolMetrics(name string, pool *Pool, opts ...MetricOption) *Metrics {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	return newMetrics(func() []poolMetricSample {
		if pool == nil {
			return nil
		}
		return []poolMetricSample{{name: name, stats: pool.Stats()}}
	}, opts...)
}

// NewMultiPoolMetrics creates a Prometheus collector for all windows in a MultiPool.
func NewMultiPoolMetrics(mp *MultiPool, opts ...MetricOption) *Metrics {
	return newMetrics(func() []poolMetricSample {
		if mp == nil {
			return nil
		}
		stats := mp.Stats()
		out := make([]poolMetricSample, 0, len(stats))
		for _, stat := range stats {
			out = append(out, poolMetricSample{name: stat.Window.Name, stats: stat.Stats})
		}
		return out
	}, opts...)
}

func newMetrics(source func() []poolMetricSample, opts ...MetricOption) *Metrics {
	cfg := &metricConfig{subsystem: "btask"}
	for _, opt := range opts {
		opt(cfg)
	}
	fqn := metricFullyQualifiedName(cfg.namespace, cfg.subsystem)
	labels := []string{"pool"}
	return &Metrics{
		source: source,
		descs: poolMetricDescs{
			parallel: prometheus.NewDesc(
				fqn("pool_parallel"),
				"Configured number of worker goroutines in the pool.",
				labels, cfg.constLabels,
			),
			queueLength: prometheus.NewDesc(
				fqn("pool_queue_length"),
				"Current sampled number of jobs buffered in the pool queue.",
				labels, cfg.constLabels,
			),
			queueCapacity: prometheus.NewDesc(
				fqn("pool_queue_capacity"),
				"Capacity of the pool queue.",
				labels, cfg.constLabels,
			),
			running: prometheus.NewDesc(
				fqn("pool_running"),
				"Current sampled number of tasks actively executing in the pool.",
				labels, cfg.constLabels,
			),
			pending: prometheus.NewDesc(
				fqn("pool_pending"),
				"Current sampled number of submitted tasks that have not completed.",
				labels, cfg.constLabels,
			),
			submitted: prometheus.NewDesc(
				fqn("pool_submitted_total"),
				"Total number of non-nil tasks submitted to the pool.",
				labels, cfg.constLabels,
			),
			completed: prometheus.NewDesc(
				fqn("pool_completed_total"),
				"Total number of submitted task wrappers completed by the pool.",
				labels, cfg.constLabels,
			),
		},
	}
}

// GetCollectors returns this collector for composition with metrics.Provider.
func (m *Metrics) GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{m}
}

func (m *Metrics) Describe(ch chan<- *prometheus.Desc) {
	ch <- m.descs.parallel
	ch <- m.descs.queueLength
	ch <- m.descs.queueCapacity
	ch <- m.descs.running
	ch <- m.descs.pending
	ch <- m.descs.submitted
	ch <- m.descs.completed
}

func (m *Metrics) Collect(ch chan<- prometheus.Metric) {
	for _, sample := range m.source() {
		stats := sample.stats
		labelValues := []string{sample.name}
		ch <- prometheus.MustNewConstMetric(m.descs.parallel, prometheus.GaugeValue, float64(stats.Parallel), labelValues...)
		ch <- prometheus.MustNewConstMetric(m.descs.queueLength, prometheus.GaugeValue, float64(stats.QueueLength), labelValues...)
		ch <- prometheus.MustNewConstMetric(m.descs.queueCapacity, prometheus.GaugeValue, float64(stats.QueueCapacity), labelValues...)
		ch <- prometheus.MustNewConstMetric(m.descs.running, prometheus.GaugeValue, float64(stats.Running), labelValues...)
		ch <- prometheus.MustNewConstMetric(m.descs.pending, prometheus.GaugeValue, float64(stats.Pending), labelValues...)
		ch <- prometheus.MustNewConstMetric(m.descs.submitted, prometheus.CounterValue, float64(stats.Submitted), labelValues...)
		ch <- prometheus.MustNewConstMetric(m.descs.completed, prometheus.CounterValue, float64(stats.Completed), labelValues...)
	}
}

func metricFullyQualifiedName(namespace, subsystem string) func(name string) string {
	switch {
	case namespace != "" && subsystem != "":
		return func(name string) string { return namespace + "_" + subsystem + "_" + name }
	case namespace != "":
		return func(name string) string { return namespace + "_" + name }
	case subsystem != "":
		return func(name string) string { return subsystem + "_" + name }
	default:
		return func(name string) string { return name }
	}
}
