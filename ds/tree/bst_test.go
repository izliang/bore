package tree_test

import (
	"testing"

	"github.com/izliang/bore/ds/algorithm/traversal"
	core "github.com/izliang/bore/ds/core"
	"github.com/izliang/bore/ds/tree"
)

// =============================================================================
// 编译期接口满足性检查
// =============================================================================

var (
	_ core.Collection[int]  = (*tree.BST[int])(nil)
	_ core.Iterable[int]    = (*tree.BST[int])(nil)
	_ core.Searchable[int]  = (*tree.BST[int])(nil)
	_ core.Mutable[int]     = (*tree.BST[int])(nil)
	_ core.Cloneable[int]   = (*tree.BST[int])(nil)
	_ core.Traversable[int] = (*tree.BST[int])(nil)
	_ core.Comparable[int]  = (*tree.BST[int])(nil)
)

// =============================================================================
// BST 基础功能测试
// =============================================================================

func TestBST_New(t *testing.T) {
	bst := tree.NewBST[int]()
	if bst == nil {
		t.Fatal("NewBST 不应返回 nil")
	}
	if !bst.IsEmpty() {
		t.Error("新树应为空")
	}
	if bst.Len() != 0 {
		t.Errorf("新树 Len() = %d, 期望 0", bst.Len())
	}
}

func TestBST_AddAndContains(t *testing.T) {
	tests := []struct {
		name  string
		items []int
	}{
		{name: "单个元素", items: []int{5}},
		{name: "多个元素", items: []int{5, 3, 7, 1, 4, 6, 8}},
		{name: "重复元素", items: []int{5, 3, 5, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bst := tree.NewBST[int]()
			for _, item := range tt.items {
				bst.Add(item)
			}
			if bst.IsEmpty() {
				t.Error("插入后树不应为空")
			}
			// Contains 验证
			seen := make(map[int]bool)
			for _, item := range tt.items {
				seen[item] = true
			}
			for item := range seen {
				if !bst.Contains(item) {
					t.Errorf("Contains(%d) 应为 true", item)
				}
			}
		})
	}
}

func TestBST_AddDuplicate(t *testing.T) {
	bst := tree.NewBST[int]()
	if !bst.Add(5) {
		t.Error("首次 Add(5) 应返回 true")
	}
	if bst.Add(5) {
		t.Error("重复 Add(5) 应返回 false")
	}
	if bst.Len() != 1 {
		t.Errorf("重复插入后 Len() = %d, 期望 1", bst.Len())
	}
}

func TestBST_Contains_Empty(t *testing.T) {
	bst := tree.NewBST[int]()
	if bst.Contains(42) {
		t.Error("空树 Contains 应返回 false")
	}
}

func TestBST_Contains_Missing(t *testing.T) {
	bst := tree.NewBST[int]()
	bst.Add(10)
	bst.Add(20)
	if bst.Contains(15) {
		t.Error("不存在的元素 Contains 应返回 false")
	}
}

// =============================================================================
// BST Remove 测试
// =============================================================================

func TestBST_Remove(t *testing.T) {
	tests := []struct {
		name   string
		insert []int
		remove int
		wantOk bool
	}{
		{name: "删除叶子", insert: []int{5, 3, 7}, remove: 3, wantOk: true},
		{name: "删除单子节点", insert: []int{5, 3, 7, 2}, remove: 3, wantOk: true},
		{name: "删除双子节点", insert: []int{5, 3, 7, 1, 4}, remove: 3, wantOk: true},
		{name: "删除根节点", insert: []int{5, 3, 7}, remove: 5, wantOk: true},
		{name: "删除不存在", insert: []int{5, 3, 7}, remove: 99, wantOk: false},
		{name: "空树删除", insert: nil, remove: 1, wantOk: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bst := tree.NewBST[int]()
			for _, item := range tt.insert {
				bst.Add(item)
			}
			got := bst.Remove(tt.remove)
			if got != tt.wantOk {
				t.Errorf("Remove(%d) = %v, 期望 %v", tt.remove, got, tt.wantOk)
			}
			if tt.wantOk {
				if bst.Contains(tt.remove) {
					t.Errorf("Remove 后 Contains(%d) 应为 false", tt.remove)
				}
			}
		})
	}
}

