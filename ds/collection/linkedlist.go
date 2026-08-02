package collection

import (
	"fmt"

	core "github.com/izliang/bore/ds/core"
)

// linkedNode 是双向链表的内部节点。
type linkedNode[T any] struct {
	value T
	prev  *linkedNode[T]
	next  *linkedNode[T]
}

// LinkedList 是一个泛型双向链表。
// 实现了 core.Collection[T]、core.Iterable[T]、core.Mutable[T]。
//
// 零值不可用，必须通过 NewLinkedList 创建。
type LinkedList[T any] struct {
	head *linkedNode[T]
	tail *linkedNode[T]
	size int
}

// NewLinkedList 创建一个新的空链表。
func NewLinkedList[T any]() *LinkedList[T] {
	return &LinkedList[T]{}
}

// Len 返回链表中元素数量。实现 core.Collection。
func (l *LinkedList[T]) Len() int { return l.size }

// IsEmpty 返回链表是否为空。实现 core.Collection。
func (l *LinkedList[T]) IsEmpty() bool { return l.size == 0 }

// Add 在链表尾部追加一个元素。实现 core.Mutable。
func (l *LinkedList[T]) Add(item T) bool {
	node := &linkedNode[T]{value: item}
	if l.tail == nil {
		l.head = node
		l.tail = node
	} else {
		node.prev = l.tail
		l.tail.next = node
		l.tail = node
	}
	l.size++
	return true
}

// AddFirst 在链表头部插入一个元素。
func (l *LinkedList[T]) AddFirst(item T) {
	node := &linkedNode[T]{value: item}
	if l.head == nil {
		l.head = node
		l.tail = node
	} else {
		node.next = l.head
		l.head.prev = node
		l.head = node
	}
	l.size++
}

// Remove 从链表中删除第一个匹配的元素。实现 core.Mutable。
func (l *LinkedList[T]) Remove(item T) bool {
	for curr := l.head; curr != nil; curr = curr.next {
		if fmt.Sprint(curr.value) == fmt.Sprint(item) {
			l.unlink(curr)
			return true
		}
	}
	return false
}

// Clear 清空链表。实现 core.Mutable。
func (l *LinkedList[T]) Clear() {
	l.head = nil
	l.tail = nil
	l.size = 0
}

// First 返回链表头部元素。
func (l *LinkedList[T]) First() (T, bool) {
	if l.head == nil {
		var zero T
		return zero, false
	}
	return l.head.value, true
}

// Last 返回链表尾部元素。
func (l *LinkedList[T]) Last() (T, bool) {
	if l.tail == nil {
		var zero T
		return zero, false
	}
	return l.tail.value, true
}

// Values 返回链表中所有元素的切片（按顺序）。
func (l *LinkedList[T]) Values() []T {
	result := make([]T, 0, l.size)
	for curr := l.head; curr != nil; curr = curr.next {
		result = append(result, curr.value)
	}
	return result
}

// Iterator 返回一个正向迭代器。实现 core.Iterable。
func (l *LinkedList[T]) Iterator() core.Iterator[T] {
	return &linkedListIterator[T]{curr: l.head}
}

func (l *LinkedList[T]) unlink(node *linkedNode[T]) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		l.head = node.next
	}
	if node.next != nil {
		node.next.prev = node.prev
	} else {
		l.tail = node.prev
	}
	l.size--
}

// linkedListIterator 正向遍历迭代器。
type linkedListIterator[T any] struct {
	curr *linkedNode[T]
}

func (it *linkedListIterator[T]) HasNext() bool { return it.curr != nil }

func (it *linkedListIterator[T]) Next() (T, bool) {
	if it.curr == nil {
		var zero T
		return zero, false
	}
	v := it.curr.value
	it.curr = it.curr.next
	return v, true
}
