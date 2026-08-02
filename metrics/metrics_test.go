package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRegisterProvider(t *testing.T) {
	reg := prometheus.NewRegistry()
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "provider_test_gauge",
		Help: "Provider test gauge.",
	})

	provider := &testProvider{collectors: []prometheus.Collector{gauge}}
	if err := Register(reg, provider); err != nil {
		t.Fatalf("register provider failed: %v", err)
	}

	gauge.Set(1)
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics failed: %v", err)
	}
	if !hasMetricFamily(families, "provider_test_gauge") {
		t.Fatal("metric family not found: provider_test_gauge")
	}
}

func TestRegisterCollectors(t *testing.T) {
	reg := prometheus.NewRegistry()
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "collector_test_total",
		Help: "Collector test counter.",
	})

	if err := RegisterCollectors(reg, counter); err != nil {
		t.Fatalf("register collector failed: %v", err)
	}
	counter.Inc()
}

func TestDefaultCollectors(t *testing.T) {
	if len(DefaultCollectors()) == 0 {
		t.Fatal("default collectors should not be empty")
	}
}

type testProvider struct {
	collectors []prometheus.Collector
}

func (p *testProvider) GetCollectors() []prometheus.Collector {
	return p.collectors
}

func hasMetricFamily(families []*dto.MetricFamily, name string) bool {
	for _, mf := range families {
		if mf.GetName() == name {
			return true
		}
	}
	return false
}
