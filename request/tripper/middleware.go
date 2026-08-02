package tripper

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"math/big"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/izliang/bore/logit"
)

// ═══════════════════════════════════════════════════════════════════
// LogitRoundTripper — HTTP 请求/响应日志中间件
// ═══════════════════════════════════════════════════════════════════

const (
	// DefaultMaxBodyLogSize 日志中记录的请求/响应体的最大大小（100KB）
	DefaultMaxBodyLogSize = 100 * 1024

	// RequestIDKey 日志中 Request ID 的字段名
	RequestIDKey = "req_rel_id"

	// DefaultBinaryHexPreview 二进制 body 的 hex 预览默认字节数（0 = 不显示 hex）
	DefaultBinaryHexPreview = 0

	// binaryPeekSize 启发式检测时检查的字节数
	binaryPeekSize = 512

	// binaryPrintableThreshold 启发式检测的 printable 比例阈值
	// 低于此比例判定为二进制
	binaryPrintableThreshold = 0.7
)

// LogitRoundTripper 提供 HTTP 请求的结构化日志记录。
// 作为最外层中间件，捕获请求的最终状态和总耗时。
//
// 特性：
//   - 为每个请求生成唯一的 Request ID，关联请求开始/完成两条日志
//   - 大响应体自动截断，避免内存问题和日志膨胀
//   - 二进制 body 自动识别，显示紧凑摘要而非乱码
//   - 记录请求体大小（不记录完整内容，避免影响请求流）
type LogitRoundTripper struct {
	next         http.RoundTripper
	logger       logit.Logger
	maxBodySize  int // 日志中记录的最大 body 大小（字节）
	binaryMaxHex int // 二进制 body 的 hex 预览字节数，0 = 仅摘要
}

// LogOption 配置 LogitRoundTripper 的选项。
type LogOption func(*LogitRoundTripper)

// LogMaxBodySize 设置日志中记录的请求/响应体的最大大小。
// 超过此大小的体将被截断并添加省略标记。默认 100KB。
func LogMaxBodySize(size int) LogOption {
	return func(l *LogitRoundTripper) {
		l.maxBodySize = size
	}
}

// LogBinaryHexPreview 设置二进制 body 的 hex 预览字节数。
//
//	0  = 仅显示 [binary: image/png, 24501 bytes]（默认）
//	16 = 显示前 16 字节的 hex，如 [binary: image/png, 24501 bytes, hex: 89 50 4E 47 ...]
func LogBinaryHexPreview(n int) LogOption {
	return func(l *LogitRoundTripper) {
		l.binaryMaxHex = n
	}
}

// NewLogitRoundTripper 创建日志中间件装饰器。
func NewLogitRoundTripper(logger logit.Logger, opts ...LogOption) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		rt := &LogitRoundTripper{
			next:         next,
			logger:       logger,
			maxBodySize:  DefaultMaxBodyLogSize,
			binaryMaxHex: DefaultBinaryHexPreview,
		}
		for _, opt := range opts {
			opt(rt)
		}
		return rt
	}
}

// NewLogger 是 NewLogitRoundTripper 的便捷别名。
func NewLogger(logger logit.Logger, opts ...LogOption) func(http.RoundTripper) http.RoundTripper {
	return NewLogitRoundTripper(logger, opts...)
}

// generateRequestID 生成唯一的请求 ID（16 字节随机数的 hex 编码）。
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// isBinaryContent 判断数据是否为二进制内容。
//
// 判定规则（二级判定）：
//  1. contentType 明确是 text 类型 → false
//  2. contentType 明确是 binary 类型 → true
//  3. contentType 缺失或未知 → 字节启发式检测
//
// text 白名单：text/*, application/json, application/xml, application/javascript
// binary 黑名单：image/*, audio/*, video/*, application/octet-stream, application/protobuf 等
func isBinaryContent(data []byte, contentType string) bool {
	// 解析 media type（去掉 charset 等参数）
	mediatype, _, _ := mime.ParseMediaType(contentType)

	// 规则 1：已知 text 类型
	if isTextMediaType(mediatype) {
		return false
	}

	// 规则 2：已知 binary 类型
	if isBinaryMediaType(mediatype) {
		return true
	}

	// 规则 3：启发式检测
	return isBinaryByHeuristic(data)
}

