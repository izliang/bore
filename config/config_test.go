package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/izliang/bore/config"
	"github.com/izliang/bore/config/source"
)

// staticSource is a test helper that implements config.Source with fixed bytes.
type staticSource struct {
	data []byte
}

func (s *staticSource) Load() ([]byte, error) {
	return s.data, nil
}

type testConfig struct {
	Service struct {
		Name string `yaml:"name"`
		Port int    `yaml:"port"`
	} `yaml:"service"`
	Database struct {
		DSN string `yaml:"dsn"`
	} `yaml:"database"`
}

func TestCfg_Load_YAML(t *testing.T) {
	cfg := config.New[testConfig]()

	yamlData := []byte(`
service:
  name: myapp
  port: 8080
database:
  dsn: "user:pass@tcp(localhost:3306)/db"
`)
	err := cfg.Load(&staticSource{data: yamlData})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := cfg.Get()
	if got.Service.Name != "myapp" {
		t.Errorf("Service.Name = %q, want %q", got.Service.Name, "myapp")
	}
	if got.Service.Port != 8080 {
		t.Errorf("Service.Port = %d, want %d", got.Service.Port, 8080)
	}
	if got.Database.DSN != "user:pass@tcp(localhost:3306)/db" {
		t.Errorf("Database.DSN = %q", got.Database.DSN)
	}
}

func TestCfg_TemplateRendering(t *testing.T) {
	cfg := config.New[testConfig]()
	cfg.SetTemplates(map[string]string{
		"Env":  "prod",
		"Host": "db.prod.example.com",
	})

	yamlData := []byte(`
service:
  name: myapp-{{.Env}}
  port: 8080
database:
  dsn: "user:pass@tcp({{.Host}}:3306)/db"
`)
	err := cfg.Load(&staticSource{data: yamlData})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := cfg.Get()
	if got.Service.Name != "myapp-prod" {
		t.Errorf("Service.Name = %q, want %q", got.Service.Name, "myapp-prod")
	}
	if got.Database.DSN != "user:pass@tcp(db.prod.example.com:3306)/db" {
		t.Errorf("Database.DSN = %q", got.Database.DSN)
	}
}

func TestCfg_TemplatesPersistAcrossReload(t *testing.T) {
	cfg := config.New[testConfig]()
	cfg.SetTemplates(map[string]string{"Env": "staging"})

	yamlData := []byte(`service: {name: "{{.Env}}", port: 1}`)
	err := cfg.Load(&staticSource{data: yamlData})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Get().Service.Name != "staging" {
		t.Errorf("Name = %q, want staging", cfg.Get().Service.Name)
	}

	err = cfg.Reload()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if cfg.Get().Service.Name != "staging" {
		t.Errorf("Name after reload = %q, want staging", cfg.Get().Service.Name)
	}
}

func TestCfg_GetBeforeLoad_ReturnsZero(t *testing.T) {
	cfg := config.New[testConfig]()
	got := cfg.Get()
	if got.Service.Name != "" {
		t.Errorf("expected empty name, got %q", got.Service.Name)
	}
	if got.Service.Port != 0 {
		t.Errorf("expected port 0, got %d", got.Service.Port)
	}
}

func TestCfg_Reload(t *testing.T) {
	cfg := config.New[testConfig]()

	err := cfg.Load(&staticSource{data: []byte(`
service:
  name: v1
  port: 80
`)})
	if err != nil {
		t.Fatalf("first Load: %v", err)
	}

	if cfg.Get().Service.Name != "v1" {
		t.Fatalf("expected v1")
	}

	err = cfg.Reload()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if cfg.Get().Service.Name != "v1" {
		t.Errorf("expected v1 after reload of static source")
	}
}

