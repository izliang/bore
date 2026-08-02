package collection

import (
	"fmt"
	"strings"

	core "github.com/izliang/bore/ds/core"
)

// Set 是一个基于哈希表的泛型集合，不允许重复元素。
// 元素类型 T 必须支持 == 操作（满足 comparable 约束）。
//
// 实现了 core.Collection[T]、core.Searchable[T]、core.Mutable[T]、
// core.Iterable[T]、core.Cloneable[T]。
//
// 零值不可用，必须通过 NewSet 创建。
type Set[T comparable] struct {
	data map[T]struct{}
}

// NewSet 创建一个新的空集合。
func NewSet[T comparable]() *Set[T] {
	return &Set[T]{data: make(map[T]struct{})}
}

// NewSetWithCapacity 创建一个指定初始容量的空集合。
func NewSetWithCapacity[T comparable](capacity int) *Set[T] {
	if capacity < 1 {
		capacity = 8
	}
	return &Set[T]{data: make(map[T]struct{}, capacity)}
}

// Len 返回集合中的元素数量。实现 core.Collection。
func (s *Set[T]) Len() int { return len(s.data) }

// IsEmpty 返回集合是否为空。实现 core.Collection。
func (s *Set[T]) IsEmpty() bool { return len(s.data) == 0 }

// Contains 返回集合中是否存在指定元素。实现 core.Searchable。
func (s *Set[T]) Contains(item T) bool {
	_, ok := s.data[item]
	return ok
}

// Add 向集合中添加一个元素。实现 core.Mutable。
func (s *Set[T]) Add(item T) bool {
	if s.Contains(item) {
		return false
	}
	s.data[item] = struct{}{}
	return true
}

// Remove 从集合中删除一个元素。实现 core.Mutable。
func (s *Set[T]) Remove(item T) bool {
	if !s.Contains(item) {
		return false
	}
	delete(s.data, item)
	return true
}

// Clear 清空集合。实现 core.Mutable。
func (s *Set[T]) Clear() {
	s.data = make(map[T]struct{})
}

// Clone 返回集合的深拷贝。实现 core.Cloneable。
func (s *Set[T]) Clone() core.Collection[T] {
	clone := NewSetWithCapacity[T](len(s.data))
	for k := range s.data {
		clone.data[k] = struct{}{}
	}
	return clone
}

// Values 返回集合中所有元素的切片（无序）。
func (s *Set[T]) Values() []T {
	result := make([]T, 0, len(s.data))
	for k := range s.data {
		result = append(result, k)
	}
	return result
}

// Iterator 返回一个遍历所有元素的迭代器。实现 core.Iterable。
func (s *Set[T]) Iterator() core.Iterator[T] {
	return &setIterator[T]{items: s.Values(), pos: 0}
}

// String 返回集合的字符串表示。
func (s *Set[T]) String() string {
	parts := make([]string, 0, len(s.data))
	for k := range s.data {
		parts = append(parts, fmt.Sprint(k))
	}
	return "Set{" + strings.Join(parts, ", ") + "}"
}

type setIterator[T any] struct {
	items []T
	pos   int
}

func (it *setIterator[T]) HasNext() bool { return it.pos < len(it.items) }

func (it *setIterator[T]) Next() (T, bool) {
	if it.pos >= len(it.items) {
		var zero T
		return zero, false
	}
	v := it.items[it.pos]
	it.pos++
	return v, true
}
