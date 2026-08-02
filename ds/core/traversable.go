package core

import "iter"

// Traversable 表示其元素可以通过邻接关系（边、子节点、下一状态等）
// 进行遍历的结构。它是所有遍历算法依赖的关键桥接接口。
//
// 算法（如 BFS、DFS）仅依赖此接口，而不依赖任何具体的
// 数据结构实现（Tree、Graph、Workflow 等），从而实现了完整的
// 算法与数据结构解耦。
//
// 任何实现 Traversable 的数据结构都可以立即使用 algorithm 包中的
// 所有遍历算法，无需修改算法代码。
type Traversable[T any] interface {
	// Adjacent 返回从给定元素出发可到达的所有元素的迭代器。
	// 迭代顺序由具体实现决定（例如，树的子节点顺序、图的邻接顺序）。
	Adjacent(from T) iter.Seq[T]
}