// isTextMediaType 判断 media type 是否为已知的文本类型。
func isTextMediaType(mediatype string) bool {
	if strings.HasPrefix(mediatype, "text/") {
		return true
	}
	switch mediatype {
	case "application/json", "application/xml", "application/javascript",
		"application/x-www-form-urlencoded", "application/x-yaml",
		"application/graphql":
		return true
	}
	return false
}

// isBinaryMediaType 判断 media type 是否为已知的二进制类型。
func isBinaryMediaType(mediatype string) bool {
	for _, prefix := range []string{"image/", "audio/", "video/"} {
		if strings.HasPrefix(mediatype, prefix) {
			return true
		}
	}
	switch mediatype {
	case "application/octet-stream", "application/protobuf",
		"application/grpc", "application/grpc+proto",
		"application/zip", "application/gzip", "application/pdf",
		"application/x-protobuf", "application/x-gzip",
		"application/x-msgpack", "application/x-www-form-urlencoded":
		return true
	}
	return false
}

// isBinaryByHeuristic 通过字节启发式判断是否为二进制。
//
// 取前 binaryPeekSize 字节，统计 printable 字符占比。
// printable 定义：ASCII 32-126 + \t \n \r + 有效 UTF-8 多字节序列。
// printable < binaryPrintableThreshold → 判定为二进制。
func isBinaryByHeuristic(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	peek := data
	if len(peek) > binaryPeekSize {
		peek = peek[:binaryPeekSize]
	}

	printable := 0
	for i := 0; i < len(peek); {
		b := peek[i]
		// ASCII printable + 常见空白
		if b >= 32 && b <= 126 || b == '\t' || b == '\n' || b == '\r' {
			printable++
			i++
			continue
		}
		// NULL 字节强烈提示二进制
		if b == 0 {
			return true
		}
		// 尝试 UTF-8 多字节序列
		r, size := utf8.DecodeRune(peek[i:])
		if r != utf8.RuneError && size > 1 {
			printable += size
			i += size
			continue
		}
		i++
	}

	return float64(printable)/float64(len(peek)) < binaryPrintableThreshold
}

// formatBinaryBody 格式化二进制 body 的日志输出。
//
// hexPreviewLen = 0: [binary: image/png, 24501 bytes]
// hexPreviewLen > 0: [binary: image/png, 24501 bytes, hex: 89 50 4E 47 ...]
func formatBinaryBody(data []byte, contentType string, totalSize int64, hexPreviewLen int) []byte {
	mediatype := contentType
	if mediatype == "" {
		mediatype = "application/octet-stream"
	}

	var buf bytes.Buffer
	_, _ = fmt.Fprintf(&buf, "[binary: %s, %d bytes", mediatype, totalSize)

	if hexPreviewLen > 0 && len(data) > 0 {
		preview := data
		if len(preview) > hexPreviewLen {
			preview = preview[:hexPreviewLen]
		}
		hexStr := hex.EncodeToString(preview)
		// 每两个 hex 字符加一个空格，便于阅读
		var spaced string
		for i := 0; i < len(hexStr); i += 2 {
			if i > 0 {
				spaced += " "
			}
			spaced += hexStr[i : i+2]
		}
		_, _ = fmt.Fprintf(&buf, ", hex: %s", strings.ToUpper(spaced))
		if len(data) > hexPreviewLen {
			buf.WriteString("...")
		}
	}

	buf.WriteString("]")
	return buf.Bytes()
}

