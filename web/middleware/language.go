package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/izliang/bore/web/lang"
)

// LanguageMiddleware 预解析请求语言并注入 gin.Context。
//
// 调用 lang.GetRequestLang 完成多级 fallback 检测（Header → Query → Cookie → …），
// 结果缓存到 gin.Context 中，后续 Handler 和错误响应器可直接使用。
//
// 建议在 RequestIDMiddleware 之后、错误处理中间件之前注册。
func LanguageMiddleware(_ ...MiddleOptionFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		// lang.GetRequestLang 内部已完成缓存（c.Set(GinContextLangKey, ...)）
		_ = lang.GetRequestLang(c)
		c.Next()
	}
}
