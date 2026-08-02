package logit

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAccessMiddleware_SkipPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	log := NewSimpleWithConfig(&buf, &Config{Level: Info})

	r := gin.New()
	r.Use(AccessMiddleware(log, &GinAccessConfig{SkipPaths: []string{"/health"}}))
	r.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
	if strings.Contains(buf.String(), "http_access") {
		t.Fatalf("skipped path should not log, got %q", buf.String())
	}

	buf.Reset()
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api", nil))
	out := buf.String()
	if !strings.Contains(out, "http_access") || !strings.Contains(out, "path[") {
		t.Fatalf("expected access log, got %q", out)
	}
}

func TestAccessMiddleware_StatusLevels(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	log := NewSimpleWithConfig(&buf, &Config{Level: Debug})
	r := gin.New()
	r.Use(AccessMiddleware(log, nil))

	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/bad", func(c *gin.Context) { c.Status(http.StatusBadRequest) })
	r.GET("/err", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ok", nil))
	if !strings.Contains(buf.String(), "INFO") {
		t.Fatalf("2xx should be info: %q", buf.String())
	}

	buf.Reset()
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/bad", nil))
	if !strings.Contains(buf.String(), "WARN") {
		t.Fatalf("4xx should warn: %q", buf.String())
	}

	buf.Reset()
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/err", nil))
	if !strings.Contains(buf.String(), "ERROR") {
		t.Fatalf("5xx should error: %q", buf.String())
	}
}
