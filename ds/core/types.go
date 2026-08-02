// Package core 定义了 ds SDK 中所有数据结构和算法所依赖的基础接口、类型和约束。
//
// 设计原则：core 不依赖 ds 中的任何包，而 ds 中的任何包都可以依赖 core。
// 这确保了严格的无环依赖图，并使算法能够与任何符合接口的数据结构一起工作。
package core

// Order 表示树和图的遍历顺序。
type Order int

const (
	// OrderPre 前序遍历：先访问当前节点，再访问子节点。
	OrderPre Order = iota
	// OrderIn 中序遍历：先访问左子节点，再访问当前节点，最后访问右子节点。
	OrderIn
	// OrderPost 后序遍历：先访问子节点，再访问当前节点。
	OrderPost
	// OrderLevel 层序遍历（广度优先）。
	OrderLevel
)

// String 返回遍历顺序的可读名称。
func (o Order) String() string {
	switch o {
	case OrderPre:
		return "PreOrder"
	case OrderIn:
		return "InOrder"
	case OrderPost:
		return "PostOrder"
	case OrderLevel:
		return "LevelOrder"
	default:
		return "Unknown"
	}
}
