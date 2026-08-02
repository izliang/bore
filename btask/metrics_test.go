package btask

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestPoolMetrics_collectsStats(t *testing.T) {
	p := NewPool(1)
	defer p.Shutdown()

	p.Go(func() {})
	p.Wait()

	reg := prometheus.NewRegistry()
	for _, collector := range NewPoolMetrics("workers", p).GetCollectors() {
		reg.MustRegister(collector)
	}

	labels := map[string]string{"pool": "workers"}
	if got := metricValue(t, reg, "btask_pool_parallel", labels); got != 1 {
		t.Fatalf("parallel = %v, want 1", got)
	}
	if got := metricValue(t, reg, "btask_pool_queue_capacity", labels); got != 1 {
		t.Fatalf("queue capacity = %v, want 1", got)
	}
	if got := metricValue(t, reg, "btask_pool_submitted_total", labels); got != 1 {
		t.Fatalf("submitted = %v, want 1", got)
	}
	if got := metricValue(t, reg, "btask_pool_completed_total", labels); got != 1 {
		t.Fatalf("completed = %v, want 1", got)
	}
	if got := metricValue(t, reg, "btask_pool_pending", labels); got != 0 {
		t.Fatalf("pending = %v, want 0", got)
	}
}

func TestPoolMetrics_defaultNameAndOptions(t *testing.T) {
	p := NewPool(2)
	defer p.Shutdown()

	reg := prometheus.NewRegistry()
	for _, collector := range NewPoolMetrics("", p,
		WithMetricNamespace("demo"),
		WithMetricSubsystem("worker"),
		WithMetricConstLabels(prometheus.Labels{"env": "test"}),
	).GetCollectors() {
		reg.MustRegister(collector)
	}

	labels := map[string]string{"pool": "default", "env": "test"}
	if got := metricValue(t, reg, "demo_worker_pool_parallel", labels); got != 2 {
		t.Fatalf("parallel = %v, want 2", got)
	}
}

func TestMultiPoolMetrics_collectsWindows(t *testing.T) {
	m := MustNewMultiPool([]Window{
		{Name: WindowFast, Parallel: 2},
		{Name: WindowSlow, Parallel: 1},
	})
	defer m.Shutdown()

	_ = m.Go(context.Background(), WindowFast, func() {})
	_ = m.Go(context.Background(), WindowSlow, func() {})
	_ = m.Go(context.Background(), WindowSlow, func() {})
	m.Wait()

	reg := prometheus.NewRegistry()
	for _, collector := range NewMultiPoolMetrics(m).GetCollectors() {
		reg.MustRegister(collector)
	}

	if got := metricValue(t, reg, "btask_pool_submitted_total", map[string]string{"pool": WindowFast}); got != 1 {
		t.Fatalf("fast submitted = %v, want 1", got)
	}
	if got := metricValue(t, reg, "btask_pool_submitted_total", map[string]string{"pool": WindowSlow}); got != 2 {
		t.Fatalf("slow submitted = %v, want 2", got)
	}
	if got := metricValue(t, reg, "btask_pool_parallel", map[string]string{"pool": WindowFast}); got != 2 {
		t.Fatalf("fast parallel = %v, want 2", got)
	}
}

func metricValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	family := btaskMetricFamily(t, reg, name)
	for _, metric := range family.GetMetric() {
		if btaskLabelsMatch(metric, labels) {
			if metric.GetGauge() != nil {
				return metric.GetGauge().GetValue()
			}
			if metric.GetCounter() != nil {
				return metric.GetCounter().GetValue()
			}
			t.Fatalf("metric %s has unsupported type", name)
		}
	}
	t.Fatalf("metric %s with labels %v not found", name, labels)
	return 0
}

func btaskMetricFamily(t *testing.T, reg *prometheus.Registry, name string) *dto.MetricFamily {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics failed: %v", err)
	}
	for _, family := range families {
		if family.GetName() == name {
			return family
		}
	}
	t.Fatalf("metric family not found: %s", name)
	return nil
}

func btaskLabelsMatch(metric *dto.Metric, labels map[string]string) bool {
	for name, value := range labels {
		found := false
		for _, label := range metric.GetLabel() {
			if label.GetName() == name && label.GetValue() == value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
