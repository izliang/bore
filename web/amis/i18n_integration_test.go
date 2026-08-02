package amis

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/izliang/bore/i18n"
)

// ============================================================
// 测试辅助函数
// ============================================================

// newTestBundle 创建测试用 i18n Bundle，包含 zh 和 en 翻译。
func newTestBundle(t *testing.T) *i18n.Bundle {
	t.Helper()
	b := i18n.New("zh")

	b.Add("zh", []i18n.Entry{
		{Key: "dashboard.title", Value: "仪表盘"},
		{Key: "dashboard.activeUsers", Value: "活跃用户"},
		{Key: "common.save", Value: "保存"},
		{Key: "common.cancel", Value: "取消"},
	})
	b.Add("en", []i18n.Entry{
		{Key: "dashboard.title", Value: "Dashboard"},
		{Key: "dashboard.activeUsers", Value: "Active Users"},
		{Key: "common.save", Value: "Save"},
		{Key: "common.cancel", Value: "Cancel"},
	})

	return b
}

// newTestBundleSingle 创建单语言测试 Bundle。
func newTestBundleSingle(t *testing.T) *i18n.Bundle {
	t.Helper()
	b := i18n.New("zh")
	b.Add("zh", []i18n.Entry{
		{Key: "greeting", Value: "你好"},
	})
	return b
}

// newTestBundleEmpty 创建无翻译的 Bundle（用于测试缺失 key 场景）。
func newTestBundleEmpty(t *testing.T) *i18n.Bundle {
	t.Helper()
	return i18n.New("zh")
}

// ============================================================
// I18nConfig.normalize
// ============================================================

func TestI18nConfig_Normalize(t *testing.T) {
	t.Run("default prefix", func(t *testing.T) {
		cfg := I18nConfig{Bundle: newTestBundle(t)}
		cfg.normalize()
		if cfg.KeyPrefix != "$t." {
			t.Errorf("KeyPrefix = %q, want %q", cfg.KeyPrefix, "$t.")
		}
	})

	t.Run("custom prefix preserved", func(t *testing.T) {
		cfg := I18nConfig{Bundle: newTestBundle(t), KeyPrefix: "@@"}
		cfg.normalize()
		if cfg.KeyPrefix != "@@" {
			t.Errorf("KeyPrefix = %q, want %q", cfg.KeyPrefix, "@@")
		}
	})

	t.Run("panics on nil bundle", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for nil Bundle")
			}
		}()
		cfg := I18nConfig{}
		cfg.normalize()
	})
}

// ============================================================
// buildReplaceText
// ============================================================

func TestI18nConfig_BuildReplaceText(t *testing.T) {
	cfg := I18nConfig{Bundle: newTestBundle(t)}
	cfg.normalize()

	t.Run("zh locale", func(t *testing.T) {
		m := cfg.buildReplaceText("zh")
		if len(m) != 4 {
			t.Fatalf("expected 4 entries, got %d", len(m))
		}
		if m["$t.dashboard.title"] != "仪表盘" {
			t.Errorf("$t.dashboard.title = %q, want %q", m["$t.dashboard.title"], "仪表盘")
		}
		if m["$t.common.save"] != "保存" {
			t.Errorf("$t.common.save = %q, want %q", m["$t.common.save"], "保存")
		}
	})

	t.Run("en locale", func(t *testing.T) {
		m := cfg.buildReplaceText("en")
		if len(m) != 4 {
			t.Fatalf("expected 4 entries, got %d", len(m))
		}
		if m["$t.dashboard.title"] != "Dashboard" {
			t.Errorf("$t.dashboard.title = %q, want %q", m["$t.dashboard.title"], "Dashboard")
		}
	})

	t.Run("missing locale returns empty", func(t *testing.T) {
		m := cfg.buildReplaceText("fr")
		if len(m) != 0 {
			t.Errorf("expected 0 entries for missing locale, got %d", len(m))
		}
	})

	t.Run("custom prefix", func(t *testing.T) {
		cfg2 := I18nConfig{Bundle: newTestBundle(t), KeyPrefix: "@@"}
		cfg2.normalize()
		m := cfg2.buildReplaceText("zh")
		if m["@@dashboard.title"] != "仪表盘" {
			t.Errorf("@@dashboard.title = %q, want %q", m["@@dashboard.title"], "仪表盘")
		}
	})
}

