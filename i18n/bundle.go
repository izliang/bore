package i18n

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"text/template"
)

// ────────────────────────────────────────────────────────────
// Bundle — 翻译包的核心实现
// ────────────────────────────────────────────────────────────

// Bundle 翻译包，应用级翻译器的唯一实现。
//
// 通过 Load() / MustLoad() 构造，线程安全，可在多个 goroutine 间共享。
//
// 典型用法：
//
//	var I18N = i18n.MustLoad(i18n.Config{
//	    Dir:         "./locales",
//	    DefaultLang: "zh",
//	    StrictMode:  true,
//	})
//
//	msg := I18N.T("zh", "greeting")
type Bundle struct {
	mu          sync.RWMutex
	data        map[string]map[string]Entry // lang → key → Entry（快速查找）
	defaultLang string
	languages   []string // 排序后的语言列表，用于稳定输出

	// 模板缓存：lang+key → 编译后的模板
	// 因为同 key 在不同语言中的模板变量完全相同，缓存 key 即可
	// 实际上 value 文本可能不同，所以缓存 lang+key
	templates map[string]*template.Template // "lang:key" → compiled template
}

// ────────────────────────────────────────────────────────────
// 构造函数
// ────────────────────────────────────────────────────────────

// Load 从 Config 加载翻译文件并构造 Bundle。
//
// 加载流程：
//  1. 扫描目录 / FS，按文件名前缀提取语言
//  2. 逐文件自动检测格式（3 种），解析为 []Entry
//  3. 同语言多文件合并（后加载的覆盖先加载的同 key）
//  4. 如果 StrictMode=true，校验所有语言的 key 是否一致
//
// 返回 error 时 Bundle 为 nil。
func Load(cfg Config) (*Bundle, error) {
	cfg.defaultConfig()

	entries, files, err := loadEntries(cfg)
	if err != nil {
		return nil, err
	}

	// Key 校验
	if cfg.StrictMode {
		if err := validateKeys(entries, files); err != nil {
			return nil, err
		}
	}

	b := buildBundle(entries, cfg.DefaultLang)
	return b, nil
}

// MustLoad 与 Load 相同，但失败时 panic。
//
// 适用于翻译文件是应用不可分割的一部分、加载失败意味着部署错误的场景。
func MustLoad(cfg Config) *Bundle {
	b, err := Load(cfg)
	if err != nil {
		panic(fmt.Sprintf("i18n: MustLoad failed: %v", err))
	}
	return b
}

// New 创建一个空的 Bundle，后续通过 Add/AddFile 动态填充。
//
// 适用于测试或程序化构造翻译数据的场景。
func New(defaultLang string) *Bundle {
	if defaultLang == "" {
		defaultLang = "en"
	}
	return &Bundle{
		data:        make(map[string]map[string]Entry),
		defaultLang: defaultLang,
		templates:   make(map[string]*template.Template),
	}
}

// buildBundle 从加载的条目构造 Bundle。
//
// 去重策略：同语言同 key 的后出现的条目覆盖先出现的（保留 merge 顺序）。
func buildBundle(raw map[string][]Entry, defaultLang string) *Bundle {
	data := make(map[string]map[string]Entry, len(raw))
	languages := make([]string, 0, len(raw))

	for lang, entries := range raw {
		keyMap := make(map[string]Entry, len(entries))
		for _, e := range entries {
			keyMap[e.Key] = e // 后出现的覆盖先出现的
		}
		data[lang] = keyMap
		languages = append(languages, lang)
	}

	sort.Strings(languages)

	return &Bundle{
		data:        data,
		defaultLang: defaultLang,
		languages:   languages,
		templates:   make(map[string]*template.Template),
	}
}

// ────────────────────────────────────────────────────────────
// 翻译方法
// ────────────────────────────────────────────────────────────

// T 将 key 翻译为 lang 对应的文本。
//
// 未找到时行为：
//   - 尝试默认语言回退（如果 lang ≠ defaultLang）
//   - 仍未找到则返回 key 本身
func (b *Bundle) T(lang string, key string) string {
	return b.Tf(lang, key, nil)
}

// Tf 带模板变量的翻译。
//
// data 中的字段按 Go text/template 语法 {{.Field}} 渲染到翻译文本中。
//
// 示例：
//
//	// 条目 Value = "你好 {{.Name}}，欢迎来到 {{.City}}"
//	bundle.Tf("zh", "greet", map[string]any{"Name": "小明", "City": "北京"})
//	// → "你好 小明，欢迎来到 北京"
func (b *Bundle) Tf(lang string, key string, data map[string]any) string {
	entry, ok := b.lookup(lang, key)
	if !ok {
		// 尝试默认语言回退
		if lang != b.defaultLang {
			if entry, ok = b.lookup(b.defaultLang, key); !ok {
				return key
			}
		} else {
			return key
		}
	}

	// 无模板变量，直接返回
	if data == nil || len(data) == 0 {
		return entry.Value
	}

	// 无模板标记，快速路径
	if !strings.Contains(entry.Value, "{{") {
		return entry.Value
	}

	tmpl, err := b.getTemplate(lang, key, entry.Value)
	if err != nil {
		// 模板编译失败，返回原始文本
		return entry.Value
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		// 模板执行失败，返回原始文本
		return entry.Value
	}

	return buf.String()
}

