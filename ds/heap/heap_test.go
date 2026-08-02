package heap_test

import (
	"testing"

	core "github.com/izliang/bore/ds/core"
	"github.com/izliang/bore/ds/heap"
)

var (
	_ core.Collection[int] = (*heap.Heap[int])(nil)
	_ core.Searchable[int] = (*heap.Heap[int])(nil)
	_ core.Mutable[int]    = (*heap.Heap[int])(nil)
	_ core.Cloneable[int]  = (*heap.Heap[int])(nil)
)

func TestHeap_NewMinHeap(t *testing.T) {
	h := heap.NewMinHeap[int]()
	if !h.IsEmpty() {
		t.Error("新堆应为空")
	}
	if h.Len() != 0 {
		t.Errorf("Len() = %d, 期望 0", h.Len())
	}
}

func TestHeap_NewMaxHeap(t *testing.T) {
	h := heap.NewMaxHeap[int]()
	h.Add(1)
	h.Add(5)
	h.Add(3)
	top, _ := h.Peek()
	if top != 5 {
		t.Errorf("MaxHeap 堆顶 = %d, 期望 5", top)
	}
}

func TestHeap_AddAndPop_MinHeap(t *testing.T) {
	tests := []struct {
		name      string
		items     []int
		wantOrder []int
	}{
		{name: "升序入堆", items: []int{1, 2, 3, 4, 5}, wantOrder: []int{1, 2, 3, 4, 5}},
		{name: "降序入堆", items: []int{5, 4, 3, 2, 1}, wantOrder: []int{1, 2, 3, 4, 5}},
		{name: "乱序入堆", items: []int{3, 1, 4, 5, 2}, wantOrder: []int{1, 2, 3, 4, 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := heap.NewMinHeap[int]()
			for _, v := range tt.items {
				h.Add(v)
			}
			for i, want := range tt.wantOrder {
				got, ok := h.Pop()
				if !ok {
					t.Fatalf("第 %d 次 Pop: ok=false", i)
				}
				if got != want {
					t.Errorf("第 %d 次 Pop = %d, 期望 %d", i, got, want)
				}
			}
		})
	}
}

func TestHeap_AddAndPop_MaxHeap(t *testing.T) {
	h := heap.NewMaxHeap[int]()
	h.Add(1)
	h.Add(5)
	h.Add(3)
	expected := []int{5, 3, 1}
	for i, want := range expected {
		got, ok := h.Pop()
		if !ok || got != want {
			t.Errorf("第 %d 次 Pop = (%d, %v), 期望 (%d, true)", i, got, ok, want)
		}
	}
}

func TestHeap_Peek(t *testing.T) {
	h := heap.NewMinHeap[int]()
	h.Add(3)
	h.Add(1)
	h.Add(2)

	top, ok := h.Peek()
	if !ok || top != 1 {
		t.Errorf("Peek = (%d, %v), 期望 (1, true)", top, ok)
	}
	if h.Len() != 3 {
		t.Error("Peek 不应移除元素")
	}
}

func TestHeap_PopEmpty(t *testing.T) {
	h := heap.NewMinHeap[int]()
	_, ok := h.Pop()
	if ok {
		t.Error("空堆 Pop 应返回 false")
	}
	_, ok = h.Peek()
	if ok {
		t.Error("空堆 Peek 应返回 false")
	}
}

func TestHeap_Contains(t *testing.T) {
	h := heap.NewMinHeap[int]()
	h.Add(10)
	h.Add(20)
	if !h.Contains(10) {
		t.Error("Contains(10) 应为 true")
	}
	if h.Contains(99) {
		t.Error("Contains(99) 应为 false")
	}
}

func TestHeap_Remove(t *testing.T) {
	h := heap.NewMinHeap[int]()
	h.Add(3)
	h.Add(1)
	h.Add(2)
	if !h.Remove(1) {
		t.Error("Remove(1) 应返回 true")
	}
	if h.Contains(1) {
		t.Error("Remove 后 Contains(1) 应为 false")
	}
	if h.Len() != 2 {
		t.Errorf("Len() = %d, 期望 2", h.Len())
	}
	// 堆性质应保持
	top, _ := h.Pop()
	if top != 2 {
		t.Errorf("Remove 后堆顶 = %d, 期望 2", top)
	}
}

func TestHeap_RemoveNonexistent(t *testing.T) {
	h := heap.NewMinHeap[int]()
	if h.Remove(99) {
		t.Error("删除不存在元素应返回 false")
	}
}

func TestHeap_Clear(t *testing.T) {
	h := heap.NewMinHeap[int]()
	h.Add(1)
	h.Add(2)
	h.Clear()
	if h.Len() != 0 {
		t.Error("Clear 后 Len() 应为 0")
	}
}

func TestHeap_Clone(t *testing.T) {
	h := heap.NewMinHeap[int]()
	h.Add(3)
	h.Add(1)
	h.Add(2)
	clone := h.Clone()
	if clone.Len() != 3 {
		t.Errorf("Clone Len() = %d, 期望 3", clone.Len())
	}
	h.Add(99)
	if clone.Len() != 3 {
		t.Error("修改原堆应不影响克隆")
	}
}

func TestHeap_String(t *testing.T) {
	h := heap.NewMinHeap[int]()
	h.Add(1)
	s := h.String()
	if s == "" {
		t.Error("String() 不应为空")
	}
}

func TestHeap_LargeVolume(t *testing.T) {
	h := heap.NewMinHeap[int]()
	const n = 1000
	for i := n; i > 0; i-- {
		h.Add(i)
	}
	for i := 1; i <= n; i++ {
		got, ok := h.Pop()
		if !ok || got != i {
			t.Fatalf("Pop = (%d, %v), 期望 (%d, true)", got, ok, i)
		}
	}
}
