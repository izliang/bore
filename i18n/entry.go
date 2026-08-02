// Package i18n 提供国际化翻译能力。
//
// 核心设计：
//   - 零外部依赖，仅用标准库 encoding/json + text/template
//   - 兼容 go-i18n v2 的三种 JSON 文件格式（自动检测）
//   - 文件名前缀确定语言：zh.json / zh_errors.json → lang="zh"
//   - 启动时 Key 全量校验（StrictMode）
//   - Entry 固定字段 + Extra map 支持扩展
package i18n

import (
	"encoding/json"
	"fmt"
)

// Entry 翻译条目 —— JSON 文件中每个对象的 Go 表示。
//
// Key 和 Value 为必填字段。其余字段均为可选。
// 自定义的未知字段会被收入 Extra map，SDK 透传。
//
// 三种文件格式归一化后的统一结构：
//
//	格式 1（新数组）  ：key/value 直接对应 JSON 的 "key"/"value"
//	格式 2（go-i18n 扁平）：JSON key 名 → Entry.Key，"other" → Entry.Value
//	格式 3（go-i18n 数组）："id" → Entry.Key，"other" → Entry.Value
type Entry struct {
	// Key 翻译键 —— 跨语言唯一的稳定标识符。
	// 命名约定：使用点分隔的层级结构，如 "error.not_found"、"ui.btn.submit"。
	Key string `json:"key"`

	// Value 翻译文本 —— 支持 Go text/template 语法的模板字符串。
	// 示例："你好 {{.Name}}，欢迎来到 {{.City}}"
	Value string `json:"value"`

	// Description 人类可读的描述信息，用于帮助翻译人员理解上下文。
	// 在 go-i18n 格式中对应 "description" 字段。
	Description string `json:"desc,omitempty"`

	// Deprecated 标记此翻译条目是否已废弃。
	// 废弃条目仍可正常翻译，但消费者可用于展示警告。
	Deprecated bool `json:"deprecated,omitempty"`

	// Tags 分类标签，用于按模块/域筛选条目。
	// 示例：["button", "form", "error"]
	Tags []string `json:"tags,omitempty"`

	// Extra JSON 中所有未被上述已知字段覆盖的字段。
	// go-i18n 格式的复数形式（zero/one/two/few/many）也会进入这里。
	// 消费者可通过类型断言读取：entry.Extra["max_length"].(float64)
	Extra map[string]any `json:"-"`
}

// UnmarshalJSON 实现自定义 JSON 反序列化。
//
// 策略：先解析到 map[string]any，提取已知字段后，剩余写入 Extra。
// 这样任意 JSON 扩展字段都能无损保留，无需修改 Entry 结构体。
func (e *Entry) UnmarshalJSON(data []byte) error {
	// 先解析到通用 map
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("i18n: unmarshal entry: %w", err)
	}

	// 提取已知字段
	extractString := func(key string) string {
		if v, ok := raw[key].(string); ok {
			delete(raw, key)
			return v
		}
		return ""
	}

	e.Key = extractString("key")
	e.Value = extractString("value")
	e.Description = extractString("desc")

	if v, ok := raw["deprecated"].(bool); ok {
		e.Deprecated = v
		delete(raw, "deprecated")
	}

	if v, ok := raw["tags"].([]any); ok {
		e.Tags = make([]string, 0, len(v))
		for _, t := range v {
			if s, ok := t.(string); ok {
				e.Tags = append(e.Tags, s)
			}
		}
		delete(raw, "tags")
	}

	// 剩余字段全部进入 Extra
	if len(raw) > 0 {
		e.Extra = raw
	}

	return nil
}

// Validate 检查 Entry 的必填字段。
func (e *Entry) Validate() error {
	if e.Key == "" {
		return fmt.Errorf("i18n: entry key is required")
	}
	if e.Value == "" {
		return fmt.Errorf("i18n: entry %q value is required", e.Key)
	}
	return nil
}
