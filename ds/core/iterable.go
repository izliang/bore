package core

// Iterable 是一个能够生成迭代器以进行顺序访问的 Collection。
// 实现此接口的数据结构使调用方可以逐个遍历其所有元素。
type Iterable[T any] interface {
	Collection[T]

	// Iterator 返回一个遍历该集合所有元素的新 Iterator。
	Iterator() Iterator[T]
}
