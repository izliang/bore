// Package kibanaproxy 实现 logprobe.LogSource 接口，通过 Kibana Console Proxy 间接访问 ES。
//
// 适用场景：ES 不直接暴露，只能在 Kibana Dev Tools 中通过 Console Proxy 访问。
// 认证依赖 Kibana session cookie，而非 ES Basic Auth / APIKey。
//
// HTTP 传输层基于 request/tripper 包声明式构建中间件链：
//
//	Logger（最外层，记录请求/响应）→ Retry（网络错误重试）→ Transport
//
// tripper 内置推荐顺序，调用者无需关心 Add/Build 的逆序细节。
//
// # 连接
//
//	src := kibanaproxy.NewSource(kibanaproxy.Config{
//	    Name:          "prod-logs",
//	    KibanaBaseURL: "https://kibana-zw.100credit.cn",
//	    Index:         "applog-prod-internal-*",
//	    Cookies:       "sid=xxx; cas_cookie_key=xxx",
//	    Logger:        logit.NewSimple(os.Stdout),   // HTTP 请求/响应日志
//	    MaxRetries:    3,                             // 网络错误重试次数
//	})
//
// # Cookie 过期处理
//
// 所有 HTTP 调用在收到 401/403 时返回 QueryError{Retryable: false,
// Hint: "Kibana session expired, refresh cookies and recreate Source"}。
// 调用方（AI 宿主）收到后应提示用户重新登录或自动刷新 cookie。
package kibanaproxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/logprobe"
	"github.com/izliang/bore/logprobe/adapters/escommon"
	"github.com/izliang/bore/request/tripper"
)

// ==========================================================================
// Config
// ==========================================================================

// Config 配置 Kibana Proxy 数据源。
type Config struct {
	// ---- 必需 ----

	// Name 数据源的人类可读名称（如 "prod-app-logs"）。
	// 同时作为 LogSource.ID() 的返回值。
	Name string

	// KibanaBaseURL Kibana 的根地址（如 "https://kibana-zw.100credit.cn"）。
	KibanaBaseURL string

	// Index ES 索引模式（如 "applog-prod-internal-*"）。
	// 支持通配符和多索引（逗号分隔）。
	Index string

	// Cookies Kibana session cookie 字符串。
	// 格式: "sid=xxx; cas_cookie_key=xxx"。
	// 注意：session cookie 会过期，需定时刷新后重建 Source。
	Cookies string

	// ---- 字段映射 ----

	// TimeField 时间戳字段名。默认 "@timestamp"。
	TimeField string

	// MessageField 消息主体字段名。默认 "message"。
	MessageField string

	// SearchFields query_string 搜索的目标字段。空 = 搜索所有字段。
	SearchFields []string

	// ScriptFields 注入每次 _search 请求的 script_fields。
	// 例如：{"message_preview": {"script": {"source": "..."}}}。
	// 注意：script_fields 的值不可通过 Expand 展开完整内容。
	ScriptFields map[string]any

	// ---- HTTP 传输 ----

	// Timeout HTTP 请求超时。默认 30s。
	Timeout time.Duration

	// Logger HTTP 请求/响应日志记录器。nil 表示不记录 HTTP 日志。
	// HTTP 传输层通过 tripper.Build 声明式构建：Logger（外层）→ Retry（内层）→ Transport
	// 自动记录每个 ES 请求的 method、URL、状态码、耗时。
	// 请求体和响应体会被截断（默认 4KB），避免大字段污染日志输出。
	Logger logit.Logger

	// MaxRetries 网络错误（连接失败、5xx）自动重试次数。
	// 0 = 不重试。默认 0。
	// 重试采用指数退避：100ms → 200ms → 400ms，上限 5s，
	// 每次附加 ±25% 的随机抖动避免惊群。
	// 注意：401/403 不重试（session 过期，重试无意义）。
	MaxRetries int

	// Transport 自定义 HTTP Transport。nil 使用 http.DefaultTransport。
	// 中间件链会包裹此 Transport。
	Transport http.RoundTripper
}

