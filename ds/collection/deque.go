package collection


// Deque 是一个泛型双端队列（Double-Ended Queue），支持在首尾两端高效插入和删除。
// 内部基于环形缓冲区实现，所有操作均为 O(1) 摊还时间。
//
// 实现了 core.Collection[T]、core.Mutable[T]。
//
// 零值不可用，必须通过 NewDeque 创建。
type Deque[T any] struct {
	buf  []T
	head int
	tail int
	size int
}

// NewDeque 创建一个新的空双端队列。
func NewDeque[T any]() *Deque[T] {
	return &Deque[T]{buf: make([]T, 8)}
}

// Len 返回队列中的元素数量。实现 core.Collection。
func (d *Deque[T]) Len() int { return d.size }

// IsEmpty 返回队列是否为空。实现 core.Collection。
func (d *Deque[T]) IsEmpty() bool { return d.size == 0 }

// Add 在队尾添加一个元素。实现 core.Mutable。
func (d *Deque[T]) Add(item T) bool {
	if d.size == len(d.buf) {
		d.grow()
	}
	d.buf[d.tail] = item
	d.tail = (d.tail + 1) % len(d.buf)
	d.size++
	return true
}

// AddFirst 在队首添加一个元素。
func (d *Deque[T]) AddFirst(item T) {
	if d.size == len(d.buf) {
		d.grow()
	}
	d.head = (d.head - 1 + len(d.buf)) % len(d.buf)
	d.buf[d.head] = item
	d.size++
}

// AddLast 在队尾添加一个元素，与 Add 等价。
func (d *Deque[T]) AddLast(item T) { d.Add(item) }

// RemoveFirst 从队首移除并返回元素。
func (d *Deque[T]) RemoveFirst() (T, bool) {
	if d.size == 0 {
		var zero T
		return zero, false
	}
	item := d.buf[d.head]
	var zero T
	d.buf[d.head] = zero
	d.head = (d.head + 1) % len(d.buf)
	d.size--
	return item, true
}

// RemoveLast 从队尾移除并返回元素。
func (d *Deque[T]) RemoveLast() (T, bool) {
	if d.size == 0 {
		var zero T
		return zero, false
	}
	d.tail = (d.tail - 1 + len(d.buf)) % len(d.buf)
	item := d.buf[d.tail]
	var zero T
	d.buf[d.tail] = zero
	d.size--
	return item, true
}

// Remove 从队首删除一个元素。实现 core.Mutable（默认行为）。
func (d *Deque[T]) Remove(_ T) bool {
	_, ok := d.RemoveFirst()
	return ok
}

// Clear 清空队列。实现 core.Mutable。
func (d *Deque[T]) Clear() {
	var zero T
	for i := 0; i < d.size; i++ {
		d.buf[(d.head+i)%len(d.buf)] = zero
	}
	d.head = 0
	d.tail = 0
	d.size = 0
}

// PeekFirst 查看队首元素但不移除。
func (d *Deque[T]) PeekFirst() (T, bool) {
	if d.size == 0 {
		var zero T
		return zero, false
	}
	return d.buf[d.head], true
}

// PeekLast 查看队尾元素但不移除。
func (d *Deque[T]) PeekLast() (T, bool) {
	if d.size == 0 {
		var zero T
		return zero, false
	}
	idx := (d.tail - 1 + len(d.buf)) % len(d.buf)
	return d.buf[idx], true
}

func (d *Deque[T]) grow() {
	newCap := len(d.buf) * 2
	if newCap < 8 {
		newCap = 8
	}
	newBuf := make([]T, newCap)
	for i := 0; i < d.size; i++ {
		newBuf[i] = d.buf[(d.head+i)%len(d.buf)]
	}
	d.buf = newBuf
	d.head = 0
	d.tail = d.size
}
