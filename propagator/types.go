package propagator

import "context"

// Carrier 是 key-value 存储的抽象，每种协议提供自己的实现。
type Carrier interface {
	Get(key string) string
	Set(key string, value string)
	Keys() []string
}

// Propagator 在服务调用链路中透传 request-scoped metadata。
// Inject 把 context 中的数据写入 carrier。
// Extract 从 carrier 读取数据并返回增强后的 context。
type Propagator interface {
	Inject(ctx context.Context, carrier Carrier)
	Extract(ctx context.Context, carrier Carrier) context.Context
}
