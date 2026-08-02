// Package simple 提供纯 Gin 路由封装 — 零 Huma 依赖，最小开销。
//
// 适用场景：
//   - 不想引入 Huma 额外依赖的项目
//   - 纯内部服务，不需要 OpenAPI 文档
//   - 追求最小二进制体积和最快编译速度
//
// 核心能力：
//   - 泛型 Handler 注册（GET/POST/PUT/DELETE）
//   - 自动请求绑定（JSON Body / Query / Path）
//   - 统一 Envelope 响应包装
//   - 自动 i18n 错误消息翻译
//   - HTTP 状态码智能决策（403/404/400/200）
//   - 可选的 Metrics 上报
package simple

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/izliang/bore/e"
	"github.com/izliang/bore/i18n"
	"github.com/izliang/bore/web"
	"github.com/izliang/bore/web/define"
	"github.com/izliang/bore/web/lang"
	"github.com/izliang/bore/web/router"
)

// ============================================================
// Router — 纯 Gin 模式
// ============================================================

// Router 纯 Gin 路由封装。
//
// 适用场景：
//   - 不想引入额外依赖的项目
//   - 纯内部服务，不需要 OpenAPI 文档
//   - 追求最小二进制体积和最快编译速度
//
// 核心能力：
//   - 泛型 Handler 注册（GET/POST/PUT/DELETE）
//   - 自动请求绑定（JSON Body / Query / Path）
//   - 统一 Envelope 响应包装
//   - 自动 i18n 错误消息翻译
//   - HTTP 状态码智能决策（403/404/400/200）
//   - 可选的 Metrics 上报
type Router struct {
	gin.IRouter
	metrics    router.MetricsReporter
	translator i18n.Translator
}

// Option 配置 Router 的选项函数。
type Option func(*Router)

// WithMetrics 注入 Metrics 上报器。
func WithMetrics(m router.MetricsReporter) Option {
	return func(r *Router) {
		r.metrics = m
	}
}

// WithTranslator 注入 i18n 翻译器，用于错误消息的国际化。
//
// 典型用法：
//
//	bundle, _ := i18n.NewBundle("./locales", i18n.FallbackDefaultLang, "zh")
//	api := simple.New(r.Group("/api/v1"), simple.WithTranslator(bundle))
//
// 如果未注入，错误消息将直接使用 e.ServiceError 的 messageID。
func WithTranslator(t i18n.Translator) Option {
	return func(r *Router) {
		r.translator = t
	}
}

// New 创建 Router。
//
// 参数：
//
//	r    — Gin 路由组（如 gin.Engine、gin.RouterGroup）
//	opts — 可选配置（WithTranslator、WithMetrics）
func New(r gin.IRouter, opts ...Option) *Router {
	sr := &Router{IRouter: r}
	for _, o := range opts {
		o(sr)
	}
	return sr
}

// ============================================================
// 泛型注册函数
// ============================================================

// POST 注册泛型 POST Handler。
//
// 内部流程：
//  1. Gin ShouldBindJSON 自动绑定请求体到 I
//  2. 调用业务 Handler（传入 c.Request.Context()）
//  3. 成功 → Envelope[O] 响应（HTTP 200）
//  4. 失败 → i18n 翻译 + Envelope 错误响应（智能 HTTP 状态码）
//
// 示例：
//
//	api := simple.New(r.Group("/api/v1"))
//	simple.POST(api, "/users", CreateUser)
func POST[I, O any](r *Router, path string, h define.TypedHandler[I, O]) {
	r.IRouter.POST(path, func(c *gin.Context) {
		start := time.Now()
		var input I
		if err := c.ShouldBindJSON(&input); err != nil {
			respondBindError(c, r, err, start)
			return
		}
		output, svcErr := h(c.Request.Context(), &input)
		if svcErr != nil {
			respondServiceError(c, r, svcErr, start)
			return
		}
		respondSuccess(c, r, output, start)
	})
}

// GET 注册泛型 GET Handler。
//
// 内部流程：Gin ShouldBindQuery + ShouldBindUri 自动绑定 Query/Path 参数到 I。
//
// 示例：
//
//	simple.GET(api, "/users/:id", GetUser)
func GET[I, O any](r *Router, path string, h define.TypedHandler[I, O]) {
	r.IRouter.GET(path, func(c *gin.Context) {
		start := time.Now()
		var input I
		if err := c.ShouldBindQuery(&input); err != nil {
			respondBindError(c, r, err, start)
			return
		}
		if err := c.ShouldBindUri(&input); err != nil {
			respondBindError(c, r, err, start)
			return
		}
		output, svcErr := h(c.Request.Context(), &input)
		if svcErr != nil {
			respondServiceError(c, r, svcErr, start)
			return
		}
		respondSuccess(c, r, output, start)
	})
}

