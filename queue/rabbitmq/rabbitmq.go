// Package rabbitmq 提供基于 RabbitMQ（AMQP 0-9-1 协议）的 queue.Queue 接口实现。
//
// 核心能力：
//   - 自动重连：连接断开后指数退避重连，消费者自动重新声明队列并重新订阅
//   - Channel 池：发布操作使用 channel pool 支持高并发，避免单 channel 瓶颈
//   - 动态并发：支持 ConcurrencyController 实现运行时 worker 扩缩容，无需重启
//   - 消费者并发控制：通过 SubscribeOptions.Concurrency 或 ConcurrencyController 配置 worker 数
//   - 重试与死信：内置 RetryPolicy，支持指数退避、抖动（jitter）、x-retry-count 追踪和 DLX
//   - 队列声明：支持持久化、TTL、最大长度、死信交换等完整配置
//   - 可观测性：可选 Prometheus 指标集成（发布/消费/重试/panic 等维度）
//   - 优雅关闭：Shutdown 等待在途消息处理完毕后关闭连接
//
// # 快速上手
//
//	q, err := rabbitmq.New(rabbitmq.Options{
//	    Host:     "localhost",
//	    Username: "guest",
//	    Password: "guest",
//	})
//	if err != nil { ... }
//	defer q.Shutdown(context.Background())
//
//	// 声明队列（持久化 + 死信）
//	q.Declare("tasks", &queue.DeclareOptions{
//	    Durable:            true,
//	    DeadLetterExchange: "tasks-dlx",
//	})
//
//	// 发布消息
//	q.Publish(ctx, "tasks", queue.NewMessage([]byte("hello")))
//
//	// 订阅消费（并发 + 重试）
//	q.Subscribe(ctx, "tasks", handler, &queue.SubscribeOptions{
//	    Concurrency: 4,
//	    Retry:       queue.DefaultRetryPolicy(),
//	})
package rabbitmq

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/propagator"
	"github.com/izliang/bore/queue"
	"github.com/prometheus/client_golang/prometheus"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Options 配置 RabbitMQ 队列实例的所有参数。
type Options struct {
	// Host 是 RabbitMQ 服务地址（必填）。
	Host string

	// Port 是 AMQP 端口。默认值：amqp → 5672，amqps → 5671。
	Port int

	// Username 和 Password 是 AMQP 认证凭据。
	Username string
	Password string

	// Vhost 是 AMQP 虚拟主机。为空时使用根 vhost "/"。
	Vhost string

	// URLQuery 是附加到 AMQP URI 的查询参数。
	URLQuery url.Values

	// TLSConfig 配置 AMQPS 连接的 TLS。每次拨号前会克隆副本。
	TLSConfig *tls.Config

	// ManagementTLSConfig 配置 RabbitMQ Management API（DeclareVhost 调用）的 TLS。
	ManagementTLSConfig *tls.Config

	// ManagementHTTPClient 可选地覆盖 DeclareVhost 使用的 HTTP 客户端。
	ManagementHTTPClient *http.Client

	// Logger 是用于运维日志记录的 logit.Logger 实例。
	// 为 nil 时默认使用 logit.NewSimple(os.Stdout)。
	Logger logit.Logger

	// MetricsRegistry 是可选的 Prometheus 注册表，用于注册队列指标。
	MetricsRegistry *prometheus.Registry

	// MetricsSubsystem 是指标子系统标签，默认为 "rabbitmq"。
	MetricsSubsystem string

	// ConcurrencyController 提供全局的、按队列的动态并发度控制。
	//
	// 设置后，Subscribe 会通过 Controller 解析初始并发度并 Watch 后续变更，
	// 实现消费者 worker 的运行时扩缩容，无需重启或重新订阅。
	//
	// 为 nil 时退回到 SubscribeOptions.Concurrency 静态值（保持向后兼容）。
	ConcurrencyController queue.ConcurrencyController

	// ProducerPoolSize 是发布 channel 池的大小，默认 5。
	// 每个 channel 处理一个并发发布请求，高吞吐场景适当调大。
	ProducerPoolSize int

	// ReconnectInitial 是重连的初始退避时间，默认 1 秒。
	ReconnectInitial time.Duration

	// ReconnectMax 是重连退避的上限，默认 30 秒。
	// 当消费者稳定运行超过此时长后，退避时间重置为 ReconnectInitial，避免永久锁定在最大退避。
	ReconnectMax time.Duration

	// Propagator 是链路传播器，用于在 MQ 消息头中注入/提取 SRE 上下文。
	Propagator propagator.Propagator

	// VhostHandle 在连接前改写 vhost，常用于多环境路由。
	VhostHandle VhostHandle

	// DeclareVhost 是否默认不创建 vhost。
	// 设为 true 可确保 vhost 不存在时仍能顺利初始化（通过 Management API 自动创建）。
	DeclareVhost bool

	// ClientProperties 是 AMQP 连接属性，显示在 RabbitMQ 管理界面中。
	// 可用于标识服务名、版本等元信息。
	ClientProperties map[string]any
}

