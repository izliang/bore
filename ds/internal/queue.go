// Package internal 提供 ds SDK 内部复用的基础数据结构实现。
// 这些类型不可被 ds 外部导入，仅供 algorithm/* 和数据结构包内部使用。
package internal

// Queue 是一个泛型 FIFO（先进先出）队列。
// 内部基于环形缓冲区实现，Enqueue 和 Dequeue 均为 O(1) 摊还时间。
// 零值不可用，必须通过 NewQueue 创建。
type Queue[T any] struct {
	buf  []T
	head int
	tail int
	size int
}

// NewQueue 创建一个新的空队列，初始容量为 8。
func NewQueue[T any]() *Queue[T] {
	return &Queue[T]{buf: make([]T, 8)}
}

// NewQueueWithCapacity 创建一个指定初始容量的空队列。
func NewQueueWithCapacity[T any](capacity int) *Queue[T] {
	if capacity < 1 {
		capacity = 8
	}
	return &Queue[T]{buf: make([]T, capacity)}
}

// Enqueue 将元素添加到队尾。
func (q *Queue[T]) Enqueue(item T) {
	if q.size == len(q.buf) {
		q.grow()
	}
	q.buf[q.tail] = item
	q.tail = (q.tail + 1) % len(q.buf)
	q.size++
}

// Dequeue 从队首移除并返回元素。
// 如果队列为空，则返回 T 的零值和 false。
func (q *Queue[T]) Dequeue() (T, bool) {
	if q.size == 0 {
		var zero T
		return zero, false
	}
	item := q.buf[q.head]
	var zero T
	q.buf[q.head] = zero // 避免内存泄漏
	q.head = (q.head + 1) % len(q.buf)
	q.size--
	return item, true
}

// Peek 返回队首元素但不移除。
// 如果队列为空，则返回 T 的零值和 false。
func (q *Queue[T]) Peek() (T, bool) {
	if q.size == 0 {
		var zero T
		return zero, false
	}
	return q.buf[q.head], true
}

// Len 返回队列中当前的元素数量。
func (q *Queue[T]) Len() int {
	return q.size
}

// IsEmpty 返回队列是否为空。
func (q *Queue[T]) IsEmpty() bool {
	return q.size == 0
}

// Clear 清空队列中的所有元素。
func (q *Queue[T]) Clear() {
	var zero T
	for i := 0; i < q.size; i++ {
		q.buf[(q.head+i)%len(q.buf)] = zero
	}
	q.head = 0
	q.tail = 0
	q.size = 0
}

// grow 将环形缓冲区扩容为原来的两倍。
func (q *Queue[T]) grow() {
	newCap := len(q.buf) * 2
	if newCap < 8 {
		newCap = 8
	}
	newBuf := make([]T, newCap)
	for i := 0; i < q.size; i++ {
		newBuf[i] = q.buf[(q.head+i)%len(q.buf)]
	}
	q.buf = newBuf
	q.head = 0
	q.tail = q.size
}
