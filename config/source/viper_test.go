package source_test

import (
	"os"
	"testing"

	"github.com/izliang/bore/config"
	"github.com/izliang/bore/config/source"
	"gopkg.in/yaml.v3"
)

func TestViper(t *testing.T) {
	tests := []struct {
		name      string
		relPath   string
		envPrefix string
		envVars   map[string]string
		want      map[string]any
		wantErr   string
	}{
		{
			name:    "YAML only, no env prefix",
			relPath: "config/source/testdata/simple.yaml",
			want:    map[string]any{"name": "myapp", "port": 8080},
		},
		{
			name:      "YAML only, with env prefix but no matching vars",
			relPath:   "config/source/testdata/simple.yaml",
			envPrefix: "APP",
			want:      map[string]any{"name": "myapp", "port": 8080},
		},
		{
			name:      "env var overrides YAML value",
			relPath:   "config/source/testdata/simple.yaml",
			envPrefix: "APP",
			envVars:   map[string]string{"APP_NAME": "overridden"},
			want:      map[string]any{"name": "overridden", "port": 8080},
		},
		{
			name:      "env var overrides nested key with type coercion",
			relPath:   "config/source/testdata/nested.yaml",
			envPrefix: "APP",
			envVars:   map[string]string{"APP_SERVICE_PORT": "9090"},
			want: map[string]any{
				"service": map[string]any{"name": "svc", "port": 9090},
			},
		},
		{
			name:      "deeply nested env override",
			relPath:   "config/source/testdata/deep.yaml",
			envPrefix: "APP",
			envVars:   map[string]string{"APP_A_B_C": "new"},
			want: map[string]any{
				"a": map[string]any{
					"b": map[string]any{"c": "new", "d": "kept"},
				},
			},
		},
		{
			name:      "string type from env",
			relPath:   "config/source/testdata/simple.yaml",
			envPrefix: "APP",
			envVars:   map[string]string{"APP_NAME": "user:pass@tcp(host)/db"},
			want:      map[string]any{"name": "user:pass@tcp(host)/db", "port": 8080},
		},
		{
			name:      "env var with underscore in value",
			relPath:   "config/source/testdata/simple.yaml",
			envPrefix: "APP",
			envVars:   map[string]string{"APP_NAME": "user_admin"},
			want:      map[string]any{"name": "user_admin", "port": 8080},
		},
		{
			name:    "empty config file",
			relPath: "config/source/testdata/empty.yaml",
			want:    map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			src := source.Viper(tt.relPath, tt.envPrefix)
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
					t.Fatalf("unmarshal result: %v\nraw:\n%s", err, string(got))
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

func TestViper_NoEnvPrefix(t *testing.T) {
	os.Clearenv()
	os.Setenv("APP_NAME", "should-not-override")

	src := source.Viper("config/source/testdata/simple.yaml", "")
	got, err := src.Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	yaml.Unmarshal(got, &result)

	if result["name"] != "myapp" {
		t.Errorf("name = %v, want myapp (env should not override when prefix is empty)", result["name"])
	}
}

func TestViper_Integration(t *testing.T) {
	// Verify Viper source integrates with Cfg[T] including templates and
	// type coercion (string env var → int struct field).
	type svcCfg struct {
		Service struct {
			Name string `yaml:"name"`
			Port int    `yaml:"port"`
		} `yaml:"service"`
	}

	os.Clearenv()
	os.Setenv("APP_SERVICE_PORT", "9090")

	cfg := config.New[svcCfg]()
	cfg.SetTemplates(map[string]string{"Env": "prod"})
	err := cfg.Load(source.Viper("config/source/testdata/nested.yaml", "APP"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := cfg.Get()
	if got.Service.Name != "svc" {
		t.Errorf("Name = %q, want svc", got.Service.Name)
	}
	if got.Service.Port != 9090 {
		t.Errorf("Port = %d, want 9090 (env override with type coercion)", got.Service.Port)
	}
}

func TestViper_BoolCoercion(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(tmpDir+"/.root", []byte{}, 0o644)
	os.WriteFile(tmpDir+"/config.yaml", []byte("debug: false\n"), 0o644)

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(tmpDir)

	os.Clearenv()
	os.Setenv("APP_DEBUG", "true")

	type dbgCfg struct {
		Debug bool `yaml:"debug"`
	}

	cfg := config.New[dbgCfg]()
	err := cfg.Load(source.Viper("config.yaml", "APP"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !cfg.Get().Debug {
		t.Error("Debug = false, want true (bool coercion from env)")
	}
}