type VhostHandle func(vhost string) string

func VhostHandleEnv(vhost string) string {
	if v := os.Getenv("sre_gray_apiCode"); v != "" {
		return v
	}
	if v := os.Getenv("ABP_ENV_NAME"); v != "" {
		return v
	}
	return vhost
}

func buildAMQPURL(opts Options) (string, error) {
	scheme := "amqp"
	if opts.TLSConfig != nil {
		scheme = "amqps"
	}
	if opts.Host == "" {
		return "", fmt.Errorf("rabbitmq: Host is required")
	}
	port := opts.Port
	if port == 0 {
		if scheme == "amqps" {
			port = 5671
		} else {
			port = 5672
		}
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("rabbitmq: invalid port %d", opts.Port)
	}

	u := url.URL{
		Scheme:   scheme,
		Host:     net.JoinHostPort(opts.Host, fmt.Sprintf("%d", port)),
		RawQuery: opts.URLQuery.Encode(),
	}
	if opts.Username != "" || opts.Password != "" {
		if opts.Password != "" {
			u.User = url.UserPassword(opts.Username, opts.Password)
		} else {
			u.User = url.User(opts.Username)
		}
	}
	vhost := opts.Vhost
	if vhost == "" || vhost == "/" {
		u.Path = "/"
	} else {
		u.Path = "/" + vhost
		u.RawPath = "/" + url.PathEscape(vhost)
	}
	return u.String(), nil
}

func cloneTLSConfig(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		return nil
	}
	return cfg.Clone()
}

func firstTLSConfig(primary, fallback *tls.Config) *tls.Config {
	if primary != nil {
		return primary
	}
	return fallback
}

func amqpURLVhost(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("rabbitmq: invalid URL: %w", err)
	}
	if u.Scheme != "amqp" && u.Scheme != "amqps" {
		return "", fmt.Errorf("rabbitmq: unsupported URL scheme %q", u.Scheme)
	}
	path := strings.TrimPrefix(u.EscapedPath(), "/")
	if path == "" {
		return "/", nil
	}
	vhost, err := url.PathUnescape(path)
	if err != nil {
		return "", fmt.Errorf("rabbitmq: invalid vhost in URL: %w", err)
	}
	return vhost, nil
}

func rewriteAMQPURLVhost(rawURL, vhost string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("rabbitmq: invalid URL: %w", err)
	}
	if vhost == "" || vhost == "/" {
		u.Path = "/"
		u.RawPath = ""
		return u.String(), nil
	}
	u.Path = "/" + vhost
	u.RawPath = "/" + url.PathEscape(vhost)
	return u.String(), nil
}

func isVhostMissingError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "vhost") && (strings.Contains(msg, "not found") || strings.Contains(msg, "no access"))
}

type managementAPIOptions struct {
	client    *http.Client
	tlsConfig *tls.Config
}

