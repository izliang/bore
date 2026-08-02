package router

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ============================================================
// 内嵌文档页面（编译时注入，零外部文件依赖）
// ============================================================

//go:embed swagger.html
var swaggerHTML string

//go:embed redoc.html
var redocHTML string

// ============================================================
// 文档暴露配置
// ============================================================

// DocExposure 文档暴露策略。
//
// 生产环境安全第一：默认 DocNever（不暴露任何文档端点）。
type DocExposure int

const (
	// DocNever 永远不暴露文档（默认，生产安全）。
	DocNever DocExposure = iota

	// DocAlways 永远暴露文档（仅限开发/测试环境）。
	DocAlways

	// DocConditional 根据 Condition 函数运行时决定是否暴露。
	// 典型用法：通过环境变量 ENV 或 APP_ENV 判断。
	//
	//	Condition: func() bool { return os.Getenv("APP_ENV") != "production" }
	DocConditional
)

// DocUI 文档 UI 类型。
type DocUI int

const (
	// DocUISwagger Swagger UI（默认）。
	DocUISwagger DocUI = iota

	// DocUIRedoc Redoc UI。
	DocUIRedoc

	// DocUINone 不提供 UI，仅暴露 OpenAPI JSON 端点。
	DocUINone
)

// DocConfig 文档配置。
//
// 默认值（DefaultDocConfig）：
//   - Exposure: DocNever（不暴露）
//   - UI: DocUISwagger
//   - OpenAPIPath: "/openapi.json"
//   - DocsPath: "/docs"
type DocConfig struct {
	// Exposure 文档暴露策略。
	Exposure DocExposure

	// Condition 当 Exposure == DocConditional 时生效。
	// 返回 true 表示暴露文档。
	Condition func() bool

	// UI 文档 UI 类型。
	UI DocUI

	// OpenAPIPath OpenAPI JSON 端点路径（默认 "/openapi.json"）。
	OpenAPIPath string

	// DocsPath 文档 UI 页面路径（默认 "/docs"）。
	DocsPath string
}

// DefaultDocConfig 返回生产安全的默认文档配置。
func DefaultDocConfig() DocConfig {
	return DocConfig{
		Exposure:    DocNever,
		UI:          DocUISwagger,
		OpenAPIPath: "/openapi.json",
		DocsPath:    "/docs",
	}
}

// ShouldExpose 判断当前是否应该暴露文档端点。
//
// 供 huma 子包在创建 Router 时判断是否启用文档端点。
func (c DocConfig) ShouldExpose() bool {
	switch c.Exposure {
	case DocAlways:
		return true
	case DocConditional:
		if c.Condition != nil {
			return c.Condition()
		}
		return false
	default: // DocNever
		return false
	}
}

// ============================================================
// 文档路由注册
// ============================================================

// RegisterDocRoutes 根据 DocConfig 决定是否注册文档路由。
//
// 供 huma 子包在创建 HumaRouter 后调用。
//
// 安全规则：
//   - DocNever: 不注册任何文档路由（生产默认）
//   - DocAlways: 注册文档 UI + OpenAPI JSON（Huma 负责生成 JSON）
//   - DocConditional: 调用 Condition() 运行时决定
func RegisterDocRoutes(r gin.IRouter, cfg DocConfig) {
	if !cfg.ShouldExpose() {
		return
	}

	openapiPath := cfg.OpenAPIPath
	if openapiPath == "" {
		openapiPath = "/openapi.json"
	}
	docsPath := cfg.DocsPath
	if docsPath == "" {
		docsPath = "/docs"
	}

	switch cfg.UI {
	case DocUISwagger:
		registerSwaggerUI(r, docsPath, openapiPath)
	case DocUIRedoc:
		registerRedocUI(r, docsPath, openapiPath)
	case DocUINone:
		// 仅 OpenAPI JSON，无 UI 页面
	}
}

// registerSwaggerUI 注册 Swagger UI 页面。
func registerSwaggerUI(r gin.IRouter, docsPath, openapiPath string) {
	r.GET(docsPath, func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, swaggerHTML)
	})
	r.GET("/swagger", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, swaggerHTML)
	})
}

// registerRedocUI 注册 Redoc 页面。
func registerRedocUI(r gin.IRouter, docsPath, openapiPath string) {
	r.GET(docsPath, func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, redocHTML)
	})
	r.GET("/redoc", func(c *gin.Context) {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, redocHTML)
	})
}
