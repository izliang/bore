package trie

import core "github.com/izliang/bore/ds/core"

// Add 插入一个键并以类型 V 的零值作为其值。实现 core.Mutable[string]。
func (t *Trie[V]) Add(item string) bool {
	var zero V
	return t.addValue(item, zero)
}

// Put 插入一个键值对。如果键已存在，则替换值并返回 false。
// 与 Add 不同，Put 允许设置键对应的值。
func (t *Trie[V]) Put(item string, value V) bool {
	return t.addValue(item, value)
}

func (t *Trie[V]) addValue(item string, value V) bool {
	curr := t.root
	for _, ch := range item {
		next, ok := curr.children[ch]
		if !ok {
			next = &trieNode[V]{children: make(map[rune]*trieNode[V])}
			curr.children[ch] = next
		}
		curr = next
	}
	existed := curr.hasValue
	curr.value = value
	curr.hasValue = true
	if !existed {
		t.size++
	}
	return !existed
}

// trieIterator 是 Trie 的字典序迭代器。
type trieIterator[V any] struct {
	keys []string
	pos  int
}

// HasNext 返回是否还有更多元素。实现 core.Iterator。
func (it *trieIterator[V]) HasNext() bool { return it.pos < len(it.keys) }

// Next 返回下一个键。实现 core.Iterator。
func (it *trieIterator[V]) Next() (string, bool) {
	if it.pos >= len(it.keys) {
		var zero string
		return zero, false
	}
	v := it.keys[it.pos]
	it.pos++
	return v, true
}

// Iterator 返回一个按字典序遍历所有键的迭代器。实现 core.Iterable[string]。
func (t *Trie[V]) Iterator() core.Iterator[string] {
	return &trieIterator[V]{keys: t.Keys()}
}
