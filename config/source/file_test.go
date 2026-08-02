package source_test

import (
	"testing"

	"github.com/izliang/bore/config"
	"github.com/izliang/bore/config/source"
	"gopkg.in/yaml.v3"
)

func TestFile(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		want    map[string]any
		wantErr string
	}{
		{
			name:    "simple YAML",
			relPath: "config/source/testdata/simple.yaml",
			want:    map[string]any{"name": "myapp", "port": 8080},
		},
		{
			name:    "nested YAML",
			relPath: "config/source/testdata/nested.yaml",
			want: map[string]any{
				"service": map[string]any{"name": "svc", "port": 3000},
			},
		},
		{
			name:    "empty file",
			relPath: "config/source/testdata/empty.yaml",
			want:    map[string]any{},
		},
		{
			name:    "deep nested YAML",
			relPath: "config/source/testdata/deep.yaml",
			want: map[string]any{
				"a": map[string]any{
					"b": map[string]any{"c": "old", "d": "kept"},
				},
			},
		},
		{
			name:    "file not found",
			relPath: "nonexistent/path.yaml",
			wantErr: "no such file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := source.File(tt.relPath)
			got, err := src.Load()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var result map[string]any
			if len(got) > 0 {
				if err := yaml.Unmarshal(got, &result); err != nil {
					t.Fatalf("unmarshal result: %v", err)
				}
			}
			if result == nil {
				result = map[string]any{}
			}

			if !deepEqual(result, tt.want) {
				t.Errorf("got:\n%v\nwant:\n%v", result, tt.want)
			}
		})
	}
}

func TestFile_Integration(t *testing.T) {
	// Verify File source integrates with Cfg[T].
	type svcCfg struct {
		Service struct {
			Name string `yaml:"name"`
			Port int    `yaml:"port"`
		} `yaml:"service"`
	}

	cfg := config.New[svcCfg]()
	err := cfg.Load(source.File("config/source/testdata/nested.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := cfg.Get()
	if got.Service.Name != "svc" {
		t.Errorf("Name = %q, want svc", got.Service.Name)
	}
	if got.Service.Port != 3000 {
		t.Errorf("Port = %d, want 3000", got.Service.Port)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func deepEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		am, aMap := av.(map[string]any)
		bm, bMap := bv.(map[string]any)
		if aMap && bMap {
			if !deepEqual(am, bm) {
				return false
			}
		} else if av != bv {
			return false
		}
	}
	return true
}
