package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestHTTP_GinMiddleware_CollectsMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	p := NewHTTP(WithSkipPathPrefixesOnly())
	reg := prometheus.NewRegistry()
	for _, c := range p.GetCollectors() {
		reg.MustRegister(c)
	}

	r := gin.New()
	r.Use(p.GinMiddleware())
	r.GET("/users/:id", func(c *gin.Context) {
		c.String(http.StatusCreated, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/users/42?q=x", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("unexpected status: got %d want %d", resp.Code, http.StatusCreated)
	}

	labels := map[string]string{
		"method": "GET",
		"path":   "/users/:id",
		"status": "201",
	}
	if got := counterValue(t, reg, "http_requests_total", labels); got != 1 {
		t.Fatalf("http_requests_total mismatch: got %v want 1", got)
	}

	hLabels := map[string]string{
		"method": "GET",
		"path":   "/users/:id",
	}
	if got := histogramCount(t, reg, "http_request_duration_seconds", hLabels); got != 1 {
		t.Fatalf("http_request_duration_seconds count mismatch: got %d want 1", got)
	}
	if got := histogramCount(t, reg, "http_request_size_bytes", hLabels); got != 1 {
		t.Fatalf("http_request_size_bytes count mismatch: got %d want 1", got)
	}
	if got := histogramCount(t, reg, "http_response_size_bytes", hLabels); got != 1 {
		t.Fatalf("http_response_size_bytes count mismatch: got %d want 1", got)
	}
	if got := gaugeValue(t, reg, "http_requests_in_flight", nil); got != 0 {
		t.Fatalf("http_requests_in_flight mismatch: got %v want 0", got)
	}
}

func TestHTTP_GinMiddleware_SkipPrefixAndSuffix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	p := NewHTTP(
		WithSkipPathPrefixesOnly("/health"),
		WithSkipPathSuffixesOnly(".html"),
	)
	reg := prometheus.NewRegistry()
	for _, c := range p.GetCollectors() {
		reg.MustRegister(c)
	}

	r := gin.New()
	r.Use(p.GinMiddleware())
	r.GET("/health/live", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/assets/index.html", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/v1/users", func(c *gin.Context) { c.Status(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health/live", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/assets/index.html", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))

	if got := totalCounterFamilyValue(t, reg, "http_requests_total"); got != 1 {
		t.Fatalf("total http_requests_total mismatch: got %v want 1", got)
	}
}

func TestHTTP_GinMiddleware_UsageExample(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// This test is an executable usage example:
	// 1) build a provider with custom options
	// 2) register collectors in a Prometheus registry
	// 3) mount middleware in Gin
	p := NewHTTP(
		WithMetricNamespace("demo"),
		WithMetricSubsystem("api"),
		WithSkipPathPrefixes("/metrics"),
	)

	reg := prometheus.NewRegistry()
	for _, c := range p.GetCollectors() {
		reg.MustRegister(c)
	}

	r := gin.New()
	r.Use(p.GinMiddleware())
	r.GET("/orders/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.Param("id")})
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/orders/100", nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", resp.Code, http.StatusOK)
	}
	if got := totalCounterFamilyValue(t, reg, "demo_api_http_requests_total"); got != 1 {
		t.Fatalf("demo_api_http_requests_total mismatch: got %v want 1", got)
	}
}

func metricFamily(t *testing.T, reg *prometheus.Registry, name string) *dto.MetricFamily {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather metrics failed: %v", err)
	}
	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}
	t.Fatalf("metric family not found: %s", name)
	return nil
}

func labelsMatch(m *dto.Metric, labels map[string]string) bool {
	if len(labels) == 0 {
		return true
	}
	for k, v := range labels {
		found := false
		for _, lp := range m.GetLabel() {
			if lp.GetName() == k && lp.GetValue() == v {
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

func counterValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	f := metricFamily(t, reg, name)
	for _, m := range f.GetMetric() {
		if labelsMatch(m, labels) {
			return m.GetCounter().GetValue()
		}
	}
	t.Fatalf("counter metric not found: %s labels=%v", name, labels)
	return 0
}

func histogramCount(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) uint64 {
	t.Helper()
	f := metricFamily(t, reg, name)
	for _, m := range f.GetMetric() {
		if labelsMatch(m, labels) {
			return m.GetHistogram().GetSampleCount()
		}
	}
	t.Fatalf("histogram metric not found: %s labels=%v", name, labels)
	return 0
}

func gaugeValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	f := metricFamily(t, reg, name)
	for _, m := range f.GetMetric() {
		if labelsMatch(m, labels) {
			return m.GetGauge().GetValue()
		}
	}
	t.Fatalf("gauge metric not found: %s labels=%v", name, labels)
	return 0
}

func totalCounterFamilyValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	f := metricFamily(t, reg, name)
	var sum float64
	for _, m := range f.GetMetric() {
		sum += m.GetCounter().GetValue()
	}
	return sum
}
