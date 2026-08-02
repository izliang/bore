// Package middleware 提供 Gin 框架通用中间件。
package middleware

import (
	"crypto/rand"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/izliang/bore/web"
)

const (
	// HeaderRequestID 请求 ID 的 HTTP Header 名。
	HeaderRequestID = "X-Request-ID"
)

// RequestIDMiddleware 生成或透传请求追踪 ID。
//
// 优先级：
//  1. 客户端传入的 X-Request-ID Header → 透传
//  2. 无 Header → 自动生成 UUID v4 风格的请求 ID
//
// 注入：
//   - gin.Context.Set(ContextKeyRequestID, id)
//   - 回写 X-Request-ID 响应 Header
//
// 建议放在中间件链的最前面，以便后续所有中间件和 Handler 都能获取到 request_id。
func RequestIDMiddleware(_ ...MiddleOptionFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader(HeaderRequestID)
		if reqID == "" {
			reqID = generateRequestID()
		}
		c.Set(web.ContextKeyRequestID, reqID)
		c.Header(HeaderRequestID, reqID)
		c.Next()
	}
}

// generateRequestID 使用 crypto/rand 生成一个 UUID v4 风格的请求 ID。
// 格式：xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	// UUID v4 格式
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
