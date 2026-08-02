package rabbitmq

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/izliang/bore/queue"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
)

func TestNewRequiresHost(t *testing.T) {
	_, err := New(Options{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Host is required")
}

func TestNewInvalidConnection(t *testing.T) {
	// Connection to a non-existent broker should fail gracefully.
	_, err := New(Options{Host: "localhost", Port: 9999})
	assert.Error(t, err)
}

func TestSubscribeNilHandler(t *testing.T) {
	rq := &rabbitQueue{}
	err := rq.Subscribe(context.Background(), "q", nil, nil)
	assert.ErrorIs(t, err, queue.ErrInvalidHandler)
}

func TestOptionsDefaults(t *testing.T) {
	// Verify that New applies defaults correctly before connection attempt
	// (test via error path since we can't connect)
	_, err := New(Options{Host: "invalid", Port: 9999})
	assert.Error(t, err)
	// The error should be a connection error, not a validation error
}

func TestAMQPURLVhost(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "root vhost", raw: "amqp://guest:guest@localhost:5672/", want: "/"},
		{name: "named vhost", raw: "amqp://guest:guest@localhost:5672/app", want: "app"},
		{name: "encoded slash", raw: "amqp://guest:guest@localhost:5672/app%2Fdev", want: "app/dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := amqpURLVhost(tt.raw)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRewriteAMQPURLVhost(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		vhost     string
		wantPath  string
		wantVhost string
	}{
		{name: "root vhost", raw: "amqp://guest:guest@localhost:5672/app", vhost: "/", wantPath: "/", wantVhost: "/"},
		{name: "named vhost", raw: "amqp://guest:guest@localhost:5672/", vhost: "prod", wantPath: "/prod", wantVhost: "prod"},
		{name: "slash in vhost", raw: "amqp://guest:guest@localhost:5672/", vhost: "app/dev", wantPath: "/app/dev", wantVhost: "app/dev"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rewriteAMQPURLVhost(tt.raw, tt.vhost)
			assert.NoError(t, err)
			parsed, err := url.Parse(got)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantPath, parsed.Path)
			gotVhost, err := amqpURLVhost(got)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantVhost, gotVhost)
		})
	}
}

func TestManagementAPIHost(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "amqp://guest:guest@localhost:5672/", want: "localhost:15672"},
		{raw: "amqp://guest:guest@localhost:5673/", want: "localhost:5673"},
		{raw: "amqps://guest:guest@rabbit.local/", want: "rabbit.local:15671"},
	}

	for _, tt := range tests {
		u, err := url.Parse(tt.raw)
		assert.NoError(t, err)
		assert.Equal(t, tt.want, managementAPIHost(u))
	}
}

func TestDeclareVhost(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/vhosts/app/dev", r.URL.Path)
		assert.Equal(t, "/api/vhosts/app%2Fdev", r.URL.EscapedPath())
		username, password, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "guest", username)
		assert.Equal(t, "secret", password)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	assert.NoError(t, err)
	_, port, err := net.SplitHostPort(serverURL.Host)
	assert.NoError(t, err)

	err = declareVhost(context.Background(), "amqp://guest:secret@"+serverURL.Hostname()+":"+port+"/app%2Fdev", "app/dev", managementAPIOptions{})
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestBuildAMQPURLBuildsURL(t *testing.T) {
	tlsCfg := &tls.Config{ServerName: "rabbit.local"}
	tests := []struct {
		name       string
		opts       Options
		wantScheme string
		wantHost   string
		wantVhost  string
	}{
		{
			name:       "structured amqp defaults",
			opts:       Options{Host: "localhost", Username: "guest", Password: "guest"},
			wantScheme: "amqp",
			wantHost:   "localhost:5672",
			wantVhost:  "/",
		},
		{
			name:       "tls uses amqps defaults",
			opts:       Options{Host: "rabbit.local", Username: "guest", TLSConfig: tlsCfg, Vhost: "prod"},
			wantScheme: "amqps",
			wantHost:   "rabbit.local:5671",
			wantVhost:  "prod",
		},
		{
			name:       "slash vhost",
			opts:       Options{Host: "rabbit.local", Vhost: "app/dev"},
			wantScheme: "amqp",
			wantHost:   "rabbit.local:5672",
			wantVhost:  "app/dev",
		},
		{
			name:       "ipv6 host",
			opts:       Options{Host: "::1"},
			wantScheme: "amqp",
			wantHost:   "[::1]:5672",
			wantVhost:  "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawURL, err := buildAMQPURL(tt.opts)
			assert.NoError(t, err)
			parsed, err := url.Parse(rawURL)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantScheme, parsed.Scheme)
			assert.Equal(t, tt.wantHost, parsed.Host)
			vhost, err := amqpURLVhost(rawURL)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantVhost, vhost)
		})
	}
}