// readAndTruncateBody 读取 body 的前 maxSize+1 字节用于日志，并返回可继续读取的 restored。
//
// 设计原则：
//  1. 只读取日志所需的量（头 maxSize+1 字节），绝不将整个 body 加载到内存。
//  2. 日志操作在独立的副本上进行，物理上不可能污染 restored。
//  3. 对大 body（> maxSize+1），restored 通过 prefix replay + body stream 实现零拷贝透传。
//
// 返回值：
//   - truncated: 日志用的 body 内容（已截断/格式化）
//   - restored:  调用方可继续完整读取的 io.ReadCloser
//   - total:     body 总字节数，大 body 时返回 -1（未知）
func readAndTruncateBody(body io.ReadCloser, maxSize int, contentType string, binaryHexPreview int) (truncated []byte, restored io.ReadCloser, total int64, err error) {
	if body == nil {
		return nil, nil, 0, nil
	}

	// 阶段1：只读头 maxSize+1 字节（常量内存，与 body 大小无关）
	var head bytes.Buffer
	head.Grow(maxSize + 1)
	headN, copyErr := io.CopyN(&head, body, int64(maxSize+1))

	if copyErr != nil && copyErr != io.EOF {
		_ = body.Close()
		return nil, nil, 0, copyErr
	}

	consumed := head.Bytes() // len = headN

	if copyErr == io.EOF {
		// Body < maxSize+1：全部读完，body 已耗尽
		_ = body.Close()
		total = headN
		restored = io.NopCloser(bytes.NewBuffer(consumed))
	} else {
		// copyErr == nil：恰好读了 maxSize+1 字节
		// 需要 peek 一字节确认 body 是否真结束了（CopyN 读满 N 返回 nil 而非 EOF）
		var peek [1]byte
		_, peekErr := body.Read(peek[:])
		if peekErr == io.EOF {
			// Body 恰好 maxSize+1 字节
			_ = body.Close()
			total = int64(len(consumed))
			restored = io.NopCloser(bytes.NewBuffer(consumed))
		} else if peekErr != nil {
			_ = body.Close()
			return nil, nil, 0, peekErr
		} else {
			// Body > maxSize+1：将 peek 的字节归位
			_ = head.WriteByte(peek[0]) // Grow 保证了容量
			consumed = head.Bytes()
			total = -1
			restored = &streamingRestored{
				head: bytes.NewReader(consumed),
				body: body,
			}
		}
	}

	// 阶段2：为日志创建独立副本（从此任何操作都不会影响 restored）
	logData := make([]byte, len(consumed))
	copy(logData, consumed)

	// 阶段3：在独立副本上进行日志处理
	if isBinaryContent(logData, contentType) {
		displaySize := total
		if displaySize < 0 {
			// 流式场景用已读大小作为下限
			displaySize = int64(len(logData))
		}
		truncated = formatBinaryBody(logData, contentType, displaySize, binaryHexPreview)
		return truncated, restored, total, nil
	}

	if len(logData) > maxSize {
		if total >= 0 {
			omitted := total - int64(maxSize)
			marker := fmt.Sprintf("... [truncated %d bytes omitted]", omitted)
			truncated = make([]byte, 0, maxSize+len(marker))
			truncated = append(truncated, logData[:maxSize]...)
			truncated = append(truncated, marker...)
		} else {
			marker := "... [truncated, remaining size unknown]"
			truncated = make([]byte, 0, maxSize+len(marker))
			truncated = append(truncated, logData[:maxSize]...)
			truncated = append(truncated, marker...)
		}
	} else {
		truncated = logData
	}

	return truncated, restored, total, nil
}

// streamingRestored 先回放已消费的 head 字节，再流式传递 body 剩余部分。
// 用于大 body 场景，避免将整个 body 加载到内存。
type streamingRestored struct {
	head   *bytes.Reader
	body   io.ReadCloser
	closed bool
}

func (r *streamingRestored) Read(p []byte) (int, error) {
	if r.head.Len() > 0 {
		return r.head.Read(p)
	}
	return r.body.Read(p)
}

func (r *streamingRestored) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.body.Close()
}

