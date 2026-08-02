package tripper

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/propagator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ═══════════════════════════════════════════════════════════════════
// 测试辅助
// ═══════════════════════════════════════════════════════════════════

type testTransport struct {
	statusCode int
	body       string
	err        error
	called     int
	delay      time.Duration
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.called++
	if t.delay > 0 {
		time.Sleep(t.delay)
	}
	if t.err != nil {
		return nil, t.err
	}
	return &http.Response{
		StatusCode: t.statusCode,
		Body:       io.NopCloser(bytes.NewBufferString(t.body)),
		Header:     make(http.Header),
	}, nil
}

type mockMetrics struct {
	requestCount     int
	durationObserved bool
}

func (m *mockMetrics) IncRequest(method, host, status string) { m.requestCount++ }
func (m *mockMetrics) ObserveRequestDuration(method, host, status string, d time.Duration) {
	m.durationObserved = true
}
func (m *mockMetrics) IncRetry(method, host string)       {}
func (m *mockMetrics) IncCircuitOpen(method, host string) {}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// ═══════════════════════════════════════════════════════════════════
// Build 测试
// ═══════════════════════════════════════════════════════════════════

func TestBuild_ZeroOptions(t *testing.T) {
	rt := Build()
	assert.NotNil(t, rt)
}

func TestBuild_Logger(t *testing.T) {
	tt := &testTransport{statusCode: 200, body: "ok"}
	logger := logit.NewSimple(io.Discard)

	rt := Build(BaseTransport(tt), Logger(logger))

	req, _ := http.NewRequest("GET", "http://example.com/test", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, tt.called)
}

func TestBuild_Retry(t *testing.T) {
	t.Run("retry on 5xx", func(t *testing.T) {
		tt := &testTransport{statusCode: 500, body: "error"}
		rt := Build(BaseTransport(tt), Retry(2))

		req, _ := http.NewRequest("GET", "http://example.com/500", nil)
		resp, err := rt.RoundTrip(req)

		require.NoError(t, err)
		assert.Equal(t, 500, resp.StatusCode)
		assert.Equal(t, 3, tt.called)
	})

	t.Run("no retry on 4xx", func(t *testing.T) {
		tt := &testTransport{statusCode: 400, body: "bad request"}
		rt := Build(BaseTransport(tt), Retry(3))

		req, _ := http.NewRequest("GET", "http://example.com/400", nil)
		resp, err := rt.RoundTrip(req)

		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
		assert.Equal(t, 1, tt.called)
	})

	t.Run("zero retries disabled", func(t *testing.T) {
		tt := &testTransport{statusCode: 500, body: "error"}
		rt := Build(BaseTransport(tt), Retry(0))

		req, _ := http.NewRequest("GET", "http://example.com/noretry", nil)
		resp, err := rt.RoundTrip(req)

		require.NoError(t, err)
		assert.Equal(t, 500, resp.StatusCode)
		assert.Equal(t, 1, tt.called)
	})
}

func TestBuild_OrderGuarantee(t *testing.T) {
	tt := &testTransport{statusCode: 500, body: "error"}
	logger := logit.NewSimple(io.Discard)

	// Option 传入顺序任意，Build 内部保证 Logger 在 Retry 外层
	rt := Build(BaseTransport(tt), Logger(logger), Retry(2))

	req, _ := http.NewRequest("GET", "http://example.com/order", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	assert.Equal(t, 3, tt.called) // 重试生效，说明 Logger 在 Retry 外层
}

func TestBuild_AllOptions(t *testing.T) {
	tt := &testTransport{statusCode: 200, body: "ok"}
	logger := logit.NewSimple(io.Discard)
	metrics := &mockMetrics{}

	rt := Build(
		BaseTransport(tt),
		Logger(logger),
		Retry(1),
		Metrics(metrics),
	)

	req, _ := http.NewRequest("GET", "http://example.com/all", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, tt.called)
	assert.Equal(t, 1, metrics.requestCount)
}

func TestBuild_Custom(t *testing.T) {
	var customCalled bool
	customDec := func(next http.RoundTripper) http.RoundTripper {
		return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			customCalled = true
			return next.RoundTrip(req)
		})
	}

	tt := &testTransport{statusCode: 200, body: "ok"}
	rt := Build(BaseTransport(tt), Custom(customDec))

	req, _ := http.NewRequest("GET", "http://example.com/custom", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, customCalled)
}

