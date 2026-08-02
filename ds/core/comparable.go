package core

// Comparable 提供集合之间的结构相等性比较。
type Comparable[T any] interface {
	Collection[T]

	// Equal 返回接收者与 other 是否包含相同的元素，
	// 并以相同的结构/顺序排列（取决于具体实现）。
	Equal(other Collection[T]) bool
}
