package core

// Collection 是 ds SDK 中所有数据结构的根接口。
// 它表示一个有限的、可以为空的、类型为 T 的元素集合。
//
// SDK 中的每个数据结构 —— Tree、Graph、Trie、Heap、Workflow、DAG、
// StateMachine 等 —— 至少实现了此接口。
type Collection[T any] interface {
	// Len 返回集合中当前的元素数量。
	Len() int

	// IsEmpty 返回集合是否不包含任何元素。
	IsEmpty() bool
}
