package provider

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestClientHTTP_CollectsMetrics(t *testing.T) {
	m := NewClientHTTP(
		WithClientHTTPNamespace("demo"),
		WithClientHTTPConstLabels(prometheus.Labels{"env": "unit"}),
	)
	reg := prometheus.NewRegistry()
	reg.MustRegister(m.GetCollectors()...)

	m.IncRequest("GET", "example.com", "200")
	m.ObserveRequestDuration("GET", "example.com", "200", 100*time.Millisecond)
	m.IncRetry("GET", "example.com")
	m.IncCircuitOpen("GET", "example.com")

	if got := counterValue(t, reg, "demo_http_client_requests_total", map[string]string{
		"method": "GET",
		"host":   "example.com",
		"status": "200",
		"env":    "unit",
	}); got != 1 {
		t.Fatalf("requests_total mismatch: got %v want 1", got)
	}
	if got := histogramCount(t, reg, "demo_http_client_request_duration_seconds", map[string]string{
		"method": "GET",
		"host":   "example.com",
		"status": "200",
		"env":    "unit",
	}); got != 1 {
		t.Fatalf("duration count mismatch: got %d want 1", got)
	}
	if got := counterValue(t, reg, "demo_http_client_retries_total", map[string]string{
		"method": "GET",
		"host":   "example.com",
		"env":    "unit",
	}); got != 1 {
		t.Fatalf("retries_total mismatch: got %v want 1", got)
	}
	if got := counterValue(t, reg, "demo_http_client_circuit_open_total", map[string]string{
		"method": "GET",
		"host":   "example.com",
		"env":    "unit",
	}); got != 1 {
		t.Fatalf("circuit_open_total mismatch: got %v want 1", got)
	}
}
