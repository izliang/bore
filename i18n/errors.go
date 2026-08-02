package i18n

import (
	"fmt"
	"strings"
)

// ValidationError key 校验失败的结构化错误。
//
// 列出每种语言缺少哪些 key，以及具体的文件名信息。
type ValidationError struct {
	// Missing 语言代码 → 缺失的 key 列表
	Missing map[string][]string
	// Files 语言代码 → 包含该语言的文件名列表（信息性）
	Files map[string][]string
}

// Error 实现 error 接口。
func (e *ValidationError) Error() string {
	var b strings.Builder
	b.WriteString("i18n: key validation failed (strict mode)\n")
	for lang, keys := range e.Missing {
		files := e.Files[lang]
		fmt.Fprintf(&b, "  [%s] missing %d keys: %v\n", lang, len(keys), keys)
		if len(files) > 0 {
			fmt.Fprintf(&b, "    files: %s\n", strings.Join(files, ", "))
		}
	}
	b.WriteString("  hint: add missing entries or set StrictMode=false")
	return b.String()
}

// LoadError 文件加载过程中的错误聚合。
type LoadError struct {
	Errors []error
}

// Error 实现 error 接口。
func (e *LoadError) Error() string {
	var b strings.Builder
	b.WriteString("i18n: load errors:\n")
	for _, err := range e.Errors {
		fmt.Fprintf(&b, "  - %s\n", err.Error())
	}
	return b.String()
}

// Unwrap 返回聚合的所有错误，支持 errors.Is / errors.As。
func (e *LoadError) Unwrap() []error {
	return e.Errors
}
