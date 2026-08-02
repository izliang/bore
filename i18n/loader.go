package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ────────────────────────────────────────────────────────────
// 加载入口
// ────────────────────────────────────────────────────────────

// loadEntries 从 Config 加载所有翻译条目。
//
// 返回：
//
//	entries — 语言代码 → 翻译条目列表（合并后，已去重）
//	files   — 语言代码 → 源文件名列表（用于错误信息）
func loadEntries(cfg Config) (map[string][]Entry, map[string][]string, error) {
	allEntries := make(map[string][]Entry)
	allFiles := make(map[string][]string)

	// 从 embed.FS 加载
	if cfg.FS != (embed.FS{}) {
		e, f, err := loadFromFS(cfg.FS, cfg.SubDir)
		if err != nil {
			return nil, nil, fmt.Errorf("i18n: load from FS: %w", err)
		}
		mergeInto(allEntries, allFiles, e, f)
	}

	// 从单一目录加载
	if cfg.Dir != "" {
		e, f, err := loadFromDir(cfg.Dir)
		if err != nil {
			return nil, nil, fmt.Errorf("i18n: load from dir %s: %w", cfg.Dir, err)
		}
		mergeInto(allEntries, allFiles, e, f)
	}

	// 从多个目录加载
	for _, dir := range cfg.Dirs {
		e, f, err := loadFromDir(dir)
		if err != nil {
			return nil, nil, fmt.Errorf("i18n: load from dir %s: %w", dir, err)
		}
		mergeInto(allEntries, allFiles, e, f)
	}

	// 过滤语言
	if len(cfg.Languages) > 0 {
		langSet := make(map[string]bool, len(cfg.Languages))
		for _, l := range cfg.Languages {
			langSet[l] = true
		}
		for lang := range allEntries {
			if !langSet[lang] {
				delete(allEntries, lang)
				delete(allFiles, lang)
			}
		}
	}

	if len(allEntries) == 0 {
		return nil, nil, fmt.Errorf("i18n: no translation files loaded")
	}

	return allEntries, allFiles, nil
}

// mergeInto 将 src 合并到 dst，后加载的同 key 覆盖先加载的。
func mergeInto(
	dstEntries map[string][]Entry, dstFiles map[string][]string,
	srcEntries map[string][]Entry, srcFiles map[string][]string,
) {
	for lang, entries := range srcEntries {
		dstEntries[lang] = append(dstEntries[lang], entries...)
		dstFiles[lang] = append(dstFiles[lang], srcFiles[lang]...)
	}
}

// ────────────────────────────────────────────────────────────
// 目录加载
// ────────────────────────────────────────────────────────────

// loadFromDir 从磁盘目录加载所有 *.json 文件。
//
// 文件按名称排序后依次加载，保证确定性。
func loadFromDir(dir string) (map[string][]Entry, map[string][]string, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read dir: %w", err)
	}

	// 按文件名排序，确保加载顺序确定性
	sort.Slice(dirEntries, func(i, j int) bool {
		return dirEntries[i].Name() < dirEntries[j].Name()
	})

	entries := make(map[string][]Entry)
	files := make(map[string][]string)

	for _, de := range dirEntries {
		if de.IsDir() || strings.HasPrefix(de.Name(), ".") || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, de.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", path, err)
		}
		if err := loadFileData(entries, files, de.Name(), data, path); err != nil {
			return nil, nil, err
		}
	}

	return entries, files, nil
}

// loadFromFS 从 embed.FS 加载所有 *.json 文件。
func loadFromFS(efs embed.FS, subDir string) (map[string][]Entry, map[string][]string, error) {
	dir := subDir
	if dir == "" {
		dir = "."
	}

	dirEntries, err := efs.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read FS dir %s: %w", dir, err)
	}

	// 按文件名排序
	sort.Slice(dirEntries, func(i, j int) bool {
		return dirEntries[i].Name() < dirEntries[j].Name()
	})

	entries := make(map[string][]Entry)
	files := make(map[string][]string)

	for _, de := range dirEntries {
		if de.IsDir() || strings.HasPrefix(de.Name(), ".") || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, de.Name())
		data, err := fs.ReadFile(efs, path)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", path, err)
		}
		if err := loadFileData(entries, files, de.Name(), data, path); err != nil {
			return nil, nil, err
		}
	}

	return entries, files, nil
}

// loadFileData 解析单个文件内容，提取语言前缀，追加到 entries map。
func loadFileData(
	entries map[string][]Entry,
	files map[string][]string,
	filename string,
	data []byte,
	sourcePath string,
) error {
	lang := extractLang(filename)

	parsed, err := detectAndParse(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", sourcePath, err)
	}

	if len(parsed) == 0 {
		return nil
	}

	// 检查文件内重复 key
	keySet := make(map[string]bool, len(parsed))
	for _, e := range parsed {
		if keySet[e.Key] {
			return fmt.Errorf("parse %s: duplicate key %q", sourcePath, e.Key)
		}
		keySet[e.Key] = true
	}

	entries[lang] = append(entries[lang], parsed...)
	files[lang] = append(files[lang], filename)
	return nil
}

// ────────────────────────────────────────────────────────────
// 格式检测与解析
// ────────────────────────────────────────────────────────────

