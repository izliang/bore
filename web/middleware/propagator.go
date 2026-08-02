// Package middleware 提供 Gin 框架通用中间件。
package middleware

import (
	"github.com/gin-gonic/gin"
	propagatornew "github.com/izliang/bore/propagator"
	"github.com/izliang/bore/propagator/carrier"
	"github.com/izliang/bore/request/propagator"
)

// Propagator 从 HTTP Header 中提取透传数据并写入 Request.Context()。
//
// 接收多个 Propagator，依次执行 Extract 并累积 context。
//
//	router.Use(middleware.PropagatorMiddleware(sreProp, traceProp))
func Propagator(props ...propagatornew.Propagator) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		carr := &carrier.HTTPHeader{Header: c.Request.Header}
		for _, p := range props {
			ctx = p.Extract(ctx, carr)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// Deprecated: 请使用Propagator
// PropagatorMiddleware 旧的方法
func PropagatorMiddleware(p propagator.Propagator, _ ...MiddleOptionFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		carr := &carrier.HTTPHeader{Header: c.Request.Header}
		ctx = p.Extract(ctx, carr)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
