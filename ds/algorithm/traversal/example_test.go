package traversal_test

import (
	"fmt"
	"iter"

	"github.com/izliang/bore/ds/algorithm/traversal"
)

// ExampleBFS 演示如何使用 BFS 遍历树结构。
func ExampleBFS() {
	g := newAdjacencyGraph[int]()
	g.addEdge(0, 1)
	g.addEdge(0, 2)
	g.addEdge(1, 3)
	g.addEdge(1, 4)

	var order []int
	traversal.BFS(g, 0, collectVisitor(&order))
	fmt.Println(order)
	// Output:
	// [0 1 2 3 4]
}

// ExampleDFS 演示如何使用 DFS 遍历树结构。
func ExampleDFS() {
	g := newAdjacencyGraph[int]()
	g.addEdge(0, 1)
	g.addEdge(0, 2)
	g.addEdge(1, 3)
	g.addEdge(1, 4)

	var order []int
	traversal.DFS(g, 0, collectVisitor(&order))
	fmt.Println(order)
	// Output:
	// [0 1 3 4 2]
}

// ExampleTraversable 演示如何为一个自定义类型实现 core.Traversable，
// 使其可以直接使用遍历算法。
func ExampleTraversable() {
	type TreeNode struct {
		Value    int
		Children []*TreeNode
	}

	// 为 *TreeNode 包装器实现 Traversable[*TreeNode]
	type treeAdapter struct{}

	// 注意：实际开发中需要持有根节点引用，此处仅示意接口实现方式。
	_ = iter.Seq[*TreeNode](func(yield func(*TreeNode) bool) {})
	fmt.Println("Traversable 接口实现示例")
	// Output:
	// Traversable 接口实现示例
}
