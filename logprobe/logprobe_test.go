package logprobe

import (
	"context"
	"testing"

	"github.com/izliang/bore/appctx"

	"github.com/stretchr/testify/assert"
)

// ==========================================================================
// TimeRange 测试
// ==========================================================================

// 零值 TimeRange 应判定为完全未设置。
func TestTimeRange_IsZero(t *testing.T) {
	assert.True(t, TimeRange{}.IsZero())
}

// ==========================================================================
// Cursor 测试
// ==========================================================================

// 零值 Cursor（Backend 为空）表示从未消费过数据。
func TestCursor_IsZero(t *testing.T) {
	assert.True(t, Cursor{}.IsZero())
	assert.False(t, Cursor{Backend: "ck"}.IsZero())
}

// ==========================================================================
// FieldValue 测试
// ==========================================================================

// 字段长度小于截断阈值时不截断，Expand 直接返回原值。
func TestNewFieldValue_NoTruncation(t *testing.T) {
	fv := NewFieldValue("short", 500)
	assert.Equal(t, "short", fv.Text)
	assert.False(t, fv.Truncated)
	assert.Equal(t, 5, fv.FullLength)

	val, err := fv.Expand(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "short", val)
}

// 字段长度超过截断阈值时应截断，Expand 返回完整值（闭包持有）。
func TestNewFieldValue_Truncated(t *testing.T) {
	fv := NewFieldValue("abcdefghij", 5)
	assert.Equal(t, "abcde", fv.Text)
	assert.True(t, fv.Truncated)
	assert.Equal(t, 10, fv.FullLength)

	val, err := fv.Expand(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "abcdefghij", val)
}

// maxLen=0 表示不截断，应返回完整值。
func TestNewFieldValue_MaxLenZero(t *testing.T) {
	fv := NewFieldValue("long value", 0)
	assert.False(t, fv.Truncated)
	assert.Equal(t, "long value", fv.Text)
}

// 惰性 FieldValue：Expand 时才调用 fn 获取完整值。
func TestNewLazyFieldValue(t *testing.T) {
	fv := NewLazyFieldValue("prefix", 100, func(ctx context.Context) (string, error) {
		return "prefix_full_value", nil
	})
	assert.Equal(t, "prefix", fv.Text)
	assert.True(t, fv.Truncated)
	assert.Equal(t, 100, fv.FullLength)

	val, err := fv.Expand(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "prefix_full_value", val)
}

// 截断状态但无 expander → Expand 降级返回前缀。
// 模拟后端无法惰性获取完整值的场景。
func TestFieldValue_Expand_NoExpander(t *testing.T) {
	fv := FieldValue{Text: "abc", Truncated: true, FullLength: 1000}
	val, err := fv.Expand(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "abc", val) // 降级返回前缀
}

// ==========================================================================
// FieldInfo 测试
// ==========================================================================

// HasRole 应正确匹配角色标签。
func TestFieldInfo_HasRole(t *testing.T) {
	fi := FieldInfo{Name: "trace_id", Roles: []string{"trace_id", "string"}}
	assert.True(t, fi.HasRole("trace_id"))
	assert.False(t, fi.HasRole("timestamp"))
}

// ==========================================================================
// QueryError 测试
// ==========================================================================

// QueryError 应实现 error 接口，Error() 返回 Message。
func TestQueryError_Error(t *testing.T) {
	e := &QueryError{Message: "查询范围过大", Retryable: true}
	assert.Equal(t, "查询范围过大", e.Error())
}

// ==========================================================================
// Registry 测试
// ==========================================================================

// Register 后 Get 应能取回同一数据源。
func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	src := &stubSource{id: "test"}
	reg.Register(src)

	got, ok := reg.Get("test")
	assert.True(t, ok)
	assert.Equal(t, "test", got.ID())

	// 未注册的 ID 返回 false。
	_, ok = reg.Get("nonexistent")
	assert.False(t, ok)
}

// 重复注册相同 ID 应覆盖旧值。
func TestRegistry_Reregister(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubSource{id: "test"})
	reg.Register(&stubSource{id: "test"})
	_, ok := reg.Get("test")
	assert.True(t, ok)
}

// ==========================================================================
// 工具定义测试
// ==========================================================================

// OpenAITools 应返回 7 个工具定义，所有必填字段不为空。
func TestOpenAITools(t *testing.T) {
	tools := OpenAITools()
	assert.Len(t, tools, 7)

	names := make(map[string]bool)
	for _, tool := range tools {
		assert.Equal(t, "function", tool.Type)
		assert.NotEmpty(t, tool.Function.Name)
		assert.NotEmpty(t, tool.Function.Description)
		names[tool.Function.Name] = true
	}
	assert.True(t, names["logprobe_describe"])
	assert.True(t, names["logprobe_search"])
	assert.True(t, names["logprobe_stats"])
	assert.True(t, names["logprobe_context"])
	assert.True(t, names["logprobe_stream"])
	assert.True(t, names["logprobe_expand"])
	assert.True(t, names["logprobe_trace"])
}

// ==========================================================================
// stubSource — 接口契约测试桩
// ==========================================================================

// stubSource 实现 LogSource 接口的所有方法（均返回 nil/零值）。
// 用于编译时验证接口契约和单元测试。
type stubSource struct {
	id string
}

func (s *stubSource) ID() string { return s.id }

func (s *stubSource) Info(atx appctx.AppContext) (*SourceInfo, error) { return nil, nil }

func (s *stubSource) Search(atx appctx.AppContext, req SearchRequest) (*SearchResult, error) {
	return nil, nil
}
func (s *stubSource) Stats(atx appctx.AppContext, req StatsRequest) (*StatsResult, error) {
	return nil, nil
}
func (s *stubSource) Context(atx appctx.AppContext, req ContextRequest) (*SearchResult, error) {
	return nil, nil
}
func (s *stubSource) Stream(atx appctx.AppContext, req StreamRequest) (*LogCursor, error) {
	return nil, nil
}
func (s *stubSource) Cursor() Cursor     { return Cursor{} }
func (s *stubSource) SetCursor(c Cursor) {}
func (s *stubSource) Expand(atx appctx.AppContext, targets []ExpandTarget) ([]ExpandResult, error) {
	return nil, nil
}

// 编译时断言：确保 stubSource 实现了 LogSource 接口。
var _ LogSource = (*stubSource)(nil)
