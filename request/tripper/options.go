package tripper

import (
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/propagator"
)

// ═══════════════════════════════════════════════════════════════════
// 传输层 Option
// ═══════════════════════════════════════════════════════════════════

// Proxy 设置 HTTP 代理 URL。
func Proxy(u *url.URL) Option {
	return func(c *config) {
		c.proxyURL = u
	}
}

// ProxyFromEnv 从环境变量读取代理配置。
//
// 支持的环境变量：
//
//	USE_PROXY=true       启用代理
//	OPENAI_PROXY_HTTP    代理 URL
func ProxyFromEnv() Option {
	return func(c *config) {
		if os.Getenv("USE_PROXY") != "true" {
			return
		}
		proxyStr := os.Getenv("OPENAI_PROXY_HTTP")
		if proxyStr == "" {
			return
		}
		u, err := url.Parse(proxyStr)
		if err != nil {
			return
		}
		c.proxyURL = u
	}
}

// TLSInsecure 跳过 TLS 证书验证（仅测试环境使用）。
func TLSInsecure() Option {
	return func(c *config) {
		c.insecureSkipVerify = true
	}
}

// ConnectTimeout 设置 TCP 连接超时。
func ConnectTimeout(d time.Duration) Option {
	return func(c *config) {
		c.connectTimeout = d
	}
}

// TLSHandshakeTimeout 设置 TLS 握手超时。
func TLSHandshakeTimeout(d time.Duration) Option {
	return func(c *config) {
		c.tlsHandshakeTimeout = d
	}
}

// MaxIdleConns 设置最大空闲连接数。
func MaxIdleConns(n int) Option {
	return func(c *config) {
		c.maxIdleConns = n
	}
}

// IdleConnTimeout 设置空闲连接保活时间。
func IdleConnTimeout(d time.Duration) Option {
	return func(c *config) {
		c.idleConnTimeout = d
	}
}

// BaseTransport 设置基础 Transport（用于传入预配置的 *http.Transport）。
//
// 注意：如果传入了 *http.Transport，Proxy/TLSInsecure 等选项会直接修改它。
// 如果传入的是非 *http.Transport 的 RoundTripper，传输层选项会被忽略。
func BaseTransport(rt http.RoundTripper) Option {
	return func(c *config) {
		c.base = rt
	}
}

// ═══════════════════════════════════════════════════════════════════
// Client 层 Option
// ═══════════════════════════════════════════════════════════════════

// ClientTimeout 设置 http.Client 的整体超时（含读取响应体）。
func ClientTimeout(d time.Duration) Option {
	return func(c *config) {
		c.clientTimeout = d
	}
}

// NoRedirect 禁止自动跟随重定向。3xx 响应会原样返回给调用方。
func NoRedirect() Option {
	return func(c *config) {
		c.noRedirect = true
	}
}

// ═══════════════════════════════════════════════════════════════════
// 中间件层 Option
// ═══════════════════════════════════════════════════════════════════

// Logger 启用请求/响应日志记录。
//
// 日志中间件作为最外层，记录完整链路耗时、请求方法/URL、状态码、
// 及请求/响应体大小（Debug 级别包含完整 body 内容）。
// 大响应体自动截断，默认截断阈值 100KB。
func Logger(logger logit.Logger) Option {
	return func(c *config) {
		c.logger = logger
	}
}

// LoggerOpts 启用请求/响应日志，并附加自定义选项。
//
//	tripper.LoggerOpts(logger, tripper.LogMaxBodySize(64*1024))
func LoggerOpts(logger logit.Logger, opts ...LogOption) Option {
	return func(c *config) {
		c.logger = logger
		c.logOpts = opts
	}
}

// Retry 启用请求重试。
//
// 默认行为：
//   - 网络错误、5xx、429 时重试
//   - 仅幂等方法（GET/HEAD/PUT/DELETE/OPTIONS/TRACE）
//   - 指数退避 + 随机抖动：100ms → 200ms → 400ms... 最大 5s
func Retry(maxRetries int) Option {
	return func(c *config) {
		c.maxRetries = maxRetries
	}
}

// RetryOpts 启用请求重试，并附加自定义选项。
//
//	tripper.RetryOpts(
//	    tripper.RetryMaxRetries(5),
//	    tripper.RetryExponentialBackoff(200*time.Millisecond, 10*time.Second),
//	)
func RetryOpts(opts ...RetryOption) Option {
	return func(c *config) {
		c.retryOpts = append(c.retryOpts, opts...)
	}
}

// Metrics 启用请求指标收集（Prometheus 等）。
func Metrics(m HTTPMetrics) Option {
	return func(c *config) {
		c.metrics = m
	}
}

// Propagation 启用 HTTP Header 透传（全链路追踪）。
func Propagation(propagators ...propagator.Propagator) Option {
	return func(c *config) {
		c.propagators = append(c.propagators, propagators...)
	}
}

// Custom 添加自定义中间件，应用在 Logger 之外（最外层）。
//
// 当内置中间件不满足需求时（如熔断、限流），通过此选项注入。
func Custom(decorators ...func(http.RoundTripper) http.RoundTripper) Option {
	return func(c *config) {
		c.customMW = append(c.customMW, decorators...)
	}
}