// ============================================================
// buildReplaceTextJS
// ============================================================

func TestI18nConfig_BuildReplaceTextJS(t *testing.T) {
	cfg := I18nConfig{Bundle: newTestBundle(t)}
	cfg.normalize()

	t.Run("valid JSON output", func(t *testing.T) {
		js := cfg.buildReplaceTextJS("zh")
		jsStr := string(js)

		// 验证是合法 JSON
		var result map[string]string
		if err := json.Unmarshal([]byte(jsStr), &result); err != nil {
			t.Fatalf("invalid JSON: %v, content: %s", err, jsStr)
		}
		if result["$t.dashboard.title"] != "仪表盘" {
			t.Errorf("$t.dashboard.title = %q, want %q", result["$t.dashboard.title"], "仪表盘")
		}
	})

	t.Run("empty locale returns empty object", func(t *testing.T) {
		js := cfg.buildReplaceTextJS("fr")
		if string(js) != "{}" {
			t.Errorf("expected {}, got %s", string(js))
		}
	})
}

// ============================================================
// collectI18nKeys
// ============================================================

func TestCollectI18nKeys(t *testing.T) {
	prefix := "$t."

	t.Run("flat object", func(t *testing.T) {
		schema := map[string]interface{}{
			"title":     "$t.dashboard.title",
			"label":     "$t.common.save",
			"type":      "page",
			"className": "bg-blue",
		}
		keys := collectI18nKeys(schema, prefix)
		if len(keys) != 2 {
			t.Fatalf("expected 2 keys, got %d: %v", len(keys), keys)
		}
		if !containsStr(keys, "dashboard.title") {
			t.Error("missing dashboard.title")
		}
		if !containsStr(keys, "common.save") {
			t.Error("missing common.save")
		}
	})

	t.Run("nested object", func(t *testing.T) {
		schema := map[string]interface{}{
			"type": "page",
			"body": map[string]interface{}{
				"title": "$t.page.title",
				"body": []interface{}{
					map[string]interface{}{
						"label": "$t.form.name",
					},
					map[string]interface{}{
						"placeholder": "$t.form.nameHint",
					},
				},
			},
		}
		keys := collectI18nKeys(schema, prefix)
		if len(keys) != 3 {
			t.Fatalf("expected 3 keys, got %d: %v", len(keys), keys)
		}
	})

	t.Run("deduplicates keys", func(t *testing.T) {
		schema := map[string]interface{}{
			"title": "$t.common.save",
			"body": []interface{}{
				map[string]interface{}{"label": "$t.common.save"},
				map[string]interface{}{"placeholder": "$t.common.save"},
			},
		}
		keys := collectI18nKeys(schema, prefix)
		if len(keys) != 1 {
			t.Fatalf("expected 1 key after dedup, got %d: %v", len(keys), keys)
		}
		if keys[0] != "common.save" {
			t.Errorf("key = %q, want %q", keys[0], "common.save")
		}
	})

	t.Run("no i18n keys", func(t *testing.T) {
		schema := map[string]interface{}{
			"type":      "page",
			"title":     "Dashboard",
			"className": "bg-blue",
		}
		keys := collectI18nKeys(schema, prefix)
		if len(keys) != 0 {
			t.Errorf("expected 0 keys, got %d", len(keys))
		}
	})

	t.Run("empty prefix not matched", func(t *testing.T) {
		// 只匹配 $t.xxx，不匹配裸 $t.
		schema := map[string]interface{}{
			"label": "$t.",
		}
		keys := collectI18nKeys(schema, prefix)
		if len(keys) != 0 {
			t.Errorf("expected 0 keys, got %d", len(keys))
		}
	})

	t.Run("array schema", func(t *testing.T) {
		schema := []interface{}{
			map[string]interface{}{"label": "$t.a.b"},
			map[string]interface{}{"label": "$t.c.d"},
		}
		keys := collectI18nKeys(schema, prefix)
		if len(keys) != 2 {
			t.Fatalf("expected 2 keys, got %d", len(keys))
		}
	})

	t.Run("non-string values ignored", func(t *testing.T) {
		schema := map[string]interface{}{
			"title":   "$t.test.key",
			"count":   42,
			"enabled": true,
			"nested":  map[string]interface{}{"x": 1},
		}
		keys := collectI18nKeys(schema, prefix)
		if len(keys) != 1 {
			t.Fatalf("expected 1 key, got %d", len(keys))
		}
	})
}

