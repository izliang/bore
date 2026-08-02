// Package queue 提供消息队列 metrics 定义。
//
// 使用示例：
//
//	reg := prometheus.NewRegistry()
//	mqMetrics := queue.NewMetrics("myapp")
//	metrics.MustRegister(reg, mqMetrics)
package queue

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics 包含消息队列操作的所有 Prometheus collectors。
// 实现 metrics.Provider 接口，支持统一注册。
type Metrics struct {
	// MessagesPublished 统计成功发布的消息数，标签：vhost/queue/exchange
	MessagesPublished *prometheus.CounterVec

	// MessagesConsumed 统计消费者接收的消息数，标签：vhost/queue
	MessagesConsumed *prometheus.CounterVec

	// MessagesAcked 统计已确认的消息数，标签：vhost/queue
	MessagesAcked *prometheus.CounterVec

	// MessagesNacked 统计未确认的消息数，标签：vhost/queue
	MessagesNacked *prometheus.CounterVec

	// MessagesDeadLettered 统计死信队列消息数，标签：vhost/queue
	MessagesDeadLettered *prometheus.CounterVec

	// HandlerPanics 统计 handler 中的 panic 次数，标签：vhost/queue
	HandlerPanics *prometheus.CounterVec

	// PublishDuration 统计发布延迟（秒），标签：vhost/queue
	PublishDuration *prometheus.HistogramVec

	// HandlerDuration 统计 handler 执行延迟（秒），标签：vhost/queue
	HandlerDuration *prometheus.HistogramVec

	// ConnectionStatus 连接状态 gauge：1 已连接，0 未连接
	ConnectionStatus prometheus.Gauge

	// ReconnectTotal 重连次数
	ReconnectTotal prometheus.Counter

	// ConsumerResubscribeTotal 消费者重订阅次数，标签：vhost/queue
	ConsumerResubscribeTotal *prometheus.CounterVec
}

// NewMetrics 创建一套新的消息队列 metrics。
// subsystem 通常是服务名或 "mq"，用于区分不同服务的指标。
//
// 创建后需要调用 metrics.MustRegister(registry, mqMetrics) 进行注册。
func NewMetrics(subsystem string) *Metrics {
	return &Metrics{
		MessagesPublished: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      "messages_published_total",
				Help:      "Total number of messages published.",
			},
			[]string{"vhost", "queue", "exchange"},
		),
		MessagesConsumed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      "messages_consumed_total",
				Help:      "Total number of messages consumed.",
			},
			[]string{"vhost", "queue"},
		),
		MessagesAcked: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      "messages_acked_total",
				Help:      "Total number of messages acknowledged.",
			},
			[]string{"vhost", "queue"},
		),
		MessagesNacked: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      "messages_nacked_total",
				Help:      "Total number of messages negatively acknowledged.",
			},
			[]string{"vhost", "queue"},
		),
		MessagesDeadLettered: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      "messages_dead_lettered_total",
				Help:      "Total number of dead-lettered messages.",
			},
			[]string{"vhost", "queue"},
		),
		HandlerPanics: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      "handler_panics_total",
				Help:      "Total number of recovered handler panics.",
			},
			[]string{"vhost", "queue"},
		),
		PublishDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Subsystem: subsystem,
				Name:      "publish_duration_seconds",
				Help:      "Publish latency in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"vhost", "queue"},
		),
		HandlerDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Subsystem: subsystem,
				Name:      "handler_duration_seconds",
				Help:      "Handler execution latency in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"vhost", "queue"},
		),
		ConnectionStatus: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Subsystem: subsystem,
				Name:      "connection_status",
				Help:      "1 if connected to broker, 0 otherwise.",
			},
		),
		ReconnectTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      "reconnect_total",
				Help:      "Total number of reconnect attempts.",
			},
		),
		ConsumerResubscribeTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      "consumer_resubscribe_total",
				Help:      "Total number of consumer resubscribe attempts after disconnect.",
			},
			[]string{"vhost", "queue"},
		),
	}
}

// GetCollectors 实现 metrics.Provider 接口，返回所有 collectors 用于注册。
func (m *Metrics) GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		m.MessagesPublished,
		m.MessagesConsumed,
		m.MessagesAcked,
		m.MessagesNacked,
		m.MessagesDeadLettered,
		m.HandlerPanics,
		m.PublishDuration,
		m.HandlerDuration,
		m.ConnectionStatus,
		m.ReconnectTotal,
		m.ConsumerResubscribeTotal,
	}
}
