package collection_test

import (
	"testing"

	"github.com/izliang/bore/ds/collection"
)

// =============================================================================
// LinkedList 测试
// =============================================================================

func TestLinkedList_New(t *testing.T) {
	ll := collection.NewLinkedList[int]()
	if !ll.IsEmpty() {
		t.Error("新链表应为空")
	}
	if ll.Len() != 0 {
		t.Errorf("Len() = %d, 期望 0", ll.Len())
	}
}

func TestLinkedList_Add(t *testing.T) {
	ll := collection.NewLinkedList[int]()
	ll.Add(1)
	ll.Add(2)
	ll.Add(3)
	if ll.Len() != 3 {
		t.Errorf("Len() = %d, 期望 3", ll.Len())
	}
	vals := ll.Values()
	if len(vals) != 3 || vals[0] != 1 || vals[2] != 3 {
		t.Errorf("Values() = %v, 期望 [1 2 3]", vals)
	}
}

func TestLinkedList_AddFirst(t *testing.T) {
	ll := collection.NewLinkedList[int]()
	ll.Add(2)
	ll.AddFirst(1)
	if v, _ := ll.First(); v != 1 {
		t.Errorf("First = %d, 期望 1", v)
	}
}

func TestLinkedList_FirstLast(t *testing.T) {
	t.Run("空链表", func(t *testing.T) {
		ll := collection.NewLinkedList[int]()
		_, ok := ll.First()
		if ok {
			t.Error("空链表 First() 应返回 false")
		}
	})
	t.Run("非空", func(t *testing.T) {
		ll := collection.NewLinkedList[int]()
		ll.Add(10)
		ll.Add(20)
		f, _ := ll.First()
		l, _ := ll.Last()
		if f != 10 || l != 20 {
			t.Errorf("First=%d Last=%d", f, l)
		}
	})
}

func TestLinkedList_Remove(t *testing.T) {
	ll := collection.NewLinkedList[int]()
	ll.Add(1)
	ll.Add(2)
	ll.Add(3)
	if !ll.Remove(2) {
		t.Error("Remove(2) 应返回 true")
	}
	vals := ll.Values()
	if len(vals) != 2 || vals[0] != 1 || vals[1] != 3 {
		t.Errorf("Values() = %v, 期望 [1 3]", vals)
	}
	if ll.Remove(99) {
		t.Error("Remove(99) 应返回 false")
	}
}

func TestLinkedList_Iterator(t *testing.T) {
	ll := collection.NewLinkedList[string]()
	ll.Add("a")
	ll.Add("b")
	it := ll.Iterator()
	var got []string
	for it.HasNext() {
		v, _ := it.Next()
		got = append(got, v)
	}
	if len(got) != 2 || got[0] != "a" {
		t.Errorf("Iterator = %v", got)
	}
}

func TestLinkedList_Clear(t *testing.T) {
	ll := collection.NewLinkedList[int]()
	ll.Add(1)
	ll.Clear()
	if ll.Len() != 0 {
		t.Error("Clear 后 Len() 应为 0")
	}
}

// =============================================================================
// Set 测试
// =============================================================================

func TestSet_New(t *testing.T) {
	s := collection.NewSet[int]()
	if !s.IsEmpty() {
		t.Error("新集合应为空")
	}
}

func TestSet_AddAndContains(t *testing.T) {
	s := collection.NewSet[int]()
	s.Add(1)
	s.Add(2)
	s.Add(3)
	if !s.Contains(2) {
		t.Error("Contains(2) 应为 true")
	}
	if s.Contains(99) {
		t.Error("Contains(99) 应为 false")
	}
	if s.Len() != 3 {
		t.Errorf("Len() = %d, 期望 3", s.Len())
	}
}

func TestSet_AddDuplicate(t *testing.T) {
	s := collection.NewSet[int]()
	if !s.Add(5) {
		t.Error("首次 Add(5) 应返回 true")
	}
	if s.Add(5) {
		t.Error("重复 Add(5) 应返回 false")
	}
	if s.Len() != 1 {
		t.Error("重复 Add 不应增加数量")
	}
}

func TestSet_Remove(t *testing.T) {
	s := collection.NewSet[int]()
	s.Add(1)
	s.Add(2)
	if !s.Remove(1) {
		t.Error("Remove(1) 应返回 true")
	}
	if s.Contains(1) {
		t.Error("Remove 后 Contains 应为 false")
	}
	if s.Len() != 1 {
		t.Errorf("Len() = %d, 期望 1", s.Len())
	}
}

func TestSet_Clear(t *testing.T) {
	s := collection.NewSet[int]()
	s.Add(1)
	s.Clear()
	if s.Len() != 0 {
		t.Error("Clear 后 Len() 应为 0")
	}
}

