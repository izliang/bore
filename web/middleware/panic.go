package middleware

import (
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/web"
)

// PanicMiddleware 捕获 panic，记录完整调用栈，返回 500 错误。
//
// 建议放在中间件链的最后（最内层），以便捕获前面所有中间件和 Handler 中发生的 panic。
//
// 参数：
//
//	logger — 用于记录 panic 详情的结构化日志器
func PanicMiddleware(logger logit.Logger, opts ...MiddleOptionFunc) gin.HandlerFunc {
	param := &middlewareParam{}
	for _, opt := range opts {
		opt(param)
	}
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				stack := string(debug.Stack())
				var panicErr error
				switch e := err.(type) {
				case error:
					panicErr = e
				case string:
					panicErr = errors.New(e)
				default:
					panicErr = fmt.Errorf("%v", e)
				}

				// 记录到日志
				if logger != nil {
					logger.Error(c.Request.Context(), panicErr,
						logit.StringField("stack", stack, logit.WithStrAllowNewline(param.panicStackNewLine)),
						logit.StringField("request_id", web.GetRequestID(c)),
						logit.StringField("path", c.Request.URL.Path),
					)
				}

				// 返回标准错误响应
				c.AbortWithStatusJSON(200, gin.H{
					"code":       "500000",
					"message":    "internal server error",
					"data":       nil,
					"request_id": web.GetRequestID(c),
				})
			}
		}()
		c.Next()
	}
}