func (c *Config) normalize() {
	if c.TimeField == "" {
		c.TimeField = "@timestamp"
	}
	if c.MessageField == "" {
		c.MessageField = "message"
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
}

// ==========================================================================
// Source
// ==========================================================================

// Source 实现 logprobe.LogSource，通过 Kibana Console Proxy 间接访问 ES。
type Source struct {
	cfg    Config
	http   *http.Client
	cursor logprobe.Cursor
}

// NewSource 创建一个 Kibana Proxy 数据源。
//
// HTTP 传输层在创建时一次性初始化中间件链（Logger → Retry → Transport），
// 之后不再修改，并发安全。
func NewSource(cfg Config) *Source {
	cfg.normalize()

	// 声明式构建中间件链，tripper 内部按推荐顺序排列（Logger 外层，Retry 内层）。
	// 调用者只需声明"我需要什么"，无需关心 Add/Build 细节。
	var opts []tripper.Option
	if cfg.Transport != nil {
		opts = append(opts, tripper.BaseTransport(cfg.Transport))
	}
	if cfg.MaxRetries > 0 {
		opts = append(opts, tripper.Retry(cfg.MaxRetries))
	}
	if cfg.Logger != nil {
		opts = append(opts, tripper.Logger(cfg.Logger))
	}
	rt := tripper.Build(opts...)

	return &Source{
		cfg: cfg,
		http: &http.Client{
			Transport: rt,
			Timeout:   cfg.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // 不跟随重定向
			},
		},
		cursor: logprobe.Cursor{Backend: "kibanaproxy"},
	}
}

func (s *Source) ID() string                  { return s.cfg.Name }
func (s *Source) Cursor() logprobe.Cursor     { return s.cursor }
func (s *Source) SetCursor(c logprobe.Cursor) { s.cursor = c }

// ==========================================================================
// HTTP 传输层
// ==========================================================================

// doRequest 通过 Kibana Console Proxy 发送 ES 请求。
//
// 协议格式：
//
//	POST /api/console/proxy?path={url-encoded-es-path}&method={method}
//	Headers: Cookie, kbn-xsrf, Content-Type
//	Body: 标准 ES 请求体（直接透传）
func (s *Source) doRequest(ctx context.Context, method, esPath string, body io.Reader) (io.ReadCloser, error) {
	proxyURL := s.buildProxyURL(esPath, method)

	req, err := http.NewRequestWithContext(ctx, "POST", proxyURL, body)
	if err != nil {
		return nil, fmt.Errorf("kibanaproxy: 创建请求失败: %w", err)
	}

	req.Header.Set("Cookie", s.cfg.Cookies)
	req.Header.Set("kbn-xsrf", "kibana")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kibanaproxy: HTTP 请求失败: %w", err)
	}

	// 401/403 = session 过期，不重试，直接返回可执行错误
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		resp.Body.Close()
		return nil, &QueryError{
			Message:   "Kibana session expired",
			Retryable: false,
			Hint:      "Refresh cookies and recreate Source",
		}
	}

	// 其他 4xx/5xx（重试后的最终结果）
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kibanaproxy: ES 返回错误 (%d): %s", resp.StatusCode, string(errBody))
	}

	return resp.Body, nil
}

// buildProxyURL 构造 Kibana Console Proxy URL。
//
// 示例：
//
//	输入: esPath="applog-*/_search", method="POST"
//	输出: "https://kibana-zw.100credit.cn/api/console/proxy?path=applog-%2A%2F_search&method=POST"
func (s *Source) buildProxyURL(esPath, method string) string {
	encoded := url.PathEscape(esPath)
	return fmt.Sprintf("%s/api/console/proxy?path=%s&method=%s",
		strings.TrimRight(s.cfg.KibanaBaseURL, "/"),
		encoded,
		method,
	)
}

// searchOpts 构建 escommon 查询选项。
func (s *Source) searchOpts() []escommon.SearchOption {
	var opts []escommon.SearchOption
	if s.cfg.TimeField != "@timestamp" {
		opts = append(opts, escommon.WithTimeField(s.cfg.TimeField))
	}
	if len(s.cfg.SearchFields) > 0 {
		opts = append(opts, escommon.WithSearchFields(s.cfg.SearchFields...))
	}
	if len(s.cfg.ScriptFields) > 0 {
		opts = append(opts, escommon.WithScriptFields(s.cfg.ScriptFields))
	}
	return opts
}

// ==========================================================================
// QueryError — 可指导 AI 修正的错误
// ==========================================================================

// QueryError 是可操作的查询错误，AI 读取后可自动修正并重试。
// 与普通 error 不同，QueryError 携带了 Hint（修正建议）。
type QueryError struct {
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Hint      string `json:"hint,omitempty"`
}

func (e *QueryError) Error() string { return e.Message }
