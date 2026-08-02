package source_test

import (
	"os"
	"testing"

	"github.com/izliang/bore/config"
	"github.com/izliang/bore/config/source"
)

func TestEnvSource(t *testing.T) {
	os.Clearenv()
	os.Setenv("MYAPP_SERVICE_NAME", "myapp")
	os.Setenv("MYAPP_SERVICE_PORT", "8080")
	os.Setenv("MYAPP_DATABASE_DSN", "user:pass@tcp(localhost:3306)/db")

	type testConfig struct {
		Service struct {
			Name string `yaml:"name"`
			Port int    `yaml:"port"`
		} `yaml:"service"`
		Database struct {
			DSN string `yaml:"dsn"`
		} `yaml:"database"`
	}

	cfg := config.New[testConfig]()
	src := source.Env("MYAPP_", "_")

	err := cfg.Load(src)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := cfg.Get()
	if got.Service.Name != "myapp" {
		t.Errorf("Service.Name = %q, want myapp", got.Service.Name)
	}
	if got.Service.Port != 8080 {
		t.Errorf("Service.Port = %d, want 8080", got.Service.Port)
	}
	if got.Database.DSN != "user:pass@tcp(localhost:3306)/db" {
		t.Errorf("Database.DSN = %q", got.Database.DSN)
	}
}

func TestEnvSource_NoMatchingVars(t *testing.T) {
	os.Clearenv()

	type emptyConfig struct {
		Name string `yaml:"name"`
	}

	cfg := config.New[emptyConfig]()
	src := source.Env("NONEXISTENT_", "_")

	err := cfg.Load(src)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Should load without error, values are zero
	got := cfg.Get()
	if got.Name != "" {
		t.Errorf("Name = %q, want empty", got.Name)
	}
}
