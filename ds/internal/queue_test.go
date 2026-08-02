package internal_test

import (
	"testing"

	"github.com/izliang/bore/ds/internal"
)

// =============================================================================
// Queue 基础功能测试
// =============================================================================

func TestQueueNew(t *testing.T) {
	q := internal.NewQueue[int]()
	if q == nil {
		t.Fatal("NewQueue 不应返回 nil")
	}
	if !q.IsEmpty() {
		t.Error("新队列应为空")
	}
	if q.Len() != 0 {
		t.Errorf("新队列 Len() = %d, 期望 0", q.Len())
	}
}

func TestQueueNewWithCapacity(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
	}{
		{name: "正常容量", capacity: 16},
		{name: "容量为 1", capacity: 1},
		{name: "容量为 0 时应回退", capacity: 0},
		{name: "负容量应回退", capacity: -5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := internal.NewQueueWithCapacity[int](tt.capacity)
			if q == nil {
				t.Fatal("NewQueueWithCapacity 不应返回 nil")
			}
			if q.Len() != 0 {
				t.Errorf("Len() = %d, 期望 0", q.Len())
			}
		})
	}
}

func TestQueueEnqueueDequeue(t *testing.T) {
	tests := []struct {
		name  string
		items []int
	}{
		{name: "空队列", items: nil},
		{name: "单元素", items: []int{42}},
		{name: "三个元素", items: []int{1, 2, 3}},
		{name: "十个元素（触发扩容）", items: []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := internal.NewQueue[int]()
			for _, item := range tt.items {
				q.Enqueue(item)
			}
			if q.Len() != len(tt.items) {
				t.Errorf("入队后 Len() = %d, 期望 %d", q.Len(), len(tt.items))
			}
			for i, want := range tt.items {
				got, ok := q.Dequeue()
				if !ok {
					t.Fatalf("第 %d 次 Dequeue: ok=false", i)
				}
				if got != want {
					t.Errorf("第 %d 次 Dequeue = %v, 期望 %v", i, got, want)
				}
			}
			if !q.IsEmpty() {
				t.Error("全部出队后 IsEmpty() 应为 true")
			}
		})
	}
}

func TestQueueDequeueEmpty(t *testing.T) {
	q := internal.NewQueue[string]()
	got, ok := q.Dequeue()
	if ok {
		t.Errorf("空队列 Dequeue 应返回 ok=false，得到 %v", got)
	}
	var zero string
	if got != zero {
		t.Errorf("空队列 Dequeue 应返回零值，得到 %v", got)
	}
}

func TestQueuePeek(t *testing.T) {
	t.Run("查看非空队列", func(t *testing.T) {
		q := internal.NewQueue[int]()
		q.Enqueue(10)
		q.Enqueue(20)
		got, ok := q.Peek()
		if !ok || got != 10 {
			t.Errorf("Peek() = (%v, %v), 期望 (10, true)", got, ok)
		}
		if q.Len() != 2 {
			t.Error("Peek 不应移除元素")
		}
	})

	t.Run("查看空队列", func(t *testing.T) {
		q := internal.NewQueue[int]()
		got, ok := q.Peek()
		if ok {
			t.Errorf("空队列 Peek 应返回 ok=false，得到 %v", got)
		}
	})
}

func TestQueueClear(t *testing.T) {
	q := internal.NewQueue[int]()
	q.Enqueue(1)
	q.Enqueue(2)
	q.Enqueue(3)
	q.Clear()

	if q.Len() != 0 {
		t.Errorf("Clear 后 Len() = %d, 期望 0", q.Len())
	}
	if !q.IsEmpty() {
		t.Error("Clear 后 IsEmpty() 应为 true")
	}
	_, ok := q.Dequeue()
	if ok {
		t.Error("Clear 后 Dequeue 应返回 false")
	}
}

func TestQueueClearTwice(t *testing.T) {
	q := internal.NewQueue[int]()
	q.Clear()
	q.Clear() // 不应 panic
	if q.Len() != 0 {
		t.Error("重复 Clear 后 Len() 应为 0")
	}
}

// =============================================================================
// Queue 环形缓冲区行为测试
// =============================================================================

