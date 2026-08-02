// Package cyberhuma 提供基于 Huma v2 的路由封装 — 自动 OpenAPI 文档和参数校验。
//
// 适用场景：
//   - 需要对外暴露 API 文档的服务
//   - 希望自动参数校验和友好的校验错误提示
//   - 前端/第三方团队需要 Swagger/Redoc 页面
//
// Handler 兼容性：
//
//	Router 使用与 SimpleRouter 完全相同的 TypedHandler[I, O] 签名。
//	从 SimpleRouter 切换到 Router 只需改变路由创建方式，Handler 代码无需任何修改。
//
// 输入结构体约定：
//   - 路径参数使用 Huma/OpenAPI 路径：/users/{id}，结构体字段写 `path:"id"`。
//   - Query 参数写 `query:"page"`，Header 写 `header:"X-Trace-ID"`，Cookie 写 `cookie:"sid"`。
//   - 标准 JSON Body 推荐用 POSTBody，业务输入结构体字段写 `json:"name"`。
//   - 同时需要 path/query/header 和 JSON Body 时，输入结构体增加名为 Body 的字段，并使用 POST/PUT。
//
// 更完整的 path/query/body/json 示例见 web/router/cyberhuma/README.md。
package cyberhuma

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"

	"github.com/gin-gonic/gin"
	"github.com/izliang/bore/e"
	"github.com/izliang/bore/web"
	"github.com/izliang/bore/web/define"
	"github.com/izliang/bore/web/router"
)

// ============================================================
// Router — OpenAPI 文档 + 自动校验
// ============================================================

// Router 基于 Huma v2 的路由 — 自动 OpenAPI 文档 + 参数校验。
//
// 适用场景：
//   - 需要对外暴露 API 文档的服务
//   - 希望自动参数校验和友好的校验错误提示
//   - 前端/第三方团队需要 Swagger/Redoc 页面
//
// Handler 兼容性：
//
//	Router 使用与 SimpleRouter 完全相同的 TypedHandler[I, O] 签名。
//	从 SimpleRouter 切换到 Router 只需改变路由创建方式，Handler 代码无需任何修改。
type Router struct {
	gin.IRouter
	api        huma.API
	basePath   string
	multiPaths map[string]bool // registered POSTMulti full paths
}

// HumaOption 配置 Router 的选项函数。
type HumaOption func(*humaConfig)

// GroupOption 配置一组 API 的选项函数。
type GroupOption func(*groupConfig)

// OperationOption 配置单个 OpenAPI Operation。
type OperationOption func(*huma.Operation)

func WithTags(tags ...string) OperationOption {
	return func(o *huma.Operation) {
		o.Tags = tags
	}
}

func operationHandlers(opts []OperationOption) []func(*huma.Operation) {
	handlers := make([]func(*huma.Operation), len(opts))
	for i, opt := range opts {
		handlers[i] = opt
	}
	return handlers
}

type humaConfig struct {
	title                     string
	version                   string
	doc                       router.DocConfig
	handlers                  []huma.Transformer
	configHandlers            []func(*huma.Config)
	withoutDefaultHandleError bool
	notAllowAdditionalProps   bool // 不允许随意添加请求体属性
}

type groupConfig struct {
	ginHandlers     []gin.HandlerFunc
	humaHandlers    []func(huma.Context, func(huma.Context))
	transformers    []huma.Transformer
	modifiers       []func(*huma.Operation, func(*huma.Operation))
	simpleModifiers []func(*huma.Operation)
}

func defaultHumaConfig() humaConfig {
	return humaConfig{
		title:   "API Documentation",
		version: "1.0.0",
		doc:     router.DefaultDocConfig(),
	}
}

// WithHumaTitle 设置 OpenAPI 文档标题和版本。
func WithHumaTitle(title, version string) HumaOption {
	return func(c *humaConfig) {
		c.title = title
		c.version = version
	}
}

func WithHumaHandles(handlers ...huma.Transformer) HumaOption {
	return func(c *humaConfig) {
		c.handlers = handlers
	}
}

