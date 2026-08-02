// Package tripper 提供统一的 HTTP 传输 + 中间件链构建。
//
// 一个包解决全部 HTTP 客户端需求：代理配置、TLS、超时、日志、重试、指标、Header 透传。
// 调用者无需关心"这是传输层问题还是中间件问题"——所有选项平铺在一个 config 里，
// 构建阶段内部自动分层。
//
// 中间件内置推荐顺序（从外到内）：
//
//	请求 → Custom → Logger → Metrics → Retry → Propagation → Transport
//
// 使用示例：
//
//	// 一行构建完整 http.Client
//	client := tripper.New(
//	    tripper.Proxy(proxyURL),
//	    tripper.Logger(logger),
//	    tripper.Retry(3),
//	    tripper.ClientTimeout(30*time.Second),
//	)
//	resp, _ := client.Get("https://api.example.com/data")
//
//	// 只构建 RoundTripper（用于自定义 http.Client）
//	rt := tripper.Build(
//	    tripper.Logger(logger),
//	    tripper.Retry(3),
//	)
//
//	// 只构建传输层（无中间件）
//	transport := tripper.TransportOnly(
//	    tripper.Proxy(proxyURL),
//	    tripper.TLSInsecure(),
//	)
package tripper

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/propagator"
)

// ═══════════════════════════════════════════════════════════════════
// Option — 统一配置入口
// ═══════════════════════════════════════════════════════════════════

// Option 是 tripper 的配置函数。所有传输层和中间件选项共用此类型。
type Option func(*config)

// config 收集全部选项，构建时内部自动分层（transport → middleware → client）。
type config struct {
	// ---- 传输层 ----
	base                http.RoundTripper // 基础 Transport（nil = 自动创建）
	proxyURL            *url.URL
	insecureSkipVerify  bool
	connectTimeout      time.Duration
	tlsHandshakeTimeout time.Duration
	maxIdleConns        int
	idleConnTimeout     time.Duration

	// ---- Client 层 ----
	clientTimeout time.Duration
	noRedirect    bool

	// ---- 中间件层 ----
	logger      logit.Logger
	logOpts     []LogOption
	maxRetries  int
	retryOpts   []RetryOption
	metrics     HTTPMetrics
	propagators []propagator.Propagator
	customMW    []func(http.RoundTripper) http.RoundTripper
}

// ═══════════════════════════════════════════════════════════════════
// 构造函数
// ═══════════════════════════════════════════════════════════════════

// Build 构建 http.RoundTripper（传输层 + 中间件链）。
//
// Option 传入顺序无关，框架内部按推荐顺序组装。零 Option 返回 http.DefaultTransport。
func Build(opts ...Option) http.RoundTripper {
	var c config
	for _, opt := range opts {
		opt(&c)
	}
	transport := c.buildTransport()
	return c.wrapMiddleware(transport)
}

