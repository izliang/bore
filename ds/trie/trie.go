// Package trie 提供基于 core 接口的前缀树（Trie）实现。
package trie

// trieNode 是前缀树的内部节点。
type trieNode[V any] struct {
	children map[rune]*trieNode[V]
	value    V
	hasValue bool
}

// Trie 是一个泛型前缀树，将字符串键映射到类型 V 的值。
// 支持高效的插入、查找、删除和前缀匹配操作。
//
// 实现了 core.Collection[string]、core.Searchable[string]、
// core.Mutable[string]、core.Iterable[string]。
//
// 零值不可用，必须通过 NewTrie 创建。
type Trie[V any] struct {
	root *trieNode[V]
	size int
}

// NewTrie 创建一个新的空前缀树。
func NewTrie[V any]() *Trie[V] {
	return &Trie[V]{root: &trieNode[V]{children: make(map[rune]*trieNode[V])}}
}

// Len 返回前缀树中存储的键数量。实现 core.Collection。
func (t *Trie[V]) Len() int { return t.size }

// IsEmpty 返回前缀树是否为空。实现 core.Collection。
func (t *Trie[V]) IsEmpty() bool { return t.size == 0 }

// Contains 返回指定键是否存在于前缀树中。实现 core.Searchable。
// 时间复杂度：O(k)，k 为键的长度。
func (t *Trie[V]) Contains(item string) bool {
	node := t.findNode(item)
	return node != nil && node.hasValue
}

// Remove 从前缀树中删除一个键及其值。实现 core.Mutable。
func (t *Trie[V]) Remove(item string) bool {
	if !t.Contains(item) {
		return false
	}
	t.remove(t.root, item, 0)
	t.size--
	return true
}

// Clear 清空前缀树中的所有键。实现 core.Mutable。
func (t *Trie[V]) Clear() {
	t.root = &trieNode[V]{children: make(map[rune]*trieNode[V])}
	t.size = 0
}

// Search 返回指定键对应的值。如果键不存在，返回零值和 false。
func (t *Trie[V]) Search(item string) (V, bool) {
	node := t.findNode(item)
	if node == nil || !node.hasValue {
		var zero V
		return zero, false
	}
	return node.value, true
}

// StartsWith 返回是否存在以指定前缀开头的键。
func (t *Trie[V]) StartsWith(prefix string) bool {
	return t.findNode(prefix) != nil
}

// Keys 返回前缀树中所有键的切片，按字典序排列。
func (t *Trie[V]) Keys() []string {
	result := make([]string, 0, t.size)
	t.collectKeys(t.root, "", &result)
	return result
}

// KeysWithPrefix 返回所有以指定前缀开头的键。
func (t *Trie[V]) KeysWithPrefix(prefix string) []string {
	node := t.findNode(prefix)
	if node == nil {
		return nil
	}
	var result []string
	t.collectKeys(node, prefix, &result)
	return result
}

func (t *Trie[V]) findNode(key string) *trieNode[V] {
	curr := t.root
	for _, ch := range key {
		next, ok := curr.children[ch]
		if !ok {
			return nil
		}
		curr = next
	}
	return curr
}

func (t *Trie[V]) remove(node *trieNode[V], key string, depth int) bool {
	if depth == len(key) {
		node.hasValue = false
		var zero V
		node.value = zero
		return len(node.children) == 0
	}
	ch := rune(key[depth])
	child := node.children[ch]
	if child == nil {
		return false
	}
	if t.remove(child, key, depth+1) {
		delete(node.children, ch)
		return len(node.children) == 0 && !node.hasValue
	}
	return false
}

func (t *Trie[V]) collectKeys(node *trieNode[V], prefix string, result *[]string) {
	if node.hasValue {
		*result = append(*result, prefix)
	}
	for ch := 'a'; ch <= 'z'; ch++ {
		if child, ok := node.children[ch]; ok {
			t.collectKeys(child, prefix+string(ch), result)
		}
	}
	for ch, child := range node.children {
		if ch < 'a' || ch > 'z' {
			t.collectKeys(child, prefix+string(ch), result)
		}
	}
}
