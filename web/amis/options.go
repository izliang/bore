package amis

// Option 是 Engine 的功能选项函数。
//
// 所有可选的配置参数都通过此模式注入，遵循 Functional Options Pattern。
type Option func(*Engine)

// WithAssets 设置自定义 AssetProvider。
//
// 使用场景：
//   - 替换 CDN 地址（如内网 npm 镜像）
//   - 使用本地静态文件服务器
//   - 锁定特定 amis 版本以保持稳定性
//
// 如果同时设置了 WithCDN，后者会被忽略（WithAssets 优先级更高）。
//
// 示例：
//
//	engine := amis.New(amis.WithAssets(myCustomProvider))
func WithAssets(assets AssetProvider) Option {
	return func(e *Engine) {
		e.assets = assets
	}
}

// WithCDN 使用 CDN 模式，快捷创建并设置 CDNAssets。
//
// version: amis 版本号（如 "6.10.0"），传空字符串使用 latest。
// theme: 主题（ThemeCXD / ThemeDark / ThemeAntd）。
//
// 等效于：
//
//	amis.WithAssets(amis.NewCDNAssets(version, theme))
//
// 如果同时设置了 WithAssets，WithCDN 会被忽略。
//
// 示例：
//
//	engine := amis.New(amis.WithCDN("6.10.0", amis.ThemeCXD))
func WithCDN(version string, theme Theme) Option {
	return WithAssets(NewCDNAssets(version, theme))
}

// WithCustomTemplate 使用自定义 HTML 模板。
//
// 模板必须包含以下占位符才能正常渲染：
//   - {{.Title}}          页面标题
//   - {{.CSSURLs}}        CSS 资源列表
//   - {{.JSURLs}}         JS 资源列表
//   - {{.SchemaJSON}}     页面 Schema（作为原始 JS 注入）
//   - {{.Extra.key}}      自定义扩展数据
//
// 使用场景：
//   - 追加企业级 Header/Footer
//   - 注入全局 CSS 变量或自定义 JS 逻辑
//   - 添加第三方监控/埋点脚本
//
// 示例：
//
//	engine := amis.New(amis.WithCustomTemplate(myTemplate))
func WithCustomTemplate(content string) Option {
	return func(e *Engine) {
		e.templateContent = content
	}
}

// WithTitle 设置所有页面的默认标题（浏览器标签页）。
//
// 单个页面注册时可通过 schema 中配置的 title 覆盖此默认值。
func WithTitle(title string) Option {
	return func(e *Engine) {
		e.title = title
	}
}

// WithExtraData 设置注入到模板渲染上下文的全局扩展数据。
//
// 在自定义模板中通过 .Extra.key 访问这些数据。
//
// 示例：
//
//	amis.New(amis.WithExtraData(map[string]interface{}{
//	    "appName": "bore Admin",
//	    "version": "1.0.0",
//	}))
func WithExtraData(data map[string]interface{}) Option {
	return func(e *Engine) {
		e.extra = data
	}
}

// WithExtraCSS 注入额外的 CSS 样式到所有页面。
//
// 内容会被渲染在 <style> 标签中，位于 SDK 基础样式之后。
// 用于业务级样式定制（品牌色、导航、暗黑模式等），无需替换整个模板。
//
// 示例：
//
//	engine := amis.New(amis.WithExtraCSS(`
//	  .cxd-Nav-item.is-active { background: #e8f0fe; }
//	`))
func WithExtraCSS(css string) Option {
	return func(e *Engine) {
		e.extraCSS = css
	}
}

// WithExtraJS 注入额外的 JavaScript 到所有页面。
//
// 内容会被渲染在 <script> 标签中，位于 SDK 核心脚本之后。
// 用于业务级脚本定制（埋点、自定义交互等），无需替换整个模板。
//
// 示例：
//
//	engine := amis.New(amis.WithExtraJS(`
//	  console.log('amis page loaded');
//	`))
func WithExtraJS(js string) Option {
	return func(e *Engine) {
		e.extraJS = js
	}
}

// WithResponseAdapter 设置 API 响应格式适配器。
//
// amis 前端期望 {status:0,msg:"",data:{...}} 格式。
// 后端通常使用不同的字段命名（code/errno/retCode 等），
// 适配器负责将这些字段映射到 amis 标准格式。
//
// 默认使用信封模式：{"code":"000000","message":"success","data":{...}}
//
// 示例：
//
//	// 适配 {errno:0,errmsg:"",result:{...}} 格式
//	engine := amis.New(amis.WithResponseAdapter(amis.ResponseAdapter{
//	    CodeField:    "errno",
//	    MessageField: "errmsg",
//	    DataField:    "result",
//	    SuccessValue: "0",
//	}))
func WithResponseAdapter(adapter ResponseAdapter) Option {
	return func(e *Engine) {
		e.adapter = adapter
	}
}

// WithI18n 启用 i18n 国际化支持。
//
// cfg.Bundle 为已加载的 *i18n.Bundle 实例。
// 设置后 Engine 会在渲染时根据请求 locale 注入 replaceText map 到 amis 渲染环境。
//
// 示例：
//
//	bundle := i18n.MustLoad(i18n.Config{Dir: "./locales", DefaultLang: "zh"})
//	engine := amis.New(amis.WithI18n(amis.I18nConfig{
//	    Bundle:     bundle,
//	    StrictMode: true,
//	}))
func WithI18n(cfg I18nConfig) Option {
	return func(e *Engine) {
		cfg.normalize()
		e.i18nCfg = &cfg
	}
}
