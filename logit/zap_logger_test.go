package logit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewZapWithConfig_OutputModes(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	tests := []struct {
		name     string
		mode     OutputMode
		filePath string
	}{
		{"stdout only", OutputStd, ""},
		{"file only", OutputFile, logFile},
		{"stdout and file", OutputStdFile, logFile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewZapWithConfig(ZapConfig{
				Mode:     tt.mode,
				Config:   Config{Level: Info, PodName: "testapp"},
				FileName: tt.filePath,
			})

			ctx := context.Background()
			logger.Info(ctx, "test message")

			if tt.filePath != "" {
				content, err := os.ReadFile(tt.filePath)
				if err != nil {
					t.Fatalf("failed to read log file: %v", err)
				}
				if !strings.Contains(string(content), "test message") {
					t.Error("expected message in log file")
				}
			}
		})
	}
}

func TestWithLevel_ZapLoggerScopedDebug(t *testing.T) {
	base := NewZapWithConfig(ZapConfig{
		Mode:   OutputStd,
		Config: Config{Level: Info, PodName: "testapp"},
	})
	zBase := base.(*ZapLogger)

	debugLogger := WithLevel(base, Debug)
	zDebug := debugLogger.(*ZapLogger)

	if zBase == zDebug {
		t.Fatal("WithLevel should return a copied ZapLogger, not mutate original")
	}
	if zBase.level != Info {
		t.Fatalf("base level changed: got %q", zBase.level)
	}
	if zDebug.level != Debug {
		t.Fatalf("scoped level = %q, want %q", zDebug.level, Debug)
	}
	if len(zBase.writers) == 0 || len(zDebug.writers) == 0 || zBase.writers[0] != zDebug.writers[0] {
		t.Fatal("scoped logger should reuse base writers")
	}
	if zBase.shouldLog(Debug) {
		t.Fatal("base logger should still filter debug")
	}
	if !zDebug.shouldLog(Debug) {
		t.Fatal("scoped logger should allow debug")
	}
}

func TestZapRequestIDCtxKey(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// Use string key (same as baseLogger uses string comparison)
	const reqIDCtxKey = "my_request_id"

	logger := NewZapWithConfig(ZapConfig{
		Mode:     OutputFile,
		Config:   Config{Level: Info, PodName: "testapp", TraceID: &DefaultTraceID{}},
		FileName: logFile,
	})

	// With request ID
	ctx := context.Background()
	// Use string key directly (matching baseLogger behavior)
	ctxWithID := context.WithValue(ctx, reqIDCtxKey, "req-12345")
	logger.Info(ctxWithID, "test message")

	// Check if request ID appears in the log
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "req-12345") {
		t.Errorf("expected request ID 'req-12345' in log output, got: %s", string(content))
	}
}

func TestZapRotationDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	logger := NewZapWithConfig(ZapConfig{
		Mode:     OutputFile,
		Config:   Config{Level: Info, PodName: "testapp"},
		FileName: logFile,
		// Use defaults for rotation
	})

	ctx := context.Background()
	logger.Info(ctx, "rotation test")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "rotation test") {
		t.Error("expected message in log file")
	}
}
