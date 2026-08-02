package source

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/izliang/bore/config"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Viper creates a config.Source backed by viper. It reads a YAML
// configuration file and overlays environment variables using viper's
// AutomaticEnv.
//
// relPath is resolved relative to the project root directory.
// envPrefix is the environment variable prefix (e.g. "APP").
// Trailing underscores are stripped automatically. If empty,
// environment variable overlay is disabled.
//
// Environment variables use "_" as the nested-key separator. For example,
// with prefix "APP", key "database.dsn" maps to "APP_DATABASE_DSN".
//
// Example:
//
//	src := source.Viper("conf/config.yaml", "MYAPP")
func Viper(relPath, envPrefix string) config.Source {
	envPrefix = strings.TrimRight(envPrefix, "_")
	return &viperSource{
		relPath:   relPath,
		envPrefix: envPrefix,
	}
}

type viperSource struct {
	relPath   string
	envPrefix string
}

// Load reads the config file and resolves all values through viper's
// layered lookup chain (env vars override file values). Environment
// variable values are type-coerced to match the config file types
// (e.g. string "9090" → int 9090).
func (s *viperSource) Load() ([]byte, error) {
	root, err := config.GetProjectRootDir()
	if err != nil {
		return nil, fmt.Errorf("viper: project root: %w", err)
	}
	absPath := filepath.Join(root, s.relPath)

	v := viper.New()
	v.SetConfigFile(absPath)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("viper: read config: %w", err)
		}
	}

	// Capture file-only settings to use as type reference. AllSettings()
	// returns only config-file values (no env vars yet).
	fileSettings := v.AllSettings()

	v.SetEnvPrefix(s.envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	allKeys := v.AllKeys()
	if len(allKeys) == 0 {
		return []byte{}, nil
	}

	// Build merged map, coercing env var string values to match the
	// types found in the config file.
	merged := make(map[string]any)
	for _, key := range allKeys {
		val := v.Get(key)
		if ref := getNested(fileSettings, key); ref != nil {
			val = coerceType(val, ref)
		}
		setNested(merged, key, val)
	}

	return yaml.Marshal(merged)
}

// getNested retrieves a value from a nested map by dotted key.
func getNested(m map[string]any, key string) any {
	parts := strings.Split(key, ".")
	cur := any(m)
	for _, p := range parts {
		cm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = cm[p]
	}
	return cur
}

// coerceType converts val to match the type of ref when val is a string
// (environment variable) and ref is a non-string scalar.
func coerceType(val, ref any) any {
	s, isStr := val.(string)
	if !isStr {
		return val
	}
	switch ref.(type) {
	case int, int64, int32, int16, int8:
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	case float64, float32:
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	case bool:
		if b, err := strconv.ParseBool(s); err == nil {
			return b
		}
	}
	return val
}

// setNested inserts val into m at the dotted key path.
func setNested(m map[string]any, key string, val any) {
	parts := strings.Split(key, ".")
	cur := m
	for i := 0; i < len(parts)-1; i++ {
		child, ok := cur[parts[i]]
		if !ok {
			child = make(map[string]any)
			cur[parts[i]] = child
		}
		cur = child.(map[string]any)
	}
	cur[parts[len(parts)-1]] = val
}