// WithHumaConfig 自定义 Huma 原生配置。
func WithHumaConfig(handler func(*huma.Config)) HumaOption {
	return func(c *humaConfig) {
		if handler != nil {
			c.configHandlers = append(c.configHandlers, handler)
		}
	}
}

func WithoutDefaultHandleError(without bool) HumaOption {
	return func(c *humaConfig) {
		c.withoutDefaultHandleError = without
	}
}

// WithHumaDocs 配置文档暴露策略（生产安全默认不暴露）。
//
// 典型用法：
//
//	// 开发环境 — 暴露 Swagger
//	huma.WithHumaDocs(router.DocConfig{
//	    Exposure: router.DocAlways,
//	    UI:       router.DocUISwagger,
//	})
//
//	// 生产环境 — 根据环境变量判断
//	huma.WithHumaDocs(router.DocConfig{
//	    Exposure: router.DocConditional,
//	    Condition: func() bool { return os.Getenv("ENV") != "prod" },
//	    UI:       router.DocUISwagger,
//	})
//
//	// 不传此选项 — 默认 DocNever（不暴露）
func WithHumaDocs(doc router.DocConfig) HumaOption {
	return func(c *humaConfig) {
		c.doc = doc
		if c.doc.OpenAPIPath == "" {
			c.doc.OpenAPIPath = "/openapi.json"
		}
		if c.doc.DocsPath == "" {
			c.doc.DocsPath = "/docs"
		}
	}
}

// New 创建 Router。
//
// 参数 r 可以是 *gin.Engine 或 *gin.RouterGroup。
//
// 行为：
//  1. router.DocConfig.Exposure 控制是否暴露 /openapi.json 和 /docs
//  2. 文档 HTML 编译时内嵌（swagger.html / redoc.html），无需外部文件
//  3. 生产环境默认 DocNever，不暴露任何文档端点
func New(r gin.IRouter, opts ...HumaOption) *Router {
	cfg := defaultHumaConfig()
	for _, o := range opts {
		o(&cfg)
	}

	api := huma.NewAPI(buildHumaConfig(cfg), newGinAdapter(r))
	hr := &Router{
		IRouter:    r,
		api:        api,
		basePath:   getGinBasePath(r),
		multiPaths: make(map[string]bool),
	}

	router.RegisterDocRoutes(r, cfg.doc)
	r.Use(hr.multipartInterceptor)

	return hr
}

// Group 创建 API Group，并支持为该组增加 Gin / Huma 中间件与 OpenAPI 定制。
func (r *Router) Group(relativePath string, opts ...GroupOption) *Router {
	cfg := groupConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	ginGroup := r.IRouter.Group(relativePath, cfg.ginHandlers...)
	humaGroup := huma.NewGroup(r.api, relativePath)
	for _, handler := range cfg.humaHandlers {
		humaGroup.UseMiddleware(handler)
	}
	for _, transformer := range cfg.transformers {
		humaGroup.UseTransformer(transformer)
	}
	for _, modifier := range cfg.modifiers {
		humaGroup.UseModifier(modifier)
	}
	for _, modifier := range cfg.simpleModifiers {
		humaGroup.UseSimpleModifier(modifier)
	}

	return &Router{
		IRouter: ginGroup,
		api: &routedGroupAPI{
			Group:   humaGroup,
			adapter: newRoutedGroupAdapter(humaGroup, relativePath, newGinAdapter(ginGroup)),
		},
	}
}

// WithoutAllowAdditionalProperties 设置不允许 JSON Body 中的多余字段。
// 拒绝多余的 JSON 字段
//
//	router := huma.New(r, huma.WithoutAllowAdditionalProperties())
func WithoutAllowAdditionalProperties() HumaOption {
	return func(c *humaConfig) {
		c.notAllowAdditionalProps = true
	}
}

