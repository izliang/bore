// Package tree 提供基于 core 接口的树形数据结构实现。
package tree

import (
	"cmp"
	"fmt"
	"iter"

	core "github.com/izliang/bore/ds/core"
)

// bstNode 是二叉搜索树的内部节点。
type bstNode[T any] struct {
	value T
	left  *bstNode[T]
	right *bstNode[T]
}

// BST 是一个泛型二叉搜索树（Binary Search Tree）。
// 键类型 T 必须支持 < 和 == 操作（即满足 cmp.Ordered 约束）。
//
// 实现了以下 core 接口：
//   - core.Collection[T]
//   - core.Iterable[T]（中序遍历）
//   - core.Searchable[T]
//   - core.Mutable[T]
//   - core.Cloneable[T]
//   - core.Traversable[T]
//   - core.Comparable[T]
//
// 零值不可用，必须通过 NewBST 创建。
type BST[T cmp.Ordered] struct {
	root    *bstNode[T]
	size    int
	nodeMap map[T]*bstNode[T] // 支持 O(1) 的 Adjacent 查找
}

// NewBST 创建一个新的空二叉搜索树。
func NewBST[T cmp.Ordered]() *BST[T] {
	return &BST[T]{nodeMap: make(map[T]*bstNode[T])}
}

// Len 返回树中的节点数量。实现 core.Collection。
func (t *BST[T]) Len() int { return t.size }

// IsEmpty 返回树是否为空。实现 core.Collection。
func (t *BST[T]) IsEmpty() bool { return t.size == 0 }

// Contains 返回树中是否存在指定值。实现 core.Searchable。
// 时间复杂度：O(h)，h 为树高。
func (t *BST[T]) Contains(item T) bool {
	return t.find(item) != nil
}

// Add 向树中插入一个值。如果值已存在，则替换并返回 false。
// 实现 core.Mutable。
// 时间复杂度：O(h)。
func (t *BST[T]) Add(item T) bool {
	existed := t.Contains(item)
	t.root = t.insert(t.root, item)
	if !existed {
		t.size++
	}
	t.nodeMap[item] = t.find(item)
	return !existed
}

// Remove 从树中删除一个值。如果值不存在，则返回 false。
// 实现 core.Mutable。
// 时间复杂度：O(h)。
func (t *BST[T]) Remove(item T) bool {
	if t.find(item) == nil {
		return false
	}
	delete(t.nodeMap, item)
	t.root = t.remove(t.root, item)
	t.size--
	return true
}

// Clear 清空树中的所有节点。实现 core.Mutable。
func (t *BST[T]) Clear() {
	t.root = nil
	t.size = 0
	t.nodeMap = make(map[T]*bstNode[T])
}

// Clone 返回树的一个深拷贝。实现 core.Cloneable。
// 时间复杂度：O(n)。
func (t *BST[T]) Clone() core.Collection[T] {
	clone := NewBST[T]()
	clone.root = t.cloneNode(t.root)
	clone.size = t.size
	clone.rebuildNodeMap(clone.root)
	return clone
}

// Equal 返回两棵树是否包含相同的元素且结构相同。
// 实现 core.Comparable。
// 时间复杂度：O(n)。
func (t *BST[T]) Equal(other core.Collection[T]) bool {
	otherBST, ok := other.(*BST[T])
	if !ok {
		return false
	}
	if t.size != otherBST.size {
		return false
	}
	return t.equalNode(t.root, otherBST.root)
}

// Adjacent 返回指定节点的左右子节点。实现 core.Traversable。
// 通过内部映射 O(1) 查找目标节点，然后返回其子节点。
func (t *BST[T]) Adjacent(from T) iter.Seq[T] {
	node := t.nodeMap[from]
	return func(yield func(T) bool) {
		if node == nil {
			return
		}
		if node.left != nil {
			if !yield(node.left.value) {
				return
			}
		}
		if node.right != nil {
			yield(node.right.value)
		}
	}
}

// Values 返回中序遍历的所有值。
func (t *BST[T]) Values() []T {
	result := make([]T, 0, t.size)
	t.inorder(t.root, func(v T) bool {
		result = append(result, v)
		return true
	})
	return result
}

// Min 返回树中的最小值。如果树为空，返回零值和 false。
func (t *BST[T]) Min() (T, bool) {
	if t.root == nil {
		var zero T
		return zero, false
	}
	return t.minNode(t.root).value, true
}

// Max 返回树中的最大值。如果树为空，返回零值和 false。
func (t *BST[T]) Max() (T, bool) {
	if t.root == nil {
		var zero T
		return zero, false
	}
	return t.maxNode(t.root).value, true
}

// String 返回中序遍历的字符串表示。
func (t *BST[T]) String() string {
	return fmt.Sprintf("BST%v", t.Values())
}

// --- 内部方法 ---

func (t *BST[T]) find(item T) *bstNode[T] {
	curr := t.root
	for curr != nil {
		switch {
		case item < curr.value:
			curr = curr.left
		case item > curr.value:
			curr = curr.right
		default:
			return curr
		}
	}
	return nil
}

func (t *BST[T]) insert(node *bstNode[T], item T) *bstNode[T] {
	if node == nil {
		return &bstNode[T]{value: item}
	}
	switch {
	case item < node.value:
		node.left = t.insert(node.left, item)
	case item > node.value:
		node.right = t.insert(node.right, item)
	default:
		node.value = item // 替换
	}
	return node
}

func (t *BST[T]) remove(node *bstNode[T], item T) *bstNode[T] {
	if node == nil {
		return nil
	}
	switch {
	case item < node.value:
		node.left = t.remove(node.left, item)
	case item > node.value:
		node.right = t.remove(node.right, item)
	default:
		if node.left == nil {
			return node.right
		}
		if node.right == nil {
			return node.left
		}
		successor := t.minNode(node.right)
		node.value = successor.value
		node.right = t.remove(node.right, successor.value)
	}
	return node
}

func (t *BST[T]) minNode(node *bstNode[T]) *bstNode[T] {
	for node.left != nil {
		node = node.left
	}
	return node
}

func (t *BST[T]) maxNode(node *bstNode[T]) *bstNode[T] {
	for node.right != nil {
		node = node.right
	}
	return node
}

func (t *BST[T]) inorder(node *bstNode[T], visit func(T) bool) bool {
	if node == nil {
		return true
	}
	if !t.inorder(node.left, visit) {
		return false
	}
	if !visit(node.value) {
		return false
	}
	return t.inorder(node.right, visit)
}

func (t *BST[T]) cloneNode(node *bstNode[T]) *bstNode[T] {
	if node == nil {
		return nil
	}
	return &bstNode[T]{
		value: node.value,
		left:  t.cloneNode(node.left),
		right: t.cloneNode(node.right),
	}
}

func (t *BST[T]) rebuildNodeMap(node *bstNode[T]) {
	if node == nil {
		return
	}
	t.nodeMap[node.value] = node
	t.rebuildNodeMap(node.left)
	t.rebuildNodeMap(node.right)
}

func (t *BST[T]) equalNode(a, b *bstNode[T]) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.value == b.value &&
		t.equalNode(a.left, b.left) &&
		t.equalNode(a.right, b.right)
}