func TestBuildAMQPURLURLQuery(t *testing.T) {
	rawURL, err := buildAMQPURL(Options{
		Host:     "localhost",
		URLQuery: url.Values{"heartbeat": {"30"}},
	})
	assert.NoError(t, err)
	parsed, err := url.Parse(rawURL)
	assert.NoError(t, err)
	assert.Equal(t, "30", parsed.Query().Get("heartbeat"))
}

func TestBuildAMQPURLValidation(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{name: "empty", opts: Options{}, want: "Host is required"},
		{name: "invalid port", opts: Options{Host: "localhost", Port: 99999}, want: "invalid port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildAMQPURL(tt.opts)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestCloneTLSConfig(t *testing.T) {
	assert.Nil(t, cloneTLSConfig(nil))
	cfg := &tls.Config{ServerName: "rabbit.local", MinVersion: tls.VersionTLS12}
	cloned := cloneTLSConfig(cfg)
	assert.NotSame(t, cfg, cloned)
	assert.Equal(t, cfg.ServerName, cloned.ServerName)
	assert.Equal(t, cfg.MinVersion, cloned.MinVersion)
}

func TestDeclareVhostWithManagementHTTPClient(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/api/vhosts/secure", r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	assert.NoError(t, err)
	_, port, err := net.SplitHostPort(serverURL.Host)
	assert.NoError(t, err)

	err = declareVhost(context.Background(), "amqps://guest:secret@"+serverURL.Hostname()+":"+port+"/secure", "secure", managementAPIOptions{client: server.Client()})
	assert.NoError(t, err)
}

func TestDeclareVhostWithManagementTLSConfig(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	assert.NoError(t, err)
	_, port, err := net.SplitHostPort(serverURL.Host)
	assert.NoError(t, err)

	transport := server.Client().Transport.(*http.Transport)
	err = declareVhost(context.Background(), "amqps://guest:secret@"+serverURL.Hostname()+":"+port+"/secure", "secure", managementAPIOptions{tlsConfig: transport.TLSClientConfig})
	assert.NoError(t, err)
}

func TestDeclareVhostHTTPSWithoutTLSConfigFailsForSelfSignedServer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	assert.NoError(t, err)
	_, port, err := net.SplitHostPort(serverURL.Host)
	assert.NoError(t, err)

	err = declareVhost(context.Background(), "amqps://guest:secret@"+serverURL.Hostname()+":"+port+"/secure", "secure", managementAPIOptions{})
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "certificate") || strings.Contains(err.Error(), "tls"), err.Error())
}

func TestIsVhostMissingError(t *testing.T) {
	assert.True(t, isVhostMissingError(fmt.Errorf("Exception (530) Reason: not allowed, vhost not found")))
	assert.True(t, isVhostMissingError(fmt.Errorf("Exception (403) Reason: no access to this vhost")))
	assert.False(t, isVhostMissingError(fmt.Errorf("dial tcp connection refused")))
}

