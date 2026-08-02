package core_test

import (
	"fmt"
	"iter"
	"testing"

	core "github.com/izliang/bore/ds/core"
)

// =============================================================================
// Option 测试
// =============================================================================

func TestOptionSome(t *testing.T) {
	tests := []struct {
		name  string
		value int
	}{
		{name: "正整数", value: 42},
		{name: "零值", value: 0},
		{name: "负整数", value: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opt := core.Some(tt.value)
			got, ok := opt.Get()
			if !ok {
				t.Fatal("Some 创建的 Option.Get() 应返回 ok=true")
			}
			if got != tt.value {
				t.Errorf("Some(%v).Get() = %v, 期望 %v", tt.value, got, tt.value)
			}
			if !opt.IsPresent() {
				t.Error("Some 创建的 Option.IsPresent() 应为 true")
			}
			if opt.MustGet() != tt.value {
				t.Errorf("Some(%v).MustGet() = %v, 期望 %v", tt.value, opt.MustGet(), tt.value)
			}
		})
	}
}

func TestOptionNone(t *testing.T) {
	opt := core.None[int]()

	if opt.IsPresent() {
		t.Error("None Option.IsPresent() 应为 false")
	}

	_, ok := opt.Get()
	if ok {
		t.Error("None Option.Get() 应返回 ok=false")
	}
}

func TestOptionMustGetPanic(t *testing.T) {
	t.Run("None 调用 MustGet 应 panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("None.MustGet() 应触发 panic")
			}
		}()
		core.None[int]().MustGet()
	})

	t.Run("Some 调用 MustGet 不应 panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Some.MustGet() 不应触发 panic，但得到了: %v", r)
			}
		}()
		if core.Some(1).MustGet() != 1 {
			t.Error("Some(1).MustGet() 应返回 1")
		}
	})
}

func TestOptionOrElse(t *testing.T) {
	tests := []struct {
		name     string
		opt      core.Option[int]
		fallback int
		want     int
	}{
		{name: "Some 忽略 fallback", opt: core.Some(10), fallback: 99, want: 10},
		{name: "None 返回 fallback", opt: core.None[int](), fallback: 99, want: 99},
		{name: "None 零值 fallback", opt: core.None[int](), fallback: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opt.OrElse(tt.fallback)
			if got != tt.want {
				t.Errorf("OrElse(%v) = %v, 期望 %v", tt.fallback, got, tt.want)
			}
		})
	}
}

func TestOptionString(t *testing.T) {
	tests := []struct {
		name string
		opt  core.Option[int]
		want string
	}{
		{name: "Some 输出", opt: core.Some(5), want: "Some(5)"},
		{name: "None 输出", opt: core.None[int](), want: "None"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opt.String()
			if got != tt.want {
				t.Errorf("String() = %q, 期望 %q", got, tt.want)
			}
		})
	}
}

func TestOptionStringType(t *testing.T) {
	t.Run("字符串类型", func(t *testing.T) {
		opt := core.Some("hello")
		if got := opt.String(); got != "Some(hello)" {
			t.Errorf("String() = %q, 期望 %q", got, "Some(hello)")
		}
	})
	t.Run("浮点类型", func(t *testing.T) {
		opt := core.Some(3.14)
		if got := opt.String(); got != "Some(3.14)" {
			t.Errorf("String() = %q, 期望 %q", got, "Some(3.14)")
		}
	})
}

// =============================================================================
// Order 测试
// =============================================================================

func TestOrderString(t *testing.T) {
	tests := []struct {
		name  string
		order core.Order
		want  string
	}{
		{name: "PreOrder", order: core.OrderPre, want: "PreOrder"},
		{name: "InOrder", order: core.OrderIn, want: "InOrder"},
		{name: "PostOrder", order: core.OrderPost, want: "PostOrder"},
		{name: "LevelOrder", order: core.OrderLevel, want: "LevelOrder"},
		{name: "未知值", order: core.Order(99), want: "Unknown"},
		{name: "负数", order: core.Order(-1), want: "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.order.String()
			if got != tt.want {
				t.Errorf("Order(%d).String() = %q, 期望 %q", tt.order, got, tt.want)
			}
		})
	}
}

func TestOrderIotaValues(t *testing.T) {
	tests := []struct {
		name  string
		order core.Order
		want  int
	}{
		{name: "OrderPre=0", order: core.OrderPre, want: 0},
		{name: "OrderIn=1", order: core.OrderIn, want: 1},
		{name: "OrderPost=2", order: core.OrderPost, want: 2},
		{name: "OrderLevel=3", order: core.OrderLevel, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int(tt.order) != tt.want {
				t.Errorf("int(Order) = %d, 期望 %d", int(tt.order), tt.want)
			}
		})
	}
}