func declareVhost(ctx context.Context, amqpURL, vhost string, opts managementAPIOptions) error {
	u, err := url.Parse(amqpURL)
	if err != nil {
		return fmt.Errorf("invalid AMQP URL: %w", err)
	}
	if u.User == nil {
		return fmt.Errorf("AMQP URL user info is required")
	}
	username := u.User.Username()
	password, _ := u.User.Password()
	apiScheme := "http"
	if u.Scheme == "amqps" {
		apiScheme = "https"
	}
	apiURL := url.URL{
		Scheme:  apiScheme,
		Host:    managementAPIHost(u),
		Path:    "/api/vhosts/" + vhost,
		RawPath: "/api/vhosts/" + url.PathEscape(vhost),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL.String(), nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(username, password)

	resp, err := managementHTTPClient(apiScheme, opts).Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("RabbitMQ management API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
}

func managementHTTPClient(apiScheme string, opts managementAPIOptions) *http.Client {
	if opts.client != nil {
		return opts.client
	}
	if apiScheme != "https" || opts.tlsConfig == nil {
		return http.DefaultClient
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = cloneTLSConfig(opts.tlsConfig)
	return &http.Client{Transport: transport}
}

func managementAPIHost(u *url.URL) string {
	hostname := u.Hostname()
	if hostname == "" {
		return u.Host
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "amqps" {
			port = "15671"
		} else {
			port = "15672"
		}
	} else if port == "5671" {
		port = "15671"
	} else if port == "5672" {
		port = "15672"
	}
	return net.JoinHostPort(hostname, port)
}

// rabbitQueue implements queue.Queue backed by RabbitMQ.
type rabbitQueue struct {
	opts       Options
	cm         *connectionManager
	pool       *channelPool
	logger     logit.Logger
	metrics    *queue.Metrics
	vhost      string
	propagator propagator.Propagator

	mu     sync.Mutex
	closed bool
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new RabbitMQ-backed Queue.
// It establishes a connection immediately and returns an error if that fails.
func New(opts Options) (queue.Queue, error) {
	amqpURL, err := buildAMQPURL(opts)
	if err != nil {
		return nil, err
	}

	vhost, err := amqpURLVhost(amqpURL)
	if err != nil {
		return nil, err
	}
	if opts.VhostHandle == nil {
		opts.VhostHandle = VhostHandleEnv
	}
	newVhost := opts.VhostHandle(vhost)
	if newVhost != vhost {
		amqpURL, err = rewriteAMQPURLVhost(amqpURL, newVhost)
		if err != nil {
			return nil, err
		}
		vhost = newVhost
	}
	if opts.Logger == nil {
		opts.Logger = logit.NewSimple(os.Stdout)
	}
	if opts.ProducerPoolSize < 1 {
		opts.ProducerPoolSize = 5
	}
	if opts.ReconnectInitial <= 0 {
		opts.ReconnectInitial = 1 * time.Second
	}
	if opts.ReconnectMax <= 0 {
		opts.ReconnectMax = 30 * time.Second
	}
	if opts.MetricsSubsystem == "" {
		opts.MetricsSubsystem = "rabbitmq"
	}

	cfg := &connectionConfig{
		reconnectInitial: opts.ReconnectInitial,
		reconnectMax:     opts.ReconnectMax,
		clientProperties: opts.ClientProperties,
		tlsConfig:        cloneTLSConfig(opts.TLSConfig),
	}
	managementOpts := managementAPIOptions{
		client:    opts.ManagementHTTPClient,
		tlsConfig: cloneTLSConfig(firstTLSConfig(opts.ManagementTLSConfig, opts.TLSConfig)),
	}

	cm, err := newConnectionManager(amqpURL, cfg, opts.Logger)
	if err != nil {
		if !opts.DeclareVhost || vhost == "" || !isVhostMissingError(err) {
			return nil, err
		}
		opts.Logger.Warn(context.Background(), "rabbitmq vhost missing, declaring vhost",
			logit.StringField("vhost", vhost),
			logit.ErrorField("error", err),
		)
		if declareErr := declareVhost(context.Background(), amqpURL, vhost, managementOpts); declareErr != nil {
			opts.Logger.Error(context.Background(), declareErr,
				logit.StringField("component", "rabbitmq-vhost-declare"),
				logit.StringField("vhost", vhost),
			)
			return nil, fmt.Errorf("%w; declare vhost %q: %v", err, vhost, declareErr)
		}
		opts.Logger.Info(context.Background(), "rabbitmq vhost declared",
			logit.StringField("vhost", vhost),
		)
		cm, err = newConnectionManager(amqpURL, cfg, opts.Logger)
		if err != nil {
			opts.Logger.Error(context.Background(), err,
				logit.StringField("component", "rabbitmq-vhost-reconnect"),
				logit.StringField("vhost", vhost),
			)
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start background reconnect loop
	cm.startReconnectLoop(ctx)

	pool, err := newChannelPool(cm, opts.ProducerPoolSize)
	if err != nil {
		cancel()
		_ = cm.close()
		return nil, err
	}

	var m *queue.Metrics
	if opts.MetricsRegistry != nil {
		m = queue.NewMetrics(opts.MetricsSubsystem)
		opts.MetricsRegistry.MustRegister(m.GetCollectors()...)
		m.ConnectionStatus.Set(1)
	}

	rq := &rabbitQueue{
		opts:       opts,
		cm:         cm,
		pool:       pool,
		logger:     opts.Logger,
		metrics:    m,
		vhost:      vhost,
		ctx:        ctx,
		cancel:     cancel,
		propagator: opts.Propagator,
	}

	// Wire reconnect callback to refresh channel pool and update metrics.
	cm.onReconnect = func() {
		if err := pool.refresh(); err != nil {
			opts.Logger.Error(ctx, err, logit.StringField("component", "rabbitmq-reconnect"))
		}
		if m != nil {
			m.ConnectionStatus.Set(1)
			m.ReconnectTotal.Inc()
		}
	}

	return rq, nil
}

// Publish sends a message to the default exchange using routingKey as the queue name.
func (rq *rabbitQueue) Publish(ctx context.Context, routingKey string, msg queue.Message) error {
	return rq.PublishWithRouting(ctx, "", routingKey, msg)
}

// PublishWithRouting sends a message to a specific exchange with an explicit routing key.
func (rq *rabbitQueue) PublishWithRouting(ctx context.Context, exchange, routingKey string, msg queue.Message) error {
	rq.mu.Lock()
	closed := rq.closed
	rq.mu.Unlock()
	if closed {
		return queue.ErrQueueClosed
	}

	p := &producer{pool: rq.pool, metrics: rq.metrics, vhost: rq.vhost, propagator: rq.propagator}
	return p.PublishWithRouting(ctx, exchange, routingKey, msg)
}

// Subscribe starts consuming messages. It blocks until ctx is cancelled.
// Automatically reconnects and resubscribes if the connection drops.
func (rq *rabbitQueue) Subscribe(ctx context.Context, queueName string, handler queue.Handler, opts *queue.SubscribeOptions) error {
	if handler == nil {
		return fmt.Errorf("%w: nil handler", queue.ErrInvalidHandler)
	}

	rq.mu.Lock()
	closed := rq.closed
	rq.mu.Unlock()
	if closed {
		return queue.ErrQueueClosed
	}

	if opts == nil {
		opts = queue.DefaultSubscribeOptions()
	}
	backoff := rq.opts.ReconnectInitial
	attempt := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		attempt++
		start := time.Now()
		c := &consumer{cm: rq.cm, logger: rq.logger, metrics: rq.metrics, vhost: rq.vhost, propagator: rq.opts.Propagator, controller: rq.opts.ConcurrencyController}
		err := c.runConsumer(ctx, queueName, handler, opts, rq.Declare)
		runtime := time.Since(start)

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if errors.Is(err, queue.ErrQueueClosed) {
			return err
		}

		// Reset backoff if consumer ran successfully for longer than ReconnectMax.
		// This prevents the backoff from permanently sticking at the max value
		// after a historical series of failures, even when the connection has
		// been stable for a long time.
		if runtime > rq.opts.ReconnectMax {
			backoff = rq.opts.ReconnectInitial
			attempt = 1
		}

		jitter := time.Duration(rand.Int63n(int64(backoff) / 2))
		sleep := backoff + jitter

		rq.logger.Warn(ctx, "consumer exited, will resubscribe",
			logit.StringField("queue", queueName),
			logit.IntField("attempt", attempt),
			logit.StringField("runtime", runtime.String()),
			logit.StringField("backoff", sleep.String()),
			logit.ErrorField("error", err),
		)

		if rq.metrics != nil {
			rq.metrics.ConsumerResubscribeTotal.WithLabelValues(rq.vhost, queueName).Inc()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}
		backoff = time.Duration(min(float64(backoff)*2, float64(rq.opts.ReconnectMax)))
	}
}

// Declare creates or ensures a queue with the given options.
func (rq *rabbitQueue) Declare(queueName string, opts *queue.DeclareOptions) error {
	rq.mu.Lock()
	closed := rq.closed
	rq.mu.Unlock()
	if closed {
		return queue.ErrQueueClosed
	}

	if opts == nil {
		opts = queue.DefaultDeclareOptions()
	}

	ch, err := rq.cm.channel()
	if err != nil {
		return fmt.Errorf("%w: %v", queue.ErrDeclareFailed, err)
	}
	defer func() {
		_ = ch.Close()
	}()

	args := amqp.Table{}
	if opts.Arguments != nil {
		for k, v := range opts.Arguments {
			args[k] = v
		}
	}
	if opts.DeadLetterExchange != "" {
		args["x-dead-letter-exchange"] = opts.DeadLetterExchange
	}
	if opts.DeadLetterRoutingKey != "" {
		args["x-dead-letter-routing-key"] = opts.DeadLetterRoutingKey
	}
	if opts.MessageTTL > 0 {
		args["x-message-ttl"] = int64(opts.MessageTTL / time.Millisecond)
	}
	if opts.MaxLength > 0 {
		args["x-max-length"] = opts.MaxLength
	}
	if opts.MaxLengthBytes > 0 {
		args["x-max-length-bytes"] = opts.MaxLengthBytes
	}

	_, err = ch.QueueDeclare(
		queueName,
		opts.Durable,
		opts.AutoDelete,
		opts.Exclusive,
		opts.NoWait,
		args,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", queue.ErrDeclareFailed, err)
	}

	rq.logger.Info(context.Background(), "queue declared",
		logit.StringField("queue", queueName),
		logit.BoolField("durable", opts.Durable),
	)

	return nil
}

// DeclareExchange declares an exchange (not part of the queue.Queue interface,
// but exposed as a convenience method for RabbitMQ-specific setups).
func (rq *rabbitQueue) DeclareExchange(name, kind string, durable, autoDelete, internal, noWait bool, args map[string]any) error {
	rq.mu.Lock()
	closed := rq.closed
	rq.mu.Unlock()
	if closed {
		return queue.ErrQueueClosed
	}

	ch, err := rq.cm.channel()
	if err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}
	defer func() {
		_ = ch.Close()
	}()

	amqpArgs := amqp.Table{}
	for k, v := range args {
		amqpArgs[k] = v
	}

	return ch.ExchangeDeclare(name, kind, durable, autoDelete, internal, noWait, amqpArgs)
}

// BindQueue binds a queue to an exchange with a routing key (convenience method).
func (rq *rabbitQueue) BindQueue(queueName, routingKey, exchange string, noWait bool, args map[string]any) error {
	rq.mu.Lock()
	closed := rq.closed
	rq.mu.Unlock()
	if closed {
		return queue.ErrQueueClosed
	}

	ch, err := rq.cm.channel()
	if err != nil {
		return fmt.Errorf("bind queue: %w", err)
	}
	defer func() {
		_ = ch.Close()
	}()

	amqpArgs := amqp.Table{}
	for k, v := range args {
		amqpArgs[k] = v
	}

	return ch.QueueBind(queueName, routingKey, exchange, noWait, amqpArgs)
}

// Shutdown gracefully shuts down the queue, closing producers, consumers,
// and the broker connection.
func (rq *rabbitQueue) Shutdown(ctx context.Context) error {
	rq.mu.Lock()
	if rq.closed {
		rq.mu.Unlock()
		return nil
	}
	rq.closed = true
	rq.mu.Unlock()

	// Signal internal context cancellation
	rq.cancel()

	if rq.metrics != nil {
		rq.metrics.ConnectionStatus.Set(0)
	}

	// Close producer pool
	rq.pool.close()

	// Close connection (stops reconnect loop)
	return rq.cm.close()
}

// IsHealthy returns true if the connection is alive.
func (rq *rabbitQueue) IsHealthy() bool {
	return rq.cm.isHealthy()
}

// Metrics returns the queue metrics (may be nil if not configured).
func (rq *rabbitQueue) Metrics() *queue.Metrics {
	return rq.metrics
}

// tableToAMQP converts map[string]any to amqp.Table.
func tableToAMQP(m map[string]any) amqp.Table {
	if m == nil {
		return nil
	}
	t := make(amqp.Table, len(m))
	for k, v := range m {
		t[k] = v
	}
	return t
}
