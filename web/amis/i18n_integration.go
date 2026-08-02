package amis

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/izliang/bore/i18n"
)

// I18nConfig amis i18n 整合配置。
//
// 桥接现有的 bore/i18n 翻译包与 amis 渲染管线：
//   - 将 Bundle 中的翻译数据转换为 amis replaceText map
//   - 在 Schema 注册时进行 key 前置校验（StrictMode）
type I18nConfig struct {
	// Bundle i18n 翻译包（必填）。
	Bundle *i18n.Bundle

	// KeyPrefix Schema 中标记翻译 key 的前缀，默认 "$t."。
	//
	// Schema 中使用 "$t.ui.btn.submit" 引用翻译 key "ui.btn.submit"。
	KeyPrefix string

	// StrictMode 控制 Schema key 前置校验：
	//   true  — RegisterSchema 时校验所有 $t.* key 是否在 Bundle 中存在
	//   false — 跳过校验，翻译缺失时前端降级显示原始 key
	// 默认 false。
	StrictMode bool

	// ValidateLocales 校验时检查的 locale 列表。
	// 空 = 检查 Bundle 中所有已加载的 locale。
	ValidateLocales []string

	// ForceLocale 强制指定渲染语言，忽略请求中的语言检测结果。
	//
	// 空字符串表示不强制（默认行为：跟随请求语言）。
	// 非空时，所有页面始终使用此 locale 渲染。
	// 示例："zh" 强制中文、"en" 强制英文。
	ForceLocale string
}

// normalize 填充默认值，必要时 panic。
func (c *I18nConfig) normalize() {
	if c.Bundle == nil {
		panic("amis: I18nConfig.Bundle is required")
	}
	if c.KeyPrefix == "" {
		c.KeyPrefix = "$t."
	}
}

// buildReplaceText 从 Bundle 构建 amis replaceText map。
//
// 返回的 map 中 key 为 "KeyPrefix" + entry.Key，value 为 entry.Value。
// 若指定 locale 在 Bundle 中无数据，返回空 map。
func (c *I18nConfig) buildReplaceText(locale string) map[string]string {
	entries := c.Bundle.Entries(locale)
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		result[c.KeyPrefix+entry.Key] = entry.Value
	}
	return result
}

// buildReplaceTextJS 生成注入模板的 JavaScript 对象字面量。
func (c *I18nConfig) buildReplaceTextJS(locale string) template.JS {
	m := c.buildReplaceText(locale)
	if len(m) == 0 {
		return template.JS("{}")
	}
	data, err := json.Marshal(m)
	if err != nil {
		return template.JS("{}")
	}
	return template.JS(string(data))
}

// resolveLocale 归一化语言代码。
func (c *I18nConfig) resolveLocale(raw string) string {
	return i18n.NormalizeLang(raw)
}

// validateSchemaKeys 校验 schema 中所有 $t.* key 是否在 Bundle 中存在。
//
// 遍历 schema 树，收集所有以 KeyPrefix 开头的字符串值，
// 去除前缀后通过 Bundle.Entry() 交叉比对（不使用 Has()，因其有默认语言回退）。
// 若 StrictMode=false，调用方不应调用此方法。
func (c *I18nConfig) validateSchemaKeys(schema interface{}) error {
	keys := collectI18nKeys(schema, c.KeyPrefix)
	if len(keys) == 0 {
		return nil
	}

	locales := c.ValidateLocales
	if len(locales) == 0 {
		locales = c.Bundle.Languages()
	}

	missing := make(map[string][]string)

	for _, lang := range locales {
		var missingKeys []string
		for _, key := range keys {
			// 使用 Entry() 而非 Has()，避免默认语言回退导致漏检
			if _, ok := c.Bundle.Entry(lang, key); !ok {
				missingKeys = append(missingKeys, key)
			}
		}
		if len(missingKeys) > 0 {
			missing[lang] = missingKeys
		}
	}

	if len(missing) > 0 {
		return &SchemaKeyError{
			Missing: missing,
		}
	}
	return nil
}

// SchemaKeyError schema key 校验失败的结构化错误。
type SchemaKeyError struct {
	// Missing locale → 缺失的 key 列表。
	Missing map[string][]string
}

// Error 实现 error 接口。
func (e *SchemaKeyError) Error() string {
	var b strings.Builder
	b.WriteString("amis: schema i18n key validation failed (strict mode)\n")
	for lang, keys := range e.Missing {
		fmt.Fprintf(&b, "  [%s] missing %d keys: %v\n", lang, len(keys), keys)
	}
	b.WriteString("  hint: add missing i18n entries or set StrictMode=false")
	return b.String()
}

// collectI18nKeys 递归遍历 schema，收集所有带指定前缀的字符串值（去除前缀后的 key）。
//
// 去重：相同 key 只收集一次。
func collectI18nKeys(v interface{}, prefix string) []string {
	var keys []string
	seen := make(map[string]bool)

	var walk func(v interface{})
	walk = func(v interface{}) {
		switch val := v.(type) {
		case map[string]interface{}:
			for _, item := range val {
				walk(item)
			}
		case []interface{}:
			for _, item := range val {
				walk(item)
			}
		case string:
			if strings.HasPrefix(val, prefix) {
				key := val[len(prefix):]
				if key != "" && !seen[key] {
					seen[key] = true
					keys = append(keys, key)
				}
			}
		}
	}

	walk(v)
	return keys
}