// New 一键构建完整 *http.Client。
//
// 自动处理 Transport 创建、中间件包裹、Timeout 设置、重定向策略。
// 覆盖 90% 的 HTTP 客户端使用场景。
//
//	client := tripper.New(
//	    tripper.Logger(logger),
//	    tripper.Retry(3),
//	    tripper.ClientTimeout(30*time.Second),
//	)
func New(opts ...Option) *http.Client {
	var c config
	for _, opt := range opts {
		opt(&c)
	}
	transport := c.buildTransport()
	rt := c.wrapMiddleware(transport)

	client := &http.Client{
		Transport: rt,
		Timeout:   c.clientTimeout,
	}
	if c.noRedirect {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client
}

// TransportOnly 只构建 *http.Transport（传输层配置，无中间件）。
//
// 用于只需代理/TLS/连接池配置、不需要日志/重试/指标的场景。
//
//	transport := tripper.TransportOnly(
//	    tripper.Proxy(proxyURL),
//	    tripper.TLSInsecure(),
//	)
func TransportOnly(opts ...Option) *http.Transport {
	var c config
	for _, opt := range opts {
		opt(&c)
	}
	rt := c.buildTransport()
	if t, ok := rt.(*http.Transport); ok {
		return t
	}
	return defaultTransport()
}

// ═══════════════════════════════════════════════════════════════════
// 内部构建逻辑
// ═══════════════════════════════════════════════════════════════════

// buildTransport 根据 config 创建 http.RoundTripper。
func (c *config) buildTransport() http.RoundTripper {
	// 如已提供 base Transport，在其基础上修改
	base := defaultTransport()
	if c.base != nil {
		if t, ok := c.base.(*http.Transport); ok {
			base = t
		}
		// 非 *http.Transport：直接将原 RoundTripper 返回，跳过传输层配置
		if _, ok := c.base.(*http.Transport); !ok {
			return c.base
		}
	}

	if c.proxyURL != nil {
		base.Proxy = http.ProxyURL(c.proxyURL)
	}

	if c.insecureSkipVerify {
		if base.TLSClientConfig == nil {
			base.TLSClientConfig = &tls.Config{}
		}
		base.TLSClientConfig.InsecureSkipVerify = true
	}

	if c.connectTimeout > 0 {
		base.DialContext = (&net.Dialer{
			Timeout:   c.connectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext
	}

	if c.tlsHandshakeTimeout > 0 {
		if base.TLSClientConfig == nil {
			base.TLSClientConfig = &tls.Config{}
		}
		base.TLSHandshakeTimeout = c.tlsHandshakeTimeout
	}

	if c.maxIdleConns > 0 {
		base.MaxIdleConns = c.maxIdleConns
	}

	if c.idleConnTimeout > 0 {
		base.IdleConnTimeout = c.idleConnTimeout
	}

	return base
}

// wrapMiddleware 按推荐顺序包裹中间件。
//
// 顺序（从内到外构建）：
//
//	Transport → Propagation → Retry → Metrics → Logger → Custom
func (c *config) wrapMiddleware(transport http.RoundTripper) http.RoundTripper {
	rt := transport

	// 内层：Propagation（注入 Header）
	if len(c.propagators) > 0 {
		rt = NewPropagatorRoundTripper(c.propagators...)(rt)
	}

	// Retry
	if c.maxRetries > 0 || len(c.retryOpts) > 0 {
		opts := c.retryOpts
		if c.maxRetries > 0 {
			opts = append([]RetryOption{RetryMaxRetries(c.maxRetries)}, opts...)
		}
		rt = NewRetryRoundTripper(opts...)(rt)
	}

	// Metrics
	if c.metrics != nil {
		rt = NewMetricsRoundTripper(c.metrics)(rt)
	}

	// 外层：Logger
	if c.logger != nil {
		rt = NewLogitRoundTripper(c.logger, c.logOpts...)(rt)
	}

	// 最外层：Custom
	for _, decorator := range c.customMW {
		rt = decorator(rt)
	}

	return rt
}

// defaultTransport 返回 http.DefaultTransport 的克隆。
func defaultTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// ═══════════════════════════════════════════════════════════════════
// Config 结构体（便于从配置文件反序列化后使用）
// ═══════════════════════════════════════════════════════════════════

// Config 是 tripper 的完整配置。零值可用。
// 适合从 YAML/JSON 配置文件读取后使用。
type Config struct {
	// Transport
	ProxyURL            string        `json:"proxy_url,omitempty"`
	InsecureSkipVerify  bool          `json:"insecure_skip_verify,omitempty"`
	ConnectTimeout      time.Duration `json:"connect_timeout,omitempty"`
	TLSHandshakeTimeout time.Duration `json:"tls_handshake_timeout,omitempty"`
	MaxIdleConns        int           `json:"max_idle_conns,omitempty"`
	IdleConnTimeout     time.Duration `json:"idle_conn_timeout,omitempty"`

	// Client
	ClientTimeout time.Duration `json:"client_timeout,omitempty"`
	NoRedirect    bool          `json:"no_redirect,omitempty"`

	// Middleware
	Logger      logit.Logger            `json:"-"`
	MaxRetries  int                     `json:"max_retries,omitempty"`
	Metrics     HTTPMetrics             `json:"-"`
	Propagators []propagator.Propagator `json:"-"`
}

// FromConfig 从 Config 结构体构建 *http.Client。
// 适合从配置文件读取后使用。
func FromConfig(cfg Config) *http.Client {
	var opts []Option
	if cfg.ProxyURL != "" {
		if u, err := url.Parse(cfg.ProxyURL); err == nil {
			opts = append(opts, Proxy(u))
		}
	}
	if cfg.InsecureSkipVerify {
		opts = append(opts, TLSInsecure())
	}
	if cfg.ConnectTimeout > 0 {
		opts = append(opts, ConnectTimeout(cfg.ConnectTimeout))
	}
	if cfg.TLSHandshakeTimeout > 0 {
		opts = append(opts, TLSHandshakeTimeout(cfg.TLSHandshakeTimeout))
	}
	if cfg.MaxIdleConns > 0 {
		opts = append(opts, MaxIdleConns(cfg.MaxIdleConns))
	}
	if cfg.IdleConnTimeout > 0 {
		opts = append(opts, IdleConnTimeout(cfg.IdleConnTimeout))
	}
	if cfg.ClientTimeout > 0 {
		opts = append(opts, ClientTimeout(cfg.ClientTimeout))
	}
	if cfg.NoRedirect {
		opts = append(opts, NoRedirect())
	}
	if cfg.Logger != nil {
		opts = append(opts, Logger(cfg.Logger))
	}
	if cfg.MaxRetries > 0 {
		opts = append(opts, Retry(cfg.MaxRetries))
	}
	if cfg.Metrics != nil {
		opts = append(opts, Metrics(cfg.Metrics))
	}
	if len(cfg.Propagators) > 0 {
		opts = append(opts, Propagation(cfg.Propagators...))
	}
	return New(opts...)
}
