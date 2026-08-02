package otel_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/tracing/otel"
)

// ── 测试辅助 ──────────────────────────────────────────────

// memExporter 内存导出器，收集 Span 用于断言。
type memExporter struct {
	mu    sync.Mutex
	spans []sdktrace.ReadOnlySpan
}

func (e *memExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.spans = append(e.spans, spans...)

	// 调试输出：打印 Span 关键字段
	infos := make([]spanInfo, len(spans))
	for i, s := range spans {
		infos[i] = toSpanInfo(s)
	}
	logger := logit.NewSimple(os.Stdout)
	logger.Info(ctx, "ExportSpans", logit.JSONDetailAbsField("spans", infos))
	return nil
}

func (e *memExporter) Shutdown(_ context.Context) error { return nil }

// find 按名称查找导出的 Span，不存在返回 nil。
func (e *memExporter) find(name string) sdktrace.ReadOnlySpan {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.spans {
		if s.Name() == name {
			return s
		}
	}
	return nil
}

// ── spanInfo：ReadOnlySpan → 可序列化结构体 ─────────────────

// spanInfo 提取 sdktrace.ReadOnlySpan 的公开方法返回值，
// 构建为可打印的结构体，解决底层字段未导出导致 JSON 序列化为 {} 的问题。
type spanInfo struct {
	Name     string            `json:"name"`
	TraceID  string            `json:"trace_id"`
	SpanID   string            `json:"span_id"`
	ParentID string            `json:"parent_id,omitempty"`
	Kind     string            `json:"kind"`
	Attrs    map[string]string `json:"attrs,omitempty"`
	Events   []string          `json:"events,omitempty"`
	Status   string            `json:"status"`
}

func toSpanInfo(s sdktrace.ReadOnlySpan) spanInfo {
	info := spanInfo{
		Name:    s.Name(),
		TraceID: s.SpanContext().TraceID().String(),
		SpanID:  s.SpanContext().SpanID().String(),
		Kind:    s.SpanKind().String(),
		Status:  fmt.Sprintf("%d:%s", s.Status().Code, s.Status().Description),
	}

	if s.Parent().IsValid() {
		info.ParentID = s.Parent().SpanID().String()
	}

	if attrs := s.Attributes(); len(attrs) > 0 {
		info.Attrs = make(map[string]string, len(attrs))
		for _, kv := range attrs {
			info.Attrs[string(kv.Key)] = kv.Value.Emit()
		}
	}

	if events := s.Events(); len(events) > 0 {
		info.Events = make([]string, len(events))
		for i, ev := range events {
			info.Events[i] = ev.Name
		}
	}

	return info
}

// attrMap 将 []attribute.KeyValue 转为 map，方便断言。
func attrMap(kvs []attribute.KeyValue) map[string]string {
	m := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		m[string(kv.Key)] = kv.Value.Emit()
	}
	return m
}

// newProvider 创建测试用 Provider，返回 exporter 以便断言。
func newProvider(t *testing.T) (*otel.Provider, *memExporter) {
	t.Helper()
	exp := &memExporter{}
	p, err := otel.NewProvider(
		otel.WithServiceName("test-service"),
		otel.WithExporter(exp),
	)
	if err != nil {
		t.Fatalf("newProvider: %v", err)
	}
	return p, exp
}

// ── Provider 校验 ─────────────────────────────────────────

