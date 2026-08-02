package traversal_test

import (
	"iter"
	"testing"

	"github.com/izliang/bore/ds/algorithm/traversal"
	core "github.com/izliang/bore/ds/core"
)

// adjacencyGraph 是一个基于邻接表的测试用图，实现 core.Traversable。
type adjacencyGraph[T comparable] struct {
	edges map[T][]T
}

func newAdjacencyGraph[T comparable]() *adjacencyGraph[T] {
	return &adjacencyGraph[T]{edges: make(map[T][]T)}
}

func (g *adjacencyGraph[T]) addEdge(from, to T) {
	g.edges[from] = append(g.edges[from], to)
}

func (g *adjacencyGraph[T]) Adjacent(from T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, n := range g.edges[from] {
			if !yield(n) {
				return
			}
		}
	}
}

// collectVisitor 创建一个收集遍历顺序的 Visitor 和结果切片。
func collectVisitor[T any](out *[]T) core.Visitor[T] {
	return func(v T) bool {
		*out = append(*out, v)
		return true
	}
}

// =============================================================================
// BFS 测试
// =============================================================================

func TestBFS_SimpleTree(t *testing.T) {
	//     0
	//    / \
	//   1   2
	//  / \   \
	// 3   4   5
	g := newAdjacencyGraph[int]()
	g.addEdge(0, 1)
	g.addEdge(0, 2)
	g.addEdge(1, 3)
	g.addEdge(1, 4)
	g.addEdge(2, 5)

	var order []int
	traversal.BFS(g, 0, collectVisitor(&order))

	// BFS: 0, 1, 2, 3, 4, 5 (层序)
	expected := []int{0, 1, 2, 3, 4, 5}
	if len(order) != len(expected) {
		t.Fatalf("BFS 访问 %d 个节点, 期望 %d: %v", len(order), len(expected), order)
	}
	// 验证层序：第 0 层在前
	if order[0] != 0 {
		t.Errorf("order[0] = %d, 期望 0", order[0])
	}
	// 第 1 层：1 和 2 在 3、4、5 之前
	pos1 := indexOf(order, 1)
	pos2 := indexOf(order, 2)
	pos3 := indexOf(order, 3)
	pos4 := indexOf(order, 4)
	pos5 := indexOf(order, 5)
	if pos1 >= pos3 || pos1 >= pos4 || pos1 >= pos5 {
		t.Errorf("节点 1 应在 3/4/5 之前: %v", order)
	}
	if pos2 >= pos3 || pos2 >= pos4 || pos2 >= pos5 {
		t.Errorf("节点 2 应在 3/4/5 之前: %v", order)
	}
}

func TestBFS_SingleNode(t *testing.T) {
	g := newAdjacencyGraph[int]()
	var order []int
	traversal.BFS(g, 42, collectVisitor(&order))
	if len(order) != 1 || order[0] != 42 {
		t.Errorf("单节点 BFS: %v, 期望 [42]", order)
	}
}

func TestBFS_TwoNodes(t *testing.T) {
	g := newAdjacencyGraph[int]()
	g.addEdge(1, 2)
	var order []int
	traversal.BFS(g, 1, collectVisitor(&order))
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("双节点 BFS: %v, 期望 [1, 2]", order)
	}
}

func TestBFS_Cycle(t *testing.T) {
	// 1 → 2 → 3 → 1（环）
	g := newAdjacencyGraph[int]()
	g.addEdge(1, 2)
	g.addEdge(2, 3)
	g.addEdge(3, 1)

	var order []int
	traversal.BFS(g, 1, collectVisitor(&order))

	// 不应无限循环，应恰好访问 3 个节点
	if len(order) != 3 {
		t.Errorf("环图 BFS 应访问 3 个节点, 实际 %d: %v", len(order), order)
	}
	visited := make(map[int]bool)
	for _, v := range order {
		if visited[v] {
			t.Errorf("节点 %d 被重复访问", v)
		}
		visited[v] = true
	}
}

func TestBFS_SelfLoop(t *testing.T) {
	g := newAdjacencyGraph[int]()
	g.addEdge(1, 1) // 自环
	var order []int
	traversal.BFS(g, 1, collectVisitor(&order))
	if len(order) != 1 {
		t.Errorf("自环节点 BFS 应访问 1 个节点, 实际 %d: %v", len(order), order)
	}
}

func TestBFS_EarlyTermination(t *testing.T) {
	g := newAdjacencyGraph[int]()
	g.addEdge(0, 1)
	g.addEdge(0, 2)
	g.addEdge(1, 3)
	g.addEdge(2, 4)

	var order []int
	traversal.BFS(g, 0, func(v int) bool {
		order = append(order, v)
		return v != 1 // 遇到节点 1 后停止
	})

	if len(order) == 5 {
		t.Error("提前终止后不应访问所有节点")
	}
}

func TestBFS_Disconnected(t *testing.T) {
	g := newAdjacencyGraph[int]()
	g.addEdge(1, 2)
	g.addEdge(3, 4) // 与 1-2 不连通

	var order []int
	traversal.BFS(g, 1, collectVisitor(&order))

	if len(order) != 2 {
		t.Errorf("从节点 1 出发只应访问联通分量, 访问了 %d 个: %v", len(order), order)
	}
}

// =============================================================================
// DFS 测试
// =============================================================================

