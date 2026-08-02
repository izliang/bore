package traversal

import (
	core "github.com/izliang/bore/ds/core"
	"github.com/izliang/bore/ds/internal"
)

// DFS 对从 start 出发可达的所有节点执行深度优先遍历。
// 每当首次访问一个节点时，调用 visit 回调。
// visit 返回 false 可提前终止遍历。
//
// DFS 使用显式栈实现（非递归），避免深层图导致的栈溢出。
// 遍历顺序：先访问当前节点，再按 Adjacent 的反序探索邻居，
// 以保证与标准递归 DFS 的访问顺序一致。
//
// 时间复杂度：O(V + E)，其中 V 为可达节点数，E 为边数。
// 空间复杂度：O(V)。
func DFS[T comparable](t core.Traversable[T], start T, visit core.Visitor[T]) {
	s := internal.NewStack[T]()
	visited := make(map[T]bool)

	s.Push(start)

	for !s.IsEmpty() {
		current, _ := s.Pop()
		if visited[current] {
			continue
		}
		visited[current] = true
		if !visit(current) {
			return
		}
		// 逆序压入邻居，保证自然顺序的访问
		var neighbors []T
		for n := range t.Adjacent(current) {
			neighbors = append(neighbors, n)
		}
		for i := len(neighbors) - 1; i >= 0; i-- {
			if !visited[neighbors[i]] {
				s.Push(neighbors[i])
			}
		}
	}
}
