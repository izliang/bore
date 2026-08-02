package tree

import (
	"cmp"

	core "github.com/izliang/bore/ds/core"
)

// bstIterator 是 BST 的中序遍历迭代器。
// 使用显式栈实现，确保 O(h) 空间复杂度和 O(1) 摊还时间。
type bstIterator[T cmp.Ordered] struct {
	stack []*bstNode[T]
}

// newBSTIterator 创建一个从根节点开始的中序迭代器。
func newBSTIterator[T cmp.Ordered](root *bstNode[T]) *bstIterator[T] {
	it := &bstIterator[T]{}
	it.pushLeft(root)
	return it
}

// HasNext 返回是否还有更多元素。实现 core.Iterator。
func (it *bstIterator[T]) HasNext() bool {
	return len(it.stack) > 0
}

// Next 返回下一个中序元素。实现 core.Iterator。
func (it *bstIterator[T]) Next() (T, bool) {
	if len(it.stack) == 0 {
		var zero T
		return zero, false
	}
	idx := len(it.stack) - 1
	node := it.stack[idx]
	it.stack = it.stack[:idx]
	it.pushLeft(node.right)
	return node.value, true
}

// pushLeft 将节点及其所有左子节点压入栈。
func (it *bstIterator[T]) pushLeft(node *bstNode[T]) {
	for node != nil {
		it.stack = append(it.stack, node)
		node = node.left
	}
}

// Iterator 返回一个中序遍历迭代器。实现 core.Iterable。
func (t *BST[T]) Iterator() core.Iterator[T] {
	return newBSTIterator(t.root)
}