// RoundTrip 实现 http.RoundTripper 接口。
func (l *LogitRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	ctx := req.Context()
	requestID := generateRequestID()

	l.logger.Info(ctx, "http request started",
		logit.StringField(RequestIDKey, requestID),
		logit.StringField("method", req.Method),
		logit.StringField("url", req.URL.String()),
		logit.StringField("host", req.URL.Host),
		logit.StringField("path", req.URL.Path),
	)

	var reqBodyForLog []byte
	var reqBodySize int64
	if req.Body != nil {
		var err error
		var restored io.ReadCloser
		reqContentType := req.Header.Get("Content-Type")
		reqBodyForLog, restored, reqBodySize, err = readAndTruncateBody(req.Body, l.maxBodySize, reqContentType, l.binaryMaxHex)
		if err != nil {
			l.logger.Warn(ctx, "failed to read request body for logging",
				logit.StringField(RequestIDKey, requestID),
				logit.ErrorField("error", err),
			)
		}
		req.Body = restored
	}

	resp, err := l.next.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		l.logger.Error(ctx, err,
			logit.StringField(RequestIDKey, requestID),
			logit.StringField("method", req.Method),
			logit.StringField("url", req.URL.String()),
			logit.StringField("host", req.URL.Host),
			logit.StringField("path", req.URL.Path),
			logit.TimeDurationField("duration", duration),
		)
		return nil, err
	}

	var respBodyForLog []byte
	var respBodySize int64
	if resp.Body != nil {
		var err error
		var restored io.ReadCloser
		respContentType := resp.Header.Get("Content-Type")
		respBodyForLog, restored, respBodySize, err = readAndTruncateBody(resp.Body, l.maxBodySize, respContentType, l.binaryMaxHex)
		if err != nil {
			l.logger.Warn(ctx, "failed to read response body for logging",
				logit.StringField(RequestIDKey, requestID),
				logit.ErrorField("error", err),
			)
		}
		resp.Body = restored
	}

	l.logger.Info(ctx, "http request completed",
		logit.StringField(RequestIDKey, requestID),
		logit.StringField("method", req.Method),
		logit.StringField("url", req.URL.String()),
		logit.StringField("host", req.URL.Host),
		logit.StringField("path", req.URL.Path),
		logit.IntField("status", resp.StatusCode),
		logit.TimeDurationField("duration", duration),
		logit.Int64Field("request_body_size", reqBodySize),
		logit.Int64Field("response_body_size", respBodySize),
	)

	l.logger.Debug(ctx, "http request detail",
		logit.StringField(RequestIDKey, requestID),
		logit.StringField("method", req.Method),
		logit.StringField("url", req.URL.String()),
		logit.IntField("status", resp.StatusCode),
		logit.JSONAbsField("request_headers", req.Header),
		logit.JSONAbsField("response_headers", resp.Header),
		logit.StringField("request_body", string(reqBodyForLog)),
		logit.StringField("response_body", string(respBodyForLog)),
	)

	return resp, nil
}

// ═══════════════════════════════════════════════════════════════════
// RetryRoundTripper — HTTP 请求重试中间件
// ═══════════════════════════════════════════════════════════════════

const (
	// DefaultMaxRetries 默认最大重试次数
	DefaultMaxRetries = 3
	// DefaultBaseDelay 默认基础重试延迟
	DefaultBaseDelay = 100 * time.Millisecond
	// DefaultMaxDelay 默认最大重试延迟
	DefaultMaxDelay = 5 * time.Second
)

// RetryPolicy 定义重试策略，返回 true 表示应该重试。
type RetryPolicy func(resp *http.Response, err error) bool

// BackoffStrategy 定义退避策略，返回第 attempt 次重试应该等待的时间。
// attempt 从 1 开始（第一次重试）。
type BackoffStrategy func(attempt int) time.Duration

// RetryRoundTripper 提供 HTTP 请求重试功能。
type RetryRoundTripper struct {
	next         http.RoundTripper
	maxRetries   int
	backoff      BackoffStrategy
	retryPolicy  RetryPolicy
	retryMethods map[string]bool
}

// RetryOption 配置 RetryRoundTripper 的选项。
type RetryOption func(*RetryRoundTripper)