func TestQueueRingBufferWrap(t *testing.T) {
	// 通过交替入队出队触发环形缓冲区绕回
	q := internal.NewQueueWithCapacity[int](4)

	// 填满：head=0, tail=0, size=4
	for i := 0; i < 4; i++ {
		q.Enqueue(i * 10)
	}

	// 出队两个：head=2, tail=0, size=2
	for i := 0; i < 2; i++ {
		v, ok := q.Dequeue()
		if !ok || v != i*10 {
			t.Fatalf("Dequeue() = (%v, %v), 期望 (%v, true)", v, ok, i*10)
		}
	}

	// 再入队三个（触发绕回）：head=2, tail=3, 触发扩容
	q.Enqueue(40)
	q.Enqueue(50)
	q.Enqueue(60) // 此时 buf 已满，触发 grow

	expected := []int{20, 30, 40, 50, 60}
	for i, want := range expected {
		got, ok := q.Dequeue()
		if !ok {
			t.Fatalf("第 %d 次 Dequeue: ok=false", i)
		}
		if got != want {
			t.Errorf("第 %d 次 Dequeue = %v, 期望 %v", i, got, want)
		}
	}
}

// =============================================================================
// Queue 不同类型测试
// =============================================================================

func TestQueueWithString(t *testing.T) {
	q := internal.NewQueue[string]()
	q.Enqueue("hello")
	q.Enqueue("world")

	got, _ := q.Dequeue()
	if got != "hello" {
		t.Errorf("Dequeue() = %q, 期望 %q", got, "hello")
	}
	got, _ = q.Dequeue()
	if got != "world" {
		t.Errorf("Dequeue() = %q, 期望 %q", got, "world")
	}
}

func TestQueueWithStruct(t *testing.T) {
	type point struct{ x, y int }
	q := internal.NewQueue[point]()
	q.Enqueue(point{1, 2})
	q.Enqueue(point{3, 4})

	p, ok := q.Dequeue()
	if !ok || p.x != 1 || p.y != 2 {
		t.Errorf("Dequeue() = (%v, %v), 期望 ({1, 2}, true)", p, ok)
	}
}

// =============================================================================
// Queue 大数据量测试
// =============================================================================

func TestQueueLargeVolume(t *testing.T) {
	q := internal.NewQueue[int]()
	const n = 10000
	for i := 0; i < n; i++ {
		q.Enqueue(i)
	}
	if q.Len() != n {
		t.Errorf("Len() = %d, 期望 %d", q.Len(), n)
	}
	for i := 0; i < n; i++ {
		v, ok := q.Dequeue()
		if !ok || v != i {
			t.Fatalf("第 %d 次 Dequeue = (%v, %v), 期望 (%d, true)", i, v, ok, i)
		}
	}
}

// =============================================================================
// Queue 交替操作测试
// =============================================================================

func TestQueueInterleaved(t *testing.T) {
	q := internal.NewQueue[int]()

	q.Enqueue(1)
	q.Enqueue(2)
	v, _ := q.Dequeue() // 1
	if v != 1 {
		t.Errorf("期望 1，得到 %d", v)
	}

	q.Enqueue(3)
	q.Enqueue(4)
	v, _ = q.Dequeue() // 2
	if v != 2 {
		t.Errorf("期望 2，得到 %d", v)
	}

	q.Enqueue(5)
	v, _ = q.Dequeue() // 3
	if v != 3 {
		t.Errorf("期望 3，得到 %d", v)
	}

	expected := []int{4, 5}
	for _, want := range expected {
		got, ok := q.Dequeue()
		if !ok || got != want {
			t.Errorf("Dequeue() = (%v, %v), 期望 (%v, true)", got, ok, want)
		}
	}
}

// =============================================================================
// Queue IsEmpty 一致性测试
// =============================================================================

func TestQueueLenIsEmptyConsistency(t *testing.T) {
	q := internal.NewQueue[int]()
	if (q.Len() == 0) != q.IsEmpty() {
		t.Error("空队列: Len()==0 与 IsEmpty() 不一致")
	}
	q.Enqueue(1)
	if (q.Len() == 0) != q.IsEmpty() {
		t.Error("非空队列: Len()==0 与 IsEmpty() 不一致")
	}
	q.Dequeue()
	if (q.Len() == 0) != q.IsEmpty() {
		t.Error("出队后变空: Len()==0 与 IsEmpty() 不一致")
	}
}
