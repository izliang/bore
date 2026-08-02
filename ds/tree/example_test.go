package tree_test

import (
	"fmt"

	"github.com/izliang/bore/ds/algorithm/traversal"
	"github.com/izliang/bore/ds/tree"
)

// ExampleBST 演示二叉搜索树的基本用法。
func ExampleBST() {
	bst := tree.NewBST[int]()
	bst.Add(5)
	bst.Add(3)
	bst.Add(7)
	bst.Add(1)
	bst.Add(4)

	fmt.Printf("大小: %d\n", bst.Len())
	fmt.Printf("包含 3: %v\n", bst.Contains(3))
	fmt.Printf("包含 9: %v\n", bst.Contains(9))
	fmt.Printf("中序: %v\n", bst.Values())
	min, _ := bst.Min()
	max, _ := bst.Max()
	fmt.Printf("最小: %d, 最大: %d\n", min, max)
	// Output:
	// 大小: 5
	// 包含 3: true
	// 包含 9: false
	// 中序: [1 3 4 5 7]
	// 最小: 1, 最大: 7
}

// Example_bstBFS 演示 BST 结合 BFS 算法使用。
func Example_bstBFS() {
	bst := tree.NewBST[int]()
	bst.Add(5)
	bst.Add(3)
	bst.Add(7)
	bst.Add(1)
	bst.Add(4)

	var order []int
	traversal.BFS(bst, 5, func(v int) bool {
		order = append(order, v)
		return true
	})
	fmt.Println(order)
	// Output:
	// [5 3 7 1 4]
}