func TestBuild_RetryOpts(t *testing.T) {
	tt := &testTransport{statusCode: 418, body: "teapot"}

	rt := Build(
		BaseTransport(tt),
		RetryOpts(
			RetryMaxRetries(2),
			RetryPolicyOption(func(resp *http.Response, err error) bool {
				if err != nil {
					return true
				}
				return resp.StatusCode >= 500 || resp.StatusCode == 429 || resp.StatusCode == 418
			}),
		),
	)

	req, _ := http.NewRequest("GET", "http://example.com/418", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 418, resp.StatusCode)
	assert.Equal(t, 3, tt.called)
}

// ═══════════════════════════════════════════════════════════════════
// New 测试
// ═══════════════════════════════════════════════════════════════════

func TestNew_FullClient(t *testing.T) {
	tt := &testTransport{statusCode: 200, body: "ok"}
	logger := logit.NewSimple(io.Discard)

	client := New(
		BaseTransport(tt),
		Logger(logger),
		Retry(2),
		ClientTimeout(30*time.Second),
	)

	assert.Equal(t, 30*time.Second, client.Timeout)
	assert.NotNil(t, client.Transport)

	req, _ := http.NewRequest("GET", "http://example.com/client", nil)
	resp, err := client.Do(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, tt.called)
}

func TestNew_NoRedirect(t *testing.T) {
	tt := &testTransport{statusCode: 302, body: ""}

	client := New(
		BaseTransport(tt),
		NoRedirect(),
	)

	req, _ := http.NewRequest("GET", "http://example.com/redirect", nil)
	resp, err := client.Do(req)

	require.NoError(t, err)
	assert.Equal(t, 302, resp.StatusCode) // 不跟随重定向
}

func TestNew_ZeroOptions(t *testing.T) {
	client := New()
	assert.NotNil(t, client)
	assert.NotNil(t, client.Transport)
}

// ═══════════════════════════════════════════════════════════════════
// TransportOnly 测试
// ═══════════════════════════════════════════════════════════════════

func TestTransportOnly_Basic(t *testing.T) {
	tr := TransportOnly()
	assert.NotNil(t, tr)
	assert.Equal(t, 100, tr.MaxIdleConns)
}

func TestTransportOnly_Proxy(t *testing.T) {
	proxyURL, _ := url.Parse("http://proxy.example.com:8080")
	tr := TransportOnly(Proxy(proxyURL))

	assert.NotNil(t, tr)
	// 验证代理已设置：构造一个请求，看 Proxy 函数返回什么
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	gotURL, err := tr.Proxy(req)
	require.NoError(t, err)
	assert.Equal(t, proxyURL.String(), gotURL.String())
}

func TestTransportOnly_TLSInsecure(t *testing.T) {
	tr := TransportOnly(TLSInsecure())
	assert.NotNil(t, tr.TLSClientConfig)
	assert.True(t, tr.TLSClientConfig.InsecureSkipVerify)
}

func TestTransportOnly_MaxIdleConns(t *testing.T) {
	tr := TransportOnly(MaxIdleConns(200))
	assert.Equal(t, 200, tr.MaxIdleConns)
}

func TestTransportOnly_IdleConnTimeout(t *testing.T) {
	tr := TransportOnly(IdleConnTimeout(60 * time.Second))
	assert.Equal(t, 60*time.Second, tr.IdleConnTimeout)
}

func TestTransportOnly_ConnectTimeout(t *testing.T) {
	tr := TransportOnly(ConnectTimeout(5 * time.Second))
	assert.NotNil(t, tr.DialContext)
}