// =============================================================================
// 哨兵错误测试
// =============================================================================

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "ErrEmpty", err: core.ErrEmpty},
		{name: "ErrNotFound", err: core.ErrNotFound},
		{name: "ErrInvalidInput", err: core.ErrInvalidInput},
		{name: "ErrOutOfBounds", err: core.ErrOutOfBounds},
		{name: "ErrNotSupported", err: core.ErrNotSupported},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatal("哨兵错误不应为 nil")
			}
			if tt.err.Error() == "" {
				t.Error("哨兵错误的 Error() 不应返回空字符串")
			}
		})
	}
}

func TestSentinelErrorsAreDistinct(t *testing.T) {
	errors := []error{
		core.ErrEmpty,
		core.ErrNotFound,
		core.ErrInvalidInput,
		core.ErrOutOfBounds,
		core.ErrNotSupported,
	}

	for i, a := range errors {
		for j, b := range errors {
			if i != j && a.Error() == b.Error() {
				t.Errorf("哨兵错误 %q 和 %q 的 Error() 完全相同", a, b)
			}
		}
	}
}

// =============================================================================
// 接口满足性验证（编译期检查）
// =============================================================================

// 测试桩结构体，用于验证接口满足关系。

type sliceCollection[T any] struct {
	data []T
}

func (c *sliceCollection[T]) Len() int             { return len(c.data) }
func (c *sliceCollection[T]) IsEmpty() bool        { return len(c.data) == 0 }
func (c *sliceCollection[T]) Contains(item T) bool { return c.indexOf(item) >= 0 }
func (c *sliceCollection[T]) Add(item T) bool      { c.data = append(c.data, item); return true }
func (c *sliceCollection[T]) Remove(item T) bool   { return false }
func (c *sliceCollection[T]) Clear()               { c.data = nil }
func (c *sliceCollection[T]) Clone() core.Collection[T] {
	return &sliceCollection[T]{data: append([]T(nil), c.data...)}
}
func (c *sliceCollection[T]) Equal(other core.Collection[T]) bool { return false }
func (c *sliceCollection[T]) Iterator() core.Iterator[T]          { return &sliceIterator[T]{data: c.data} }
func (c *sliceCollection[T]) Adjacent(from T) iter.Seq[T]         { return func(yield func(T) bool) {} }

func (c *sliceCollection[T]) indexOf(item T) int {
	for i, v := range c.data {
		if fmt.Sprint(v) == fmt.Sprint(item) {
			return i
		}
	}
	return -1
}

type sliceIterator[T any] struct {
	data []T
	pos  int
}

func (it *sliceIterator[T]) HasNext() bool { return it.pos < len(it.data) }
func (it *sliceIterator[T]) Next() (T, bool) {
	if it.pos >= len(it.data) {
		var zero T
		return zero, false
	}
	v := it.data[it.pos]
	it.pos++
	return v, true
}

// 编译期接口满足性验证。
var (
	_ core.Collection[int]  = (*sliceCollection[int])(nil)
	_ core.Iterable[int]    = (*sliceCollection[int])(nil)
	_ core.Searchable[int]  = (*sliceCollection[int])(nil)
	_ core.Mutable[int]     = (*sliceCollection[int])(nil)
	_ core.Cloneable[int]   = (*sliceCollection[int])(nil)
	_ core.Comparable[int]  = (*sliceCollection[int])(nil)
	_ core.Traversable[int] = (*sliceCollection[int])(nil)
)

// =============================================================================
// Collection 接口行为测试（通过测试桩验证）
// =============================================================================

func TestCollectionLenAndIsEmpty(t *testing.T) {
	tests := []struct {
		name      string
		data      []int
		wantLen   int
		wantEmpty bool
	}{
		{name: "空集合", data: []int{}, wantLen: 0, wantEmpty: true},
		{name: "单元素", data: []int{1}, wantLen: 1, wantEmpty: false},
		{name: "多元素", data: []int{1, 2, 3}, wantLen: 3, wantEmpty: false},
		{name: "nil data", data: nil, wantLen: 0, wantEmpty: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c core.Collection[int] = &sliceCollection[int]{data: tt.data}
			if got := c.Len(); got != tt.wantLen {
				t.Errorf("Len() = %d, 期望 %d", got, tt.wantLen)
			}
			if got := c.IsEmpty(); got != tt.wantEmpty {
				t.Errorf("IsEmpty() = %v, 期望 %v", got, tt.wantEmpty)
			}
		})
	}
}

