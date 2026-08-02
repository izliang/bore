package graph_test

import (
	"testing"

	"github.com/izliang/bore/ds/algorithm/traversal"
	core "github.com/izliang/bore/ds/core"
	"github.com/izliang/bore/ds/graph"
)

// 编译期接口检查
var (
	_ core.Collection[int]  = (*graph.Graph[int])(nil)
	_ core.Searchable[int]  = (*graph.Graph[int])(nil)
	_ core.Mutable[int]     = (*graph.Graph[int])(nil)
	_ core.Cloneable[int]   = (*graph.Graph[int])(nil)
	_ core.Traversable[int] = (*graph.Graph[int])(nil)
	_ core.Weighted         = graph.Edge[int]{}
)

// =============================================================================
// 基础测试
// =============================================================================

func TestGraph_New(t *testing.T) {
	t.Run("有向图", func(t *testing.T) {
		g := graph.NewDirected[int]()
		if !g.IsDirected() {
			t.Error("有向图 IsDirected() 应为 true")
		}
		if !g.IsEmpty() {
			t.Error("新图应为空")
		}
		if g.Len() != 0 {
			t.Errorf("Len() = %d, 期望 0", g.Len())
		}
	})
	t.Run("无向图", func(t *testing.T) {
		g := graph.NewUndirected[int]()
		if g.IsDirected() {
			t.Error("无向图 IsDirected() 应为 false")
		}
		if !g.IsEmpty() {
			t.Error("新图应为空")
		}
	})
}

func TestGraph_AddVertex(t *testing.T) {
	g := graph.NewDirected[int]()
	if !g.Add(1) {
		t.Error("Add(1) 应返回 true")
	}
	if g.Add(1) {
		t.Error("重复 Add(1) 应返回 false")
	}
	if g.Len() != 1 {
		t.Errorf("Len() = %d, 期望 1", g.Len())
	}
	if !g.Contains(1) {
		t.Error("Contains(1) 应为 true")
	}
}

func TestGraph_AddEdge(t *testing.T) {
	t.Run("有向图", func(t *testing.T) {
		g := graph.NewDirected[int]()
		g.AddEdge(1, 2, 5.0)
		if g.Len() != 2 {
			t.Errorf("有向图 1→2 应有 2 个顶点, 实际 %d", g.Len())
		}
		if g.EdgeCount() != 1 {
			t.Errorf("EdgeCount = %d, 期望 1", g.EdgeCount())
		}
	})

	t.Run("无向图", func(t *testing.T) {
		g := graph.NewUndirected[int]()
		g.AddEdge(1, 2, 3.0)
		if g.Len() != 2 {
			t.Errorf("无向图应有 2 个顶点, 实际 %d", g.Len())
		}
		if g.EdgeCount() != 1 {
			t.Errorf("无向图 EdgeCount = %d, 期望 1", g.EdgeCount())
		}
	})
}

func TestGraph_WeightedEdge(t *testing.T) {
	e := graph.Edge[int]{From: 1, To: 2, W: 7.5}
	if e.Weight() != 7.5 {
		t.Errorf("Weight() = %v, 期望 7.5", e.Weight())
	}
	if e.From != 1 || e.To != 2 {
		t.Errorf("Edge 字段: From=%d To=%d, 期望 1, 2", e.From, e.To)
	}
}

// =============================================================================
// 遍历测试
// =============================================================================

func TestGraph_Traversable_BFS(t *testing.T) {
	g := graph.NewDirected[int]()
	g.AddEdge(0, 1, 1)
	g.AddEdge(0, 2, 1)
	g.AddEdge(1, 3, 1)
	g.AddEdge(2, 4, 1)

	var order []int
	traversal.BFS(g, 0, func(v int) bool {
		order = append(order, v)
		return true
	})
	if len(order) != 5 {
		t.Errorf("BFS 应访问 5 个节点, 实际 %d: %v", len(order), order)
	}
	if order[0] != 0 {
		t.Errorf("BFS 应从 0 开始: %v", order)
	}
}

func TestGraph_Traversable_DFS(t *testing.T) {
	g := graph.NewDirected[int]()
	g.AddEdge(0, 1, 1)
	g.AddEdge(0, 2, 1)
	g.AddEdge(1, 3, 1)
	g.AddEdge(2, 4, 1)

	var order []int
	traversal.DFS(g, 0, func(v int) bool {
		order = append(order, v)
		return true
	})
	if len(order) != 5 {
		t.Errorf("DFS 应访问 5 个节点, 实际 %d", len(order))
	}
}