// ============================================================
// validateSchemaKeys
// ============================================================

func TestI18nConfig_ValidateSchemaKeys(t *testing.T) {
	t.Run("all keys exist", func(t *testing.T) {
		cfg := I18nConfig{Bundle: newTestBundle(t)}
		cfg.normalize()

		schema := map[string]interface{}{
			"title": "$t.dashboard.title",
			"label": "$t.common.save",
		}
		err := cfg.validateSchemaKeys(schema)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing key returns error", func(t *testing.T) {
		cfg := I18nConfig{Bundle: newTestBundle(t)}
		cfg.normalize()

		schema := map[string]interface{}{
			"title": "$t.dashboard.title",
			"label": "$t.nonexistent.key",
		}
		err := cfg.validateSchemaKeys(schema)
		if err == nil {
			t.Fatal("expected error for missing key")
		}
		var keyErr *SchemaKeyError
		if !errorsAs(err, &keyErr) {
			t.Fatalf("expected SchemaKeyError, got %T: %v", err, err)
		}
	})

	t.Run("no i18n keys in schema", func(t *testing.T) {
		cfg := I18nConfig{Bundle: newTestBundle(t)}
		cfg.normalize()

		schema := map[string]interface{}{
			"type":  "page",
			"title": "Plain English",
		}
		err := cfg.validateSchemaKeys(schema)
		if err != nil {
			t.Errorf("unexpected error for no i18n keys: %v", err)
		}
	})

	t.Run("validates against specified locales only", func(t *testing.T) {
		b := newTestBundle(t)
		// Add incomplete ja locale
		b.Add("ja", []i18n.Entry{
			{Key: "dashboard.title", Value: "ダッシュボード"},
			// Missing dashboard.activeUsers
		})

		// Validate only zh and en (both complete)
		cfg := I18nConfig{
			Bundle:          b,
			ValidateLocales: []string{"zh", "en"},
		}
		cfg.normalize()

		schema := map[string]interface{}{
			"title": "$t.dashboard.title",
			"label": "$t.dashboard.activeUsers",
		}
		err := cfg.validateSchemaKeys(schema)
		if err != nil {
			t.Errorf("unexpected error when validating only zh/en: %v", err)
		}

		// Now validate ja too, should fail
		cfg2 := I18nConfig{
			Bundle:          b,
			ValidateLocales: []string{"ja"},
		}
		cfg2.normalize()

		err = cfg2.validateSchemaKeys(schema)
		if err == nil {
			t.Error("expected error when validating ja with missing key")
		}
	})

	t.Run("empty bundle no keys", func(t *testing.T) {
		cfg := I18nConfig{Bundle: newTestBundleEmpty(t), ValidateLocales: []string{"zh"}}
		cfg.normalize()

		schema := map[string]interface{}{
			"title": "$t.some.key",
		}
		err := cfg.validateSchemaKeys(schema)
		if err == nil {
			t.Fatal("expected error for key missing in empty bundle")
		}
	})

	t.Run("default ValidateLocales checks all", func(t *testing.T) {
		b := newTestBundle(t)
		// Add ja with incomplete keys
		b.Add("ja", []i18n.Entry{
			{Key: "dashboard.title", Value: "ダッシュボード"},
			// Missing dashboard.activeUsers
		})

		cfg := I18nConfig{Bundle: b} // ValidateLocales empty → all locales
		cfg.normalize()

		schema := map[string]interface{}{
			"title": "$t.dashboard.title",
			"label": "$t.dashboard.activeUsers",
		}
		err := cfg.validateSchemaKeys(schema)
		if err == nil {
			t.Fatal("expected error: ja is missing dashboard.activeUsers")
		}
	})
}

// ============================================================
// SchemaKeyError
// ============================================================

func TestSchemaKeyError_Error(t *testing.T) {
	err := &SchemaKeyError{
		Missing: map[string][]string{
			"zh": {"common.save", "common.cancel"},
			"ja": {"common.save"},
		},
	}
	msg := err.Error()
	if !strings.Contains(msg, "strict mode") {
		t.Error("error message should mention strict mode")
	}
	if !strings.Contains(msg, "[zh]") {
		t.Error("error message should mention zh")
	}
	if !strings.Contains(msg, "common.save") {
		t.Error("error message should list missing keys")
	}
	if !strings.Contains(msg, "StrictMode=false") {
		t.Error("error message should hint about StrictMode")
	}
}

