// Package config provides a type-safe configuration container with hot-reload
// and template support.
//
// Core concept: users define their own struct, and Cfg[T] wraps it with
// loading, template rendering, atomic access, and change watching.
//
// Usage:
//
//	cfg := config.New[AppConfig]()
//	cfg.SetTemplates(map[string]string{"Env": "prod"})
//	cfg.Load(source.Viper("conf/config.yaml", "APP_"))
//	port := cfg.Get().Service.Port  // type-safe, IDE-completable
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"text/template"

	"gopkg.in/yaml.v3"
)

// rootMarkers are files whose presence in a directory indicates it is
// the project root. .root is for production deployments; go.mod is for
// development.
var rootMarkers = []string{".root", "go.mod"}

// Source provides raw configuration bytes. Implementations live in the
// config/source sub-package:
//
//	source.File(relPath)         — reads a YAML file from the project root
//	source.Viper(relPath, prefix) — viper-backed with env var overlay
//	source.Env(prefix, delim)    — environment variables only
type Source interface {
	Load() ([]byte, error)
}

// Cfg is a type-safe configuration container that supports hot-reload
// and template rendering. T is the user-defined configuration struct.
//
// A zero-value Cfg[T] is safe to call Get() on (returns zero T).
// Always create via New[T]().
type Cfg[T any] struct {
	mu        sync.RWMutex
	current   T
	source    Source
	templates map[string]string
	loaded    bool
	watchers  []func(old, new T)
}

// New creates a new Cfg[T].
func New[T any]() *Cfg[T] {
	return &Cfg[T]{}
}

// SetTemplates registers Go template variables for {{.Var}} replacement
// in raw config bytes. Templates are applied transparently on every
// Load() and Reload() call.
//
// Example:
//
//	cfg.SetTemplates(map[string]string{"Env": "prod"})
//	// YAML: "name: myapp-{{.Env}}" → "name: myapp-prod"
func (c *Cfg[T]) SetTemplates(tmpl map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.templates = tmpl
}

// Get returns a copy of the current configuration.
// Safe for concurrent use. Returns zero T if Load was never called.
func (c *Cfg[T]) Get() T {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

// Load loads configuration from the given source.
// Subsequent calls to Reload() re-read from the same source.
func (c *Cfg[T]) Load(source Source) error {
	val, err := c.loadFrom(source)
	if err != nil {
		return err
	}

	c.mu.Lock()
	old := c.current
	c.current = val
	c.source = source
	c.loaded = true
	c.mu.Unlock()

	c.notify(old, val)
	return nil
}

// Reload re-reads configuration from the previously used source.
// Returns an error if Load was never called.
func (c *Cfg[T]) Reload() error {
	c.mu.RLock()
	s := c.source
	loaded := c.loaded
	c.mu.RUnlock()

	if !loaded {
		return fmt.Errorf("config: Reload called before Load")
	}

	val, err := c.loadFrom(s)
	if err != nil {
		return err
	}

	c.mu.Lock()
	old := c.current
	c.current = val
	c.mu.Unlock()

	c.notify(old, val)
	return nil
}

// Watch registers a callback that fires on every successful Load or Reload.
// Returns a cancel function to unregister.
func (c *Cfg[T]) Watch(fn func(old, new T)) (cancel func()) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.watchers = append(c.watchers, fn)
	idx := len(c.watchers) - 1

	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if idx < len(c.watchers) {
			c.watchers[idx] = nil
		}
	}
}

// Loaded returns whether Load has been called successfully.
func (c *Cfg[T]) Loaded() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.loaded
}

// LoadRootFile loads a YAML configuration file relative to the project
// root directory. See GetProjectRootDir for how the root is determined.
//
// Example:
//
//	cfg.LoadRootFile("conf/config.yaml")
func (c *Cfg[T]) LoadRootFile(relPath string) error {
	root, err := GetProjectRootDir()
	if err != nil {
		return fmt.Errorf("config: LoadRootFile: %w", err)
	}
	return c.Load(&rootFileSource{path: filepath.Join(root, relPath)})
}

// rootFileSource is an internal Source that reads a file from an
// absolute path. Used by LoadRootFile.
type rootFileSource struct {
	path string
}

func (s *rootFileSource) Load() ([]byte, error) {
	return os.ReadFile(s.path)
}

func (c *Cfg[T]) loadFrom(s Source) (T, error) {
	var zero T

	raw, err := s.Load()
	if err != nil {
		return zero, fmt.Errorf("config: load: %w", err)
	}

	c.mu.RLock()
	templates := c.templates
	c.mu.RUnlock()

	if len(templates) > 0 {
		raw, err = renderRaw(raw, templates)
		if err != nil {
			return zero, fmt.Errorf("config: template render: %w", err)
		}
	}

	var val T
	if err := yaml.Unmarshal(raw, &val); err != nil {
		return zero, fmt.Errorf("config: decode: %w", err)
	}
	return val, nil
}

func (c *Cfg[T]) notify(old, new T) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, w := range c.watchers {
		if w != nil {
			w(old, new)
		}
	}
}

// renderRaw renders Go template syntax {{.Var}} in raw config bytes.
func renderRaw(raw []byte, vars map[string]string) ([]byte, error) {
	tpl, err := template.New("cfg").
		Option("missingkey=zero").
		Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("template parse: %w", err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("template execute: %w", err)
	}
	return buf.Bytes(), nil
}

// GetProjectRootDir determines the project root directory using a layered
// strategy that works across development and production deployments:
//
//  1. Walk upward from the executable's directory, looking for a root marker
//     file (.root or go.mod)
//  2. Walk upward from the working directory, looking for a root marker file
//  3. Fall back to the executable's directory itself
func GetProjectRootDir() (string, error) {
	// Strategy 1: walk up from executable directory
	if exe, err := os.Executable(); err == nil {
		if root := findRootDir(filepath.Dir(exe)); root != "" {
			return root, nil
		}
	}

	// Strategy 2: walk up from working directory
	if wd, err := os.Getwd(); err == nil {
		if root := findRootDir(wd); root != "" {
			return root, nil
		}
	}

	// Strategy 3: last resort — executable directory
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe), nil
	}

	return "", fmt.Errorf("config: cannot determine project root; place a .root marker file")
}

// findRootDir walks up from dir looking for a root marker file.
func findRootDir(dir string) string {
	for {
		for _, marker := range rootMarkers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
