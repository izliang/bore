package amis

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Engine 是 amis 页面引擎的核心结构体。
//
// 负责管理 AssetProvider、模板引擎、已注册的页面 Schema，
// 并提供 Gin 路由适配能力。
//
// 零值不可用，必须通过 New 创建。
type Engine struct {
	assets          AssetProvider
	tmpl            *template.Template
	templateContent string // 自定义模板内容，空表示使用默认
	title           string
	adapter         ResponseAdapter // 响应格式适配器
	extraCSS        string           // 注入模板 <style> 的额外 CSS
	extraJS         string           // 注入模板 <script> 的额外 JS
	extra           map[string]interface{}
	i18nCfg         *I18nConfig // i18n 整合配置，nil 表示未启用

	mu    sync.RWMutex
	pages map[string]interface{} // path -> schema（interface{} 类型：string / map / struct）
}

// New 创建 Engine 实例。
//
// 可选参数通过 Option 函数传入。未指定时使用默认配置：
//   - AssetProvider: CDN latest + ThemeCXD
//   - Title: "amis"
//   - Template: 内嵌默认模板
//
// 示例：
//
//	engine := amis.New(
//	    amis.WithCDN("6.10.0", amis.ThemeCXD),
//	    amis.WithTitle("管理后台"),
//	)
func New(opts ...Option) *Engine {
	e := &Engine{
		pages: make(map[string]interface{}),
		title: "amis",
	}

	// 应用选项
	for _, opt := range opts {
		opt(e)
	}

	// 默认 AssetProvider
	if e.assets == nil {
		e.assets = NewCDNAssets("", ThemeCXD)
	}

	// 默认响应适配器
	if e.adapter.CodeField == "" {
		e.adapter = DefaultResponseAdapter()
	}

	// 初始化模板
	tmpl, err := parseTemplate("amis-default", e.templateContent)
	if err != nil {
		panic(fmt.Sprintf("amis: failed to parse template: %v", err))
	}
	e.tmpl = tmpl

	return e
}

// RegisterSchema 注册一个 amis 页面 Schema。
//
// path: 页面 URL 路径（如 "/dashboard" 或 "/admin/users"）。
// schema: 页面 Schema，支持以下类型：
//   - string: JSON 字符串
//   - map[string]interface{} / []interface{}: 已解析的 JSON 对象
//   - struct: 任意 Go 结构体（自动序列化为 JSON）
//
// 若启用了 I18nConfig.StrictMode，注册时会校验 schema 中所有 $t.* key
// 是否在 Bundle 中存在。
//
// 示例：
//
//	engine.RegisterSchema("/dashboard", map[string]interface{}{
//	    "type": "page",
//	    "title": "$t.dashboard.title",
//	    "body": "欢迎使用 amis",
//	})
func (e *Engine) RegisterSchema(path string, schema interface{}) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 规范化路径：确保以 / 开头，去除末尾 /
	path = cleanPath(path)

	// 将 schema 转为可存储的形式，同时验证是否可序列化
	if _, err := toJSON(schema); err != nil {
		return fmt.Errorf("amis: register schema %q: %w", path, err)
	}

	// i18n key 前置校验（StrictMode）
	if e.i18nCfg != nil && e.i18nCfg.StrictMode {
		if err := e.i18nCfg.validateSchemaKeys(schema); err != nil {
			return fmt.Errorf("amis: register schema %q: %w", path, err)
		}
	}

	e.pages[path] = schema
	return nil
}

// RegisterJSONFile 从本地 JSON 文件注册一个 amis 页面 Schema。
//
// path: 页面 URL 路径。
// filePath: 本地 JSON 文件路径。
//
// 示例：
//
//	engine.RegisterJSONFile("/users", "schemas/users.json")
func (e *Engine) RegisterJSONFile(path string, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("amis: read json file %q: %w", filePath, err)
	}

	var schema interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("amis: parse json file %q: %w", filePath, err)
	}

	return e.RegisterSchema(path, schema)
}