// ============================================================
// Engine.RegisterSchema with i18n
// ============================================================

func TestEngine_RegisterSchema_StrictMode(t *testing.T) {
	b := newTestBundle(t)
	engine := New(
		WithI18n(I18nConfig{
			Bundle:     b,
			StrictMode: true,
		}),
	)

	t.Run("valid schema registers", func(t *testing.T) {
		schema := map[string]interface{}{
			"type":  "page",
			"title": "$t.dashboard.title",
		}
		err := engine.RegisterSchema("/test", schema)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		// Verify it was stored
		pages := engine.Pages()
		if !containsStr(pages, "/test") {
			t.Error("schema not registered")
		}
	})

	t.Run("missing key fails", func(t *testing.T) {
		schema := map[string]interface{}{
			"type":  "page",
			"title": "$t.nonexistent.key.xyz",
		}
		err := engine.RegisterSchema("/bad", schema)
		if err == nil {
			t.Fatal("expected error for missing key in strict mode")
		}
		if !strings.Contains(err.Error(), "strict mode") {
			t.Errorf("error should mention strict mode: %v", err)
		}
	})

	t.Run("no i18n keys registers fine", func(t *testing.T) {
		schema := map[string]interface{}{
			"type":  "page",
			"title": "Plain text title",
		}
		err := engine.RegisterSchema("/plain", schema)
		if err != nil {
			t.Errorf("unexpected error for schema without i18n keys: %v", err)
		}
	})
}

func TestEngine_RegisterSchema_NonStrictMode(t *testing.T) {
	b := newTestBundle(t)
	engine := New(
		WithI18n(I18nConfig{
			Bundle:     b,
			StrictMode: false,
		}),
	)

	// Even with missing keys, non-strict mode should allow registration
	schema := map[string]interface{}{
		"type":  "page",
		"title": "$t.nonexistent.key",
	}
	err := engine.RegisterSchema("/lenient", schema)
	if err != nil {
		t.Errorf("unexpected error in non-strict mode: %v", err)
	}
}

// ============================================================
// Engine.renderWithLocale
// ============================================================

func TestEngine_RenderWithLocale(t *testing.T) {
	b := newTestBundle(t)
	engine := New(
		WithTitle("Test App"),
		WithI18n(I18nConfig{Bundle: b}),
	)

	schema := map[string]interface{}{
		"type":  "page",
		"title": "$t.dashboard.title",
	}
	if err := engine.RegisterSchema("/test", schema); err != nil {
		t.Fatalf("register schema: %v", err)
	}

	t.Run("zh locale renders", func(t *testing.T) {
		html := engine.renderWithLocale("/test", "zh")
		if html == "" {
			t.Fatal("expected non-empty HTML")
		}
		// Verify replaceText is injected
		if !strings.Contains(html, `"$t.dashboard.title":"仪表盘"`) {
			t.Error("HTML should contain replaceText for zh")
		}
		// Verify locale is set
		if !strings.Contains(html, `locale`) {
			t.Error("HTML should contain locale in props")
		}
	})

	t.Run("en locale renders", func(t *testing.T) {
		html := engine.renderWithLocale("/test", "en")
		if html == "" {
			t.Fatal("expected non-empty HTML")
		}
		if !strings.Contains(html, `"$t.dashboard.title":"Dashboard"`) {
			t.Error("HTML should contain replaceText for en")
		}
	})

	t.Run("missing locale renders empty replaceText", func(t *testing.T) {
		html := engine.renderWithLocale("/test", "fr")
		if html == "" {
			t.Fatal("expected non-empty HTML")
		}
		// Should have empty replaceText
		if !strings.Contains(html, `replaceText`) {
			t.Error("HTML should contain replaceText key")
		}
	})

	t.Run("missing path returns empty", func(t *testing.T) {
		html := engine.renderWithLocale("/nonexistent", "zh")
		if html != "" {
			t.Errorf("expected empty HTML for missing path, got length %d", len(html))
		}
	})
}

// ============================================================
// Engine.Render (backward compatibility)
// ============================================================

