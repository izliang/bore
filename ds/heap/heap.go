// Package heap 提供基于 core 接口的二叉堆实现。
// 默认是最小堆（MinHeap），可通过 NewMaxHeap 创建最大堆。
package heap

import (
	"cmp"
	"fmt"

	core "github.com/izliang/bore/ds/core"
)

// Heap 是一个泛型二叉堆。
// 元素类型 T 必须支持 < 和 == 操作（满足 cmp.Ordered 约束）。
//
// 实现了 core.Collection[T]、core.Searchable[T]、core.Mutable[T]、
// core.Cloneable[T]。
//
// 零值不可用，必须通过 NewMinHeap 或 NewMaxHeap 创建。
type Heap[T cmp.Ordered] struct {
	items  []T
	minTop bool // true = 最小堆, false = 最大堆
}

// NewMinHeap 创建一个新的最小堆（堆顶为最小值）。
func NewMinHeap[T cmp.Ordered]() *Heap[T] {
	return &Heap[T]{minTop: true}
}

// NewMaxHeap 创建一个新的最大堆（堆顶为最大值）。
func NewMaxHeap[T cmp.Ordered]() *Heap[T] {
	return &Heap[T]{minTop: false}
}

// Len 返回堆中的元素数量。实现 core.Collection。
func (h *Heap[T]) Len() int { return len(h.items) }

// IsEmpty 返回堆是否为空。实现 core.Collection。
func (h *Heap[T]) IsEmpty() bool { return len(h.items) == 0 }

// Contains 返回堆中是否存在指定元素。实现 core.Searchable。
func (h *Heap[T]) Contains(item T) bool {
	for _, v := range h.items {
		if v == item {
			return true
		}
	}
	return false
}

// Add 向堆中插入一个元素。实现 core.Mutable。
func (h *Heap[T]) Add(item T) bool {
	h.items = append(h.items, item)
	h.siftUp(len(h.items) - 1)
	return true
}

// Remove 从堆中删除指定元素。实现 core.Mutable。
func (h *Heap[T]) Remove(item T) bool {
	idx := h.indexOf(item)
	if idx < 0 {
		return false
	}
	h.removeAt(idx)
	return true
}

// Clear 清空堆中的所有元素。实现 core.Mutable。
func (h *Heap[T]) Clear() {
	h.items = nil
}

// Clone 返回堆的深拷贝。实现 core.Cloneable。
func (h *Heap[T]) Clone() core.Collection[T] {
	clone := &Heap[T]{minTop: h.minTop}
	clone.items = make([]T, len(h.items))
	copy(clone.items, h.items)
	return clone
}

// Peek 返回堆顶元素但不移除。O(1)。
func (h *Heap[T]) Peek() (T, bool) {
	if len(h.items) == 0 {
		var zero T
		return zero, false
	}
	return h.items[0], true
}

// Pop 移除并返回堆顶元素。O(log n)。
func (h *Heap[T]) Pop() (T, bool) {
	if len(h.items) == 0 {
		var zero T
		return zero, false
	}
	top := h.items[0]
	h.removeAt(0)
	return top, true
}

// String 返回堆的字符串表示。
func (h *Heap[T]) String() string {
	kind := "Min"
	if !h.minTop {
		kind = "Max"
	}
	return fmt.Sprintf("%sHeap(%v)", kind, h.items)
}

func (h *Heap[T]) less(i, j int) bool {
	if h.minTop {
		return h.items[i] < h.items[j]
	}
	return h.items[i] > h.items[j]
}

func (h *Heap[T]) swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
}

func (h *Heap[T]) siftUp(idx int) {
	for idx > 0 {
		parent := (idx - 1) / 2
		if !h.less(idx, parent) {
			break
		}
		h.swap(idx, parent)
		idx = parent
	}
}

func (h *Heap[T]) siftDown(idx int) {
	n := len(h.items)
	for {
		smallest := idx
		left := 2*idx + 1
		right := 2*idx + 2
		if left < n && h.less(left, smallest) {
			smallest = left
		}
		if right < n && h.less(right, smallest) {
			smallest = right
		}
		if smallest == idx {
			break
		}
		h.swap(idx, smallest)
		idx = smallest
	}
}

func (h *Heap[T]) removeAt(idx int) {
	last := len(h.items) - 1
	if idx == last {
		var zero T
		h.items[last] = zero
		h.items = h.items[:last]
		return
	}
	h.swap(idx, last)
	var zero T
	h.items[last] = zero
	h.items = h.items[:last]
	h.siftDown(idx)
	h.siftUp(idx)
}

func (h *Heap[T]) indexOf(item T) int {
	for i, v := range h.items {
		if v == item {
			return i
		}
	}
	return -1
}
