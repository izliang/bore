package tripper

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// ═══════════════════════════════════════════════════════════════════
// MockRoundTripper — 测试用 RoundTripper
// ═══════════════════════════════════════════════════════════════════

// MockRoundTripper 是一个用于测试的 RoundTripper 实现。
type MockRoundTripper struct {
	// Response 固定返回的响应
	Response *http.Response
	// Err 固定返回的错误
	Err error
	// RoundTripFunc 自定义处理函数，设置后优先使用
	RoundTripFunc func(*http.Request) (*http.Response, error)
	// LastRequest 记录最后一次收到的请求
	LastRequest *http.Request
	// CallCount 记录被调用的次数
	CallCount int
}

// NewMockRoundTripper 创建返回指定状态码和响应体的 MockRoundTripper。
func NewMockRoundTripper(statusCode int, body string) *MockRoundTripper {
	return &MockRoundTripper{
		Response: &http.Response{
			StatusCode: statusCode,
			Body:       io.NopCloser(bytes.NewBufferString(body)),
			Header:     make(http.Header),
		},
	}
}

// RoundTrip 实现 http.RoundTripper 接口。
func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.CallCount++
	m.LastRequest = req.Clone(req.Context())

	if m.RoundTripFunc != nil {
		return m.RoundTripFunc(req)
	}
	if m.Err != nil {
		return nil, m.Err
	}
	if m.Response != nil {
		respBody, _ := io.ReadAll(m.Response.Body)
		m.Response.Body.Close()
		m.Response.Body = io.NopCloser(bytes.NewBuffer(respBody))
		return m.Response, nil
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString("OK")),
		Header:     make(http.Header),
	}, nil
}

// Reset 重置 MockRoundTripper 的状态。
func (m *MockRoundTripper) Reset() {
	m.LastRequest = nil
	m.CallCount = 0
}

// AssertCalled 断言 MockRoundTripper 至少被调用过一次。
func (m *MockRoundTripper) AssertCalled() error {
	if m.CallCount == 0 {
		return fmt.Errorf("expected MockRoundTripper to be called, but it wasn't")
	}
	return nil
}

// AssertCalledCount 断言 MockRoundTripper 被调用了指定次数。
func (m *MockRoundTripper) AssertCalledCount(count int) error {
	if m.CallCount != count {
		return fmt.Errorf("expected MockRoundTripper to be called %d times, but was called %d times", count, m.CallCount)
	}
	return nil
}
