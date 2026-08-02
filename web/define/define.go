// Package define 定义 web 脚手架的核心类型：Handler 签名、统一响应信封、
// 以及 Huma 适配桥接类型。
//
// 设计原则：
//   - 零应用层依赖：Handler 接收标准库 context.Context，不依赖 appctx 等应用层抽象
//   - 用户可在 Handler 内部自行创建 appctx.AppContext，实现关注点分离
package define

import (
	"context"
	"net/http"

	"github.com/izliang/bore/e"
)

// ============================================================
// Handler 签名 — SimpleRouter 和 HumaRouter 的共同契约
// ============================================================

// TypedHandler 泛型业务 Handler。
//
// 一个 Handler 可同时用于 SimpleRouter 和 HumaRouter，实现"一次编写，双模运行"。
//
// 类型参数：
//
//	I — 业务请求载荷类型
//	  Simple 模式：直接用作 Gin ShouldBindJSON / ShouldBindQuery 的目标类型
//	  Huma 模式：  自动包装为 BodyWrapper[I]，Huma 从 I 的 struct tags 提取 OpenAPI schema
//
//	O — 响应数据类型，自动包装进 Envelope[O].Data
//
// 参数：
//
//	ctx   — 标准库 context.Context（即 c.Request.Context()）
//	input — 已绑定并校验的请求载荷
//
// 返回值：
//
//	error — 必须是 e.ServiceError，携带错误码、消息 ID、模板数据
//
// 如果需要在 Handler 中使用 appctx.AppContext：
//
//	func MyHandler(ctx context.Context, input *Payload) (*Result, e.ServiceError) {
//	    appCtx := appctx.New(appctx.WithContext(ctx)) // 按需创建
//	    appCtx.Info("processing request")
//	    ...
//	}
type TypedHandler[I, O any] func(ctx context.Context, input *I) (*O, e.ServiceError)

// RawHandler 原始 Handler — 完全控制 HTTP 响应。
//
// 用于文件上传下载、SSE、WebSocket 等无法用 TypedHandler 表达的场景。
// 返回 *http.Response 使调用方可以自由控制状态码、Header、Body。
type RawHandler func(ctx context.Context, r *http.Request) (*http.Response, e.ServiceError)

// ============================================================
// 统一响应信封 — 整个应用唯一的 JSON 响应格式
// ============================================================

// Envelope 是每一个 API 响应的标准信封。
//
// 成功响应示例：
//
//	{"code":"000000","message":"success","data":{...},"request_id":"abc123"}
//
// 错误响应示例：
//
//	{"code":"404001","message":"用户(ID:123)不存在","data":null,"request_id":"abc123"}
type Envelope[T any] struct {
	Code      string `json:"code"`    // 成功="000000"，业务错误="404001" 等
	Message   string `json:"message"` // 已 i18n 翻译的可读消息
	Data      *T     `json:"data,omitempty"`
	RequestID string `json:"request_id"` // 请求追踪 ID
}

// ============================================================
// Huma 适配桥接类型 — 让 TypedHandler 可接入 Huma v2
// ============================================================

// BodyWrapper 包装业务载荷 I，适配 Huma 的请求体提取约定。
//
// Huma v2 从 BodyWrapper.Body 的 struct tags 自动提取：
//   - JSON schema（字段名、类型、必填/可选）
//   - 参数校验规则（required、enum、minimum、maxLength 等）
//   - 文档描述（doc 标签）
//
// BodyWrapper 本身不导入 huma 包，仅作为数据载体。
// 实际的 Huma 注册逻辑在 router/huma.go（build tag: huma）中完成。
type BodyWrapper[B any] struct {
	Body B
}

// HumaResponse 适配 Huma 的响应格式要求。
//
// Huma v2 要求 handler 返回的响应体包含 HTTP 状态码信息。
// Status 字段被 Huma 识别为 HTTP 状态码，Body 字段作为响应体序列化为 JSON。
//
// 实际 HTTP 响应格式（Huma 序列化后）：
//
//	{"status":200,"body":{"code":"000000","message":"success","data":{...},"request_id":"abc"}}
type HumaResponse[T any] struct {
	Status int         `json:"status"`
	Body   Envelope[T] `json:"body"`
}