// PUT 注册泛型 PUT Handler。
func PUT[I, O any](r *Router, path string, h define.TypedHandler[I, O]) {
	r.IRouter.PUT(path, func(c *gin.Context) {
		start := time.Now()
		var input I
		if err := c.ShouldBindJSON(&input); err != nil {
			respondBindError(c, r, err, start)
			return
		}
		output, svcErr := h(c.Request.Context(), &input)
		if svcErr != nil {
			respondServiceError(c, r, svcErr, start)
			return
		}
		respondSuccess(c, r, output, start)
	})
}

// DELETE 注册泛型 DELETE Handler。
func DELETE[I, O any](r *Router, path string, h define.TypedHandler[I, O]) {
	r.IRouter.DELETE(path, func(c *gin.Context) {
		start := time.Now()
		var input I
		if err := c.ShouldBindQuery(&input); err != nil {
			respondBindError(c, r, err, start)
			return
		}
		if err := c.ShouldBindUri(&input); err != nil {
			respondBindError(c, r, err, start)
			return
		}
		output, svcErr := h(c.Request.Context(), &input)
		if svcErr != nil {
			respondServiceError(c, r, svcErr, start)
			return
		}
		respondSuccess(c, r, output, start)
	})
}

// ============================================================
// 原始 Handler 注册（逃生舱）
// ============================================================

// RawPOST 注册原始 POST Handler — 完全控制 HTTP 响应。
func RawPOST(r *Router, path string, h define.RawHandler) {
	r.IRouter.POST(path, func(c *gin.Context) {
		start := time.Now()
		resp, svcErr := h(c.Request.Context(), c.Request)
		if svcErr != nil {
			respondServiceError(c, r, svcErr, start)
			return
		}
		for k, vs := range resp.Header {
			for _, v := range vs {
				c.Header(k, v)
			}
		}
		c.DataFromReader(resp.StatusCode, resp.ContentLength,
			resp.Header.Get("Content-Type"),
			resp.Body, nil)
	})
}

// RawGET 注册原始 GET Handler。
func RawGET(r *Router, path string, h define.RawHandler) {
	r.IRouter.GET(path, func(c *gin.Context) {
		start := time.Now()
		resp, svcErr := h(c.Request.Context(), c.Request)
		if svcErr != nil {
			respondServiceError(c, r, svcErr, start)
			return
		}
		for k, vs := range resp.Header {
			for _, v := range vs {
				c.Header(k, v)
			}
		}
		c.DataFromReader(resp.StatusCode, resp.ContentLength,
			resp.Header.Get("Content-Type"),
			resp.Body, nil)
	})
}

// ============================================================
// 内部响应处理
// ============================================================

// respondSuccess 构造并返回成功响应。
func respondSuccess[O any](c *gin.Context, r *Router, output *O, start time.Time) {
	env := define.Envelope[O]{
		Code:      e.SuccessCode,
		Message:   "success",
		Data:      output,
		RequestID: web.GetRequestID(c),
	}
	c.JSON(http.StatusOK, env)
	router.ReportMetricsOnce(r.metrics, router.APINameFromPath(c.FullPath()), e.SuccessCode, start, nil)
}

// respondServiceError 构造并返回业务错误响应。
func respondServiceError(c *gin.Context, r *Router, svcErr e.ServiceError, start time.Time) {
	langStr := lang.GetRequestLang(c)
	message := translateError(r, langStr, svcErr)

	code := svcErr.GetCode()
	httpStatus := router.HTTPStatusFromError(svcErr)

	env := define.Envelope[any]{
		Code:      code,
		Message:   message,
		Data:      nil,
		RequestID: web.GetRequestID(c),
	}
	c.JSON(httpStatus, env)
	router.ReportMetricsOnce(r.metrics, router.APINameFromPath(c.FullPath()), code, start, svcErr)
}

// respondBindError 处理请求绑定/校验失败。
func respondBindError(c *gin.Context, r *Router, err error, start time.Time) {
	env := define.Envelope[any]{
		Code:      "400000",
		Message:   err.Error(),
		Data:      nil,
		RequestID: web.GetRequestID(c),
	}
	c.JSON(http.StatusBadRequest, env)
	router.ReportMetricsOnce(r.metrics, router.APINameFromPath(c.FullPath()), "400000", start, err)
}

// translateError 使用注入的 Translator 翻译错误消息。
func translateError(r *Router, lang string, svcErr e.ServiceError) string {
	if r.translator == nil {
		return svcErr.GetMessage()
	}
	msg := r.translator.Tf(lang, svcErr.GetMessage(), svcErr.TemplateData())
	if msg == "" {
		return svcErr.GetMessage()
	}
	return msg
}
