package core

// Iterator 提供对集合元素的有序遍历访问。
// 它是迭代器设计模式的泛型实现，与 Go 的 for-range 和 iter.Seq 互补。
//
// 典型用法：
//
//	it := collection.Iterator()
//	for it.HasNext() {
//	    v, ok := it.Next()
//	    // 使用 v
//	}
type Iterator[T any] interface {
	// HasNext 返回是否还有更多元素可供遍历。
	HasNext() bool

	// Next 返回下一个元素。如果遍历已完成，则返回 T 的零值和 false。
	Next() (T, bool)
}