func TestSet_Clone(t *testing.T) {
	s := collection.NewSet[int]()
	s.Add(1)
	s.Add(2)
	clone := s.Clone()
	if clone.Len() != 2 {
		t.Errorf("Clone Len() = %d", clone.Len())
	}
	s.Add(99)
	if clone.Len() != 2 {
		t.Error("修改原集不应影响克隆")
	}
}

func TestSet_Values(t *testing.T) {
	s := collection.NewSet[int]()
	s.Add(3)
	s.Add(1)
	s.Add(2)
	vals := s.Values()
	if len(vals) != 3 {
		t.Errorf("Values() 长度 = %d", len(vals))
	}
}

func TestSet_Iterator(t *testing.T) {
	s := collection.NewSet[int]()
	s.Add(1)
	s.Add(2)
	it := s.Iterator()
	count := 0
	for it.HasNext() {
		_, ok := it.Next()
		if !ok {
			t.Fatal("HasNext=true 时 Next 不应返回 false")
		}
		count++
	}
	if count != 2 {
		t.Errorf("Iterator 返回 %d 个, 期望 2", count)
	}
}

// =============================================================================
// Deque 测试
// =============================================================================

func TestDeque_New(t *testing.T) {
	d := collection.NewDeque[int]()
	if !d.IsEmpty() {
		t.Error("新 Deque 应为空")
	}
}

func TestDeque_AddRemove(t *testing.T) {
	d := collection.NewDeque[int]()
	d.AddFirst(1)
	d.AddLast(2)
	d.AddFirst(0)

	// 应为 [0, 1, 2]
	v, _ := d.RemoveFirst()
	if v != 0 {
		t.Errorf("RemoveFirst = %d, 期望 0", v)
	}
	v, _ = d.RemoveLast()
	if v != 2 {
		t.Errorf("RemoveLast = %d, 期望 2", v)
	}
	v, _ = d.RemoveFirst()
	if v != 1 {
		t.Errorf("RemoveFirst = %d, 期望 1", v)
	}
	if d.Len() != 0 {
		t.Error("全部移除后 Len() 应为 0")
	}
}

func TestDeque_EmptyOps(t *testing.T) {
	d := collection.NewDeque[int]()
	_, ok := d.RemoveFirst()
	if ok {
		t.Error("空 Deque RemoveFirst 应返回 false")
	}
	_, ok = d.RemoveLast()
	if ok {
		t.Error("空 Deque RemoveLast 应返回 false")
	}
	_, ok = d.PeekFirst()
	if ok {
		t.Error("空 Deque PeekFirst 应返回 false")
	}
}

func TestDeque_Peek(t *testing.T) {
	d := collection.NewDeque[int]()
	d.AddFirst(1)
	d.AddLast(2)
	v, _ := d.PeekFirst()
	if v != 1 {
		t.Errorf("PeekFirst = %d, 期望 1", v)
	}
	v, _ = d.PeekLast()
	if v != 2 {
		t.Errorf("PeekLast = %d, 期望 2", v)
	}
	if d.Len() != 2 {
		t.Error("Peek 不应移除元素")
	}
}

func TestDeque_Clear(t *testing.T) {
	d := collection.NewDeque[int]()
	d.Add(1)
	d.Add(2)
	d.Clear()
	if d.Len() != 0 {
		t.Error("Clear 后 Len() 应为 0")
	}
}

func TestDeque_Interleaved(t *testing.T) {
	d := collection.NewDeque[int]()
	d.AddFirst(1)          // [1]
	d.AddLast(2)           // [1, 2]
	d.AddFirst(0)          // [0, 1, 2]
	v, _ := d.RemoveLast() // 移除 2
	if v != 2 {
		t.Error("期望 2")
	}
	d.AddLast(3)           // [0, 1, 3]
	v, _ = d.RemoveFirst() // 移除 0
	if v != 0 {
		t.Error("期望 0")
	}
	v, _ = d.RemoveFirst() // 移除 1
	if v != 1 {
		t.Error("期望 1")
	}
	v, _ = d.RemoveFirst() // 移除 3
	if v != 3 {
		t.Error("期望 3")
	}
}

func TestDeque_LargeVolume(t *testing.T) {
	d := collection.NewDeque[int]()
	const n = 1000
	for i := 0; i < n; i++ {
		d.AddLast(i)
	}
	for i := 0; i < n; i++ {
		v, ok := d.RemoveFirst()
		if !ok || v != i {
			t.Fatalf("RemoveFirst = (%d, %v), 期望 (%d, true)", v, ok, i)
		}
	}
}

func TestDeque_Mutable_Remove(t *testing.T) {
	d := collection.NewDeque[int]()
	d.Add(1)
	d.Add(2)
	d.Remove(0) // Mutable.Remove - 移除队首
	if d.Len() != 1 {
		t.Errorf("Remove 后 Len() = %d, 期望 1", d.Len())
	}
}