func TestCfg_Watch(t *testing.T) {
	cfg := config.New[testConfig]()

	var oldName, newName string
	cancel := cfg.Watch(func(old, new testConfig) {
		oldName = old.Service.Name
		newName = new.Service.Name
	})

	err := cfg.Load(&staticSource{data: []byte(`
service:
  name: first
  port: 80
`)})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if oldName != "" {
		t.Errorf("oldName should be empty on first load, got %q", oldName)
	}
	if newName != "first" {
		t.Errorf("newName = %q, want first", newName)
	}

	err = cfg.Reload()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if oldName != "first" {
		t.Errorf("oldName = %q, want first", oldName)
	}
	if newName != "first" {
		t.Errorf("newName = %q, want first", newName)
	}

	cancel()

	err = cfg.Reload()
	if err != nil {
		t.Fatalf("Reload after cancel: %v", err)
	}
}

func TestCfg_Loaded(t *testing.T) {
	cfg := config.New[testConfig]()

	if cfg.Loaded() {
		t.Error("Loaded() should be false before Load")
	}

	cfg.Load(&staticSource{data: []byte(`service: {name: x, port: 1}`)})
	if !cfg.Loaded() {
		t.Error("Loaded() should be true after Load")
	}
}

func TestCfg_LoadRootFile(t *testing.T) {
	// Create a temp YAML file in a temp directory structure that simulates
	// a project root. LoadRootFile resolves against GetProjectRootDir which
	// requires go.mod or .root marker.
	tmpDir := t.TempDir()

	// Place a .root marker so GetProjectRootDir finds it.
	os.WriteFile(filepath.Join(tmpDir, ".root"), []byte{}, 0o644)

	confDir := filepath.Join(tmpDir, "conf")
	os.MkdirAll(confDir, 0o755)

	yamlContent := []byte("service:\n  name: rootapp\n  port: 9090\n")
	os.WriteFile(filepath.Join(confDir, "config.yaml"), yamlContent, 0o644)

	// Save and change working directory so GetProjectRootDir resolves correctly.
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(tmpDir)

	cfg := config.New[testConfig]()
	err := cfg.LoadRootFile("conf/config.yaml")
	if err != nil {
		t.Fatalf("LoadRootFile: %v", err)
	}

	got := cfg.Get()
	if got.Service.Name != "rootapp" {
		t.Errorf("Name = %q, want rootapp", got.Service.Name)
	}
	if got.Service.Port != 9090 {
		t.Errorf("Port = %d, want 9090", got.Service.Port)
	}
}

func TestCfg_LoadRootFile_WithTemplates(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, ".root"), []byte{}, 0o644)

	confDir := filepath.Join(tmpDir, "conf")
	os.MkdirAll(confDir, 0o755)

	os.WriteFile(filepath.Join(confDir, "config.yaml"), []byte(`
service:
  name: myapp-{{.Env}}
  port: 8080
`), 0o644)

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	os.Chdir(tmpDir)

	cfg := config.New[testConfig]()
	cfg.SetTemplates(map[string]string{"Env": "prod"})
	err := cfg.LoadRootFile("conf/config.yaml")
	if err != nil {
		t.Fatalf("LoadRootFile: %v", err)
	}

	got := cfg.Get()
	if got.Service.Name != "myapp-prod" {
		t.Errorf("Name = %q, want myapp-prod", got.Service.Name)
	}
}

func TestCfg_ReloadBeforeLoad(t *testing.T) {
	cfg := config.New[testConfig]()
	err := cfg.Reload()
	if err == nil {
		t.Fatal("expected error from Reload before Load")
	}
}

func TestCfg_EnvSource(t *testing.T) {
	os.Clearenv()
	os.Setenv("MYAPP_SERVICE_NAME", "fromenv")
	os.Setenv("MYAPP_SERVICE_PORT", "3000")

	cfg := config.New[testConfig]()
	err := cfg.Load(source.Env("MYAPP_", "_"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := cfg.Get()
	if got.Service.Name != "fromenv" {
		t.Errorf("Name = %q, want fromenv", got.Service.Name)
	}
	if got.Service.Port != 3000 {
		t.Errorf("Port = %d, want 3000", got.Service.Port)
	}
}