// LoadPagesDir 扫描指定目录下的所有 .json 文件，自动映射为 amis 页面路由。
//
// prefix: URL 路径前缀（如 "/admin"）。
// dir: 本地目录路径（如 "./pages"）。
//
// 路由映射规则：
//   - dir/dashboard.json     → prefix/dashboard
//   - dir/users/list.json    → prefix/users/list
//   - dir/models/form.json   → prefix/models/form
//
// 示例：
//
//	engine.LoadPagesDir("/admin", "./pages")
//	// 生成路由: /admin/dashboard, /admin/users/list, /admin/models/form
func (e *Engine) LoadPagesDir(prefix string, dir string) error {
	prefix = cleanPath(prefix)

	return filepath.Walk(dir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".json") || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// 计算相对路径作为路由后缀
		relPath, err := filepath.Rel(dir, filePath)
		if err != nil {
			return fmt.Errorf("amis: resolve relative path %q: %w", filePath, err)
		}

		// 去除 .json 扩展名，将路径分隔符转为 /
		route := strings.TrimSuffix(relPath, ".json")
		route = filepath.ToSlash(route)

		// 拼接完整路由
		fullPath := prefix + "/" + route
		fullPath = cleanPath(fullPath)

		return e.RegisterJSONFile(fullPath, filePath)
	})
}

// getSchema 获取指定路径的页面 Schema（JSON 字符串形式）。
//
// 返回空字符串表示路径未注册。
func (e *Engine) getSchema(path string) (string, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	schema, ok := e.pages[path]
	if !ok {
		return "", false
	}

	jsonStr, err := toJSON(schema)
	if err != nil {
		return "", false
	}
	return jsonStr, true
}

// Render 渲染指定路径的 amis 页面为完整 HTML。
//
// 若路径未注册，返回空字符串。
// 向后兼容：不注入 locale/replaceText（使用 amis 默认行为）。
func (e *Engine) Render(path string) string {
	return e.renderWithLocale(path, "")
}


// resolveEffectiveLocale 返回实际生效的渲染 locale。
//
// 优先级：ForceLocale（若配置）> 请求 locale。
func (e *Engine) resolveEffectiveLocale(requestLocale string) string {
	if e.i18nCfg != nil && e.i18nCfg.ForceLocale != "" {
		return e.i18nCfg.resolveLocale(e.i18nCfg.ForceLocale)
	}
	return requestLocale
}

// renderWithLocale 使用指定 locale 渲染 amis 页面。
//
// 若未启用 i18n 或 locale 为空，不注入 i18n 相关数据。
func (e *Engine) renderWithLocale(path string, locale string) string {
	locale = e.resolveEffectiveLocale(locale)
	schemaJSON, ok := e.getSchema(path)
	if !ok {
		return ""
	}

	ctx := &PageContext{
		Title:      e.title,
		CSSURLs:    e.assets.CSSURLs(),
		JSURLs:     e.assets.JSURLs(),
		SchemaJSON: template.JS(schemaJSON),
		AdapterJS:  e.adapter.ToJS(),
		ExtraCSS:   template.CSS(e.extraCSS),
		ExtraJS:    template.JS(e.extraJS),
		Extra:      e.extra,
	}

	// i18n 数据注入
	if e.i18nCfg != nil && locale != "" {
		locale = e.i18nCfg.resolveLocale(locale)
		ctx.Locale = locale
		ctx.ReplaceTextJSON = e.i18nCfg.buildReplaceTextJS(locale)
	}

	html, err := renderPage(e.tmpl, ctx)
	if err != nil {
		return ""
	}
	return html
}

// Pages 返回所有已注册的页面路径列表。
func (e *Engine) Pages() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	paths := make([]string, 0, len(e.pages))
	for p := range e.pages {
		paths = append(paths, p)
	}
	return paths
}

// I18n 返回 i18n 配置（若已启用），否则返回 nil。
func (e *Engine) I18n() *I18nConfig {
	return e.i18nCfg
}

// ============================================================
// 内部辅助函数
// ============================================================

// cleanPath 规范化 URL 路径：
//   - 确保以 / 开头
//   - 去除末尾 /
//   - 空路径转为 /
func cleanPath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "/"
	}
	return path
}

// toJSON 将 schema 统一序列化为格式化的 JSON 字符串。
func toJSON(v interface{}) (string, error) {
	switch val := v.(type) {
	case string:
		// 验证是否为合法 JSON，同时格式化
		var tmp interface{}
		if err := json.Unmarshal([]byte(val), &tmp); err != nil {
			return "", fmt.Errorf("invalid JSON string: %w", err)
		}
		b, err := json.Marshal(tmp)
		return string(b), err
	case []byte:
		return toJSON(string(val))
	default:
		b, err := json.Marshal(val)
		return string(b), err
	}
}