func TestEngine_Render_BackwardCompat(t *testing.T) {
	// Without i18n, Render should work as before
	engine := New(WithTitle("Test App"))

	schema := map[string]interface{}{
		"type":  "page",
		"title": "Plain Title",
	}
	if err := engine.RegisterSchema("/test", schema); err != nil {
		t.Fatalf("register schema: %v", err)
	}

	html := engine.Render("/test")
	if html == "" {
		t.Fatal("expected non-empty HTML")
	}
	// Should NOT contain replaceText when i18n is disabled
	if strings.Contains(html, `replaceText`) {
		t.Error("HTML should NOT contain replaceText when i18n is disabled")
	}
}

func TestEngine_Render_WithI18n(t *testing.T) {
	b := newTestBundle(t)
	engine := New(
		WithTitle("Test App"),
		WithI18n(I18nConfig{Bundle: b}),
	)

	schema := map[string]interface{}{
		"type":  "page",
		"title": "$t.dashboard.title",
	}
	if err := engine.RegisterSchema("/test", schema); err != nil {
		t.Fatalf("register schema: %v", err)
	}

	// Render with default (no locale) → still works but no i18n injection
	html := engine.Render("/test")
	if html == "" {
		t.Fatal("expected non-empty HTML")
	}
	// Should have schema JSON with $t. placeholder
	if !strings.Contains(html, `$t.dashboard.title`) {
		t.Error("HTML should contain original $t.dashboard.title in schema")
	}
}

// ============================================================
// WithI18n option
// ============================================================

func TestWithI18n_Option(t *testing.T) {
	b := newTestBundle(t)
	engine := New(WithI18n(I18nConfig{
		Bundle:     b,
		StrictMode: true,
		KeyPrefix:  "@@",
	}))

	cfg := engine.I18n()
	if cfg == nil {
		t.Fatal("I18n() should return non-nil config")
	}
	if cfg.Bundle != b {
		t.Error("Bundle mismatch")
	}
	if !cfg.StrictMode {
		t.Error("StrictMode should be true")
	}
	if cfg.KeyPrefix != "@@" {
		t.Errorf("KeyPrefix = %q, want %q", cfg.KeyPrefix, "@@")
	}
}

func TestWithI18n_NilWhenNotSet(t *testing.T) {
	engine := New()
	if engine.I18n() != nil {
		t.Error("I18n() should return nil when i18n not configured")
	}
}

// ============================================================
// resolveLocale
// ============================================================