// RetryMaxRetries 设置最大重试次数。默认 3。
func RetryMaxRetries(n int) RetryOption {
	return func(r *RetryRoundTripper) {
		r.maxRetries = n
	}
}

// RetryBackoff 设置自定义退避策略。
func RetryBackoff(strategy BackoffStrategy) RetryOption {
	return func(r *RetryRoundTripper) {
		r.backoff = strategy
	}
}

// RetryExponentialBackoff 使用指数退避策略 + ±25% 抖动。
func RetryExponentialBackoff(baseDelay, maxDelay time.Duration) RetryOption {
	return func(r *RetryRoundTripper) {
		r.backoff = func(attempt int) time.Duration {
			delay := baseDelay * time.Duration(math.Pow(2, float64(attempt-1)))
			if delay > maxDelay {
				delay = maxDelay
			}
			jitter := int64(delay) / 4
			if jitter > 0 {
				n, _ := rand.Int(rand.Reader, big.NewInt(jitter*2))
				delay = delay - time.Duration(jitter) + time.Duration(n.Int64())
			}
			return delay
		}
	}
}

// RetryPolicyOption 设置自定义重试策略。
func RetryPolicyOption(policy RetryPolicy) RetryOption {
	return func(r *RetryRoundTripper) {
		r.retryPolicy = policy
	}
}

// RetryMethods 设置允许重试的 HTTP 方法。默认幂等方法。
func RetryMethods(methods ...string) RetryOption {
	return func(r *RetryRoundTripper) {
		r.retryMethods = make(map[string]bool)
		for _, m := range methods {
			r.retryMethods[m] = true
		}
	}
}

func defaultRetryPolicy(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
		return true
	}
	if resp.StatusCode == 429 {
		return true
	}
	return false
}

var defaultIdempotentMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodPut:     true,
	http.MethodDelete:  true,
	http.MethodOptions: true,
	http.MethodTrace:   true,
}

// NewRetryRoundTripper 创建重试中间件装饰器。
func NewRetryRoundTripper(opts ...RetryOption) func(http.RoundTripper) http.RoundTripper {
	return func(next http.RoundTripper) http.RoundTripper {
		rt := &RetryRoundTripper{
			next:         next,
			maxRetries:   DefaultMaxRetries,
			retryPolicy:  defaultRetryPolicy,
			retryMethods: defaultIdempotentMethods,
		}
		RetryExponentialBackoff(DefaultBaseDelay, DefaultMaxDelay)(rt)
		for _, opt := range opts {
			opt(rt)
		}
		return rt
	}
}

// NewRetry 是 NewRetryRoundTripper 的便捷别名。
func NewRetry(maxRetries int, opts ...RetryOption) func(http.RoundTripper) http.RoundTripper {
	allOpts := append([]RetryOption{RetryMaxRetries(maxRetries)}, opts...)
	return NewRetryRoundTripper(allOpts...)
}

func (r *RetryRoundTripper) shouldRetry(req *http.Request, resp *http.Response, err error) bool {
	if !r.retryMethods[req.Method] {
		return false
	}
	return r.retryPolicy(resp, err)
}

func drainBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

// RoundTrip 实现 http.RoundTripper 接口。
func (r *RetryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()

	var reqBody []byte
	if req.Body != nil {
		var err error
		reqBody, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
	}

	var lastErr error
	var lastResp *http.Response

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			delay := r.backoff(attempt)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				drainBody(lastResp)
				return nil, ctx.Err()
			}
		}

		if reqBody != nil {
			req.Body = io.NopCloser(bytes.NewBuffer(reqBody))
		}

		reqCopy := req.Clone(ctx)
		if reqBody != nil {
			reqCopy.Body = io.NopCloser(bytes.NewBuffer(reqBody))
		}

		resp, err := r.next.RoundTrip(reqCopy)

		if !r.shouldRetry(req, resp, err) {
			return resp, err
		}

		lastErr = err
		lastResp = resp
		drainBody(lastResp)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	if lastResp != nil {
		return lastResp, lastErr
	}
	return nil, lastErr
}
