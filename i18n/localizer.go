package i18n

// ────────────────────────────────────────────────────────────
// RequestLocalizer — 请求级翻译器（Localizer 接口的实现）
// ────────────────────────────────────────────────────────────

// RequestLocalizer 请求级翻译器，实现 Localizer 接口。
//
// 将 Translator 与固定语言绑定，避免每个翻译调用处都传递 lang 参数。
// 适用于一次请求/会话对应一种语言的场景。
//
// 零分配设计：仅持有 Translator 接口值 + 语言字符串，不复制翻译数据。
//
// 典型用法：
//
//	loc := bundle.L("zh")
//	msg := loc.T("greeting")
//	msg := loc.Tf("greet_name", map[string]any{"Name": "小明"})
type RequestLocalizer struct {
	translator Translator
	lang       string
}

// L 创建请求级翻译器，绑定指定语言。
//
// lang 会自动归一化（如 "zh-CN" → "zh"）。
func (b *Bundle) L(lang string) *RequestLocalizer {
	return &RequestLocalizer{
		translator: b,
		lang:       NormalizeLang(lang),
	}
}

// T 实现 Localizer 接口。使用绑定语言翻译 key。
func (l *RequestLocalizer) T(key string) string {
	return l.translator.T(l.lang, key)
}

// Tf 实现 Localizer 接口。使用绑定语言带模板变量翻译 key。
func (l *RequestLocalizer) Tf(key string, data map[string]any) string {
	return l.translator.Tf(l.lang, key, data)
}

// Lang 实现 Localizer 接口。返回当前绑定的语言代码（已归一化）。
func (l *RequestLocalizer) Lang() string {
	return l.lang
}