func TestI18nConfig_ResolveLocale(t *testing.T) {
	cfg := I18nConfig{Bundle: newTestBundle(t)}
	cfg.normalize()

	tests := []struct {
		input string
		want  string
	}{
		{"zh-CN", "zh"},
		{"zh_cn", "zh"},
		{"en-US", "en"},
		{"en", "en"},
		{"ja", "ja"},
		{"", "en"}, // fallback
	}
	for _, tt := range tests {
		got := cfg.resolveLocale(tt.input)
		if got != tt.want {
			t.Errorf("resolveLocale(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ============================================================
// 集成测试：完整 Schema 注册 + 渲染
// ============================================================

func TestEngine_FullI18nFlow(t *testing.T) {
	b := newTestBundle(t)
	engine := New(
		WithTitle("My App"),
		WithI18n(I18nConfig{
			Bundle:     b,
			StrictMode: true,
		}),
	)

	// 注册一个包含多个 $t. key 的复杂 schema
	schema := map[string]interface{}{
		"type":  "page",
		"title": "$t.dashboard.title",
		"body": []interface{}{
			map[string]interface{}{
				"type": "form",
				"body": []interface{}{
					map[string]interface{}{
						"type":        "input-text",
						"label":       "$t.common.save",
						"placeholder": "$t.common.cancel",
					},
				},
			},
		},
	}

	err := engine.RegisterSchema("/complex", schema)
	if err != nil {
		t.Fatalf("register schema: %v", err)
	}

	html := engine.renderWithLocale("/complex", "zh")
	if html == "" {
		t.Fatal("expected non-empty HTML")
	}

	// Verify all translations are present
	checks := []string{
		`"$t.dashboard.title":"仪表盘"`,
		`"$t.common.save":"保存"`,
		`"$t.common.cancel":"取消"`,
	}
	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("HTML missing: %s", check)
		}
	}

	// Verify original placeholder is in schema JSON
	if !strings.Contains(html, `$t.dashboard.title`) {
		t.Error("HTML should contain original $t.dashboard.title in schema")
	}
}

func TestEngine_I18n_NonStrictWithMissingKeys(t *testing.T) {
	b := newTestBundle(t)
	engine := New(
		WithTitle("My App"),
		WithI18n(I18nConfig{
			Bundle:     b,
			StrictMode: false,
		}),
	)

	// Schema with key not in bundle
	schema := map[string]interface{}{
		"type":  "page",
		"title": "$t.missing.key.here",
	}

	err := engine.RegisterSchema("/missing-key", schema)
	if err != nil {
		t.Fatalf("unexpected error in non-strict mode: %v", err)
	}

	html := engine.renderWithLocale("/missing-key", "zh")
	if html == "" {
		t.Fatal("expected non-empty HTML")
	}

	// The missing key should remain as-is in the schema (not in replaceText)
	if !strings.Contains(html, `$t.missing.key.here`) {
		t.Error("HTML should contain the untranslated $t.missing.key.here in schema")
	}
}

// ============================================================
// 辅助函数
// ============================================================

func containsStr(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// errorsAs 简单版 errors.As，兼容测试。
func errorsAs(err error, target interface{}) bool {
	switch e := err.(type) {
	case *SchemaKeyError:
		if t, ok := target.(**SchemaKeyError); ok {
			*t = e
			return true
		}
	}
	return false
}

// ============================================================
// ForceLocale
// ============================================================

func TestEngine_ForceLocale(t *testing.T) {
	b := newTestBundle(t)
	engine := New(
		WithTitle("Force Test"),
		WithI18n(I18nConfig{
			Bundle:      b,
			ForceLocale: "zh",
		}),
	)

	schema := map[string]interface{}{
		"type":  "page",
		"title": "$t.dashboard.title",
	}
	if err := engine.RegisterSchema("/force", schema); err != nil {
		t.Fatalf("register schema: %v", err)
	}

	t.Run("request locale ignored when ForceLocale set", func(t *testing.T) {
		// 请求说 en，但 ForceLocale=zh 应该生效
		html := engine.renderWithLocale("/force", "en")
		if html == "" {
			t.Fatal("expected non-empty HTML")
		}
		if !strings.Contains(html, `"$t.dashboard.title":"仪表盘"`) {
			t.Error("ForceLocale=zh should produce Chinese translation even when request says en")
		}
	})

	t.Run("Render uses ForceLocale", func(t *testing.T) {
		html := engine.Render("/force")
		if html == "" {
			t.Fatal("expected non-empty HTML")
		}
		if !strings.Contains(html, `"$t.dashboard.title":"仪表盘"`) {
			t.Error("Render() should use ForceLocale=zh")
		}
	})
}

func TestEngine_ForceLocale_Empty(t *testing.T) {
	b := newTestBundle(t)
	// ForceLocale="" → 跟随请求语言（默认行为）
	engine := New(
		WithTitle("No Force"),
		WithI18n(I18nConfig{
			Bundle:      b,
			ForceLocale: "",
		}),
	)

	schema := map[string]interface{}{
		"type":  "page",
		"title": "$t.dashboard.title",
	}
	if err := engine.RegisterSchema("/noforce", schema); err != nil {
		t.Fatalf("register schema: %v", err)
	}

	t.Run("follows request locale when ForceLocale empty", func(t *testing.T) {
		html := engine.renderWithLocale("/noforce", "en")
		if !strings.Contains(html, `"$t.dashboard.title":"Dashboard"`) {
			t.Error("should use en translation when ForceLocale is empty and request says en")
		}
	})
}

func TestI18nConfig_ForceLocale_Normalized(t *testing.T) {
	b := newTestBundle(t)
	engine := New(
		WithI18n(I18nConfig{
			Bundle:      b,
			ForceLocale: "zh-CN", // 区域变体 → 应归一化为 "zh"
		}),
	)

	schema := map[string]interface{}{
		"type":  "page",
		"title": "$t.dashboard.title",
	}
	if err := engine.RegisterSchema("/normalized", schema); err != nil {
		t.Fatalf("register schema: %v", err)
	}

	html := engine.renderWithLocale("/normalized", "en")
	if !strings.Contains(html, `"$t.dashboard.title":"仪表盘"`) {
		t.Error("ForceLocale=zh-CN should be normalized to zh and produce Chinese")
	}
}
