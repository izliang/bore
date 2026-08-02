// Package web 提供 Gin 框架与 bore 基础设施之间的桥接函数。
package web

import (
	"github.com/gin-gonic/gin"
)

// 上下文键常量。
// Middleware 和 Router 层通过 gin.Context.Set/Get 共享这些键。
const (
	// ContextKeyRequestID 在 gin.Context 中存储请求追踪 ID 的键。
	// 与 middleware.RequestIDMiddleware 配合使用。
	ContextKeyRequestID = "requestId"
)

// GetRequestID 从 gin.Context 提取请求追踪 ID。
//
// 该值由 middleware.RequestIDMiddleware 设置。
//
// 返回值：
//
//	请求 ID 字符串；如果中间件未启用，返回 ""。
func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get(ContextKeyRequestID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
