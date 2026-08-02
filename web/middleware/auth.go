package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/izliang/bore/e"
	"github.com/izliang/bore/i18n"
	"github.com/izliang/bore/web"
	"github.com/izliang/bore/web/lang"
)

// AuthFunc 用户自定义的认证逻辑。
//
// 返回值：
//   - nil：认证通过
//   - e.ServiceError：认证失败，中间件自动返回标准错误响应
type AuthFunc func(c *gin.Context) e.ServiceError

// AuthMiddleware 通用认证中间件。
//
// 认证失败时，自动返回统一的 Envelope 错误响应（HTTP 403）并 abort 请求链。
// 错误消息通过 translator 进行 i18n 翻译。
//
// 参数：
//
//	authFn     — 自定义认证逻辑
//	translator — i18n 翻译器（用于翻译错误消息）
//
// 示例：
//
//	bundle, _ := i18n.NewBundle("./locales", i18n.FallbackDefaultLang, "zh")
//
//	authGroup := r.Group("/api/v1")
//	authGroup.Use(middleware.AuthMiddleware(
//	    func(c *gin.Context) e.ServiceError {
//	        token := c.GetHeader("Authorization")
//	        if token == "" {
//	            return e.MustNewByCode("403001").Build()
//	        }
//	        return nil
//	    },
//	    bundle,
//	))
func AuthMiddleware(authFn AuthFunc, translator i18n.Translator) gin.HandlerFunc {
	return func(c *gin.Context) {
		svcErr := authFn(c)
		if svcErr == nil {
			c.Next()
			return
		}

		// 获取请求语言并翻译错误消息
		langStr := lang.GetRequestLang(c)
		message := translator.Tf(langStr, svcErr.GetMessage(), svcErr.TemplateData())
		if message == "" {
			message = svcErr.GetMessage()
		}

		requestID := web.GetRequestID(c)

		c.AbortWithStatusJSON(403, gin.H{
			"code":       svcErr.GetCode(),
			"message":    message,
			"data":       svcErr.GetData(),
			"request_id": requestID,
		})
	}
}