func TestDFS_SimpleTree(t *testing.T) {
	//     0
	//    / \
	//   1   2
	//  / \   \
	// 3   4   5
	g := newAdjacencyGraph[int]()
	g.addEdge(0, 1)
	g.addEdge(0, 2)
	g.addEdge(1, 3)
	g.addEdge(1, 4)
	g.addEdge(2, 5)

	var order []int
	traversal.DFS(g, 0, collectVisitor(&order))

	// DFS 深搜：先走一条路径到底
	if len(order) != 6 {
		t.Fatalf("DFS 应访问 6 个节点, 实际 %d: %v", len(order), order)
	}
	if order[0] != 0 {
		t.Errorf("DFS 应从 start 开始: %v", order)
	}
	// 验证深度优先：3 和 4 应在 2 和 5 之前（沿 0→1→... 分支先走完）
	pos2 := indexOf(order, 2)
	pos3 := indexOf(order, 3)
	pos4 := indexOf(order, 4)
	if pos3 > pos2 || pos4 > pos2 {
		t.Errorf("DFS 应先深入 1 的分支再访问 2: %v", order)
	}
}

func TestDFS_SingleNode(t *testing.T) {
	g := newAdjacencyGraph[int]()
	var order []int
	traversal.DFS(g, 42, collectVisitor(&order))
	if len(order) != 1 || order[0] != 42 {
		t.Errorf("单节点 DFS: %v, 期望 [42]", order)
	}
}

func TestDFS_TwoNodes(t *testing.T) {
	g := newAdjacencyGraph[int]()
	g.addEdge(1, 2)
	var order []int
	traversal.DFS(g, 1, collectVisitor(&order))
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Errorf("双节点 DFS: %v, 期望 [1, 2]", order)
	}
}

func TestDFS_Cycle(t *testing.T) {
	g := newAdjacencyGraph[int]()
	g.addEdge(1, 2)
	g.addEdge(2, 3)
	g.addEdge(3, 1)

	var order []int
	traversal.DFS(g, 1, collectVisitor(&order))

	if len(order) != 3 {
		t.Errorf("环图 DFS 应访问 3 个节点, 实际 %d: %v", len(order), order)
	}
	visited := make(map[int]bool)
	for _, v := range order {
		if visited[v] {
			t.Errorf("节点 %d 被重复访问", v)
		}
		visited[v] = true
	}
}

func TestDFS_SelfLoop(t *testing.T) {
	g := newAdjacencyGraph[int]()
	g.addEdge(1, 1)
	var order []int
	traversal.DFS(g, 1, collectVisitor(&order))
	if len(order) != 1 {
		t.Errorf("自环节点 DFS 应访问 1 个节点, 实际 %d", len(order))
	}
}

func TestDFS_EarlyTermination(t *testing.T) {
	g := newAdjacencyGraph[int]()
	g.addEdge(0, 1)
	g.addEdge(0, 2)
	g.addEdge(2, 3)

	var order []int
	traversal.DFS(g, 0, func(v int) bool {
		order = append(order, v)
		return v != 2
	})

	if len(order) == 4 {
		t.Error("提前终止后不应访问所有节点")
	}
}

func TestDFS_IsDeepFirst(t *testing.T) {
	// 线形图：0 → 1 → 2 → 3 → 4
	g := newAdjacencyGraph[int]()
	g.addEdge(0, 1)
	g.addEdge(1, 2)
	g.addEdge(2, 3)
	g.addEdge(3, 4)

	var order []int
	traversal.DFS(g, 0, collectVisitor(&order))

	// 对于线形图，DFS 和 BFS 顺序相同
	expected := []int{0, 1, 2, 3, 4}
	if len(order) != len(expected) {
		t.Fatalf("线形图 DFS: %v, 期望 %v", order, expected)
	}
	for i, v := range order {
		if v != expected[i] {
			t.Errorf("order[%d] = %d, 期望 %d", i, v, expected[i])
		}
	}
}

// =============================================================================
// BFS vs DFS 对比测试
// =============================================================================

func TestBFSvsDFS_CycleGraph(t *testing.T) {
	// 1 ↔ 2, 2 ↔ 3, 3 ↔ 4, 1 ↔ 4（正方形环）
	g := newAdjacencyGraph[int]()
	g.addEdge(1, 2)
	g.addEdge(2, 1)
	g.addEdge(2, 3)
	g.addEdge(3, 2)
	g.addEdge(3, 4)
	g.addEdge(4, 3)
	g.addEdge(4, 1)
	g.addEdge(1, 4)

	var bfsOrder, dfsOrder []int
	traversal.BFS(g, 1, collectVisitor(&bfsOrder))
	traversal.DFS(g, 1, collectVisitor(&dfsOrder))

	if len(bfsOrder) != 4 || len(dfsOrder) != 4 {
		t.Errorf("BFS=%d, DFS=%d, 均期望 4 个节点", len(bfsOrder), len(dfsOrder))
	}
	// BFS 应先访问直接邻居（同层），DFS 沿一条路走到底
	if bfsOrder[1] == dfsOrder[1] {
		t.Logf("BFS 和 DFS 第二位可能相同（取决于邻接顺序）: BFS=%v, DFS=%v", bfsOrder, dfsOrder)
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

func indexOf(s []int, v int) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
