// Package graph 提供基于 core 接口的图数据结构实现。
// 支持有向图和无向图，带权边。
package graph

import (
	"fmt"
	"iter"

	core "github.com/izliang/bore/ds/core"
)

// Edge 表示图中的一条边，包含源顶点、目标顶点和权重
// （字段名 W 避免与 Weight() 方法冲突，W 是图论中权重的通用缩写）。
type Edge[V comparable] struct {
	From V
	To   V
	W    float64
}

// Weight 返回边的权重。实现 core.Weighted 接口。
func (e Edge[V]) Weight() float64 { return e.W }

// Graph 是一个基于邻接表的泛型图。
// 顶点类型 V 必须支持 == 操作（满足 comparable 约束）。
//
// 实现了 core.Collection[V]、Searchable[V]、Mutable[V]、
// Cloneable[V]、Traversable[V]。
//
// 零值不可用，必须通过 NewDirected 或 NewUndirected 创建。
type Graph[V comparable] struct {
	directed bool
	adjList  map[V][]Edge[V]
}

// NewDirected 创建一个新的有向图。
func NewDirected[V comparable]() *Graph[V] {
	return &Graph[V]{directed: true, adjList: make(map[V][]Edge[V])}
}

// NewUndirected 创建一个新的无向图。
// AddEdge(v1, v2, w) 会同时添加 v1→v2 和 v2→v1。
func NewUndirected[V comparable]() *Graph[V] {
	return &Graph[V]{directed: false, adjList: make(map[V][]Edge[V])}
}

// IsDirected 返回图是否为有向图。
func (g *Graph[V]) IsDirected() bool { return g.directed }

// Len 返回图中的顶点数。实现 core.Collection。
func (g *Graph[V]) Len() int { return len(g.adjList) }

// IsEmpty 返回图是否不包含任何顶点。实现 core.Collection。
func (g *Graph[V]) IsEmpty() bool { return len(g.adjList) == 0 }

// Contains 返回图中是否存在指定顶点。实现 core.Searchable。
func (g *Graph[V]) Contains(item V) bool {
	_, ok := g.adjList[item]
	return ok
}

// Add 向图中添加一个顶点。实现 core.Mutable。
func (g *Graph[V]) Add(item V) bool {
	if g.Contains(item) {
		return false
	}
	g.adjList[item] = nil
	return true
}

// Remove 从图中删除一个顶点及其所有关联的边。实现 core.Mutable。
func (g *Graph[V]) Remove(item V) bool {
	if !g.Contains(item) {
		return false
	}
	delete(g.adjList, item)
	for v := range g.adjList {
		g.adjList[v] = g.removeEdgesTo(g.adjList[v], item)
	}
	return true
}

// Clear 清空图中的所有顶点和边。实现 core.Mutable。
func (g *Graph[V]) Clear() {
	g.adjList = make(map[V][]Edge[V])
}

// AddEdge 添加一条从 from 到 to 的加权边。顶点不存在时自动创建。
func (g *Graph[V]) AddEdge(from, to V, weight float64) {
	g.ensureVertex(from)
	g.ensureVertex(to)
	g.adjList[from] = append(g.adjList[from], Edge[V]{From: from, To: to, W: weight})
	if !g.directed {
		g.adjList[to] = append(g.adjList[to], Edge[V]{From: to, To: from, W: weight})
	}
}

// RemoveEdge 删除从 from 到 to 的边。无向图中同时删除反向边。
func (g *Graph[V]) RemoveEdge(from, to V) {
	g.adjList[from] = g.removeEdgesTo(g.adjList[from], to)
	if !g.directed {
		g.adjList[to] = g.removeEdgesTo(g.adjList[to], from)
	}
}

// Edges 返回从指定顶点出发的所有边。
func (g *Graph[V]) Edges(from V) []Edge[V] {
	edges := g.adjList[from]
	result := make([]Edge[V], len(edges))
	copy(result, edges)
	return result
}

// EdgeCount 返回图中的边数。无向图中每条边只计数一次。
func (g *Graph[V]) EdgeCount() int {
	count := 0
	for _, edges := range g.adjList {
		count += len(edges)
	}
	if !g.directed {
		count /= 2
	}
	return count
}

// Vertices 返回图中所有顶点的切片。
func (g *Graph[V]) Vertices() []V {
	result := make([]V, 0, len(g.adjList))
	for v := range g.adjList {
		result = append(result, v)
	}
	return result
}

// Neighbors 返回指定顶点的所有邻居顶点。
func (g *Graph[V]) Neighbors(from V) []V {
	edges := g.adjList[from]
	result := make([]V, len(edges))
	for i, e := range edges {
		result[i] = e.To
	}
	return result
}

// Adjacent 返回从给定顶点出发可达的所有邻居顶点。实现 core.Traversable。
func (g *Graph[V]) Adjacent(from V) iter.Seq[V] {
	edges := g.adjList[from]
	return func(yield func(V) bool) {
		for _, e := range edges {
			if !yield(e.To) {
				return
			}
		}
	}
}

// Clone 返回图的深拷贝。实现 core.Cloneable。
func (g *Graph[V]) Clone() core.Collection[V] {
	clone := &Graph[V]{directed: g.directed, adjList: make(map[V][]Edge[V], len(g.adjList))}
	for v, edges := range g.adjList {
		clone.adjList[v] = make([]Edge[V], len(edges))
		copy(clone.adjList[v], edges)
	}
	return clone
}

// String 返回图的字符串表示。
func (g *Graph[V]) String() string {
	kind := "无向"
	if g.directed {
		kind = "有向"
	}
	return fmt.Sprintf("%s图(顶点:%d, 边:%d)", kind, g.Len(), g.EdgeCount())
}

func (g *Graph[V]) ensureVertex(v V) {
	if !g.Contains(v) {
		g.adjList[v] = nil
	}
}

func (g *Graph[V]) removeEdgesTo(edges []Edge[V], target V) []Edge[V] {
	n := 0
	for _, e := range edges {
		if e.To != target {
			edges[n] = e
			n++
		}
	}
	var zero Edge[V]
	for i := n; i < len(edges); i++ {
		edges[i] = zero
	}
	return edges[:n]
}
