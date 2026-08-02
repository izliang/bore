package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// ============================================================
// 测试辅助函数
// ============================================================

// writeLocales 在临时目录中创建翻译 JSON 文件并返回目录路径。
// files 格式：map[文件名（不含 .json 后缀）]内容。
func writeLocales(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write locale file %s: %v", path, err)
		}
	}
	return dir
}

// mustLoad 创建 Bundle，失败则终止测试。
func mustLoad(t *testing.T, cfg Config) *Bundle {
	t.Helper()
	b, err := Load(cfg)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return b
}

// sortedStrings 返回排序后的字符串切片。
func sortedStrings(s []string) []string {
	result := make([]string, len(s))
	copy(result, s)
	sort.Strings(result)
	return result
}

// ============================================================
// Entry
// ============================================================

func TestEntry_UnmarshalJSON(t *testing.T) {
	t.Run("basic key and value", func(t *testing.T) {
		e, err := parseEntryJSON(`{"key": "greeting", "value": "你好"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e.Key != "greeting" {
			t.Errorf("Key = %q, want %q", e.Key, "greeting")
		}
		if e.Value != "你好" {
			t.Errorf("Value = %q, want %q", e.Value, "你好")
		}
	})

	t.Run("with description", func(t *testing.T) {
		e, err := parseEntryJSON(`{"key": "greeting", "value": "你好", "desc": "问候语"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e.Description != "问候语" {
			t.Errorf("Description = %q, want %q", e.Description, "问候语")
		}
	})

	t.Run("with deprecated flag", func(t *testing.T) {
		e, err := parseEntryJSON(`{"key": "old", "value": "旧文本", "deprecated": true}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !e.Deprecated {
			t.Error("expected Deprecated=true")
		}
	})

	t.Run("with tags", func(t *testing.T) {
		e, err := parseEntryJSON(`{"key": "btn", "value": "提交", "tags": ["button", "form"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(e.Tags) != 2 || e.Tags[0] != "button" || e.Tags[1] != "form" {
			t.Errorf("Tags = %v, want [button form]", e.Tags)
		}
	})

	t.Run("extra fields preserved", func(t *testing.T) {
		e, err := parseEntryJSON(`{"key": "label", "value": "名称", "max_length": 50, "auditor": "张三"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if e.Extra == nil {
			t.Fatal("expected Extra to be non-nil")
		}
		if v, ok := e.Extra["max_length"].(float64); !ok || v != 50 {
			t.Errorf("Extra[max_length] = %v, want 50", e.Extra["max_length"])
		}
		if v, ok := e.Extra["auditor"].(string); !ok || v != "张三" {
			t.Errorf("Extra[auditor] = %v, want '张三'", e.Extra["auditor"])
		}
	})
}

func TestEntry_Validate(t *testing.T) {
	t.Run("valid entry", func(t *testing.T) {
		e := Entry{Key: "greeting", Value: "你好"}
		if err := e.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		e := Entry{Value: "你好"}
		if err := e.Validate(); err == nil {
			t.Error("expected error for missing key")
		}
	})

	t.Run("empty key", func(t *testing.T) {
		e := Entry{Key: "", Value: "你好"}
		if err := e.Validate(); err == nil {
			t.Error("expected error for empty key")
		}
	})

	t.Run("empty value", func(t *testing.T) {
		e := Entry{Key: "greeting", Value: ""}
		if err := e.Validate(); err == nil {
			t.Error("expected error for empty value")
		}
	})
}

// parseEntryJSON 解析单个 Entry JSON，用于测试 UnmarshalJSON。
func parseEntryJSON(s string) (Entry, error) {
	var e Entry
	if err := e.UnmarshalJSON([]byte(s)); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// ============================================================
// NormalizeLang & extractLang
// ============================================================

func TestNormalizeLang(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// 标准两字母
		{"zh", "zh"}, {"en", "en"}, {"ja", "ja"}, {"ko", "ko"},
		// 带 region（短横线）
		{"zh-CN", "zh"}, {"zh-TW", "zh"}, {"zh-HK", "zh"},
		{"en-US", "en"}, {"en-GB", "en"},
		// 带 region（下划线）
		{"zh_cn", "zh"}, {"zh_tw", "zh"},
		{"en_us", "en"},
		// hans/hant
		{"zh-Hans", "zh"}, {"zh-Hant", "zh"}, {"zh_hans", "zh"},
		// 大小写不敏感
		{"ZH-CN", "zh"}, {"En-Us", "en"},
		// 前后空格
		{"  zh-CN  ", "zh"}, {"  en  ", "en"},
		// 其他语言两字母截断
		{"fr", "fr"}, {"es-MX", "es"}, {"pt-BR", "pt"},
		// 未知语言短于 2 字符 fallback 到 en
		{"x", "en"}, {"", "en"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeLang(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeLang(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractLang(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"zh.json", "zh"},
		{"zh_errors.json", "zh"},
		{"zh_common.json", "zh"},
		{"en.json", "en"},
		{"en_ui.json", "en"},
		{"ja.json", "ja"},
		{"ko_messages.json", "ko"},
		// 边缘 case
		{"zh-CN.json", "zh-CN"}, // 无下划线，整个作为前缀
		{"_.json", ""},          // 下划线在前面
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := extractLang(tt.filename)
			if got != tt.want {
				t.Errorf("extractLang(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestGetLangFromEnv(t *testing.T) {
	t.Run("LC_ALL takes priority", func(t *testing.T) {
		os.Setenv("LC_ALL", "en-US")
		os.Setenv("LANG", "zh-CN")
		defer func() { os.Unsetenv("LC_ALL"); os.Unsetenv("LANG") }()
		if got := GetLangFromEnv(); got != "en" {
			t.Errorf("GetLangFromEnv() = %q, want %q", got, "en")
		}
	})

	t.Run("LANG used when LC_ALL empty", func(t *testing.T) {
		os.Unsetenv("LC_ALL")
		os.Setenv("LANG", "zh-CN")
		defer os.Unsetenv("LANG")
		if got := GetLangFromEnv(); got != "zh" {
			t.Errorf("GetLangFromEnv() = %q, want %q", got, "zh")
		}
	})

	t.Run("neither set falls back to en", func(t *testing.T) {
		os.Unsetenv("LC_ALL")
		os.Unsetenv("LANG")
		if got := GetLangFromEnv(); got != "en" {
			t.Errorf("GetLangFromEnv() = %q, want %q", got, "en")
		}
	})
}

// ============================================================
// 格式 1：新数组格式
// ============================================================

func TestLoad_Format1_NewArray(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[
			{"key": "greeting", "value": "你好", "desc": "问候语"},
			{"key": "farewell", "value": "再见"},
			{"key": "error.not_found", "value": "未找到: {{.ID}}"}
		]`,
		"en": `[
			{"key": "greeting", "value": "Hello", "desc": "Greeting"},
			{"key": "farewell", "value": "Goodbye"},
			{"key": "error.not_found", "value": "Not found: {{.ID}}"}
		]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	// 基本翻译
	if got := b.T("zh", "greeting"); got != "你好" {
		t.Errorf("T(zh, greeting) = %q, want %q", got, "你好")
	}
	if got := b.T("en", "greeting"); got != "Hello" {
		t.Errorf("T(en, greeting) = %q, want %q", got, "Hello")
	}

	// 模板翻译
	got := b.Tf("zh", "error.not_found", map[string]any{"ID": "123"})
	if got != "未找到: 123" {
		t.Errorf("Tf = %q, want %q", got, "未找到: 123")
	}

	// Has
	if !b.Has("zh", "greeting") {
		t.Error("expected Has(zh, greeting) = true")
	}
	if b.Has("zh", "not_exist") {
		t.Error("expected Has(zh, not_exist) = false")
	}

	// Languages
	langs := sortedStrings(b.Languages())
	expected := []string{"en", "zh"}
	if !stringSlicesEqual(langs, expected) {
		t.Errorf("Languages = %v, want %v", langs, expected)
	}

	// Entry 访问
	entry, ok := b.Entry("zh", "greeting")
	if !ok {
		t.Fatal("expected entry found")
	}
	if entry.Description != "问候语" {
		t.Errorf("Description = %q, want %q", entry.Description, "问候语")
	}
}

// ============================================================
// 格式 2：go-i18n 扁平格式
// ============================================================

func TestLoad_Format2_GoI18nFlat(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `{
			"greeting":       {"other": "你好", "description": "问候语"},
			"farewell":       {"other": "再见 {{.Name}}"},
			"error.not_found": {"other": "未找到资源: {{.ID}}"}
		}`,
		"en": `{
			"greeting":       {"other": "Hello", "description": "Greeting"},
			"farewell":       {"other": "Goodbye {{.Name}}"},
			"error.not_found": {"other": "Resource not found: {{.ID}}"}
		}`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	if got := b.T("zh", "greeting"); got != "你好" {
		t.Errorf("T(zh, greeting) = %q, want %q", got, "你好")
	}
	if got := b.T("en", "farewell"); got != "Goodbye {{.Name}}" {
		t.Errorf("T(en, farewell) = %q, want %q", got, "Goodbye {{.Name}}")
	}

	// 模板
	got := b.Tf("en", "farewell", map[string]any{"Name": "John"})
	if got != "Goodbye John" {
		t.Errorf("Tf = %q, want %q", got, "Goodbye John")
	}

	// Description 从 go-i18n 的 "description" 映射
	entry, ok := b.Entry("zh", "greeting")
	if !ok {
		t.Fatal("expected entry found")
	}
	if entry.Description != "问候语" {
		t.Errorf("Description = %q, want %q", entry.Description, "问候语")
	}
}

// ============================================================
// 格式 3：go-i18n 数组格式
// ============================================================

func TestLoad_Format3_GoI18nArray(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[
			{"id": "greeting", "other": "你好", "description": "问候语"},
			{"id": "farewell", "other": "再见 {{.Name}}"},
			{"id": "error.not_found", "other": "未找到: {{.ID}}", "one": "未找到1条"}
		]`,
		"en": `[
			{"id": "greeting", "other": "Hello"},
			{"id": "farewell", "other": "Goodbye {{.Name}}"},
			{"id": "error.not_found", "other": "Not found: {{.ID}}"}
		]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	if got := b.T("zh", "greeting"); got != "你好" {
		t.Errorf("T(zh, greeting) = %q, want %q", got, "你好")
	}

	// 复数形式进入 Extra
	entry, ok := b.Entry("zh", "error.not_found")
	if !ok {
		t.Fatal("expected entry found")
	}
	if v, ok := entry.Extra["one"].(string); !ok || v != "未找到1条" {
		t.Errorf("Extra[one] = %v, want '未找到1条'", entry.Extra["one"])
	}
}

// ============================================================
// 混合格式
// ============================================================

func TestLoad_MixedFormats(t *testing.T) {
	// zh.json 用 go-i18n 扁平格式，en.json 用新数组格式，ja.json 用 go-i18n 数组格式
	dir := writeLocales(t, map[string]string{
		"zh": `{
			"greeting": {"other": "你好"},
			"farewell": {"other": "再见"}
		}`,
		"en": `[
			{"key": "greeting", "value": "Hello"},
			{"key": "farewell", "value": "Goodbye"}
		]`,
		"ja": `[
			{"id": "greeting", "other": "こんにちは"},
			{"id": "farewell", "other": "さようなら"}
		]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh", StrictMode: true})

	if got := b.T("zh", "greeting"); got != "你好" {
		t.Errorf("zh = %q", got)
	}
	if got := b.T("en", "greeting"); got != "Hello" {
		t.Errorf("en = %q", got)
	}
	if got := b.T("ja", "greeting"); got != "こんにちは" {
		t.Errorf("ja = %q", got)
	}
}

// ============================================================
// 语言前缀归并（同语言多文件）
// ============================================================

func TestLoad_LangPrefixMerge(t *testing.T) {
	// zh.json + zh_errors.json → 合并到 lang="zh"
	dir := writeLocales(t, map[string]string{
		"zh": `{
			"greeting": {"other": "你好"},
			"farewell": {"other": "再见"}
		}`,
		"zh_errors": `[
			{"key": "error.not_found", "value": "未找到资源"},
			{"key": "error.timeout", "value": "请求超时"}
		]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	// 两个文件的 key 都能访问
	if got := b.T("zh", "greeting"); got != "你好" {
		t.Errorf("greeting = %q", got)
	}
	if got := b.T("zh", "error.not_found"); got != "未找到资源" {
		t.Errorf("error.not_found = %q", got)
	}
	if got := b.T("zh", "error.timeout"); got != "请求超时" {
		t.Errorf("error.timeout = %q", got)
	}

	// 语言列表应该只有 zh
	langs := b.Languages()
	if len(langs) != 1 || langs[0] != "zh" {
		t.Errorf("Languages = %v, want [zh]", langs)
	}
}

func TestLoad_LangPrefixMerge_OverrideLater(t *testing.T) {
	// go-i18nFlat 中 greeting=你好，langPrefix 中 greeting=您好（覆盖）
	// 文件名排序：zh.json < zh_override.json，后者覆盖前者
	dir := writeLocales(t, map[string]string{
		"zh": `{
			"greeting": {"other": "你好"},
			"farewell": {"other": "再见"}
		}`,
		"zh_override": `[
			{"key": "greeting", "value": "您好"}
		]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	// 后加载的文件覆盖同 key
	if got := b.T("zh", "greeting"); got != "您好" {
		t.Errorf("greeting = %q, want %q (override)", got, "您好")
	}
	// 未被覆盖的 key 保持不变
	if got := b.T("zh", "farewell"); got != "再见" {
		t.Errorf("farewell = %q", got)
	}
}

// ============================================================
// StrictMode Key 校验
// ============================================================

func TestLoad_StrictMode_MissingKeys(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[
			{"key": "greeting", "value": "你好"},
			{"key": "farewell", "value": "再见"},
			{"key": "extra_zh", "value": "仅中文"}
		]`,
		"en": `[
			{"key": "greeting", "value": "Hello"},
			{"key": "farewell", "value": "Goodbye"},
			{"key": "extra_en", "value": "Only English"}
		]`,
	})

	_, err := Load(Config{Dir: dir, DefaultLang: "zh", StrictMode: true})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	valErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}

	// zh 应该缺少 extra_en，en 应该缺少 extra_zh
	if keys, ok := valErr.Missing["zh"]; !ok || !containsString(keys, "extra_en") {
		t.Errorf("zh should be missing extra_en, got %v", valErr.Missing["zh"])
	}
	if keys, ok := valErr.Missing["en"]; !ok || !containsString(keys, "extra_zh") {
		t.Errorf("en should be missing extra_zh, got %v", valErr.Missing["en"])
	}
}

func TestLoad_StrictMode_AllKeysPresent(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[{"key": "a", "value": "啊"}, {"key": "b", "value": "吧"}]`,
		"en": `[{"key": "a", "value": "A"}, {"key": "b", "value": "B"}]`,
	})

	_, err := Load(Config{Dir: dir, DefaultLang: "zh", StrictMode: true})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoad_NonStrictMode(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[{"key": "a", "value": "啊"}]`,
		"en": `[{"key": "a", "value": "A"}, {"key": "b", "value": "B"}]`,
	})

	b, err := Load(Config{Dir: dir, DefaultLang: "zh", StrictMode: false})
	if err != nil {
		t.Fatalf("non-strict mode should not error: %v", err)
	}

	// zh 缺少 b，但 non-strict 模式不报错
	if b.Has("zh", "b") {
		t.Error("zh should not have key b")
	}
	// 回退：zh 没有 b，且 zh 就是 defaultLang，不再尝试其他语言，返回 key 本身
	got := b.T("zh", "b")
	if got != "b" {
		t.Errorf("T(zh, b) = %q, want %q (key itself, since zh is defaultLang)", got, "b")
	}
	// en 有 b，正常命中
	if got := b.T("en", "b"); got != "B" {
		t.Errorf("T(en, b) = %q, want %q", got, "B")
	}
}

// ============================================================
// 翻译回退
// ============================================================

func TestBundle_FallbackToDefaultLang(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[{"key": "greeting", "value": "你好"}]`,
		"en": `[{"key": "greeting", "value": "Hello"}, {"key": "only_en", "value": "English only"}]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	// zh 没有 only_en，应回退到默认语言 zh... 但 zh 也没有，再回退到 en
	// 实际行为：lang=zh 没找到 → 回退到 defaultLang=zh（也没找到）→ 返回 key 本身
	// 等一下，defaultLang 就是 zh，但 zh 没有这个 key
	// 回退逻辑是：lang 没找到 → 尝试 defaultLang → 没找到则返回 key
	// 但如果 defaultLang == lang，就直接返回 key
	got := b.T("zh", "only_en")
	if got != "only_en" {
		t.Errorf("T(zh, only_en) = %q, want %q (key itself)", got, "only_en")
	}

	// en 有 only_en，直接命中
	if got := b.T("en", "only_en"); got != "English only" {
		t.Errorf("T(en, only_en) = %q, want 'English only'", got)
	}
}

// ============================================================
// 模板渲染
// ============================================================

func TestBundle_TemplateRendering(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[{"key": "welcome", "value": "你好 {{.Name}}，欢迎来到 {{.City}}"}]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	tests := []struct {
		name string
		data map[string]any
		want string
	}{
		{
			name: "full data",
			data: map[string]any{"Name": "小明", "City": "北京"},
			want: "你好 小明，欢迎来到 北京",
		},
		{
			name: "nil data",
			data: nil,
			want: "你好 {{.Name}}，欢迎来到 {{.City}}",
		},
		{
			name: "empty data",
			data: map[string]any{},
			want: "你好 {{.Name}}，欢迎来到 {{.City}}",
		},
		{
			name: "partial data",
			data: map[string]any{"Name": "小明"},
			want: "你好 小明，欢迎来到 <no value>", // Go template 对缺失变量渲染 <no value>
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.Tf("zh", "welcome", tt.data)
			if got != tt.want {
				t.Errorf("Tf = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBundle_TemplateCaching(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[{"key": "msg", "value": "{{.X}}"}]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	// 多次调用应该使用缓存的模板
	for i := 0; i < 100; i++ {
		got := b.Tf("zh", "msg", map[string]any{"X": fmt.Sprintf("v%d", i)})
		if got != fmt.Sprintf("v%d", i) {
			t.Errorf("iteration %d: got %q", i, got)
		}
	}
}

// ============================================================
// Localizer
// ============================================================

func TestLocalizer(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[{"key": "greeting", "value": "你好"}, {"key": "name", "value": "{{.Name}}"}]`,
		"en": `[{"key": "greeting", "value": "Hello"}, {"key": "name", "value": "{{.Name}}"}]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	t.Run("L basic", func(t *testing.T) {
		loc := b.L("zh")
		if got := loc.T("greeting"); got != "你好" {
			t.Errorf("T = %q, want %q", got, "你好")
		}
		if loc.Lang() != "zh" {
			t.Errorf("Lang = %q, want %q", loc.Lang(), "zh")
		}
	})

	t.Run("L with normalization", func(t *testing.T) {
		loc := b.L("zh-CN")
		if loc.Lang() != "zh" {
			t.Errorf("Lang after normalize = %q, want %q", loc.Lang(), "zh")
		}
		if got := loc.T("greeting"); got != "你好" {
			t.Errorf("T = %q, want %q", got, "你好")
		}
	})

	t.Run("L template", func(t *testing.T) {
		loc := b.L("zh")
		got := loc.Tf("name", map[string]any{"Name": "小明"})
		if got != "小明" {
			t.Errorf("Tf = %q, want %q", got, "小明")
		}
	})
}

// ============================================================
// NewRequestLocalizer 向后兼容
// ============================================================

func TestNewRequestLocalizer_BackwardCompat(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[{"key": "greeting", "value": "你好"}]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	// 通过接口调用
	var tr Translator = b
	loc := NewRequestLocalizer(tr, "zh-CN")

	if loc.Lang() != "zh" {
		t.Errorf("Lang = %q, want %q", loc.Lang(), "zh")
	}
	if got := loc.T("greeting"); got != "你好" {
		t.Errorf("T = %q, want %q", got, "你好")
	}
}

// ============================================================
// Entries / Entry / DecodeEntries
// ============================================================

func TestBundle_Entries(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[
			{"key": "b", "value": "B值"},
			{"key": "a", "value": "A值"},
			{"key": "c", "value": "C值"}
		]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	entries := b.Entries("zh")
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// 应该按 key 排序
	if entries[0].Key != "a" || entries[1].Key != "b" || entries[2].Key != "c" {
		t.Errorf("entries not sorted by key: %v", entries)
	}

	// 查询不存在的语言
	if entries := b.Entries("fr"); entries != nil {
		t.Errorf("expected nil for non-existent language, got %v", entries)
	}
}

func TestBundle_DecodeEntries(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[
			{"key": "greeting", "value": "你好", "desc": "问候语", "max_length": 50}
		]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	type MyEntry struct {
		Key       string `json:"key"`
		Value     string `json:"value"`
		Desc      string `json:"desc"`
		MaxLength int    `json:"max_length"`
	}

	var entries []MyEntry
	if err := b.DecodeEntries("zh", &entries); err != nil {
		t.Fatalf("DecodeEntries: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Key != "greeting" || entries[0].Value != "你好" {
		t.Errorf("entry = %+v", entries[0])
	}
	if entries[0].MaxLength != 50 {
		t.Errorf("MaxLength = %d, want 50", entries[0].MaxLength)
	}
}

// ============================================================
// 动态追加
// ============================================================

func TestBundle_Add(t *testing.T) {
	b := New("zh")

	b.Add("zh", []Entry{
		{Key: "greeting", Value: "你好"},
		{Key: "farewell", Value: "再见"},
	})

	if got := b.T("zh", "greeting"); got != "你好" {
		t.Errorf("T = %q, want %q", got, "你好")
	}

	// 追加覆盖
	b.Add("zh", []Entry{
		{Key: "greeting", Value: "您好"},
	})
	if got := b.T("zh", "greeting"); got != "您好" {
		t.Errorf("after override: T = %q, want %q", got, "您好")
	}

	// 新语言
	b.Add("en", []Entry{
		{Key: "greeting", Value: "Hello"},
	})
	if got := b.T("en", "greeting"); got != "Hello" {
		t.Errorf("T(en) = %q, want %q", got, "Hello")
	}
}

func TestBundle_AddFile(t *testing.T) {
	b := New("zh")

	dir := writeLocales(t, map[string]string{
		"zh": `[{"key": "greeting", "value": "你好"}]`,
	})

	if err := b.AddFile("zh", filepath.Join(dir, "zh.json")); err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	if got := b.T("zh", "greeting"); got != "你好" {
		t.Errorf("T = %q, want %q", got, "你好")
	}
}

// ============================================================
// Config 多目录 + 语言过滤
// ============================================================

func TestLoad_MultipleDirs(t *testing.T) {
	dir1 := writeLocales(t, map[string]string{
		"zh": `[{"key": "base", "value": "基础"}, {"key": "common", "value": "通用"}]`,
	})
	dir2 := writeLocales(t, map[string]string{
		"zh": `[{"key": "common", "value": "覆盖值"}, {"key": "extra", "value": "额外"}]`,
	})

	b := mustLoad(t, Config{Dirs: []string{dir1, dir2}, DefaultLang: "zh"})

	// dir1 的值
	if got := b.T("zh", "base"); got != "基础" {
		t.Errorf("base = %q", got)
	}
	// dir2 覆盖 dir1 的同 key
	if got := b.T("zh", "common"); got != "覆盖值" {
		t.Errorf("common = %q, want '覆盖值' (override)", got)
	}
	// dir2 的新 key
	if got := b.T("zh", "extra"); got != "额外" {
		t.Errorf("extra = %q", got)
	}
}

func TestLoad_LanguageFilter(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[{"key": "greeting", "value": "你好"}]`,
		"en": `[{"key": "greeting", "value": "Hello"}]`,
		"ja": `[{"key": "greeting", "value": "こんにちは"}]`,
	})

	// 只加载 zh 和 en
	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh", Languages: []string{"zh", "en"}})

	langs := sortedStrings(b.Languages())
	if !stringSlicesEqual(langs, []string{"en", "zh"}) {
		t.Errorf("Languages = %v, want [en zh]", langs)
	}
}

// ============================================================
// 错误场景
// ============================================================

func TestLoad_EmptyDir(t *testing.T) {
	dir := writeLocales(t, map[string]string{})

	_, err := Load(Config{Dir: dir, DefaultLang: "zh"})
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
	if !strings.Contains(err.Error(), "no translation files") {
		t.Errorf("error = %q, should mention 'no translation files'", err.Error())
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `not json`,
	})

	_, err := Load(Config{Dir: dir, DefaultLang: "zh"})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_UnrecognizedFormat(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `["just", "an", "array"]`,
	})

	_, err := Load(Config{Dir: dir, DefaultLang: "zh"})
	if err == nil {
		t.Fatal("expected error for unrecognized format")
	}
	if !strings.Contains(err.Error(), "unrecognized") && !strings.Contains(err.Error(), "format") {
		t.Errorf("error = %q, should mention format issue", err.Error())
	}
}

func TestLoad_IgnoresHiddenFiles(t *testing.T) {
	// macOS 会生成 ._en.json 等 Apple Double 文件，必须过滤掉
	dir := writeLocales(t, map[string]string{
		"zh":     `[{"key": "greeting", "value": "你好"}]`,
		"._zh":   `[{"key": "greeting", "value": "BAD"}]`,
		".DS_Store": `{}`,
		".hidden": `[{"key": "x", "value": "y"}]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	// 隐藏文件被忽略，只加载 zh.json
	if got := b.T("zh", "greeting"); got != "你好" {
		t.Errorf("greeting = %q, want %q", got, "你好")
	}
	langs := b.Languages()
	if len(langs) != 1 || langs[0] != "zh" {
		t.Errorf("Languages = %v, want [zh]", langs)
	}
}

func TestLoad_DuplicateKey(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[
			{"key": "greeting", "value": "你好"},
			{"key": "greeting", "value": "您好"}
		]`,
	})

	_, err := Load(Config{Dir: dir, DefaultLang: "zh"})
	if err == nil {
		t.Fatal("expected error for duplicate key")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q, should mention 'duplicate'", err.Error())
	}
}

func TestLoad_EntryMissingKey(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[{"value": "你好"}]`,
	})

	_, err := Load(Config{Dir: dir, DefaultLang: "zh"})
	if err == nil {
		t.Fatal("expected error for entry with missing key")
	}
}

// ============================================================
// MustLoad
// ============================================================

func TestMustLoad_Success(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[{"key": "greeting", "value": "你好"}]`,
	})

	b := MustLoad(Config{Dir: dir, DefaultLang: "zh"})
	if b == nil {
		t.Fatal("expected non-nil bundle")
	}
}

func TestMustLoad_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()

	MustLoad(Config{Dir: "/nonexistent/path/xyz", DefaultLang: "zh"})
}

// ============================================================
// 并发安全
// ============================================================

func TestBundle_Concurrency(t *testing.T) {
	dir := writeLocales(t, map[string]string{
		"zh": `[
			{"key": "greeting", "value": "你好"},
			{"key": "farewell", "value": "再见 {{.Name}}"}
		]`,
		"en": `[
			{"key": "greeting", "value": "Hello"},
			{"key": "farewell", "value": "Goodbye {{.Name}}"}
		]`,
	})

	b := mustLoad(t, Config{Dir: dir, DefaultLang: "zh"})

	const goroutines = 50
	const iterations = 100
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// 读操作
				b.T("zh", "greeting")
				b.T("en", "greeting")
				b.Tf("zh", "farewell", map[string]any{"Name": fmt.Sprintf("u%d", id)})
				b.Has("zh", "greeting")
				b.Languages()
				b.Entries("zh")
				b.Entry("zh", "greeting")

				// 写操作（Add）
				b.Add("zh", []Entry{{Key: fmt.Sprintf("k_%d_%d", id, j), Value: "v"}})

				// Localizer
				loc := b.L("zh")
				loc.T("greeting")
			}
		}(i)
	}

	wg.Wait()
}

// ============================================================
// 辅助函数
// ============================================================

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
