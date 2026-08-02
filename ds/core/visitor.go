package core

// Visitor 是遍历或检查元素时使用的回调函数类型。
// 返回 true 表示继续遍历，返回 false 表示提前终止。
//
// 典型用法：
//
//	traversal.BFS(traversable, start, func(v int) bool {
//	    fmt.Println(v)
//	    return true // 继续遍历
//	})
type Visitor[T any] func(T) bool
