package provider

import (
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// HTTP provides reusable HTTP server metrics and implements metrics.Provider.
// Path cardinality strategy:
//   - For matched routes, labels use gin.FullPath() route templates (for example /users/:id).
//   - For unmatched routes, labels use URL.Path only (query string excluded), normalized by path.Clean.
type HTTP struct {
	cfg httpConfig

	skipPrefixes []string
	skipSuffixes []string

	requestsTotal    *prometheus.CounterVec
	requestDuration  *prometheus.HistogramVec
	requestSize      *prometheus.HistogramVec
	responseSize     *prometheus.HistogramVec
	requestsInFlight prometheus.Gauge
}

type httpConfig struct {
	namespace   string
	subsystem   string
	constLabels prometheus.Labels

	durationBuckets []float64
	sizeBuckets     []float64

	skipper     gin.Skipper
	pathLabeler func(*gin.Context) string
	methodLabel func(string) string
	statusLabel func(int) string
}

var defaultSkipPathPrefixes = []string{"/ping", "/metrics"}

func defaultHTTPConfig() httpConfig {
	return httpConfig{
		durationBuckets: durationBuckets(),
		sizeBuckets:     sizeBuckets(),
		pathLabeler:     defaultPathLabel,
		methodLabel:     strings.ToUpper,
		statusLabel: func(status int) string {
			return strconv.Itoa(status)
		},
	}
}

// HTTPOption customizes HTTP provider behavior and metric options.
type HTTPOption func(*HTTP)

// WithSkipPathPrefixes skips metrics collection for exact path prefixes.
// Typical values include /metrics or /health to avoid self-scrape noise.
func WithSkipPathPrefixes(prefixes ...string) HTTPOption {
	return func(h *HTTP) {
		for _, p := range prefixes {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			h.skipPrefixes = append(h.skipPrefixes, path.Clean(p))
		}
	}
}

// WithSkipPathSuffixes skips metrics collection for exact path suffixes.
// This is useful when routes are discovered by trailing fragments, such as /index.html or /metrics.
func WithSkipPathSuffixes(suffixes ...string) HTTPOption {
	return func(h *HTTP) {
		for _, s := range suffixes {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			h.skipSuffixes = append(h.skipSuffixes, path.Clean(s))
		}
	}
}

// WithSkipPathPrefixesOnly replaces the entire skip-prefix set.
// Pass no values to disable path-prefix skipping completely.
func WithSkipPathPrefixesOnly(prefixes ...string) HTTPOption {
	return func(h *HTTP) {
		h.skipPrefixes = h.skipPrefixes[:0]
		for _, p := range prefixes {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			h.skipPrefixes = append(h.skipPrefixes, path.Clean(p))
		}
	}
}

// WithSkipPathSuffixesOnly replaces the entire skip-suffix set.
// Pass no values to disable path-suffix skipping completely.
func WithSkipPathSuffixesOnly(suffixes ...string) HTTPOption {
	return func(h *HTTP) {
		h.skipSuffixes = h.skipSuffixes[:0]
		for _, s := range suffixes {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			h.skipSuffixes = append(h.skipSuffixes, path.Clean(s))
		}
	}
}

// WithSkipper sets a request-level skip function (same shape as gin.LoggerConfig.Skip).
func WithSkipper(skip gin.Skipper) HTTPOption {
	return func(h *HTTP) {
		h.cfg.skipper = skip
	}
}

// WithPathLabeler overrides how the path label is computed.
func WithPathLabeler(labeler func(*gin.Context) string) HTTPOption {
	return func(h *HTTP) {
		if labeler != nil {
			h.cfg.pathLabeler = labeler
		}
	}
}

// WithMethodLabeler overrides how HTTP method is mapped to label value.
func WithMethodLabeler(labeler func(string) string) HTTPOption {
	return func(h *HTTP) {
		if labeler != nil {
			h.cfg.methodLabel = labeler
		}
	}
}

// WithStatusLabeler overrides how response status code is mapped to label value.
func WithStatusLabeler(labeler func(int) string) HTTPOption {
	return func(h *HTTP) {
		if labeler != nil {
			h.cfg.statusLabel = labeler
		}
	}
}

// WithMetricNamespace sets Prometheus metric namespace.
func WithMetricNamespace(namespace string) HTTPOption {
	return func(h *HTTP) {
		h.cfg.namespace = strings.TrimSpace(namespace)
	}
}

// WithMetricSubsystem sets Prometheus metric subsystem.
func WithMetricSubsystem(subsystem string) HTTPOption {
	return func(h *HTTP) {
		h.cfg.subsystem = strings.TrimSpace(subsystem)
	}
}

// WithMetricConstLabels sets const labels on all metrics in this provider.
func WithMetricConstLabels(labels prometheus.Labels) HTTPOption {
	return func(h *HTTP) {
		if len(labels) == 0 {
			return
		}
		cl := make(prometheus.Labels, len(labels))
		for k, v := range labels {
			cl[k] = v
		}
		h.cfg.constLabels = cl
	}
}

// WithDurationBuckets overrides latency histogram buckets (seconds).
func WithDurationBuckets(buckets []float64) HTTPOption {
	return func(h *HTTP) {
		if len(buckets) > 0 {
			h.cfg.durationBuckets = append([]float64(nil), buckets...)
		}
	}
}

// WithSizeBuckets overrides request/response size histogram buckets (bytes).
func WithSizeBuckets(buckets []float64) HTTPOption {
	return func(h *HTTP) {
		if len(buckets) > 0 {
			h.cfg.sizeBuckets = append([]float64(nil), buckets...)
		}
	}
}

// NewHTTP registers a default RED-style HTTP metrics set:
// request count, latency histogram, request/response size histograms, and in-flight gauge.
func NewHTTP(opts ...HTTPOption) *HTTP {
	h := &HTTP{
		cfg:          defaultHTTPConfig(),
		skipPrefixes: append([]string(nil), defaultSkipPathPrefixes...),
	}
	for _, o := range opts {
		o(h)
	}

	h.requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   h.cfg.namespace,
			Subsystem:   h.cfg.subsystem,
			Name:        "http_requests_total",
			Help:        "Total number of HTTP requests partitioned by method, path template, and status.",
			ConstLabels: h.cfg.constLabels,
		},
		[]string{"method", "path", "status"},
	)
	h.requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   h.cfg.namespace,
			Subsystem:   h.cfg.subsystem,
			Name:        "http_request_duration_seconds",
			Help:        "HTTP request processing duration in seconds, partitioned by method and path template.",
			Buckets:     h.cfg.durationBuckets,
			ConstLabels: h.cfg.constLabels,
		},
		[]string{"method", "path"},
	)
	h.requestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   h.cfg.namespace,
			Subsystem:   h.cfg.subsystem,
			Name:        "http_request_size_bytes",
			Help:        "HTTP request size in bytes from Content-Length. Unknown values are recorded as 0.",
			Buckets:     h.cfg.sizeBuckets,
			ConstLabels: h.cfg.constLabels,
		},
		[]string{"method", "path"},
	)
	h.responseSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   h.cfg.namespace,
			Subsystem:   h.cfg.subsystem,
			Name:        "http_response_size_bytes",
			Help:        "HTTP response size in bytes from gin.ResponseWriter.Size().",
			Buckets:     h.cfg.sizeBuckets,
			ConstLabels: h.cfg.constLabels,
		},
		[]string{"method", "path"},
	)
	h.requestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace:   h.cfg.namespace,
			Subsystem:   h.cfg.subsystem,
			Name:        "http_requests_in_flight",
			Help:        "Current number of HTTP requests being served.",
			ConstLabels: h.cfg.constLabels,
		},
	)

	return h
}

