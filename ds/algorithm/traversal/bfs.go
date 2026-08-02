// Package traversal 提供基于 core.Traversable 接口的通用遍历算法。
// 所有算法仅依赖 core 包中的接口，不依赖任何具体数据结构实现。
package traversal

import (
	core "github.com/izliang/bore/ds/core"
	"github.com/izliang/bore/ds/internal"
)

// BFS 对从 start 出发可达的所有节点执行广度优先遍历。
// 每当首次访问一个节点时，调用 visit 回调。
// visit 返回 false 可提前终止遍历。
//
// BFS 保证同层节点先于下一层节点被访问，但同层内的顺序由
// Traversable.Adjacent 的迭代顺序决定。
//
// 时间复杂度：O(V + E)，其中 V 为可达节点数，E 为边数。
// 空间复杂度：O(V)。
func BFS[T comparable](t core.Traversable[T], start T, visit core.Visitor[T]) {
	q := internal.NewQueue[T]()
	visited := make(map[T]bool)

	visited[start] = true
	q.Enqueue(start)

	for !q.IsEmpty() {
		current, _ := q.Dequeue()
		if !visit(current) {
			return
		}
		for neighbor := range t.Adjacent(current) {
			if visited[neighbor] {
				continue
			}
			visited[neighbor] = true
			q.Enqueue(neighbor)
		}
	}
}