func TestTransportOnly_TLSHandshakeTimeout(t *testing.T) {
	tr := TransportOnly(TLSHandshakeTimeout(15 * time.Second))
	assert.NotNil(t, tr.TLSClientConfig)
	assert.Equal(t, 15*time.Second, tr.TLSHandshakeTimeout)
}

func TestTransportOnly_BaseTransport(t *testing.T) {
	custom := &http.Transport{MaxIdleConns: 50}
	tr := TransportOnly(BaseTransport(custom))
	assert.Equal(t, 50, tr.MaxIdleConns) // 传入的自定义 Transport 被直接使用
}

// ═══════════════════════════════════════════════════════════════════
// FromConfig 测试
// ═══════════════════════════════════════════════════════════════════

func TestFromConfig(t *testing.T) {
	tt := &testTransport{statusCode: 200, body: "ok"}
	logger := logit.NewSimple(io.Discard)

	client := FromConfig(Config{
		Logger:     logger,
		MaxRetries: 2,
	})
	client.Transport = Build(BaseTransport(tt), Logger(logger), Retry(2))

	req, _ := http.NewRequest("GET", "http://example.com/config", nil)
	resp, err := client.Do(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// ═══════════════════════════════════════════════════════════════════
// 独立装饰器测试
// ═══════════════════════════════════════════════════════════════════

func TestNewLogger_Standalone(t *testing.T) {
	tt := &testTransport{statusCode: 200, body: "ok"}
	logger := logit.NewSimple(io.Discard)

	rt := NewLogger(logger)(tt)

	req, _ := http.NewRequest("GET", "http://example.com/logger", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestNewRetry_Standalone(t *testing.T) {
	tt := &testTransport{statusCode: 500, body: "error"}

	rt := NewRetry(2)(tt)

	req, _ := http.NewRequest("GET", "http://example.com/retry", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	assert.Equal(t, 3, tt.called)
}

func TestNewMetrics_Standalone(t *testing.T) {
	tt := &testTransport{statusCode: 200, body: "ok"}
	metrics := &mockMetrics{}

	rt := NewMetrics(metrics)(tt)

	req, _ := http.NewRequest("GET", "http://example.com/metrics", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	_ = resp
	assert.Equal(t, 1, metrics.requestCount)
}

func TestNewPropagation_Standalone(t *testing.T) {
	tt := &testTransport{statusCode: 200, body: "ok"}
	prop := &testPropagator{headerKey: "X-Test-Id", headerValue: "test-value"}

	rt := NewPropagation(prop)(tt)

	req, _ := http.NewRequest("GET", "http://example.com/propagate", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestManualChain(t *testing.T) {
	tt := &testTransport{statusCode: 500, body: "error"}
	logger := logit.NewSimple(io.Discard)

	var rt http.RoundTripper = tt
	rt = NewRetry(2)(rt)
	rt = NewLogger(logger)(rt)

	req, _ := http.NewRequest("GET", "http://example.com/chain", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	assert.Equal(t, 3, tt.called)
}

// ═══════════════════════════════════════════════════════════════════
// 中间件单元测试
// ═══════════════════════════════════════════════════════════════════

func TestLogitRoundTripper_Error(t *testing.T) {
	expectedErr := fmt.Errorf("network error")
	tt := &testTransport{err: expectedErr}
	logger := logit.NewSimple(io.Discard)

	rt := NewLogger(logger)(tt)

	req, _ := http.NewRequest("GET", "http://example.com/error", nil)
	resp, err := rt.RoundTrip(req)

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, 1, tt.called)
}

func TestLogitRoundTripper_LargeBody(t *testing.T) {
	largeBody := strings.Repeat("x", 20*1024)
	tt := &testTransport{statusCode: 200, body: largeBody}
	logger := logit.NewSimple(io.Discard)

	rt := NewLogger(logger, LogMaxBodySize(1024))(tt)

	req, _ := http.NewRequest("GET", "http://example.com/large", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	respBody, _ := io.ReadAll(resp.Body)
	assert.Equal(t, len(largeBody), len(respBody)) // 原始响应体完整
}

func TestRetryRoundTripper_ContextCancel(t *testing.T) {
	tt := &testTransport{statusCode: 500, body: "error", delay: 50 * time.Millisecond}

	rt := Build(BaseTransport(tt), Retry(5))

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com/cancel", nil)

	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	resp, err := rt.RoundTrip(req)
	assert.Error(t, err)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func TestRetryRoundTripper_NonIdempotent(t *testing.T) {
	tt := &testTransport{statusCode: 500, body: "error"}
	rt := Build(BaseTransport(tt), Retry(3))

	req, _ := http.NewRequest("POST", "http://example.com/post", bytes.NewBufferString("data"))
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	assert.Equal(t, 1, tt.called) // POST 默认不重试
}

func TestRetryRoundTripper_CustomMethods(t *testing.T) {
	tt := &testTransport{statusCode: 500, body: "error"}

	rt := Build(
		BaseTransport(tt),
		RetryOpts(RetryMaxRetries(2), RetryMethods("POST", "GET")),
	)

	req, _ := http.NewRequest("POST", "http://example.com/post-retry", bytes.NewBufferString("data"))
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
	assert.Equal(t, 3, tt.called) // POST 现在可以重试
}

func TestMetricsRoundTripper_NilMetrics(t *testing.T) {
	tt := &testTransport{statusCode: 200, body: "ok"}
	rt := NewMetrics(nil)(tt)

	req, _ := http.NewRequest("GET", "http://example.com/noop", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestReadAndTruncateBody(t *testing.T) {
	t.Run("body smaller than max", func(t *testing.T) {
		body := io.NopCloser(bytes.NewBufferString("small body"))
		truncated, restored, total, err := readAndTruncateBody(body, 100, "", 0)

		require.NoError(t, err)
		assert.Equal(t, "small body", string(truncated))
		assert.Equal(t, int64(10), total)
		restoredBody, _ := io.ReadAll(restored)
		assert.Equal(t, "small body", string(restoredBody))
	})

	t.Run("body larger than max", func(t *testing.T) {
		// 使用可打印字符（字母 'x'），确保不会被启发式判定为二进制
		largeBody := strings.Repeat("x", 20*1024)
		body := io.NopCloser(bytes.NewBufferString(largeBody))
		truncated, restored, total, err := readAndTruncateBody(body, 1024, "", 0)

		require.NoError(t, err)
		assert.Contains(t, string(truncated), "truncated")
		// 大 body 流式场景 total 返回 -1（未知总大小）
		assert.Equal(t, int64(-1), total)
		restoredBody, _ := io.ReadAll(restored)
		assert.Equal(t, len(largeBody), len(restoredBody))
		// restored 绝不能包含日志截断标记（核心安全断言）
		assert.NotContains(t, string(restoredBody), "truncated")
		// restored 应与原始完全一致
		assert.Equal(t, largeBody, string(restoredBody))
	})

	t.Run("nil body", func(t *testing.T) {
		truncated, restored, total, err := readAndTruncateBody(nil, 100, "", 0)
		require.NoError(t, err)
		assert.Nil(t, truncated)
		assert.Nil(t, restored)
		assert.Equal(t, int64(0), total)
	})

	t.Run("empty body", func(t *testing.T) {
		body := io.NopCloser(bytes.NewBufferString(""))
		truncated, restored, total, err := readAndTruncateBody(body, 100, "", 0)

		require.NoError(t, err)
		assert.Empty(t, truncated)
		assert.Equal(t, int64(0), total)
		restoredBody, _ := io.ReadAll(restored)
		assert.Empty(t, restoredBody)
	})
}

func TestReadAndTruncateBody_Binary(t *testing.T) {
	t.Run("binary with image content-type", func(t *testing.T) {
		// PNG magic bytes: 89 50 4E 47 0D 0A 1A 0A
		pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52}
		padding := bytes.Repeat([]byte{0x00}, 100)
		pngData := append(pngMagic, padding...)
		body := io.NopCloser(bytes.NewBuffer(pngData))

		truncated, restored, total, err := readAndTruncateBody(body, 1024, "image/png", 0)

		require.NoError(t, err)
		// 应该显示紧凑摘要，而非原始乱码
		assert.Contains(t, string(truncated), "[binary:")
		assert.Contains(t, string(truncated), "image/png")
		assert.Equal(t, int64(len(pngData)), total)
		// restored body 应完整
		restoredBody, _ := io.ReadAll(restored)
		assert.Equal(t, len(pngData), len(restoredBody))
	})

	t.Run("binary with hex preview", func(t *testing.T) {
		pngMagic := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		body := io.NopCloser(bytes.NewBuffer(pngMagic))

		truncated, _, _, err := readAndTruncateBody(body, 1024, "image/png", 16)

		require.NoError(t, err)
		assert.Contains(t, string(truncated), "[binary:")
		assert.Contains(t, string(truncated), "hex:")
		assert.Contains(t, string(truncated), "89 50 4E 47 0D 0A 1A 0A")
	})

	t.Run("binary heuristic detection no content-type", func(t *testing.T) {
		// 随机二进制数据（非 printable）
		randomBytes := make([]byte, 256)
		for i := range randomBytes {
			randomBytes[i] = byte(i) // 0x00-0xFF 全覆盖，大量非 printable
		}
		body := io.NopCloser(bytes.NewBuffer(randomBytes))

		truncated, _, _, err := readAndTruncateBody(body, 1024, "", 0)

		require.NoError(t, err)
		// 启发式应检测为二进制
		assert.Contains(t, string(truncated), "[binary:")
		assert.Contains(t, string(truncated), "application/octet-stream")
	})

	t.Run("json body stays as text", func(t *testing.T) {
		jsonData := []byte(`{"status": "ok", "data": {"id": 1, "name": "test"}}`)
		body := io.NopCloser(bytes.NewBuffer(jsonData))

		truncated, _, _, err := readAndTruncateBody(body, 1024, "application/json", 0)

		require.NoError(t, err)
		// JSON 应作为文本处理，不显示 [binary:] 标记
		assert.Equal(t, string(jsonData), string(truncated))
		assert.NotContains(t, string(truncated), "[binary:")
	})

	t.Run("chinese text heuristic stays as text", func(t *testing.T) {
		chinese := []byte("你好世界 Hello World こんにちは")
		body := io.NopCloser(bytes.NewBuffer(chinese))

		truncated, _, _, err := readAndTruncateBody(body, 1024, "", 0)

		require.NoError(t, err)
		// UTF-8 多字节字符应被识别为 printable（启发式 pass）
		assert.Equal(t, string(chinese), string(truncated))
		assert.NotContains(t, string(truncated), "[binary:")
	})
}

// ═══════════════════════════════════════════════════════════════════
// MockRoundTripper 测试
// ═══════════════════════════════════════════════════════════════════

// ═══════════════════════════════════════════════════════════════════
// isBinaryContent 单元测试
// ═══════════════════════════════════════════════════════════════════

func TestIsBinaryContent(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		contentType string
		expected    bool
	}{
		// Content-Type 白名单 → text
		{name: "text/plain", data: []byte("hello"), contentType: "text/plain", expected: false},
		{name: "application/json", data: []byte(`{"a":1}`), contentType: "application/json", expected: false},
		{name: "application/xml", data: []byte("<x/>"), contentType: "application/xml", expected: false},
		{name: "text/html utf-8", data: []byte("<html>"), contentType: "text/html; charset=utf-8", expected: false},

		// Content-Type 黑名单 → binary
		{name: "image/png", data: []byte{0x89, 0x50, 0x4E, 0x47}, contentType: "image/png", expected: true},
		{name: "application/octet-stream", data: []byte{0x00}, contentType: "application/octet-stream", expected: true},
		{name: "application/protobuf", data: []byte{0x0A, 0x03}, contentType: "application/protobuf", expected: true},
		{name: "application/zip", data: []byte{0x50, 0x4B}, contentType: "application/zip", expected: true},
		{name: "audio/mpeg", data: []byte{0xFF, 0xFB}, contentType: "audio/mpeg", expected: true},

		// 未知 Content-Type + 二进制数据 → heuristic binary
		{name: "random bytes no CT", data: makeRandomBytes(256), contentType: "", expected: true},
		{name: "NULL bytes no CT", data: bytes.Repeat([]byte{0x00}, 100), contentType: "", expected: true},

		// 未知 Content-Type + 文本数据 → heuristic text
		{name: "printable ascii no CT", data: []byte("hello world"), contentType: "", expected: false},
		{name: "json no CT", data: []byte(`{"key":"value"}`), contentType: "", expected: false},
		{name: "utf8 chinese no CT", data: []byte("你好世界"), contentType: "", expected: false},

		// 空数据 → text
		{name: "empty", data: []byte{}, contentType: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBinaryContent(tt.data, tt.contentType)
			assert.Equal(t, tt.expected, result, "isBinaryContent(%q, %q)", tt.data, tt.contentType)
		})
	}
}

func TestFormatBinaryBody(t *testing.T) {
	t.Run("no hex preview", func(t *testing.T) {
		result := formatBinaryBody([]byte{0x89, 0x50}, "image/png", 24501, 0)
		assert.Equal(t, "[binary: image/png, 24501 bytes]", string(result))
	})

	t.Run("with hex preview", func(t *testing.T) {
		result := formatBinaryBody([]byte{0x89, 0x50, 0x4E, 0x47}, "image/png", 24501, 16)
		assert.Contains(t, string(result), "[binary: image/png, 24501 bytes, hex:")
		assert.Contains(t, string(result), "89 50 4E 47")
	})

	t.Run("partial hex preview", func(t *testing.T) {
		result := formatBinaryBody([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A}, "image/png", 24501, 4)
		// hexPreviewLen=4 → 只显示前4字节，且有 ... 后缀
		assert.Contains(t, string(result), "hex: 89 50 4E 47...")
	})

	t.Run("empty content-type defaults to octet-stream", func(t *testing.T) {
		result := formatBinaryBody([]byte{0x00}, "", 100, 0)
		assert.Contains(t, string(result), "application/octet-stream")
	})
}

// makeRandomBytes 创建包含所有 0-255 字节值的切片，用于二进制检测测试。
func makeRandomBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 256)
	}
	return b
}

func TestMockRoundTripper(t *testing.T) {
	t.Run("default response", func(t *testing.T) {
		mock := &MockRoundTripper{}
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		resp, err := mock.RoundTrip(req)

		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, 1, mock.CallCount)
	})

	t.Run("custom response", func(t *testing.T) {
		mock := NewMockRoundTripper(404, "not found")
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		resp, err := mock.RoundTrip(req)

		require.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("custom round trip function", func(t *testing.T) {
		called := false
		mock := &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				called = true
				return &http.Response{
					StatusCode: 201,
					Body:       io.NopCloser(bytes.NewBufferString("created")),
				}, nil
			},
		}
		req, _ := http.NewRequest("POST", "http://example.com", nil)
		resp, err := mock.RoundTrip(req)

		require.NoError(t, err)
		assert.Equal(t, 201, resp.StatusCode)
		assert.True(t, called)
	})

	t.Run("reset state", func(t *testing.T) {
		mock := &MockRoundTripper{}
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		_, _ = mock.RoundTrip(req)
		assert.Equal(t, 1, mock.CallCount)

		mock.Reset()
		assert.Equal(t, 0, mock.CallCount)
		assert.Nil(t, mock.LastRequest)
	})

	t.Run("assert called", func(t *testing.T) {
		mock := &MockRoundTripper{}
		assert.Error(t, mock.AssertCalled())

		req, _ := http.NewRequest("GET", "http://example.com", nil)
		_, _ = mock.RoundTrip(req)
		assert.NoError(t, mock.AssertCalled())
	})
}

// ═══════════════════════════════════════════════════════════════════
// PropagatorRoundTripper 测试
// ═══════════════════════════════════════════════════════════════════

type testPropagator struct {
	headerKey   string
	headerValue string
}

func (p *testPropagator) Inject(_ context.Context, carrier propagator.Carrier) {
	carrier.Set(p.headerKey, p.headerValue)
}
func (p *testPropagator) Extract(ctx context.Context, _ propagator.Carrier) context.Context {
	return ctx
}

var _ propagator.Propagator = (*testPropagator)(nil)

func TestPropagation_Build(t *testing.T) {
	prop := &testPropagator{headerKey: "X-Test-Id", headerValue: "test-value"}
	tt := &testTransport{statusCode: 200, body: "ok"}

	rt := Build(BaseTransport(tt), Propagation(prop))

	req, _ := http.NewRequest("GET", "http://example.com/propagate", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// ═══════════════════════════════════════════════════════════════════
// 端到端全链路测试
// ═══════════════════════════════════════════════════════════════════

func TestFullMiddlewareChain(t *testing.T) {
	logger := logit.NewSimple(io.Discard)
	metrics := &mockMetrics{}
	tt := &testTransport{statusCode: 200, body: "success"}

	rt := Build(
		BaseTransport(tt),
		Logger(logger),
		Metrics(metrics),
		Retry(2),
	)

	req, _ := http.NewRequest("GET", "http://example.com/full", nil)
	resp, err := rt.RoundTrip(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, tt.called)
	assert.Equal(t, 1, metrics.requestCount)
}

func TestNew_FullStack(t *testing.T) {
	tt := &testTransport{statusCode: 200, body: "success"}
	logger := logit.NewSimple(io.Discard)
	metrics := &mockMetrics{}

	client := New(
		BaseTransport(tt),
		Logger(logger),
		Retry(2),
		Metrics(metrics),
		ClientTimeout(30*time.Second),
		NoRedirect(),
	)

	req, _ := http.NewRequest("GET", "http://example.com/fullstack", nil)
	resp, err := client.Do(req)

	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, metrics.requestCount)
}

// ═══════════════════════════════════════════════════════════════════
// Benchmark
// ═══════════════════════════════════════════════════════════════════

func BenchmarkBuild(b *testing.B) {
	tt := &testTransport{statusCode: 200, body: "ok"}
	logger := logit.NewSimple(io.Discard)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt := Build(BaseTransport(tt), Logger(logger), Retry(3))
		req, _ := http.NewRequest("GET", "http://example.com/bench", nil)
		_, _ = rt.RoundTrip(req)
	}
}

func BenchmarkNew(b *testing.B) {
	tt := &testTransport{statusCode: 200, body: "ok"}
	logger := logit.NewSimple(io.Discard)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := New(BaseTransport(tt), Logger(logger), Retry(3), ClientTimeout(30*time.Second))
		req, _ := http.NewRequest("GET", "http://example.com/bench", nil)
		_, _ = client.Do(req)
	}
}

// ═══════════════════════════════════════════════════════════════════
// 编译期验证
// ═══════════════════════════════════════════════════════════════════

var _ Option = Logger(nil)
var _ Option = Retry(0)
var _ Option = Metrics(nil)
var _ Option = Custom()
var _ Option = Proxy(nil)
var _ Option = TLSInsecure()

var _ HTTPMetrics = (*mockMetrics)(nil)
var _ HTTPMetrics = (*NoopMetrics)(nil)

var _ http.RoundTripper = Build()

var _ = fmt.Sprintf
var _ = context.Background
