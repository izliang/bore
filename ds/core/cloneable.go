package core

// Cloneable 表示一个可以生成自身深拷贝的集合。
// 返回的 Collection 与原集合互不影响。
type Cloneable[T any] interface {
	// Clone 返回接收者的一个深拷贝。
	Clone() Collection[T]
}
