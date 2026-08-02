package i18n

import (
	"os"
	"strings"
)

// NormalizeLang 归一化语言代码为两字母 ISO 639-1 格式。
//
// 处理常见的区域变体和书写方式：
//
//	"zh-CN" → "zh"
//	"zh_cn" → "zh"
//	"en-US" → "en"
//	"ja"    → "ja"
//
// 未知语言代码取前两个字符，少于两个字符 fallback 到 "en"。
func NormalizeLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))

	// 中文变体
	switch lang {
	case "zh-cn", "zh_cn", "zh-hans", "zh_hans", "zh-tw", "zh_tw", "zh-hk", "zh_hk":
		return "zh"
	case "en-us", "en_us", "en-gb", "en_gb":
		return "en"
	}

	// 通用规则：取前两个字符
	if len(lang) >= 2 {
		return lang[:2]
	}
	return "en"
}

// GetLangFromEnv 从环境变量获取语言代码。
//
// 检测优先级：LC_ALL → LANG → "en"。
func GetLangFromEnv() string {
	lang := os.Getenv("LC_ALL")
	if lang == "" {
		lang = os.Getenv("LANG")
	}
	if lang == "" {
		return "en"
	}
	return NormalizeLang(lang)
}

// extractLang 从文件名提取语言代码。
//
// 规则：文件名以下划线 _ 分隔，第一个段（前缀）即为语言代码。
// 无下划线时，整个文件名（去掉 .json 后缀）为语言代码。
//
//	zh.json        → "zh"
//	zh_errors.json → "zh"
//	en_ui.json     → "en"
func extractLang(filename string) string {
	// 去掉 .json 后缀
	base := strings.TrimSuffix(filename, ".json")
	// 取第一个下划线之前的部分；无下划线则取全部
	if idx := strings.Index(base, "_"); idx >= 0 {
		return base[:idx]
	}
	return base
}
