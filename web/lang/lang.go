package lang

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/izliang/bore/i18n"
)

const (
	// GinContextLangKey 存在 Gin Context 中的语言 key
	GinContextLangKey = "i18n_lang"

	// HeaderNameCustomLang 自定义语言头
	HeaderNameCustomLang = "Language"
	// HeaderNameAcceptLang 标准 Accept-Language 头
	HeaderNameAcceptLang = "Accept-Language"
	// QueryNameLang Query 参数名
	QueryNameLang = "lang"
	// CookieNameLang Cookie 名
	CookieNameLang = "lang"

	// DefaultLang 默认语言
	DefaultLang = "zh"
)

// GetRequestLang 获取请求的语言，带多级 fallback。
//
// 检测优先级（从高到低）：
//  1. Gin Context 缓存值
//  2. 自定义 Language Header
//  3. Accept-Language Header
//  4. Query 参数 ?lang=
//  5. Cookie
//  6. 已登录用户偏好
//  7. DefaultLang
func GetRequestLang(c *gin.Context) string {
	if c == nil {
		return DefaultLang
	}

	// 1. 优先从 Gin Context 取（中间件可能已经解析过）
	if lang, ok := c.Get(GinContextLangKey); ok {
		if s, ok := lang.(string); ok && s != "" {
			return s
		}
	}

	// 2. 从自定义 Header 取
	if lang := strings.TrimSpace(c.GetHeader(HeaderNameCustomLang)); lang != "" {
		normalized := i18n.NormalizeLang(lang)
		c.Set(GinContextLangKey, normalized)
		return normalized
	}

	// 3. 从 Accept-Language 取
	if lang := strings.TrimSpace(c.GetHeader(HeaderNameAcceptLang)); lang != "" {
		normalized := i18n.NormalizeLang(lang)
		c.Set(GinContextLangKey, normalized)
		return normalized
	}

	// 4. 从 Query 参数取
	if lang := strings.TrimSpace(c.Query(QueryNameLang)); lang != "" {
		normalized := i18n.NormalizeLang(lang)
		c.Set(GinContextLangKey, normalized)
		return normalized
	}

	// 5. 从 Cookie 取
	if lang, err := c.Cookie(CookieNameLang); err == nil {
		if normalized := strings.TrimSpace(lang); normalized != "" {
			normalized = i18n.NormalizeLang(normalized)
			c.Set(GinContextLangKey, normalized)
			return normalized
		}
	}

	// 6. 从用户信息取（如果已登录，用户偏好设置）
	if userLang := getUserLangFromContext(c); userLang != "" {
		normalized := i18n.NormalizeLang(userLang)
		c.Set(GinContextLangKey, normalized)
		return normalized
	}

	// 7. Fallback 到默认
	c.Set(GinContextLangKey, DefaultLang)
	return DefaultLang
}

// SetRequestLang 手动设置请求语言。
func SetRequestLang(c *gin.Context, lang string) {
	normalized := i18n.NormalizeLang(lang)
	c.Set(GinContextLangKey, normalized)
}

// GetLocalizer 创建请求级翻译器。
//
// 从 gin.Context 中自动检测语言，与传入的 Translator 绑定。
//
// 参数：
//
//	t — 应用级 Translator（通常为 i18n.Bundle 实例）
func GetLocalizer(c *gin.Context, t i18n.Translator) i18n.Localizer {
	lang := GetRequestLang(c)
	return i18n.NewRequestLocalizer(t, lang)
}

// getUserLangFromContext 从已登录用户信息中获取语言偏好。
func getUserLangFromContext(c *gin.Context) string {
	// 如果你的系统有用户上下文，可以从这里取
	// 示例：
	// if user, ok := c.Get("current_user"); ok {
	//     if u, ok := user.(*User); ok {
	//         return u.Lang
	//     }
	// }
	return ""
}
