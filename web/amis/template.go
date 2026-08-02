package amis

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"strings"
)

//go:embed template.html
var defaultTemplateContent string

// PageContext 模板渲染上下文，承载一个 amis 页面所需的所有数据。
type PageContext struct {
	// Title 页面标题，显示在浏览器标签页。
	Title string

	// CSSURLs 需要加载的 CSS 样式表 URL 列表。
	CSSURLs []string

	// JSURLs 需要加载的 JavaScript 脚本 URL 列表。
	JSURLs []string

	// SchemaJSON 已格式化的 amis 页面 Schema JSON（作为原始 JS 注入，不做 HTML 转义）。
	SchemaJSON template.JS

	// AdapterJS 响应格式适配器 JavaScript 代码。
	// 由 ResponseAdapter.ToJS() 生成，注入到 <script> 中。
	AdapterJS template.JS

	// ExtraCSS 用户注入的额外 CSS 样式，渲染在 <style> 标签内。
	// 用于业务定制（品牌色、导航样式、暗黑模式配色等），避免修改 SDK 模板。
	ExtraCSS template.CSS

	// ExtraJS 用户注入的额外 JavaScript，渲染在 <script> 标签内。
	// 用于业务定制（埋点、自定义交互逻辑等）。
	ExtraJS template.JS

	// Extra 用户自定义的额外模板数据，可在自定义模板中通过 .Extra.key 访问。
	Extra map[string]interface{}

	// Locale 当前请求的语言代码（已归一化），如 "zh"、"en"。
	// 注入到 amis embed 的 props.locale 和 <html lang=""> 属性。
	// 空字符串表示未启用 i18n。
	Locale string

	// ReplaceTextJSON amis replaceText map 的 JSON 序列化结果。
	// 注入到 amis embed 的 env.replaceText。
	// 空对象 "{}" 表示无替换。
	ReplaceTextJSON template.JS
}

// parseTemplate 解析 HTML 模板内容为 template.Template。
//
// content: 模板字符串。若为空，使用内嵌的默认模板。
func parseTemplate(name string, content string) (*template.Template, error) {
	if content == "" {
		content = defaultTemplateContent
	}
	tmpl, err := template.New(name).Parse(content)
	if err != nil {
		return nil, fmt.Errorf("amis: parse template %q: %w", name, err)
	}
	return tmpl, nil
}

// renderPage 将 PageContext 渲染为完整的 HTML 字符串。
func renderPage(tmpl *template.Template, ctx *PageContext) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("amis: render page %q: %w", ctx.Title, err)
	}
	// 移除模板渲染可能产生的多余空行，保持输出整洁
	return strings.TrimSpace(buf.String()), nil
}
