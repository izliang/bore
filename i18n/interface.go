package i18n

// ────────────────────────────────────────────────────────────
// 接口定义（保留，用于 web/lang 等消费者的 Mock/解耦需求）
// ────────────────────────────────────────────────────────────

// Translator 应用级翻译器接口。
//
// Bundle 实现了此接口。消费者如需 Mock 测试，可自行定义子集接口
//（Go 隐式接口满足），或直接使用此接口。
//
// 日常推荐直接使用 *Bundle 具体类型，IDE 跳转更直接。
type Translator interface {
	// T 将 key 翻译为 lang 对应的文本。
	T(lang string, key string) string

	// Tf 带模板变量的翻译。
	Tf(lang string, key string, data map[string]any) string

	// Has 判断 key 在 lang 下是否有翻译。
	Has(lang string, key string) bool

	// Languages 返回所有已加载的语言代码列表。
	Languages() []string
}

// Localizer 请求级翻译器接口。
//
// 将 Translator 与固定语言绑定，避免每个调用都传递 lang。
//
// *RequestLocalizer 结构体实现了此接口。
type Localizer interface {
	// T 使用绑定语言翻译 key。
	T(key string) string

	// Tf 使用绑定语言带模板翻译 key。
	Tf(key string, data map[string]any) string

	// Lang 返回绑定的语言代码。
	Lang() string
}

// NewRequestLocalizer 创建请求级翻译器（向后兼容旧 API）。
//
// 推荐使用 bundle.L(lang) 代替此函数。
// 保留此函数仅为向后兼容 web/lang 等已有调用方。
//
// 参数：
//
//	t    — Translator 实例（通常是 *Bundle）
//	lang — 请求语言代码（如 "zh"、"en"）
func NewRequestLocalizer(t Translator, lang string) Localizer {
	return &RequestLocalizer{
		translator: t,
		lang:       NormalizeLang(lang),
	}
}
