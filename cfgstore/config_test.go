package cfgstore

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/izliang/bore/logit"
)

var logger = logit.NewSimple(os.Stdout)

// mockLoader 用于测试的 Loader 实现
type mockLoader struct {
	data      map[string]string
	callCount int
}

func (m *mockLoader) LoadAll(ctx context.Context, logger logit.Logger) (LoadResult, error) {
	m.callCount++
	return LoadResult{
		Data:      m.data,
		NextSince: time.Now(),
	}, nil
}

func (m *mockLoader) LoadSince(ctx context.Context, logger logit.Logger, since time.Time) (LoadResult, error) {
	m.callCount++
	return LoadResult{
		Data:      m.data,
		NextSince: time.Now(),
	}, nil
}

func TestNew_InitSuccess(t *testing.T) {
	ctx := context.Background()
	loader := &mockLoader{
		data: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	cfg, err := New(ctx, loader, logger)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cfg.Stop()

	if v := cfg.Get("key1"); v != "value1" {
		t.Errorf("want 'value1', got %q", v)
	}

	if v := cfg.Get("key2"); v != "value2" {
		t.Errorf("want 'value2', got %q", v)
	}

	if loader.callCount != 1 {
		t.Errorf("want 1 LoadAll call, got %d", loader.callCount)
	}
}

func TestConfig_GetDefault(t *testing.T) {
	ctx := context.Background()
	loader := &mockLoader{data: map[string]string{"exist": "yes"}}

	cfg, _ := New(ctx, loader, logger)
	defer cfg.Stop()

	if v := cfg.GetDefault("exist", "default"); v != "yes" {
		t.Errorf("want 'yes', got %q", v)
	}

	if v := cfg.GetDefault("not_exist", "default"); v != "default" {
		t.Errorf("want 'default', got %q", v)
	}
}

func TestConfig_GetAll(t *testing.T) {
	ctx := context.Background()
	data := map[string]string{"k1": "v1", "k2": "v2"}
	loader := &mockLoader{data: data}

	cfg, _ := New(ctx, loader, logger)
	defer cfg.Stop()

	all := cfg.GetAll()
	if len(all) != 2 || all["k1"] != "v1" || all["k2"] != "v2" {
		t.Errorf("want %v, got %v", data, all)
	}

	// 修改返回的 map 不应该影响内部
	all["k1"] = "modified"
	if v := cfg.Get("k1"); v != "v1" {
		t.Error("GetAll returned map should be a copy")
	}
}

func TestConfig_GetJSON(t *testing.T) {
	ctx := context.Background()
	loader := &mockLoader{
		data: map[string]string{
			"words": `["hello", "world"]`,
		},
	}

	cfg, _ := New(ctx, loader, logger)
	defer cfg.Stop()

	words, _, err := GetJSON[[]string](cfg, "words")
	if err != nil {
		t.Fatalf("GetJSON failed: %v", err)
	}
	if len(words) != 2 || words[0] != "hello" || words[1] != "world" {
		t.Errorf("want [hello world], got %v", words)
	}
}

func TestConfig_GetWithParser(t *testing.T) {
	ctx := context.Background()
	loader := &mockLoader{
		data: map[string]string{
			"threshold": "42",
		},
	}

	cfg, _ := New(ctx, loader, logger)
	defer cfg.Stop()

	val, _, err := GetWithParser(cfg, "threshold", strconv.Atoi)
	if err != nil {
		t.Fatalf("GetWithParser failed: %v", err)
	}
	if val != 42 {
		t.Errorf("want 42, got %d", val)
	}
}

func TestConfig_Refresh(t *testing.T) {
	ctx := context.Background()
	loader := &mockLoader{data: map[string]string{"key": "old"}}

	cfg, _ := New(ctx, loader, logger)
	defer cfg.Stop()

	if v := cfg.Get("key"); v != "old" {
		t.Errorf("want 'old', got %q", v)
	}

	// 更新数据
	loader.data["key"] = "new"

	// 刷新
	if err := cfg.Refresh(ctx); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if v := cfg.Get("key"); v != "new" {
		t.Errorf("want 'new', got %q", v)
	}
}

func TestConfig_AutoRefresh(t *testing.T) {
	ctx := context.Background()
	loader := &mockLoader{data: map[string]string{"key": "v1"}}

	cfg, err := New(ctx, loader, logger, WithRefreshInterval(50*time.Millisecond))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer cfg.Stop()

	// 等待自动刷新
	time.Sleep(120 * time.Millisecond)

	// 应该至少调用了初始 LoadAll + 2次刷新
	if loader.callCount < 3 {
		t.Errorf("want at least 3 calls, got %d", loader.callCount)
	}
}

func TestConfig_EmptyKey(t *testing.T) {
	ctx := context.Background()
	loader := &mockLoader{data: map[string]string{}}

	cfg, _ := New(ctx, loader, logger)
	defer cfg.Stop()

	if v := cfg.Get("not_exist"); v != "" {
		t.Errorf("want empty string, got %q", v)
	}
}