func TestGraph_Undirected_Traversal(t *testing.T) {
	g := graph.NewUndirected[int]()
	g.AddEdge(1, 2, 1)
	g.AddEdge(2, 3, 1)

	var order []int
	traversal.BFS(g, 1, func(v int) bool {
		order = append(order, v)
		return true
	})
	if len(order) != 3 {
		t.Errorf("无向图 BFS 应访问 3 个节点, 实际 %d: %v", len(order), order)
	}
}

// =============================================================================
// Remove 测试
// =============================================================================

func TestGraph_RemoveVertex(t *testing.T) {
	g := graph.NewDirected[int]()
	g.AddEdge(1, 2, 1)
	g.AddEdge(2, 3, 1)
	g.AddEdge(3, 1, 1) // 环

	if !g.Remove(1) {
		t.Error("Remove(1) 应返回 true")
	}
	if g.Contains(1) {
		t.Error("Remove 后 Contains(1) 应为 false")
	}
	if g.Len() != 2 {
		t.Errorf("移除 1 后顶点数 = %d, 期望 2", g.Len())
	}
}

func TestGraph_RemoveNonexistent(t *testing.T) {
	g := graph.NewDirected[int]()
	if g.Remove(99) {
		t.Error("删除不存在的顶点应返回 false")
	}
}

func TestGraph_RemoveEdge(t *testing.T) {
	g := graph.NewDirected[int]()
	g.AddEdge(1, 2, 1)
	g.AddEdge(1, 3, 1)
	g.RemoveEdge(1, 2)

	if g.EdgeCount() != 1 {
		t.Errorf("删边后 EdgeCount = %d, 期望 1", g.EdgeCount())
	}
	neighbors := g.Neighbors(1)
	if len(neighbors) != 1 || neighbors[0] != 3 {
		t.Errorf("Neighbors(1) = %v, 期望 [3]", neighbors)
	}
}

// =============================================================================
// Clone 测试
// =============================================================================

func TestGraph_Clone(t *testing.T) {
	g := graph.NewDirected[int]()
	g.AddEdge(1, 2, 5)
	g.AddEdge(2, 3, 10)

	clone := g.Clone()
	if clone.Len() != g.Len() {
		t.Errorf("Clone Len() = %d, 期望 %d", clone.Len(), g.Len())
	}
	cloneGraph := clone.(*graph.Graph[int])
	if cloneGraph.EdgeCount() != g.EdgeCount() {
		t.Errorf("Clone EdgeCount = %d, 期望 %d", cloneGraph.EdgeCount(), g.EdgeCount())
	}
	// 深拷贝验证
	g.Add(99)
	if clone.Len() != 3 {
		t.Errorf("修改原图后 Clone Len() = %d, 期望 3", clone.Len())
	}
}

// =============================================================================
// Clear 测试
// =============================================================================

func TestGraph_Clear(t *testing.T) {
	g := graph.NewDirected[int]()
	g.AddEdge(1, 2, 1)
	g.AddEdge(3, 4, 1)
	g.Clear()
	if g.Len() != 0 {
		t.Error("Clear 后 Len() 应为 0")
	}
	if !g.IsEmpty() {
		t.Error("Clear 后 IsEmpty() 应为 true")
	}
	if g.EdgeCount() != 0 {
		t.Error("Clear 后 EdgeCount 应为 0")
	}
}

// =============================================================================
// 大体量测试
// =============================================================================

func TestGraph_Large(t *testing.T) {
	g := graph.NewDirected[int]()
	const n = 500
	for i := 0; i < n; i++ {
		g.AddEdge(i, (i+1)%n, 1.0)
	}
	if g.Len() != n {
		t.Errorf("Len() = %d, 期望 %d", g.Len(), n)
	}
	if g.EdgeCount() != n {
		t.Errorf("EdgeCount = %d, 期望 %d", g.EdgeCount(), n)
	}
}

// =============================================================================
// String 测试
// =============================================================================

func TestGraph_String(t *testing.T) {
	g := graph.NewDirected[int]()
	g.AddEdge(1, 2, 1)
	s := g.String()
	if s == "" {
		t.Error("String() 不应返回空")
	}
}
