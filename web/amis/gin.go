package amis

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/izliang/bore/web/lang"
)

// GinHandler 为已注册的 amis 页面创建一个 Gin 原生处理函数。
//
// 返回的 handler 自动设置 Content-Type: text/html; charset=utf-8，
// 并在页面未注册时返回 404。
//
// 若已挂载 LanguageMiddleware 并启用 i18n，handler 会自动从
// gin.Context 读取语言偏好并注入翻译数据。
func (e *Engine) GinHandler(path string) gin.HandlerFunc {
	return func(c *gin.Context) {
		locale := ""
		if e.i18nCfg != nil {
			locale = lang.GetRequestLang(c)
		}
		html := e.renderWithLocale(path, locale)
		if html == "" {
			c.String(http.StatusNotFound, "amis page not found: %s", path)
			return
		}
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, html)
	}
}

// BindGin 将所有匹配路由组前缀的 amis 页面一键挂载到指定的 Gin 路由上。
//
// 接受 gin.IRouter 接口（*gin.Engine 和 *gin.RouterGroup 均可）。
//
// 智能前缀处理：
//   - 绑定到根 Engine：注册所有页面，路径保持不变
//   - 绑定到带前缀的 RouterGroup（如 "/admin"）：仅注册路径以该前缀
//     开头的页面，并自动剥离前缀。path == prefix 时返回 ""（无尾部斜杠）。
//
// 示例：
//
//	engine.RegisterJSONFile("/admin/users", "u.json") // → 引擎路径 /admin/users
//
//	protected := r.Group("/admin", middleware)
//	engine.BindGin(protected)
//	// → GET /admin    (app page, 无尾部斜杠)
//	// → GET /admin/users
func (e *Engine) BindGin(r gin.IRouter) {
	var prefix string
	if rg, ok := r.(*gin.RouterGroup); ok {
		prefix = rg.BasePath()
	}

	for _, path := range e.Pages() {
		// 仅挂载匹配路由组前缀的页面
		if prefix != "" && prefix != "/" && !strings.HasPrefix(path, prefix) {
			continue
		}
		routePath := stripGroupPrefix(path, prefix)
		r.GET(routePath, e.GinHandler(path))
	}
}

// stripGroupPrefix 从引擎内部路径中剥离路由组前缀。
//
// 特殊处理：path == prefix 时返回 ""（空字符串），
// Gin 的 joinPaths 会将其正确解析为不含尾部斜杠的路径。
//
// 示例：
//
//	stripGroupPrefix("/admin",          "/admin") → ""           → Gin 注册为 /admin
//	stripGroupPrefix("/admin/dashboard", "/admin") → "/dashboard" → Gin 注册为 /admin/dashboard
//	stripGroupPrefix("/login",           "/admin") → "/login"     → 跳过（不匹配前缀）
func stripGroupPrefix(path, prefix string) string {
	if prefix == "" || prefix == "/" {
		return path
	}
	// 精确匹配前缀：返回空字符串，避免 Gin 追加尾部斜杠导致 301 重定向循环
	if path == prefix {
		return ""
	}
	if strings.HasPrefix(path, prefix) {
		return path[len(prefix):]
	}
	return path
}
