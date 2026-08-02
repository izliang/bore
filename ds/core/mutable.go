package core

// Mutable 是一个可以修改其内容的 Collection。
type Mutable[T any] interface {
	Collection[T]

	// Add 向集合中插入一个元素。如果插入成功，返回 true。
	Add(item T) bool

	// Remove 从集合中删除一个元素。如果该元素存在并已被删除，返回 true。
	Remove(item T) bool

	// Clear 移除集合中的所有元素。
	Clear()
}