// detectAndParse 自动检测 JSON 格式并解析为 []Entry。
//
// 检测顺序：
//  1. 尝试解析为 JSON 数组 → 检查首元素含 "key"（格式 1）还是 "id"（格式 3）
//  2. 尝试解析为 JSON 对象 → 检查值含 "other"（格式 2）
func detectAndParse(data []byte) ([]Entry, error) {
	// 第一步：尝试数组格式
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil && len(arr) > 0 {
		var probe map[string]any
		if err := json.Unmarshal(arr[0], &probe); err == nil {
			if _, ok := probe["key"]; ok {
				return parseNewFormat(arr)
			}
			if _, ok := probe["id"]; ok {
				return parseGoI18nArray(arr)
			}
		}
		return nil, fmt.Errorf("i18n: array format detected but unrecognized fields in first element")
	}

	// 第二步：尝试扁平对象格式（go-i18n）
	var flat map[string]json.RawMessage
	if err := json.Unmarshal(data, &flat); err == nil {
		return parseGoI18nFlat(flat)
	}

	return nil, fmt.Errorf("i18n: unrecognized JSON format (expected array with key/id fields or object with other fields)")
}

// parseNewFormat 解析格式 1（新数组格式）。
//
// JSON 示例：
//
//	[{"key": "greeting", "value": "你好", "desc": "问候语"}]
func parseNewFormat(raw []json.RawMessage) ([]Entry, error) {
	entries := make([]Entry, 0, len(raw))
	for i, item := range raw {
		var entry Entry
		if err := json.Unmarshal(item, &entry); err != nil {
			return nil, fmt.Errorf("entry[%d]: %w", i, err)
		}
		if err := entry.Validate(); err != nil {
			return nil, fmt.Errorf("entry[%d]: %w", i, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// parseGoI18nFlat 解析格式 2（go-i18n v2 扁平格式）。
//
// JSON 示例：
//
//	{"greeting": {"other": "你好", "description": "问候语"}}
//
// 映射规则：
//   - JSON key 名 → Entry.Key
//   - "other"      → Entry.Value
//   - "description" → Entry.Description
//   - 其余字段     → Entry.Extra
func parseGoI18nFlat(raw map[string]json.RawMessage) ([]Entry, error) {
	// 快速校验：至少有一个值包含 "other"
	hasOther := false
	for _, val := range raw {
		var probe map[string]any
		if json.Unmarshal(val, &probe) == nil {
			if _, ok := probe["other"]; ok {
				hasOther = true
				break
			}
		}
	}
	if !hasOther {
		return nil, fmt.Errorf("i18n: object format detected but no 'other' field found — not a valid go-i18n flat file")
	}

	entries := make([]Entry, 0, len(raw))
	for key, val := range raw {
		var fields map[string]any
		if err := json.Unmarshal(val, &fields); err != nil {
			return nil, fmt.Errorf("entry %q: %w", key, err)
		}

		entry := Entry{Key: key}

		if v, ok := fields["other"].(string); ok {
			entry.Value = v
			delete(fields, "other")
		}
		if v, ok := fields["description"].(string); ok {
			entry.Description = v
			delete(fields, "description")
		}

		if len(fields) > 0 {
			entry.Extra = fields
		}

		if err := entry.Validate(); err != nil {
			return nil, fmt.Errorf("entry %q: %w", key, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// parseGoI18nArray 解析格式 3（go-i18n v2 数组格式）。
//
// JSON 示例：
//
//	[{"id": "greeting", "other": "你好", "description": "问候语"}]
//
// 映射规则：
//   - "id"          → Entry.Key
//   - "other"       → Entry.Value
//   - "description" → Entry.Description
//   - 其余字段      → Entry.Extra
func parseGoI18nArray(raw []json.RawMessage) ([]Entry, error) {
	entries := make([]Entry, 0, len(raw))
	for i, item := range raw {
		var fields map[string]any
		if err := json.Unmarshal(item, &fields); err != nil {
			return nil, fmt.Errorf("entry[%d]: %w", i, err)
		}

		entry := Entry{}

		if v, ok := fields["id"].(string); ok {
			entry.Key = v
			delete(fields, "id")
		}
		if v, ok := fields["other"].(string); ok {
			entry.Value = v
			delete(fields, "other")
		}
		if v, ok := fields["description"].(string); ok {
			entry.Description = v
			delete(fields, "description")
		}

		if len(fields) > 0 {
			entry.Extra = fields
		}

		if err := entry.Validate(); err != nil {
			return nil, fmt.Errorf("entry[%d]: %w", i, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ────────────────────────────────────────────────────────────
// Key 校验
// ────────────────────────────────────────────────────────────

// validateKeys 校验所有语言的 key 一致性。
//
// 以所有语言 key 的并集为基准，检查每种语言是否包含全部 key。
// 缺失信息聚合到 ValidationError 中返回。
func validateKeys(entries map[string][]Entry, files map[string][]string) error {
	// 构建全量 key 集合
	allKeys := make(map[string]bool)
	langKeys := make(map[string]map[string]bool)

	for lang, langEntries := range entries {
		keys := make(map[string]bool, len(langEntries))
		for _, e := range langEntries {
			keys[e.Key] = true
			allKeys[e.Key] = true
		}
		langKeys[lang] = keys
	}

	// 检查每种语言是否缺少 key
	missing := make(map[string][]string)
	for lang, keys := range langKeys {
		var missingKeys []string
		for k := range allKeys {
			if !keys[k] {
				missingKeys = append(missingKeys, k)
			}
		}
		if len(missingKeys) > 0 {
			sort.Strings(missingKeys)
			missing[lang] = missingKeys
		}
	}

	if len(missing) > 0 {
		return &ValidationError{Missing: missing, Files: files}
	}
	return nil
}