func TestBST_Clear(t *testing.T) {
	bst := tree.NewBST[int]()
	bst.Add(1)
	bst.Add(2)
	bst.Add(3)
	bst.Clear()

	if bst.Len() != 0 {
		t.Errorf("Clear 后 Len() = %d, 期望 0", bst.Len())
	}
	if !bst.IsEmpty() {
		t.Error("Clear 后 IsEmpty() 应为 true")
	}
	bst.Clear() // 重复 Clear 不应 panic
}

// =============================================================================
// BST 遍历测试
// =============================================================================

func TestBST_Values(t *testing.T) {
	bst := tree.NewBST[int]()
	input := []int{5, 3, 7, 1, 4, 6, 8}
	for _, v := range input {
		bst.Add(v)
	}
	got := bst.Values()
	expected := []int{1, 3, 4, 5, 6, 7, 8}
	if len(got) != len(expected) {
		t.Fatalf("Values() = %v, 期望 %v", got, expected)
	}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("Values()[%d] = %d, 期望 %d", i, v, expected[i])
		}
	}
}

func TestBST_Iterator(t *testing.T) {
	bst := tree.NewBST[string]()
	words := []string{"delta", "alpha", "charlie", "bravo"}
	for _, w := range words {
		bst.Add(w)
	}

	it := bst.Iterator()
	var got []string
	for it.HasNext() {
		v, ok := it.Next()
		if !ok {
			t.Fatal("HasNext 为 true 时 Next 应返回 ok=true")
		}
		got = append(got, v)
	}
	// 验证中序（字母序）
	expected := []string{"alpha", "bravo", "charlie", "delta"}
	if len(got) != len(expected) {
		t.Fatalf("Iterator = %v, 期望 %v", got, expected)
	}
	for i := range got {
		if got[i] != expected[i] {
			t.Errorf("[%d] = %s, 期望 %s", i, got[i], expected[i])
		}
	}
}

func TestBST_IteratorExhausted(t *testing.T) {
	bst := tree.NewBST[int]()
	it := bst.Iterator()
	if it.HasNext() {
		t.Error("空迭代器 HasNext() 应为 false")
	}
	_, ok := it.Next()
	if ok {
		t.Error("空迭代器 Next() 应返回 ok=false")
	}
}

// =============================================================================
// BST Min/Max 测试
// =============================================================================

func TestBST_MinMax(t *testing.T) {
	t.Run("非空树", func(t *testing.T) {
		bst := tree.NewBST[int]()
		bst.Add(5)
		bst.Add(3)
		bst.Add(7)
		min, ok := bst.Min()
		if !ok || min != 3 {
			t.Errorf("Min() = (%d, %v), 期望 (3, true)", min, ok)
		}
		max, ok := bst.Max()
		if !ok || max != 7 {
			t.Errorf("Max() = (%d, %v), 期望 (7, true)", max, ok)
		}
	})
	t.Run("空树", func(t *testing.T) {
		bst := tree.NewBST[int]()
		_, ok := bst.Min()
		if ok {
			t.Error("空树 Min() 应返回 ok=false")
		}
		_, ok = bst.Max()
		if ok {
			t.Error("空树 Max() 应返回 ok=false")
		}
	})
}

// =============================================================================
// BST Clone 测试
// =============================================================================

func TestBST_Clone(t *testing.T) {
	bst := tree.NewBST[int]()
	for _, v := range []int{5, 3, 7, 1, 4} {
		bst.Add(v)
	}
	clone := bst.Clone()

	if clone.Len() != bst.Len() {
		t.Errorf("Clone Len() = %d, 期望 %d", clone.Len(), bst.Len())
	}
	// 深拷贝：修改原树不影响克隆
	bst.Add(99)
	if clone.Len() != 5 {
		t.Errorf("修改原树后 Clone Len() = %d, 期望 5", clone.Len())
	}
	// 克隆应包含原有元素
	cloneBST := clone.(*tree.BST[int])
	if !cloneBST.Contains(3) {
		t.Error("克隆应包含原树的元素")
	}
}

