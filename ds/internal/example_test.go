package internal_test

import (
	"fmt"

	"github.com/izliang/bore/ds/internal"
)

// ExampleQueue 演示 FIFO 队列的基本用法。
func ExampleQueue() {
	q := internal.NewQueue[string]()
	q.Enqueue("第一个")
	q.Enqueue("第二个")
	q.Enqueue("第三个")

	for !q.IsEmpty() {
		v, _ := q.Dequeue()
		fmt.Println(v)
	}
	// Output:
	// 第一个
	// 第二个
	// 第三个
}

// ExampleStack 演示 LIFO 栈的基本用法。
func ExampleStack() {
	s := internal.NewStack[string]()
	s.Push("第一个")
	s.Push("第二个")
	s.Push("第三个")

	for !s.IsEmpty() {
		v, _ := s.Pop()
		fmt.Println(v)
	}
	// Output:
	// 第三个
	// 第二个
	// 第一个
}