func durationBuckets() []float64 {
	// Compact default latency buckets from 50ms to 30s.
	return []float64{0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 30}
}

func sizeBuckets() []float64 {
	return prometheus.ExponentialBuckets(256, 4, 10)
}

func (h *HTTP) GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		h.requestsTotal,
		h.requestDuration,
		h.requestSize,
		h.responseSize,
		h.requestsInFlight,
	}
}

// GinMiddleware records metrics for each request.
// Register routes before using this middleware so c.FullPath() is stable.
func (h *HTTP) GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.shouldSkip(c) {
			c.Next()
			return
		}

		h.requestsInFlight.Inc()
		defer h.requestsInFlight.Dec()

		start := time.Now()
		c.Next()

		pathLabel := h.cfg.pathLabeler(c)
		method := h.cfg.methodLabel(c.Request.Method)
		if method == "" {
			method = c.Request.Method
		}

		status := c.Writer.Status()
		if status <= 0 {
			status = 200
		}
		statusLabel := h.cfg.statusLabel(status)
		if statusLabel == "" {
			statusLabel = strconv.Itoa(status)
		}

		reqSize := float64(0)
		if cl := c.Request.ContentLength; cl >= 0 {
			reqSize = float64(cl)
		}

		respSize := c.Writer.Size()
		if respSize < 0 {
			respSize = 0
		}

		h.requestsTotal.WithLabelValues(method, pathLabel, statusLabel).Inc()
		h.requestDuration.WithLabelValues(method, pathLabel).Observe(time.Since(start).Seconds())
		h.requestSize.WithLabelValues(method, pathLabel).Observe(reqSize)
		h.responseSize.WithLabelValues(method, pathLabel).Observe(float64(respSize))
	}
}

func (h *HTTP) shouldSkip(c *gin.Context) bool {
	if h.cfg.skipper != nil && h.cfg.skipper(c) {
		return true
	}
	if len(h.skipPrefixes) == 0 && len(h.skipSuffixes) == 0 {
		return false
	}
	p := path.Clean(c.Request.URL.Path)
	for _, prefix := range h.skipPrefixes {
		if prefix == "/" {
			return true
		}
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	for _, suffix := range h.skipSuffixes {
		if p == suffix || strings.HasSuffix(p, suffix) {
			return true
		}
	}
	return false
}

func defaultPathLabel(c *gin.Context) string {
	if fp := c.FullPath(); fp != "" {
		return fp
	}
	p := c.Request.URL.Path
	if p == "" {
		p = "/"
	}
	return path.Clean(p)
}
