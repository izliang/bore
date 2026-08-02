package trie_test

import (
	"testing"

	core "github.com/izliang/bore/ds/core"
	"github.com/izliang/bore/ds/trie"
)

var (
	_ core.Collection[string] = (*trie.Trie[int])(nil)
	_ core.Searchable[string] = (*trie.Trie[int])(nil)
	_ core.Mutable[string]    = (*trie.Trie[int])(nil)
	_ core.Iterable[string]   = (*trie.Trie[int])(nil)
)

func TestTrie_New(t *testing.T) {
	tr := trie.NewTrie[int]()
	if !tr.IsEmpty() {
		t.Error("新 Trie 应为空")
	}
	if tr.Len() != 0 {
		t.Errorf("Len() = %d, 期望 0", tr.Len())
	}
}

func TestTrie_AddAndContains(t *testing.T) {
	tests := []struct {
		name string
		keys []string
	}{
		{name: "单个键", keys: []string{"hello"}},
		{name: "多个键", keys: []string{"a", "ab", "abc", "abcd"}},
		{name: "重叠前缀", keys: []string{"app", "apple", "application"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := trie.NewTrie[int]()
			for _, key := range tt.keys {
				if !tr.Add(key) {
					t.Errorf("首次 Add(%q) 应返回 true", key)
				}
			}
			for _, key := range tt.keys {
				if !tr.Contains(key) {
					t.Errorf("Contains(%q) 应为 true", key)
				}
			}
		})
	}
}

func TestTrie_AddDuplicate(t *testing.T) {
	tr := trie.NewTrie[int]()
	if !tr.Add("key") {
		t.Error("首次 Add 应返回 true")
	}
	if tr.Add("key") {
		t.Error("重复 Add 应返回 false")
	}
	if tr.Len() != 1 {
		t.Errorf("Len() = %d, 期望 1", tr.Len())
	}
}

func TestTrie_Put(t *testing.T) {
	tr := trie.NewTrie[int]()
	tr.Put("age", 25)
	tr.Put("year", 2024)

	v, ok := tr.Search("age")
	if !ok || v != 25 {
		t.Errorf("Search(age) = (%d, %v), 期望 (25, true)", v, ok)
	}
	v, ok = tr.Search("year")
	if !ok || v != 2024 {
		t.Errorf("Search(year) = (%d, %v), 期望 (2024, true)", v, ok)
	}
}

func TestTrie_SearchMissing(t *testing.T) {
	tr := trie.NewTrie[int]()
	_, ok := tr.Search("nope")
	if ok {
		t.Error("Search 不存在的键应返回 false")
	}
}

func TestTrie_ContainsMissing(t *testing.T) {
	tr := trie.NewTrie[int]()
	tr.Add("apple")
	if tr.Contains("app") {
		t.Error("app 是前缀但不是键，Contains 应为 false")
	}
	if tr.Contains("application") {
		t.Error("不存在的键 Contains 应为 false")
	}
}

func TestTrie_Remove(t *testing.T) {
	tests := []struct {
		name   string
		keys   []string
		remove string
		wantOk bool
	}{
		{name: "删除叶子", keys: []string{"a", "ab"}, remove: "ab", wantOk: true},
		{name: "删除非叶子", keys: []string{"ab", "abc"}, remove: "ab", wantOk: true},
		{name: "删除不存在", keys: []string{"a"}, remove: "x", wantOk: false},
		{name: "空树删除", keys: nil, remove: "x", wantOk: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := trie.NewTrie[int]()
			for _, k := range tt.keys {
				tr.Add(k)
			}
			got := tr.Remove(tt.remove)
			if got != tt.wantOk {
				t.Errorf("Remove(%q) = %v, 期望 %v", tt.remove, got, tt.wantOk)
			}
			if tt.wantOk && tr.Contains(tt.remove) {
				t.Error("Remove 后 Contains 应为 false")
			}
		})
	}
}

func TestTrie_Clear(t *testing.T) {
	tr := trie.NewTrie[int]()
	tr.Add("a")
	tr.Add("b")
	tr.Clear()
	if tr.Len() != 0 {
		t.Error("Clear 后 Len() 应为 0")
	}
	if !tr.IsEmpty() {
		t.Error("Clear 后 IsEmpty() 应为 true")
	}
}

func TestTrie_StartsWith(t *testing.T) {
	tr := trie.NewTrie[int]()
	tr.Add("hello")
	tr.Add("help")
	tr.Add("world")

	if !tr.StartsWith("hel") {
		t.Error("StartsWith(hel) 应为 true")
	}
	if !tr.StartsWith("hello") {
		t.Error("StartsWith(hello) 应为 true（完整键也是前缀）")
	}
	if tr.StartsWith("xyz") {
		t.Error("StartsWith(xyz) 应为 false")
	}
	if tr.StartsWith("helloa") {
		t.Error("StartsWith(helloa) 应为 false")
	}
}

func TestTrie_Keys(t *testing.T) {
	tr := trie.NewTrie[int]()
	input := []string{"c", "a", "b", "ab", "aa"}
	for _, k := range input {
		tr.Add(k)
	}
	keys := tr.Keys()
	if len(keys) != len(input) {
		t.Fatalf("Keys() 长度 = %d, 期望 %d", len(keys), len(input))
	}
	// 验证字典序
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Errorf("Keys 未按字典序: %v", keys)
			break
		}
	}
}

func TestTrie_KeysWithPrefix(t *testing.T) {
	tr := trie.NewTrie[int]()
	tr.Add("app")
	tr.Add("apple")
	tr.Add("application")
	tr.Add("banana")

	got := tr.KeysWithPrefix("app")
	if len(got) != 3 {
		t.Errorf("KeysWithPrefix(app) = %v, 期望 3 个", got)
	}
}

func TestTrie_Iterator(t *testing.T) {
	tr := trie.NewTrie[int]()
	tr.Add("go")
	tr.Add("rust")
	tr.Add("zig")

	it := tr.Iterator()
	var got []string
	for it.HasNext() {
		v, ok := it.Next()
		if !ok {
			t.Fatal("HasNext 为 true 时 Next 不应返回 false")
		}
		got = append(got, v)
	}
	if len(got) != 3 {
		t.Errorf("Iterator 应返回 3 个键, 实际 %d", len(got))
	}
}

func TestTrie_IteratorExhausted(t *testing.T) {
	tr := trie.NewTrie[int]()
	it := tr.Iterator()
	if it.HasNext() {
		t.Error("空迭代器 HasNext() 应为 false")
	}
	_, ok := it.Next()
	if ok {
		t.Error("空迭代器 Next() 应返回 false")
	}
}

func TestTrie_Unicode(t *testing.T) {
	tr := trie.NewTrie[int]()
	tr.Add("你好")
	tr.Add("世界")
	if !tr.Contains("你好") {
		t.Error("包含中文字符的键应可查询")
	}
	if tr.Len() != 2 {
		t.Errorf("Len() = %d, 期望 2", tr.Len())
	}
}
