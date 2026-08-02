package core_test

import (
	"fmt"

	core "github.com/izliang/bore/ds/core"
)

// Example_sliceCollectionAsIterable 演示如何使用 Iterable 接口遍历集合。
func Example_sliceCollectionAsIterable() {
	c := &sliceCollection[string]{data: []string{"a", "b", "c"}}
	it := c.Iterator()
	for it.HasNext() {
		v, _ := it.Next()
		fmt.Println(v)
	}
	// Output:
	// a
	// b
	// c
}

// ExampleOption_some 演示创建和读取有值的 Option。
func ExampleOption_some() {
	opt := core.Some(42)
	fmt.Println(opt.String())
	fmt.Println(opt.IsPresent())
	v, _ := opt.Get()
	fmt.Println(v)
	// Output:
	// Some(42)
	// true
	// 42
}

// ExampleOption_none 演示空 Option 的回退行为。
func ExampleOption_none() {
	opt := core.None[string]()
	fmt.Println(opt.String())
	fmt.Println(opt.IsPresent())
	fmt.Println(opt.OrElse("默认值"))
	// Output:
	// None
	// false
	// 默认值
}

// weightedItem 实现 core.Weighted 接口，用于示例。
type weightedItem struct{ w float64 }

func (wi weightedItem) Weight() float64 { return wi.w }

// ExampleWeighted 演示 Weighted 接口的使用。
func ExampleWeighted() {
	i := weightedItem{w: 3.5}
	fmt.Printf("权重: %.1f\n", i.Weight())
	// Output:
	// 权重: 3.5
}