// WithGroupGinMiddleware 为当前 API Group 增加 Gin 中间件。
func WithGroupGinMiddleware(handlers ...gin.HandlerFunc) GroupOption {
	return func(c *groupConfig) {
		c.ginHandlers = append(c.ginHandlers, handlers...)
	}
}

// WithGroupHumaMiddleware 为当前 API Group 增加 Huma 中间件。
func WithGroupHumaMiddleware(handlers ...func(huma.Context, func(huma.Context))) GroupOption {
	return func(c *groupConfig) {
		c.humaHandlers = append(c.humaHandlers, handlers...)
	}
}

// WithGroupTransformers 为当前 API Group 增加响应转换器。
func WithGroupTransformers(transformers ...huma.Transformer) GroupOption {
	return func(c *groupConfig) {
		c.transformers = append(c.transformers, transformers...)
	}
}

// WithGroupOperationModifier 为当前 API Group 增加 Operation 修改器。
func WithGroupOperationModifier(modifier func(*huma.Operation, func(*huma.Operation))) GroupOption {
	return func(c *groupConfig) {
		if modifier != nil {
			c.modifiers = append(c.modifiers, modifier)
		}
	}
}

// WithGroupSimpleOperationModifier 为当前 API Group 增加简单 Operation 修改器。
func WithGroupSimpleOperationModifier(modifier func(*huma.Operation)) GroupOption {
	return func(c *groupConfig) {
		if modifier != nil {
			c.simpleModifiers = append(c.simpleModifiers, modifier)
		}
	}
}

type ginAdapter struct {
	handler http.Handler
	router  gin.IRouter
}

func newGinAdapter(r gin.IRouter) huma.Adapter {
	adapter := &ginAdapter{router: r}
	if handler, ok := r.(http.Handler); ok {
		adapter.handler = handler
	}
	return adapter
}