func TestBST_Clone_Empty(t *testing.T) {
	bst := tree.NewBST[int]()
	clone := bst.Clone()
	if clone.Len() != 0 {
		t.Errorf("空树 Clone Len() = %d, 期望 0", clone.Len())
	}
}

// =============================================================================
// BST Equal 测试
// =============================================================================

func TestBST_Equal(t *testing.T) {
	tests := []struct {
		name      string
		a, b      []int
		wantEqual bool
	}{
		{name: "相同结构", a: []int{5, 3, 7}, b: []int{5, 3, 7}, wantEqual: true},
		{name: "不同结构", a: []int{5, 3, 7}, b: []int{3, 5, 7}, wantEqual: false},
		{name: "大小不同", a: []int{5, 3}, b: []int{5, 3, 7}, wantEqual: false},
		{name: "都为空", a: nil, b: nil, wantEqual: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tree.NewBST[int]()
			for _, v := range tt.a {
				a.Add(v)
			}
			b := tree.NewBST[int]()
			for _, v := range tt.b {
				b.Add(v)
			}
			if got := a.Equal(b); got != tt.wantEqual {
				t.Errorf("Equal() = %v, 期望 %v", got, tt.wantEqual)
			}
		})
	}
}

// =============================================================================
// BST Traversable（算法桥接）测试
// =============================================================================

func TestBST_Traversable_BFS(t *testing.T) {
	bst := tree.NewBST[int]()
	for _, v := range []int{5, 3, 7, 1, 4, 6, 8} {
		bst.Add(v)
	}
	var order []int
	traversal.BFS(bst, 5, func(v int) bool {
		order = append(order, v)
		return true
	})
	// BFS 层序：5, 3, 7, 1, 4, 6, 8
	if len(order) != 7 {
		t.Errorf("BFS 应访问 7 个节点, 实际 %d: %v", len(order), order)
	}
}

func TestBST_Traversable_DFS(t *testing.T) {
	bst := tree.NewBST[int]()
	for _, v := range []int{5, 3, 7, 1, 4, 6, 8} {
		bst.Add(v)
	}
	var order []int
	traversal.DFS(bst, 5, func(v int) bool {
		order = append(order, v)
		return true
	})
	if len(order) != 7 {
		t.Errorf("DFS 应访问 7 个节点, 实际 %d: %v", len(order), order)
	}
}

// =============================================================================
// BST 大数据量测试
// =============================================================================

func TestBST_LargeVolume(t *testing.T) {
	bst := tree.NewBST[int]()
	const n = 1000
	for i := 0; i < n; i++ {
		bst.Add(i)
	}
	if bst.Len() != n {
		t.Errorf("Len() = %d, 期望 %d", bst.Len(), n)
	}
	vals := bst.Values()
	if len(vals) != n {
		t.Errorf("Values() 长度 = %d, 期望 %d", len(vals), n)
	}
	for i := 0; i < n; i++ {
		if vals[i] != i {
			t.Errorf("Values()[%d] = %d, 期望 %d", i, vals[i], i)
		}
	}
}

// =============================================================================
// BST String 测试
// =============================================================================

func TestBST_String(t *testing.T) {
	bst := tree.NewBST[int]()
	bst.Add(2)
	bst.Add(1)
	bst.Add(3)
	s := bst.String()
	if s == "" {
		t.Error("String() 不应返回空字符串")
	}
}

// =============================================================================
// BST Add 后 Len 一致性
// =============================================================================

func TestBST_LenConsistency(t *testing.T) {
	bst := tree.NewBST[int]()
	n := 50
	for i := 0; i < n; i++ {
		bst.Add(i)
		if bst.Len() != i+1 {
			t.Errorf("第 %d 次 Add 后 Len() = %d, 期望 %d", i, bst.Len(), i+1)
		}
	}
	for i := 0; i < n; i++ {
		bst.Remove(i)
		if bst.Len() != n-i-1 {
			t.Errorf("第 %d 次 Remove 后 Len() = %d, 期望 %d", i, bst.Len(), n-i-1)
		}
	}
}
