package core

// Weighted 表示携带数值权重的实体。
// 通常由图中的边类型或优先集合中的元素包装器实现。
type Weighted interface {
	// Weight 返回与此实体关联的权重值。
	Weight() float64
}
