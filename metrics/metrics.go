// Package metrics 提供 Prometheus collector 的组合和注册辅助。
//
// 本包不持有全局 registry。调用方应自行决定使用 prometheus.DefaultRegisterer、
// prometheus.NewRegistry()，或 prometheus.WrapRegistererWith(...) 包装后的 registerer。
//
// 使用示例：
//
//	reg := prometheus.NewRegistry()
//	metrics.MustRegisterCollectors(reg, metrics.DefaultCollectors()...)
//
//	httpMetrics := provider.NewHTTP(provider.WithMetricNamespace("myservice"))
//	dbMetrics := db.NewDBStats(gormDB, db.WithDBName("primary"))
//	metrics.MustRegister(reg, httpMetrics, dbMetrics)
//
//	r := gin.Default()
//	r.GET("/metrics", metrics.GinHandler(reg))
package metrics

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Provider 定义 metrics 提供者接口。
// 任何可以提供 Prometheus collectors 的组件都可以实现此接口。
type Provider interface {
	GetCollectors() []prometheus.Collector
}

// DefaultCollectors 返回 Go runtime、process、build_info 等基础 collectors。
// 调用方可以按需注册到自己的 registry。
func DefaultCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewBuildInfoCollector(),
	}
}

// Register 将 providers 中的 collectors 注册到指定 registerer。
// registerer 为 nil 时使用 prometheus.DefaultRegisterer。
func Register(registerer prometheus.Registerer, providers ...Provider) error {
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		if err := RegisterCollectors(registerer, provider.GetCollectors()...); err != nil {
			return err
		}
	}
	return nil
}

// MustRegister 将 providers 中的 collectors 注册到指定 registerer。
// registerer 为 nil 时使用 prometheus.DefaultRegisterer。
// 注册失败时 panic。
func MustRegister(registerer prometheus.Registerer, providers ...Provider) {
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		MustRegisterCollectors(registerer, provider.GetCollectors()...)
	}
}

// RegisterCollectors 将 collectors 注册到指定 registerer。
// registerer 为 nil 时使用 prometheus.DefaultRegisterer。
func RegisterCollectors(registerer prometheus.Registerer, collectors ...prometheus.Collector) error {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	for _, collector := range collectors {
		if collector == nil {
			continue
		}
		if err := registerer.Register(collector); err != nil {
			return err
		}
	}
	return nil
}

// MustRegisterCollectors 将 collectors 注册到指定 registerer。
// registerer 为 nil 时使用 prometheus.DefaultRegisterer。
// 注册失败时 panic。
func MustRegisterCollectors(registerer prometheus.Registerer, collectors ...prometheus.Collector) {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	for _, collector := range collectors {
		if collector == nil {
			continue
		}
		registerer.MustRegister(collector)
	}
}

// Handler 返回暴露 metrics 的 http.Handler。
// gatherer 为 nil 时使用 prometheus.DefaultGatherer。
func Handler(gatherer prometheus.Gatherer) http.Handler {
	if gatherer == nil {
		gatherer = prometheus.DefaultGatherer
	}
	return promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{})
}

// GinHandler 返回 Gin 处理函数，用于暴露 /metrics 端点。
// gatherer 为 nil 时使用 prometheus.DefaultGatherer。
func GinHandler(gatherer prometheus.Gatherer) gin.HandlerFunc {
	return gin.WrapH(Handler(gatherer))
}