func TestProviderValidation(t *testing.T) {
	tests := []struct {
		name    string
		opts    []otel.Option
		wantErr string
	}{
		{
			name:    "no options",
			opts:    nil,
			wantErr: "WithServiceName",
		},
		{
			name:    "service name only",
			opts:    []otel.Option{otel.WithServiceName("test")},
			wantErr: "WithExporter",
		},
		{
			name:    "exporter only",
			opts:    []otel.Option{otel.WithExporter(&memExporter{})},
			wantErr: "WithServiceName",
		},
		{
			name: "both provided",
			opts: []otel.Option{
				otel.WithServiceName("test"),
				otel.WithExporter(&memExporter{}),
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := otel.NewProvider(tt.opts...)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

// ── 单 Span：属性写入 ──────────────────────────────────────

func TestSingleSpan(t *testing.T) {
	provider, exp := newProvider(t)
	defer provider.Shutdown(context.Background())

	tracer := provider.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "my-span")
	span.SetAttributes(
		logit.StringField("key1", "val1"),
		logit.StringField("key2", "val2"),
	)
	span.End()
	_ = ctx

	_ = provider.Shutdown(context.Background()) // flush

	got := exp.find("my-span")
	if got == nil {
		t.Fatal("span not exported")
	}

	attrs := attrMap(got.Attributes())
	if attrs["key1"] != "val1" {
		t.Errorf("key1 = %q, want %q", attrs["key1"], "val1")
	}
	if attrs["key2"] != "val2" {
		t.Errorf("key2 = %q, want %q", attrs["key2"], "val2")
	}
}

// ── 属性类型：表格驱动 ─────────────────────────────────────

func TestSpanAttributesTable(t *testing.T) {
	tests := []struct {
		name     string
		fields   []logit.FieldFunc
		wantAttr map[string]string
	}{
		{
			name: "string fields",
			fields: []logit.FieldFunc{
				logit.StringField("user.id", "42"),
				logit.StringField("action", "login"),
			},
			wantAttr: map[string]string{"user.id": "42", "action": "login"},
		},
		{
			name: "int fields",
			fields: []logit.FieldFunc{
				logit.IntField("http.status", 200),
				logit.IntField("retry.count", 3),
			},
			wantAttr: map[string]string{"http.status": "200", "retry.count": "3"},
		},
		{
			name:     "empty attributes",
			fields:   nil,
			wantAttr: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, exp := newProvider(t)
			defer provider.Shutdown(context.Background())

			tracer := provider.Tracer("test")
			_, span := tracer.Start(context.Background(), tt.name)
			span.SetAttributes(tt.fields...)
			span.End()

			_ = provider.Shutdown(context.Background())

			got := exp.find(tt.name)
			if got == nil {
				t.Fatal("span not exported")
			}
			gotAttr := attrMap(got.Attributes())
			for k, want := range tt.wantAttr {
				if gotAttr[k] != want {
					t.Errorf("attr %q = %q, want %q", k, gotAttr[k], want)
				}
			}
		})
	}
}

// ── 父子嵌套 ──────────────────────────────────────────────

func TestSpanNesting(t *testing.T) {
	provider, exp := newProvider(t)
	defer provider.Shutdown(context.Background())

	tracer := provider.Tracer("test")

	ctx, parent := tracer.Start(context.Background(), "parent")
	_, child := tracer.Start(ctx, "child")
	child.End()
	parent.End()

	_ = provider.Shutdown(context.Background())

	ps := exp.find("parent")
	cs := exp.find("child")
	if ps == nil || cs == nil {
		t.Fatal("spans not exported")
	}

	if ps.SpanContext().TraceID() != cs.SpanContext().TraceID() {
		t.Error("parent and child should share same TraceID")
	}
	if cs.Parent().SpanID() != ps.SpanContext().SpanID() {
		t.Error("child parent should match parent span ID")
	}
}

// ── 错误记录 ──────────────────────────────────────────────

func TestRecordError(t *testing.T) {
	provider, exp := newProvider(t)
	defer provider.Shutdown(context.Background())

	tracer := provider.Tracer("test")

	_, span := tracer.Start(context.Background(), "failing-span")
	span.RecordError(fmt.Errorf("something went wrong"))
	span.End()

	_ = provider.Shutdown(context.Background())

	got := exp.find("failing-span")
	if got == nil {
		t.Fatal("span not exported")
	}
	if len(got.Events()) == 0 && got.Status().Code != 2 {
		t.Error("expected error event or error status, got none")
	}
}

// ── 端到端演示：HTTP 请求链路 ──────────────────────────────

func TestDemoHTTPRequestTrace(t *testing.T) {
	provider, exp := newProvider(t)
	defer provider.Shutdown(context.Background())

	httpTracer := provider.Tracer("http")
	dbTracer := provider.Tracer("db")

	ctx, root := httpTracer.Start(context.Background(), "GET /api/users")
	root.SetAttributes(logit.StringField("http.method", "GET"))

	ctx, auth := httpTracer.Start(ctx, "Authenticate")
	auth.SetAttributes(logit.StringField("user.id", "42"))
	auth.End()

	ctx, dbSpan := dbTracer.Start(ctx, "QueryUsers")
	dbSpan.SetAttributes(
		logit.StringField("db.table", "users"),
		logit.IntField("db.rows", 10),
	)
	dbSpan.End()

	_, cache := dbTracer.Start(ctx, "ReadCache")
	cache.SetAttributes(logit.StringField("cache.key", "userlist:42"))
	cache.End()

	root.End()
	_ = provider.Shutdown(context.Background())

	wantSpans := []string{
		"GET /api/users", "Authenticate", "QueryUsers", "ReadCache",
	}
	for _, name := range wantSpans {
		if exp.find(name) == nil {
			t.Errorf("span %q not exported", name)
		}
	}

	rootSpan := exp.find("GET /api/users")
	traceID := rootSpan.SpanContext().TraceID()
	for _, name := range wantSpans[1:] {
		s := exp.find(name)
		if s.SpanContext().TraceID() != traceID {
			t.Errorf("span %q has different TraceID from root", name)
		}
	}
}

// ── 多 Span 并发安全 ───────────────────────────────────────

func TestConcurrentSpans(t *testing.T) {
	provider, exp := newProvider(t)
	defer provider.Shutdown(context.Background())

	tracer := provider.Tracer("test")

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("concurrent-%d", id)
			_, span := tracer.Start(context.Background(), name)
			span.SetAttributes(logit.IntField("id", id))
			span.End()
		}(i)
	}

	wg.Wait()
	_ = provider.Shutdown(context.Background())

	for i := 0; i < n; i++ {
		name := fmt.Sprintf("concurrent-%d", i)
		if exp.find(name) == nil {
			t.Errorf("span %q not exported", name)
		}
	}
}

