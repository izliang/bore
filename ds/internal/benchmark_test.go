package internal_test

import (
	"testing"

	"github.com/izliang/bore/ds/internal"
)

// =============================================================================
// Queue Benchmark
// =============================================================================

func BenchmarkQueueEnqueue(b *testing.B) {
	q := internal.NewQueue[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
	}
}

func BenchmarkQueueDequeue(b *testing.B) {
	q := internal.NewQueue[int]()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Dequeue()
	}
}

func BenchmarkQueueEnqueueDequeue(b *testing.B) {
	q := internal.NewQueue[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
		q.Dequeue()
	}
}

func BenchmarkQueueEnqueueDequeueLarge(b *testing.B) {
	q := internal.NewQueueWithCapacity[int](b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(i)
	}
	for i := 0; i < b.N; i++ {
		q.Dequeue()
	}
}

// =============================================================================
// Stack Benchmark
// =============================================================================

func BenchmarkStackPush(b *testing.B) {
	s := internal.NewStack[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Push(i)
	}
}

func BenchmarkStackPop(b *testing.B) {
	s := internal.NewStack[int]()
	for i := 0; i < b.N; i++ {
		s.Push(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Pop()
	}
}

func BenchmarkStackPushPop(b *testing.B) {
	s := internal.NewStack[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Push(i)
		s.Pop()
	}
}
