// Package otel 提供基于 OpenTelemetry 的 tracing.Provider 实现。
//
// 本包将 tracing 包的抽象接口适配到 OpenTelemetry Go SDK：
//   - tracing.Provider → otel.TracerProvider
//   - tracing.Tracer   → otel.Tracer
//   - tracing.Span     → otel.Span
//
// Exporter 由调用方创建并通过 WithExporter 注入，本包不依赖任何具体导出器，
// 可自由搭配 OTLP gRPC、OTLP HTTP、Jaeger、stdout 等任意 SpanExporter。
//
// 典型用法：
//
//	import (
//	    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
//	    "git.100credit.cn/.../bore.git/tracing/otel"
//	)
//
//	exp, _ := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint("jaeger:4317"))
//	provider, _ := otel.NewProvider(
//	    otel.WithServiceName("my-service"),
//	    otel.WithExporter(exp),
//	)
//	defer provider.Shutdown(ctx)
package otel

import (
	"context"
	"fmt"

	"github.com/izliang/bore/tracing"
	"go.opentelemetry.io/otel"
	otelResource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// config 持有 Provider 的配置。
type config struct {
	serviceName string
	exporter    sdktrace.SpanExporter
	sampler     sdktrace.Sampler
	resource    *otelResource.Resource
}

// Option 是 Provider 的功能配置函数。
type Option func(*config)

// WithServiceName 设置服务名称，必填。
//
// 映射为 OTel Resource 的 service.name 属性。
func WithServiceName(name string) Option {
	return func(c *config) {
		c.serviceName = name
	}
}

// WithExporter 设置 Span 导出器，必填。
//
// 接受任意 sdktrace.SpanExporter 实现。可传入 OTLP gRPC、OTLP HTTP、
// Jaeger、stdout 等导出器。未设置时 NewProvider 返回错误。
func WithExporter(exp sdktrace.SpanExporter) Option {
	return func(c *config) {
		c.exporter = exp
	}
}

// WithSampler 设置采样器，选填。默认 AlwaysSample。
//
// 可传入 sdktrace.ParentBased(...) 或自定义策略。
func WithSampler(s sdktrace.Sampler) Option {
	return func(c *config) {
		c.sampler = s
	}
}

// WithResource 覆盖自动创建的 Resource，选填。
//
// 默认根据 WithServiceName 自动创建，仅含 service.name。
// 需要附加 service.version、deployment.environment 等属性时使用。
func WithResource(r *otelResource.Resource) Option {
	return func(c *config) {
		c.resource = r
	}
}

// Provider 是 tracing.Provider 的 OpenTelemetry 实现。
//
// 内部持有 *sdktrace.TracerProvider，负责 Span 的创建、采样和导出。
// 构造时自动调用 otel.SetTracerProvider，使 gorm/redis 等
// 第三方 OTel 插件能自动接入同一链路。
//
// Shutdown 需在进程退出前调用，确保缓存 Span 全部导出。
type Provider struct {
	tp *sdktrace.TracerProvider
}

var _ tracing.Provider = (*Provider)(nil)

// NewProvider 创建 Provider。
//
// 必填项：
//   - WithServiceName
//   - WithExporter
//
// 选填项：
//   - WithSampler（默认 AlwaysSample）
//   - WithResource（默认自动创建）
func NewProvider(opts ...Option) (*Provider, error) {
	cfg := &config{}
	for _, o := range opts {
		o(cfg)
	}

	if cfg.serviceName == "" {
		return nil, fmt.Errorf("otel: WithServiceName is required")
	}
	if cfg.exporter == nil {
		return nil, fmt.Errorf("otel: WithExporter is required")
	}

	res := cfg.resource
	if res == nil {
		var err error
		res, err = otelResource.New(context.Background(),
			otelResource.WithAttributes(semconv.ServiceName(cfg.serviceName)),
		)
		if err != nil {
			return nil, fmt.Errorf("otel: create resource: %w", err)
		}
	}

	sampler := cfg.sampler
	if sampler == nil {
		sampler = sdktrace.AlwaysSample()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(cfg.exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)

	return &Provider{tp: tp}, nil
}

// Tracer 返回指定 Scope 的 Tracer 实例。
//
// scope 标识 instrumentation 范围（如 "http"、"db"），映射为
// OTel instrumentation scope，导出端可用作分组维度。
func (p *Provider) Tracer(scope tracing.Scope) tracing.Tracer {
	return &tracer{inner: p.tp.Tracer(string(scope))}
}

// Shutdown 优雅关闭，flush 所有未导出 Span。
//
// 需在进程退出前调用。超时由外部 ctx 控制。
func (p *Provider) Shutdown(ctx context.Context) error {
	return p.tp.Shutdown(ctx)
}