// ── Scope 隔离 ─────────────────────────────────────────────

func TestScopeIsolation(t *testing.T) {
	provider, exp := newProvider(t)
	defer provider.Shutdown(context.Background())

	a := provider.Tracer("scope-a")
	b := provider.Tracer("scope-b")

	_, s1 := a.Start(context.Background(), "span-a")
	s1.End()

	_, s2 := b.Start(context.Background(), "span-b")
	s2.End()

	_ = provider.Shutdown(context.Background())

	if exp.find("span-a") == nil || exp.find("span-b") == nil {
		t.Error("spans from different scopes should both be exported")
	}
}

// ── TraceID ───────────────────────────────────────────────

func TestTraceID(t *testing.T) {
	tid := &otel.TraceID{}

	t.Run("no span in context", func(t *testing.T) {
		got := tid.TraceID(context.Background())
		if got != "-" {
			t.Errorf("expected '-', got %q", got)
		}
	})

	t.Run("valid span", func(t *testing.T) {
		provider, exp := newProvider(t)
		defer provider.Shutdown(context.Background())

		tracer := provider.Tracer("test")
		ctx, span := tracer.Start(context.Background(), "test-span")
		span.End()

		got := tid.TraceID(ctx)
		if got == "-" {
			t.Fatal("expected valid traceID, got '-'")
		}

		// 验证格式: traceID-spanID (两个 hex 字符串，用 - 连接)
		parts := strings.SplitN(got, "-", 2)
		if len(parts) != 2 {
			t.Fatalf("expected 'traceID-spanID' format, got %q", got)
		}
		if len(parts[0]) != 32 || len(parts[1]) != 16 {
			t.Errorf("expected 32-hex traceID and 16-hex spanID, got %s-%s", parts[0], parts[1])
		}

		// 验证与导出 Span 一致
		_ = provider.Shutdown(context.Background())
		exported := exp.find("test-span")
		if exported == nil {
			t.Fatal("span not exported")
		}
		want := exported.SpanContext().TraceID().String() + "-" + exported.SpanContext().SpanID().String()
		if got != want {
			t.Errorf("TraceID = %q, want %q", got, want)
		}
	})

	t.Run("nested spans return leaf spanID", func(t *testing.T) {
		provider, _ := newProvider(t)
		defer provider.Shutdown(context.Background())

		tracer := provider.Tracer("test")

		ctx, parent := tracer.Start(context.Background(), "parent")
		parentTrace := tid.TraceID(ctx)

		ctx2, child := tracer.Start(ctx, "child")
		childTrace := tid.TraceID(ctx2)

		child.End()
		parent.End()

		// 父子 traceID 前缀相同，spanID 不同
		parentParts := strings.SplitN(parentTrace, "-", 2)
		childParts := strings.SplitN(childTrace, "-", 2)
		if parentParts[0] != childParts[0] {
			t.Error("parent and child should share same traceID")
		}
		if parentParts[1] == childParts[1] {
			t.Error("parent and child should have different spanID")
		}
	})
}

// TestTraceIDInLog 验证 logit 日志行中正确输出 traceID-spanID。
func TestTraceIDInLog(t *testing.T) {
	var buf bytes.Buffer
	logger := logit.NewSimpleWithConfig(&buf, &logit.Config{
		TraceID: &otel.TraceID{},
	})

	provider, _ := newProvider(t)
	defer provider.Shutdown(context.Background())

	tracer := provider.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "my-span")

	logger.Info(ctx, "hello world", logit.StringField("key", "val"))
	span.End()
	_ = provider.Shutdown(context.Background())

	out := buf.String()
	t.Logf("log output:\n%s", out)

	// 验证 traceID 出现在 [traceID] 槽位
	tid := &otel.TraceID{}
	wantTrace := tid.TraceID(ctx)
	if !strings.Contains(out, "["+wantTrace+"]") {
		t.Errorf("log should contain [%s], got:\n%s", wantTrace, out)
	}

	// 验证消息和字段
	if !strings.Contains(out, "msg[hello world]") {
		t.Error("log should contain msg[hello world]")
	}
	if !strings.Contains(out, "key[val]") {
		t.Error("log should contain key[val]")
	}
}