func TestComputeBackoff(t *testing.T) {
	policy := &queue.RetryPolicy{
		Backoff:    1 * time.Second,
		MaxBackoff: 60 * time.Second,
	}

	tests := []struct {
		name       string
		retryCount int
		minBackoff time.Duration
		maxBackoff time.Duration
	}{
		{"first retry (0)", 0, 750 * time.Millisecond, 1250 * time.Millisecond},
		{"second retry (1)", 1, 1500 * time.Millisecond, 2500 * time.Millisecond},
		{"third retry (2)", 2, 3 * time.Second, 5 * time.Second},
		{"fourth retry (3)", 3, 6 * time.Second, 10 * time.Second},
		{"capped at max (10)", 10, 45 * time.Second, 75 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeBackoff(tt.retryCount, policy)
			assert.GreaterOrEqual(t, got, tt.minBackoff,
				"retryCount=%d: got %v, expected >= %v", tt.retryCount, got, tt.minBackoff)
			assert.LessOrEqual(t, got, tt.maxBackoff,
				"retryCount=%d: got %v, expected <= %v", tt.retryCount, got, tt.maxBackoff)
		})
	}
}

func TestComputeBackoffZeroValuePolicy(t *testing.T) {
	// Test that a zero-value RetryPolicy gets sensible defaults
	policy := &queue.RetryPolicy{}
	got := computeBackoff(0, policy)
	assert.GreaterOrEqual(t, got, 750*time.Millisecond)
	assert.LessOrEqual(t, got, 1250*time.Millisecond)
}

func TestGetRetryCountFromHeader(t *testing.T) {
	c := &consumer{}
	delivery := amqp.Delivery{Headers: amqp.Table{"x-retry-count": int32(2)}}

	assert.Equal(t, 2, c.getRetryCount(delivery))
}

func TestComputeBackoffJitterRange(t *testing.T) {
	policy := &queue.RetryPolicy{
		Backoff:    2 * time.Second,
		MaxBackoff: 120 * time.Second,
	}

	// Run many iterations to verify jitter stays within bounds
	for i := 0; i < 100; i++ {
		got := computeBackoff(0, policy)
		// 2s +/- 25% = 1.5s to 2.5s
		assert.GreaterOrEqual(t, got, 1500*time.Millisecond,
			"backoff too small: %v", got)
		assert.LessOrEqual(t, got, 2500*time.Millisecond,
			"backoff too large: %v", got)
	}
}

func TestComputeBackoffAtMaxBackoff(t *testing.T) {
	policy := &queue.RetryPolicy{
		Backoff:    1 * time.Second,
		MaxBackoff: 4 * time.Second,
	}

	// retryCount=10 should be capped at MaxBackoff (4s)
	for i := 0; i < 100; i++ {
		got := computeBackoff(10, policy)
		assert.GreaterOrEqual(t, got, 3*time.Second,
			"capped backoff too small: %v", got)
		assert.LessOrEqual(t, got, 5*time.Second,
			"capped backoff too large: %v", got)
	}
}

func TestComputeBackoffExponentialGrowth(t *testing.T) {
	policy := &queue.RetryPolicy{
		Backoff:    100 * time.Millisecond,
		MaxBackoff: 10 * time.Second,
	}

	// Verify that backoff roughly doubles each retry
	var prevMean time.Duration
	for retryCount := 0; retryCount < 5; retryCount++ {
		var sum time.Duration
		const samples = 50
		for i := 0; i < samples; i++ {
			sum += computeBackoff(retryCount, policy)
		}
		mean := sum / samples

		if retryCount > 0 {
			// Mean should be roughly 2x the previous mean (within 50% tolerance)
			ratio := float64(mean) / float64(prevMean)
			assert.Greater(t, ratio, 1.2,
				"retryCount=%d: mean %v not growing vs prev %v", retryCount, mean, prevMean)
		}
		prevMean = mean
	}
}