func (a *ginAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	path := strings.ReplaceAll(op.Path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Handle(op.Method, path, func(c *gin.Context) {
		handler(humagin.NewContext(op, c))
	})
}

func (a *ginAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.handler != nil {
		a.handler.ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

type routedGroupAdapter struct {
	group   *huma.Group
	prefix  string
	adapter huma.Adapter
}

func newRoutedGroupAdapter(group *huma.Group, prefix string, adapter huma.Adapter) huma.Adapter {
	return &routedGroupAdapter{
		group:   group,
		prefix:  strings.TrimRight(prefix, "/"),
		adapter: adapter,
	}
}

func (a *routedGroupAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	a.group.ModifyOperation(op, func(modified *huma.Operation) {
		runtimeOp := *modified
		runtimeOp.Path = trimGroupPrefix(runtimeOp.Path, a.prefix)
		a.adapter.Handle(&runtimeOp, handler)
	})
}

func (a *routedGroupAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.adapter.ServeHTTP(w, r)
}

type routedGroupAPI struct {
	*huma.Group
	adapter huma.Adapter
}

func (a *routedGroupAPI) Adapter() huma.Adapter {
	return a.adapter
}

func trimGroupPrefix(path, prefix string) string {
	if prefix == "" || prefix == "/" {
		return path
	}
	if path == prefix {
		return "/"
	}
	if strings.HasPrefix(path, prefix+"/") {
		return strings.TrimPrefix(path, prefix)
	}
	return path
}

func buildHumaConfig(cfg humaConfig) huma.Config {
	hConfig := huma.DefaultConfig(cfg.title, cfg.version)
	hConfig.AllowAdditionalPropertiesByDefault = true
	if cfg.notAllowAdditionalProps {
		hConfig.AllowAdditionalPropertiesByDefault = false
	}
	if cfg.doc.ShouldExpose() {
		hConfig.SchemasPath = cfg.doc.OpenAPIPath
		hConfig.DocsPath = ""
	} else {
		hConfig.SchemasPath = ""
		hConfig.DocsPath = ""
	}
	hConfig.CreateHooks = nil
	if !cfg.withoutDefaultHandleError {
		hConfig.Transformers = append(hConfig.Transformers, DefaultHandleError)
	}
	hConfig.Transformers = append(hConfig.Transformers, cfg.handlers...)
	for _, handler := range cfg.configHandlers {
		handler(&hConfig)
	}
	return hConfig
}

func DefaultHandleError(ctx huma.Context, status string, v any) (any, error) {
	errModel, ok := v.(*huma.ErrorModel)
	if !ok {
		return v, nil
	}

	ctx.SetStatus(http.StatusBadRequest)
	return define.Envelope[any]{
		Code:      "400000",
		Message:   humaErrorMessage(errModel),
		Data:      nil,
		RequestID: web.GetRequestID(humagin.Unwrap(ctx)),
	}, nil
}

func humaErrorMessage(errModel *huma.ErrorModel) string {
	messages := make([]string, 0, 1+len(errModel.Errors))
	if errModel.Detail != "" {
		messages = append(messages, errModel.Detail)
	}
	for _, detail := range errModel.Errors {
		if detail == nil {
			continue
		}
		msg := detail.Message
		if detail.Location != "" {
			msg = detail.Location + ": " + msg
		}
		if msg != "" {
			messages = append(messages, msg)
		}
	}
	if len(messages) == 0 {
		return errModel.Error()
	}
	return strings.Join(messages, "; ")
}

// ============================================================
// 泛型注册函数
// ============================================================

// POST 注册泛型 POST Handler + 自动 OpenAPI 文档。
//
// Handler 签名与 SimpleRouter 完全一致：
//
//	func(ctx context.Context, input *Payload) (*Result, e.ServiceError)
//
// 内部流程：
//  1. 包装为 BodyWrapper[Payload]（Huma 从 Payload 的 struct tags 生成 schema）
//  2. Huma 自动校验（required / enum / minLength 等标签）
//  3. 调用业务 Handler
//  4. 成功时返回 HumaResponse[Result]
//  5. 业务错误时返回带错误码的 HumaResponse
func POST[I, O any](r *Router, path string, h define.TypedHandler[I, O], opts ...OperationOption) {
	huma.Post[I, define.HumaResponse[O]](
		r.api,
		path,
		func(ctx context.Context, input *I) (*define.HumaResponse[O], error) {
			output, svcErr := h(ctx, input)
			if svcErr != nil {
				return &define.HumaResponse[O]{
					Status: router.HTTPStatusFromError(svcErr),
					Body: define.Envelope[O]{
						Code:    svcErr.GetCode(),
						Message: svcErr.GetMessage(),
					},
				}, nil
			}
			return &define.HumaResponse[O]{
				Status: http.StatusOK,
				Body: define.Envelope[O]{
					Code:    e.SuccessCode,
					Message: "success",
					Data:    output,
				},
			}, nil
		},
		operationHandlers(opts)...,
	)
}

func POSTBody[I, O any](r *Router, path string, h define.TypedHandler[I, O], opts ...OperationOption) {
	huma.Post[define.BodyWrapper[I], define.HumaResponse[O]](
		r.api,
		path,
		func(ctx context.Context, input *define.BodyWrapper[I]) (*define.HumaResponse[O], error) {
			output, svcErr := h(ctx, &input.Body)
			if svcErr != nil {
				return &define.HumaResponse[O]{
					Status: router.HTTPStatusFromError(svcErr),
					Body: define.Envelope[O]{
						Code:    svcErr.GetCode(),
						Message: svcErr.GetMessage(),
					},
				}, nil
			}
			return &define.HumaResponse[O]{
				Status: http.StatusOK,
				Body: define.Envelope[O]{
					Code:    e.SuccessCode,
					Message: "success",
					Data:    output,
				},
			}, nil
		},
		operationHandlers(opts)...,
	)
}

// getGinBasePath extracts the base path from a gin router.
func getGinBasePath(r gin.IRouter) string {
	if rg, ok := r.(interface{ BasePath() string }); ok {
		return rg.BasePath()
	}
	return ""
}

// MultipartFormCtxKey is the context key for storing the parsed multipart form.
// Used by POSTMulti middleware to pass multipart data to handlers.
const MultipartFormCtxKey = "__huma_multipart_form"

// GetMultipartForm retrieves the multipart form stored by POSTMulti middleware.
// Returns nil, false if no multipart form is present (i.e., the request was JSON).
func GetMultipartForm(ctx context.Context) (*multipart.Form, bool) {
	v := ctx.Value(MultipartFormCtxKey)
	if v == nil {
		return nil, false
	}
	form, ok := v.(*multipart.Form)
	return form, ok
}

// multipartInterceptor is a Gin middleware that intercepts multipart/form-data requests
// on POSTMulti-registered paths. It parses the multipart form, stores it in the Gin context
// under key "__huma_multipart", and replaces the request body with empty JSON "{}".
func (r *Router) multipartInterceptor(c *gin.Context) {
	if !r.multiPaths[c.Request.URL.Path] {
		c.Next()
		return
	}
	ct := c.GetHeader("Content-Type")
	if !strings.HasPrefix(ct, "multipart/form-data") {
		c.Next()
		return
	}
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Store in both gin context and request context for handler access
	c.Set("__huma_multipart", c.Request.MultipartForm)
	newCtx := context.WithValue(c.Request.Context(), MultipartFormCtxKey, c.Request.MultipartForm)
	c.Request = c.Request.WithContext(newCtx)

	c.Request.Body = io.NopCloser(strings.NewReader("{}"))
	c.Request.ContentLength = 2
	c.Request.Header.Set("Content-Type", "application/json")
	c.Next()
}

// WithMultipartForm adds multipart/form-data schema to the OpenAPI operation documentation.
// T must be a struct with form tags describing multipart fields, e.g.:
//
//	type MyForm struct {
//	    Meta string        `form:"meta" contentType:"application/json" required:"true" doc:"JSON metadata"`
//	    File huma.FormFile `form:"file" contentType:"application/octet-stream" required:"true" doc:"Binary file"`
//	}
//
// Swagger UI will show a file upload button for fields typed as huma.FormFile.
func WithMultipartForm[T any]() OperationOption {
	t := reflect.TypeFor[T]()
	schema, encoding := buildMultipartSchema(t)
	return func(o *huma.Operation) {
		if o.RequestBody == nil {
			o.RequestBody = &huma.RequestBody{Required: false}
		}
		if o.RequestBody.Content == nil {
			o.RequestBody.Content = map[string]*huma.MediaType{}
		}
		o.RequestBody.Required = false
		o.RequestBody.Content["multipart/form-data"] = &huma.MediaType{
			Schema:   schema,
			Encoding: encoding,
		}
	}
}

// buildMultipartSchema generates an OpenAPI schema and encoding map from a multipart form struct.
// Mirrors huma's internal multiPartFormFileSchema / multiPartContentEncoding logic.
func buildMultipartSchema(t reflect.Type) (*huma.Schema, map[string]*huma.Encoding) {
	nFields := t.NumField()
	s := &huma.Schema{
		Type:       "object",
		Properties: make(map[string]*huma.Schema, nFields),
	}
	enc := make(map[string]*huma.Encoding, nFields)
	var required []string

	formFileType := reflect.TypeFor[huma.FormFile]()
	formFilesType := reflect.TypeFor[[]huma.FormFile]()

	for i := range nFields {
		f := t.Field(i)
		name := f.Tag.Get("form")
		if name == "" {
			name = f.Name
		}

		var contentType string
		switch {
		case f.Type == formFileType:
			contentType = tagOr(f.Tag, "contentType", "application/octet-stream")
			s.Properties[name] = &huma.Schema{
				Type:        "string",
				Format:      "binary",
				Description: f.Tag.Get("doc"),
			}
		case f.Type == formFilesType:
			contentType = tagOr(f.Tag, "contentType", "application/octet-stream")
			s.Properties[name] = &huma.Schema{
				Type:        "array",
				Items:       &huma.Schema{Type: "string", Format: "binary"},
				Description: f.Tag.Get("doc"),
			}
		default:
			contentType = "text/plain"
			if ct := f.Tag.Get("contentType"); ct != "" {
				contentType = ct
			}
			s.Properties[name] = &huma.Schema{
				Type:        "string",
				Description: f.Tag.Get("doc"),
			}
		}
		enc[name] = &huma.Encoding{ContentType: contentType}

		if f.Tag.Get("required") == "true" {
			required = append(required, name)
		}
	}

	if len(required) > 0 {
		s.Required = required
	}
	return s, enc
}

func tagOr(tag reflect.StructTag, key, fallback string) string {
	if v := tag.Get(key); v != "" {
		return v
	}
	return fallback
}

// POSTMulti registers a POST endpoint that supports both application/json and
// multipart/form-data body. JSON requests go through Huma's standard flow with
// auto OpenAPI docs and validation. Multipart requests are intercepted by a Gin
// middleware; the raw multipart.Form is stored in Gin context key "__huma_multipart",
// and the handler can access it via humagin.Unwrap(ctx).Get("__huma_multipart").
func POSTMulti[I, O any](r *Router, path string, h define.TypedHandler[I, O], opts ...OperationOption) {
	r.multiPaths[r.basePath+path] = true
	POST[I, O](r, path, h, opts...)
}

// GET 注册泛型 GET Handler + 自动 OpenAPI 文档。
func GET[I, O any](r *Router, path string, h define.TypedHandler[I, O], opts ...OperationOption) {
	huma.Get[I, define.HumaResponse[O]](
		r.api,
		path,
		func(ctx context.Context, input *I) (*define.HumaResponse[O], error) {
			output, svcErr := h(ctx, input)
			if svcErr != nil {
				return &define.HumaResponse[O]{
					Status: http.StatusOK,
					Body: define.Envelope[O]{
						Code:    svcErr.GetCode(),
						Message: svcErr.GetMessage(),
					},
				}, nil
			}
			return &define.HumaResponse[O]{
				Status: http.StatusOK,
				Body: define.Envelope[O]{
					Code:    e.SuccessCode,
					Message: "success",
					Data:    output,
				},
			}, nil
		},
		operationHandlers(opts)...,
	)
}

// PUT 注册泛型 PUT Handler + 自动 OpenAPI 文档。
func PUT[I, O any](r *Router, path string, h define.TypedHandler[I, O], opts ...OperationOption) {
	huma.Put[I, define.HumaResponse[O]](
		r.api,
		path,
		func(ctx context.Context, input *I) (*define.HumaResponse[O], error) {
			output, svcErr := h(ctx, input)
			if svcErr != nil {
				return &define.HumaResponse[O]{
					Status: http.StatusOK,
					Body: define.Envelope[O]{
						Code:    svcErr.GetCode(),
						Message: svcErr.GetMessage(),
					},
				}, nil
			}
			return &define.HumaResponse[O]{
				Status: http.StatusOK,
				Body: define.Envelope[O]{
					Code:    e.SuccessCode,
					Message: "success",
					Data:    output,
				},
			}, nil
		},
		operationHandlers(opts)...,
	)
}

// DELETE 注册泛型 DELETE Handler + 自动 OpenAPI 文档。
func DELETE[I, O any](r *Router, path string, h define.TypedHandler[I, O], opts ...OperationOption) {
	huma.Delete[I, define.HumaResponse[O]](
		r.api,
		path,
		func(ctx context.Context, input *I) (*define.HumaResponse[O], error) {
			output, svcErr := h(ctx, input)
			if svcErr != nil {
				return &define.HumaResponse[O]{
					Status: http.StatusOK,
					Body: define.Envelope[O]{
						Code:    svcErr.GetCode(),
						Message: svcErr.GetMessage(),
					},
				}, nil
			}
			return &define.HumaResponse[O]{
				Status: http.StatusOK,
				Body: define.Envelope[O]{
					Code:    e.SuccessCode,
					Message: "success",
					Data:    output,
				},
			}, nil
		},
		operationHandlers(opts)...,
	)
}
