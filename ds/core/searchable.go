package core

// Searchable 是一个支持成员查询的 Collection。
type Searchable[T any] interface {
	Collection[T]

	// Contains 返回给定元素是否存在于集合中。
	Contains(item T) bool
}