func TestCollectionLenIsEmptyConsistency(t *testing.T) {
	for size := 0; size <= 10; size++ {
		data := make([]int, size)
		c := &sliceCollection[int]{data: data}
		if c.Len() == 0 != c.IsEmpty() {
			t.Errorf("不一致: Len()=%d, IsEmpty()=%v（数据量=%d）", c.Len(), c.IsEmpty(), size)
		}
	}
}

// =============================================================================
// Iterator 测试
// =============================================================================

func TestIterator(t *testing.T) {
	tests := []struct {
		name     string
		data     []int
		wantVals []int
	}{
		{name: "空迭代器", data: []int{}, wantVals: nil},
		{name: "单元素", data: []int{42}, wantVals: []int{42}},
		{name: "多元素", data: []int{1, 2, 3, 4, 5}, wantVals: []int{1, 2, 3, 4, 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := &sliceIterator[int]{data: tt.data}
			var got []int
			for it.HasNext() {
				v, ok := it.Next()
				if !ok {
					t.Fatalf("Next() 返回 ok=false，但 HasNext() 为 true（位置 %d）", len(got))
				}
				got = append(got, v)
			}
			if len(got) != len(tt.wantVals) {
				t.Errorf("遍历得到 %d 个元素，期望 %d 个", len(got), len(tt.wantVals))
				return
			}
			for i, v := range got {
				if v != tt.wantVals[i] {
					t.Errorf("位置 %d: got %d, want %d", i, v, tt.wantVals[i])
				}
			}
		})
	}
}

func TestIteratorExhausted(t *testing.T) {
	it := &sliceIterator[int]{data: []int{1}}

	// 正常取值
	v, ok := it.Next()
	if !ok || v != 1 {
		t.Fatalf("第一次 Next() = (%d, %v), 期望 (1, true)", v, ok)
	}

	// 已耗尽
	if it.HasNext() {
		t.Error("迭代器已耗尽，HasNext() 应为 false")
	}

	v2, ok2 := it.Next()
	if ok2 {
		t.Errorf("耗尽后 Next() = (%d, %v), 期望 ok=false", v2, ok2)
	}

	var zero int
	if v2 != zero {
		t.Errorf("耗尽后 Next() 的值应为零值, 得到 %v", v2)
	}
}

func TestIteratorMultipleExhaustion(t *testing.T) {
	it := &sliceIterator[int]{data: []int{}}
	for i := 0; i < 5; i++ {
		if it.HasNext() {
			t.Errorf("第 %d 次: 空迭代器 HasNext() 应为 false", i)
		}
		if _, ok := it.Next(); ok {
			t.Errorf("第 %d 次: 空迭代器 Next() 应返回 ok=false", i)
		}
	}
}

// =============================================================================
// Iterable 测试
// =============================================================================

func TestIterable(t *testing.T) {
	tests := []struct {
		name      string
		data      []string
		wantEmpty bool
		wantLen   int
	}{
		{name: "空", data: nil, wantEmpty: true, wantLen: 0},
		{name: "单元素", data: []string{"a"}, wantEmpty: false, wantLen: 1},
		{name: "多元素", data: []string{"x", "y", "z"}, wantEmpty: false, wantLen: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &sliceCollection[string]{data: tt.data}
			if c.IsEmpty() != tt.wantEmpty {
				t.Errorf("IsEmpty() = %v, 期望 %v", c.IsEmpty(), tt.wantEmpty)
			}
			if c.Len() != tt.wantLen {
				t.Errorf("Len() = %d, 期望 %d", c.Len(), tt.wantLen)
			}
			it := c.Iterator()
			count := 0
			for it.HasNext() {
				_, ok := it.Next()
				if !ok {
					t.Fatal("Iterator.Next() 在 HasNext() 为 true 时返回 ok=false")
				}
				count++
			}
			if count != tt.wantLen {
				t.Errorf("Iterator 遍历得到 %d 个元素, 期望 %d", count, tt.wantLen)
			}
		})
	}
}

// =============================================================================
// Mutable 测试
// =============================================================================

func TestMutableAdd(t *testing.T) {
	c := &sliceCollection[int]{}
	startLen := c.Len()

	if !c.Add(1) {
		t.Error("Add(1) 应返回 true")
	}
	if c.Len() != startLen+1 {
		t.Errorf("Add 后 Len() = %d, 期望 %d", c.Len(), startLen+1)
	}
	if c.IsEmpty() {
		t.Error("Add 后 IsEmpty() 应为 false")
	}
}

func TestMutableAddMultiple(t *testing.T) {
	c := &sliceCollection[int]{}
	items := []int{10, 20, 30, 40, 50}

	for i, item := range items {
		if !c.Add(item) {
			t.Errorf("第 %d 次 Add(%d) 应返回 true", i, item)
		}
	}
	if c.Len() != len(items) {
		t.Errorf("最终 Len() = %d, 期望 %d", c.Len(), len(items))
	}
}

