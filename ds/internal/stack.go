package internal

// Stack 是一个泛型 LIFO（后进先出）栈。
// 内部基于切片实现，Push 和 Pop 均为 O(1) 摊还时间。
// 零值不可用，必须通过 NewStack 创建。
type Stack[T any] struct {
	items []T
}

// NewStack 创建一个新的空栈。
func NewStack[T any]() *Stack[T] {
	return &Stack[T]{}
}

// NewStackWithCapacity 创建一个指定初始容量的空栈。
func NewStackWithCapacity[T any](capacity int) *Stack[T] {
	if capacity < 1 {
		capacity = 8
	}
	return &Stack[T]{items: make([]T, 0, capacity)}
}

// Push 将元素压入栈顶。
func (s *Stack[T]) Push(item T) {
	s.items = append(s.items, item)
}

// Pop 从栈顶弹出并返回元素。
// 如果栈为空，则返回 T 的零值和 false。
func (s *Stack[T]) Pop() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	idx := len(s.items) - 1
	item := s.items[idx]
	var zero T
	s.items[idx] = zero // 避免内存泄漏
	s.items = s.items[:idx]
	return item, true
}

// Peek 返回栈顶元素但不弹出。
// 如果栈为空，则返回 T 的零值和 false。
func (s *Stack[T]) Peek() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	return s.items[len(s.items)-1], true
}

// Len 返回栈中当前的元素数量。
func (s *Stack[T]) Len() int {
	return len(s.items)
}

// IsEmpty 返回栈是否为空。
func (s *Stack[T]) IsEmpty() bool {
	return len(s.items) == 0
}

// Clear 清空栈中的所有元素。
func (s *Stack[T]) Clear() {
	var zero T
	for i := range s.items {
		s.items[i] = zero
	}
	s.items = s.items[:0]
}
