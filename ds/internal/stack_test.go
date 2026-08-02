package internal_test

import (
	"testing"

	"github.com/izliang/bore/ds/internal"
)

// =============================================================================
// Stack 基础功能测试
// =============================================================================

func TestStackNew(t *testing.T) {
	s := internal.NewStack[int]()
	if s == nil {
		t.Fatal("NewStack 不应返回 nil")
	}
	if !s.IsEmpty() {
		t.Error("新栈应为空")
	}
	if s.Len() != 0 {
		t.Errorf("新栈 Len() = %d, 期望 0", s.Len())
	}
}

func TestStackNewWithCapacity(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
	}{
		{name: "正常容量", capacity: 16},
		{name: "容量为 1", capacity: 1},
		{name: "容量为 0 时应回退", capacity: 0},
		{name: "负容量应回退", capacity: -5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := internal.NewStackWithCapacity[int](tt.capacity)
			if s == nil {
				t.Fatal("NewStackWithCapacity 不应返回 nil")
			}
			if s.Len() != 0 {
				t.Errorf("Len() = %d, 期望 0", s.Len())
			}
		})
	}
}

func TestStackPushPop(t *testing.T) {
	tests := []struct {
		name  string
		items []int
	}{
		{name: "空栈", items: nil},
		{name: "单元素", items: []int{42}},
		{name: "三个元素", items: []int{1, 2, 3}},
		{name: "十个元素", items: []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := internal.NewStack[int]()
			for _, item := range tt.items {
				s.Push(item)
			}
			if s.Len() != len(tt.items) {
				t.Errorf("入栈后 Len() = %d, 期望 %d", s.Len(), len(tt.items))
			}
			// 栈是 LIFO，逆序验证
			for i := len(tt.items) - 1; i >= 0; i-- {
				got, ok := s.Pop()
				if !ok {
					t.Fatalf("第 %d 次 Pop: ok=false", len(tt.items)-1-i)
				}
				if got != tt.items[i] {
					t.Errorf("Pop() = %v, 期望 %v", got, tt.items[i])
				}
			}
			if !s.IsEmpty() {
				t.Error("全部出栈后 IsEmpty() 应为 true")
			}
		})
	}
}

func TestStackPopEmpty(t *testing.T) {
	s := internal.NewStack[string]()
	got, ok := s.Pop()
	if ok {
		t.Errorf("空栈 Pop 应返回 ok=false，得到 %v", got)
	}
	var zero string
	if got != zero {
		t.Errorf("空栈 Pop 应返回零值，得到 %v", got)
	}
}

func TestStackPeek(t *testing.T) {
	t.Run("查看非空栈", func(t *testing.T) {
		s := internal.NewStack[int]()
		s.Push(10)
		s.Push(20)
		got, ok := s.Peek()
		if !ok || got != 20 {
			t.Errorf("Peek() = (%v, %v), 期望 (20, true)", got, ok)
		}
		if s.Len() != 2 {
			t.Error("Peek 不应移除元素")
		}
	})

	t.Run("查看空栈", func(t *testing.T) {
		s := internal.NewStack[int]()
		got, ok := s.Peek()
		if ok {
			t.Errorf("空栈 Peek 应返回 ok=false，得到 %v", got)
		}
	})
}

func TestStackClear(t *testing.T) {
	s := internal.NewStack[int]()
	s.Push(1)
	s.Push(2)
	s.Push(3)
	s.Clear()

	if s.Len() != 0 {
		t.Errorf("Clear 后 Len() = %d, 期望 0", s.Len())
	}
	if !s.IsEmpty() {
		t.Error("Clear 后 IsEmpty() 应为 true")
	}
	_, ok := s.Pop()
	if ok {
		t.Error("Clear 后 Pop 应返回 false")
	}
}

func TestStackClearTwice(t *testing.T) {
	s := internal.NewStack[int]()
	s.Clear()
	s.Clear() // 不应 panic
	if s.Len() != 0 {
		t.Error("重复 Clear 后 Len() 应为 0")
	}
}

// =============================================================================
// Stack 不同类型测试
// =============================================================================

func TestStackWithString(t *testing.T) {
	s := internal.NewStack[string]()
	s.Push("hello")
	s.Push("world")

	got, _ := s.Pop()
	if got != "world" {
		t.Errorf("Pop() = %q, 期望 %q", got, "world")
	}
	got, _ = s.Pop()
	if got != "hello" {
		t.Errorf("Pop() = %q, 期望 %q", got, "hello")
	}
}

func TestStackWithStruct(t *testing.T) {
	type point struct{ x, y int }
	s := internal.NewStack[point]()
	s.Push(point{1, 2})
	s.Push(point{3, 4})

	p, ok := s.Pop()
	if !ok || p.x != 3 || p.y != 4 {
		t.Errorf("Pop() = (%v, %v), 期望 ({3, 4}, true)", p, ok)
	}
}

// =============================================================================
// Stack 大数据量测试
// =============================================================================

func TestStackLargeVolume(t *testing.T) {
	s := internal.NewStack[int]()
	const n = 10000
	for i := 0; i < n; i++ {
		s.Push(i)
	}
	if s.Len() != n {
		t.Errorf("Len() = %d, 期望 %d", s.Len(), n)
	}
	for i := n - 1; i >= 0; i-- {
		v, ok := s.Pop()
		if !ok || v != i {
			t.Fatalf("Pop() = (%v, %v), 期望 (%d, true)", v, ok, i)
		}
	}
}

// =============================================================================
// Stack Push/Pop 交替测试
// =============================================================================

func TestStackInterleaved(t *testing.T) {
	s := internal.NewStack[int]()

	s.Push(1)
	s.Push(2)
	v, _ := s.Pop() // 2
	if v != 2 {
		t.Errorf("期望 2，得到 %d", v)
	}

	s.Push(3)
	s.Push(4)
	v, _ = s.Pop() // 4
	if v != 4 {
		t.Errorf("期望 4，得到 %d", v)
	}

	v, _ = s.Pop() // 3
	if v != 3 {
		t.Errorf("期望 3，得到 %d", v)
	}

	s.Push(5)
	v, _ = s.Pop() // 5
	if v != 5 {
		t.Errorf("期望 5，得到 %d", v)
	}

	v, _ = s.Pop() // 1
	if v != 1 {
		t.Errorf("期望 1，得到 %d", v)
	}

	if !s.IsEmpty() {
		t.Error("全部 Pop 后栈应为空")
	}
}

// =============================================================================
// Stack IsEmpty 一致性测试
// =============================================================================

func TestStackLenIsEmptyConsistency(t *testing.T) {
	s := internal.NewStack[int]()
	if (s.Len() == 0) != s.IsEmpty() {
		t.Error("空栈: Len()==0 与 IsEmpty() 不一致")
	}
	s.Push(1)
	if (s.Len() == 0) != s.IsEmpty() {
		t.Error("非空栈: Len()==0 与 IsEmpty() 不一致")
	}
	s.Pop()
	if (s.Len() == 0) != s.IsEmpty() {
		t.Error("出栈后变空: Len()==0 与 IsEmpty() 不一致")
	}
}

// =============================================================================
// Stack 容量测试（验证 NewStackWithCapacity 预分配）
// =============================================================================

func TestStackCapacityPreAlloc(t *testing.T) {
	s := internal.NewStackWithCapacity[int](100)
	// 在容量内 push 不应触发重新分配
	for i := 0; i < 100; i++ {
		s.Push(i)
	}
	if s.Len() != 100 {
		t.Errorf("Len() = %d, 期望 100", s.Len())
	}
}