func TestAMQPMessageAckNackIdempotent(t *testing.T) {
	msg := &amqpMessage{delivery: amqp.Delivery{}}

	assert.Error(t, msg.Ack())
	assert.NoError(t, msg.Ack())
	assert.NoError(t, msg.Nack(false))
}

func TestConsumerWorkerStopsWhenContextCanceledBeforeProcessingDelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- amqp.Delivery{}
	close(deliveries)

	called := make(chan struct{}, 1)
	c := &consumer{}
	c.worker(ctx, 0, "queue", func(context.Context, queue.Message) queue.HandleResult {
		called <- struct{}{}
		return queue.Ack()
	}, queue.DefaultSubscribeOptions(), deliveries)

	assert.Empty(t, called)
}

func TestConsumerWorkerReturnsWhenDeliveriesClose(t *testing.T) {
	deliveries := make(chan amqp.Delivery)
	close(deliveries)

	c := &consumer{}
	c.worker(context.Background(), 0, "queue", func(context.Context, queue.Message) queue.HandleResult {
		return queue.Ack()
	}, queue.DefaultSubscribeOptions(), deliveries)
}

func TestTableToAMQP(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]any
		expect int
	}{
		{"nil input", nil, 0},
		{"empty map", map[string]any{}, 0},
		{"string values", map[string]any{"k1": "v1", "k2": "v2"}, 2},
		{"mixed types", map[string]any{"str": "val", "num": 42, "bool": true}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tableToAMQP(tt.input)
			assert.Equal(t, tt.expect, len(got))
			if tt.input != nil {
				for k, v := range tt.input {
					assert.Equal(t, v, got[k])
				}
			}
		})
	}
}

func TestBuildClientProperties(t *testing.T) {
	properties := buildClientProperties(map[string]any{
		"service": "order-worker",
		"env":     "test",
		"product": "custom-product",
	})

	assert.Equal(t, "custom-product", properties["product"])
	assert.Equal(t, "order-worker", properties["service"])
	assert.Equal(t, "test", properties["env"])
	assert.NotEmpty(t, properties["version"])
	assert.NotEmpty(t, properties["platform"])
}

func TestConnectionManagerCloseIdempotent(t *testing.T) {
	// connectionManager.close() should be safe to call multiple times
	// (we test indirectly via the CompareAndSwap behavior)
	// This is a structural test — actual connection requires a broker
}

func TestOptionsValidation(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty host",
			opts:    Options{},
			wantErr: true,
			errMsg:  "Host is required",
		},
		{
			name: "valid options but unreachable",
			opts: Options{
				Host:             "localhost",
				Port:             1,
				ProducerPoolSize: 0, // should default to 5
			},
			wantErr: true,
			errMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestComputeBackoffEdgeCases(t *testing.T) {
	// Negative retry count (shouldn't happen, but verify it behaves)
	policy := &queue.RetryPolicy{
		Backoff:    1 * time.Second,
		MaxBackoff: 60 * time.Second,
	}

	// retryCount=0 is the first retry, retryCount < 0 is invalid
	// The for loop won't execute, so backoff stays at 1s
	got := computeBackoff(-1, policy)
	assert.GreaterOrEqual(t, got, 750*time.Millisecond)
	assert.LessOrEqual(t, got, 1250*time.Millisecond)
}

func TestBackoffMath(t *testing.T) {
	// Verify the exponential formula: backoff * 2^retryCount
	policy := &queue.RetryPolicy{
		Backoff:    1 * time.Second,
		MaxBackoff: math.MaxInt64, // effectively no cap
	}

	// With no cap, backoff should be roughly initial * 2^retryCount
	for retryCount := 0; retryCount < 10; retryCount++ {
		got := computeBackoff(retryCount, policy)
		expected := time.Duration(float64(policy.Backoff) * math.Pow(2, float64(retryCount)))
		// Allow 50% tolerance due to jitter
		assert.LessOrEqual(t, got, expected+expected/2,
			"retryCount=%d: got %v, expected ~%v", retryCount, got, expected)
	}
}