func TestMutableClear(t *testing.T) {
	c := &sliceCollection[int]{data: []int{1, 2, 3}}
	if c.Len() == 0 {
		t.Fatal("初始集合不应为空")
	}
	c.Clear()
	if c.Len() != 0 {
		t.Errorf("Clear 后 Len() = %d, 期望 0", c.Len())
	}
	if !c.IsEmpty() {
		t.Error("Clear 后 IsEmpty() 应为 true")
	}
	c.Clear() // 重复 Clear 不应 panic
}

func TestMutableRemove(t *testing.T) {
	c := &sliceCollection[int]{data: []int{1, 2, 3}}
	removed := c.Remove(2)
	if !removed {
		t.Log("Remove 返回 false（测试桩不支持 Remove，这是预期行为）")
	}
}

// =============================================================================
// Cloneable 测试
// =============================================================================

func TestCloneable(t *testing.T) {
	original := &sliceCollection[int]{data: []int{1, 2, 3}}
	cloned := original.Clone()

	if cloned.Len() != original.Len() {
		t.Errorf("克隆后 Len() = %d, 期望 %d", cloned.Len(), original.Len())
	}
	if cloned.IsEmpty() != original.IsEmpty() {
		t.Error("克隆后的 IsEmpty() 应与原集合一致")
	}

	// 验证深拷贝：修改原集合不应影响克隆
	original.Add(99)
	if cloned.Len() != 3 {
		t.Errorf("修改原集合后克隆的 Len() = %d, 期望 3（应为深拷贝）", cloned.Len())
	}
}

// =============================================================================
// Comparable 测试
// =============================================================================

func TestComparable(t *testing.T) {
	a := &sliceCollection[int]{data: []int{1, 2}}
	b := &sliceCollection[int]{data: []int{1, 2}}

	if a.Equal(b) {
		t.Log("Equal 返回 true（取决于实现，此处仅验证不 panic）")
	}
	if a.Equal(a) {
		t.Log("自身 Equal 自身返回 true（取决于实现）")
	}
}

// =============================================================================
// Traversable 测试
// =============================================================================

func TestTraversable(t *testing.T) {
	c := &sliceCollection[int]{data: []int{1, 2, 3}}

	count := 0
	for range c.Adjacent(1) {
		count++
	}
	if count != 0 {
		t.Logf("Adjacent(1) 产生 %d 个元素（测试桩返回空 seq）", count)
	}
}

// =============================================================================
// Weighted 测试
// =============================================================================

type weightedValue struct {
	value int
	w     float64
}

func (w weightedValue) Weight() float64 { return w.w }

func TestWeighted(t *testing.T) {
	tests := []struct {
		name   string
		entity core.Weighted
		want   float64
	}{
		{name: "正权重", entity: weightedValue{w: 5.5}, want: 5.5},
		{name: "零权重", entity: weightedValue{w: 0}, want: 0},
		{name: "负权重", entity: weightedValue{w: -2.0}, want: -2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entity.Weight()
			if got != tt.want {
				t.Errorf("Weight() = %v, 期望 %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Visitor 类型测试
// =============================================================================

func TestVisitor(t *testing.T) {
	t.Run("继续遍历", func(t *testing.T) {
		var v core.Visitor[int] = func(n int) bool { return n < 10 }
		if !v(5) {
			t.Error("Visitor(5) 应返回 true（继续遍历）")
		}
		if v(10) {
			t.Error("Visitor(10) 应返回 false（停止遍历）")
		}
	})

	t.Run("nil Visitor 行为", func(t *testing.T) {
		var v core.Visitor[int]
		if v != nil {
			_ = v(1) // 仅验证类型兼容性
		}
	})
}

// =============================================================================
// 接口组合测试（验证一个类型可实现多个能力接口）
// =============================================================================

func TestInterfaceComposition(t *testing.T) {
	c := &sliceCollection[int]{data: []int{1, 2, 3}}

	var _ core.Collection[int] = c
	var _ core.Iterable[int] = c
	var _ core.Searchable[int] = c
	var _ core.Mutable[int] = c
	var _ core.Cloneable[int] = c
	var _ core.Comparable[int] = c
	var _ core.Traversable[int] = c

	t.Run("组合后基础操作", func(t *testing.T) {
		if c.Len() != 3 {
			t.Errorf("Len() = %d, 期望 3", c.Len())
		}
	})
}

// =============================================================================
// Visitor 使用示例（文档化用例）
// =============================================================================