// Has 判断 key 在 lang 下是否有翻译。
func (b *Bundle) Has(lang string, key string) bool {
	_, ok := b.lookup(lang, key)
	if !ok && lang != b.defaultLang {
		_, ok = b.lookup(b.defaultLang, key)
	}
	return ok
}

// Languages 返回所有已加载的语言代码列表（排序）。
func (b *Bundle) Languages() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]string, len(b.languages))
	copy(result, b.languages)
	return result
}

// ────────────────────────────────────────────────────────────
// 内部查找
// ────────────────────────────────────────────────────────────

// lookup 在指定语言中查找 key，返回 Entry 和是否找到。
func (b *Bundle) lookup(lang string, key string) (Entry, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	lang = NormalizeLang(lang)
	keyMap, ok := b.data[lang]
	if !ok {
		return Entry{}, false
	}
	entry, ok := keyMap[key]
	return entry, ok
}

// getTemplate 获取或编译模板。
func (b *Bundle) getTemplate(lang, key, value string) (*template.Template, error) {
	cacheKey := lang + ":" + key

	b.mu.RLock()
	tmpl, ok := b.templates[cacheKey]
	b.mu.RUnlock()
	if ok {
		return tmpl, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// 双重检查
	if tmpl, ok = b.templates[cacheKey]; ok {
		return tmpl, nil
	}

	tmpl, err := template.New(key).Parse(value)
	if err != nil {
		return nil, err
	}

	b.templates[cacheKey] = tmpl
	return tmpl, nil
}

// ────────────────────────────────────────────────────────────
// 条目访问
// ────────────────────────────────────────────────────────────

// Entries 返回某语言的全部翻译条目（按 key 排序）。
//
// 条目是副本，修改不影响 Bundle 内部数据。
func (b *Bundle) Entries(lang string) []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	lang = NormalizeLang(lang)
	keyMap, ok := b.data[lang]
	if !ok {
		return nil
	}

	keys := make([]string, 0, len(keyMap))
	for k := range keyMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]Entry, 0, len(keys))
	for _, k := range keys {
		result = append(result, keyMap[k])
	}
	return result
}

// Entry 返回单个翻译条目。
func (b *Bundle) Entry(lang string, key string) (Entry, bool) {
	return b.lookup(lang, key)
}

// DecodeEntries 将某语言的原始 JSON 条目反序列化到自定义结构体。
//
// 逃生舱：当消费者需要类型安全的扩展字段访问时使用。
//
// 示例：
//
//	type MyEntry struct {
//	    Key       string `json:"key"`
//	    Value     string `json:"value"`
//	    MaxLength int    `json:"max_length"`
//	}
//	var entries []MyEntry
//	bundle.DecodeEntries("zh", &entries)
func (b *Bundle) DecodeEntries(lang string, target any) error {
	entries := b.Entries(lang)
	if entries == nil {
		return fmt.Errorf("i18n: no entries for language %q", lang)
	}

	// 将 []Entry 重新序列化再反序列化到 target
	// 这样可以利用 target 的 json tags 进行字段映射
	data, err := jsonMarshalEntries(entries)
	if err != nil {
		return fmt.Errorf("i18n: marshal entries: %w", err)
	}

	if err := jsonUnmarshal(data, target); err != nil {
		return fmt.Errorf("i18n: decode entries: %w", err)
	}
	return nil
}

// ────────────────────────────────────────────────────────────
// 动态追加
// ────────────────────────────────────────────────────────────

// Add 程序化追加翻译条目。
//
// 同 key 的已有条目被覆盖。
func (b *Bundle) Add(lang string, entries []Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	lang = NormalizeLang(lang)
	if _, ok := b.data[lang]; !ok {
		b.data[lang] = make(map[string]Entry)
		b.languages = append(b.languages, lang)
		sort.Strings(b.languages)
	}

	for _, e := range entries {
		b.data[lang][e.Key] = e
	}
}

// AddFile 从文件追加翻译条目。
//
// 文件格式自动检测。同 key 的已有条目被覆盖。
func (b *Bundle) AddFile(lang string, path string) error {
	data, err := osReadFile(path)
	if err != nil {
		return fmt.Errorf("i18n: read %s: %w", path, err)
	}

	entries, err := detectAndParse(data)
	if err != nil {
		return fmt.Errorf("i18n: parse %s: %w", path, err)
	}

	b.Add(lang, entries)
	return nil
}

// ────────────────────────────────────────────────────────────
// 内部辅助
// ────────────────────────────────────────────────────────────

// jsonMarshalEntries 将 []Entry 序列化为 JSON。
// 用于 DecodeEntries 的中间转换。
func jsonMarshalEntries(entries []Entry) ([]byte, error) {
	raw := make([]map[string]any, len(entries))
	for i, e := range entries {
		m := map[string]any{
			"key":   e.Key,
			"value": e.Value,
		}
		if e.Description != "" {
			m["desc"] = e.Description
		}
		if e.Deprecated {
			m["deprecated"] = e.Deprecated
		}
		if len(e.Tags) > 0 {
			m["tags"] = e.Tags
		}
		for k, v := range e.Extra {
			m[k] = v
		}
		raw[i] = m
	}

	return jsonMarshal(raw)
}

// 可覆盖的 I/O 函数，方便测试。
var (
	osReadFile  = osReadFileImpl
	jsonMarshal = json.Marshal
	jsonUnmarshal = json.Unmarshal
)

func osReadFileImpl(path string) ([]byte, error) {
	return os.ReadFile(path)
}

