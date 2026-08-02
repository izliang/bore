package source

import (
	"fmt"
	"os"
	"strings"

	"github.com/izliang/bore/config"
)

// Env creates a config.Source that reads from environment variables.
// Raw output is a synthetic YAML document built from matching env vars.
//
// keyPrefix filters which env vars to include (e.g. "APP_").
// delim controls the key separator (e.g. "_" maps APP_DB_HOST → db.host).
//
// Example:
//
//	source.Env("MYAPP_", "_")  →  MYAPP_DB_HOST=localhost  →  db.host: localhost
func Env(keyPrefix, delim string) config.Source {
	return &envSource{
		prefix: keyPrefix,
		delim:  delim,
	}
}

type envSource struct {
	prefix string
	delim  string
}

func (s *envSource) Load() ([]byte, error) {
	if s.delim == "" {
		s.delim = "_"
	}

	pairs := make(map[string]string)
	prefixLower := strings.ToLower(s.prefix)

	for _, e := range os.Environ() {
		kv := strings.SplitN(e, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := kv[0]
		val := kv[1]

		lowerKey := strings.ToLower(key)
		if !strings.HasPrefix(lowerKey, prefixLower) {
			continue
		}

		stripped := strings.TrimPrefix(lowerKey, prefixLower)
		configKey := strings.ReplaceAll(stripped, s.delim, ".")
		pairs[configKey] = val
	}

	if len(pairs) == 0 {
		return []byte{}, nil
	}

	return buildYAML(pairs), nil
}

func buildYAML(pairs map[string]string) []byte {
	root := make(map[string]any)

	for key, val := range pairs {
		parts := strings.Split(key, ".")
		insertIntoTree(root, parts, val)
	}

	return marshalTree(root, 0)
}

func insertIntoTree(node map[string]any, parts []string, val string) {
	if len(parts) == 1 {
		node[parts[0]] = val
		return
	}

	child, ok := node[parts[0]]
	if !ok {
		child = make(map[string]any)
		node[parts[0]] = child
	}

	childMap, ok := child.(map[string]any)
	if !ok {
		childMap = make(map[string]any)
		node[parts[0]] = childMap
	}

	insertIntoTree(childMap, parts[1:], val)
}

func marshalTree(node map[string]any, indent int) []byte {
	var sb strings.Builder
	prefix := strings.Repeat("  ", indent)

	keys := make([]string, 0, len(node))
	for k := range node {
		keys = append(keys, k)
	}
	sortStrings(keys)

	for _, k := range keys {
		v := node[k]
		sb.WriteString(prefix)
		sb.WriteString(k)
		sb.WriteString(":")

		switch child := v.(type) {
		case map[string]any:
			sb.WriteString("\n")
			sb.Write(marshalTree(child, indent+1))
		case string:
			sb.WriteString(" ")
			if needsQuoting(child) {
				fmt.Fprintf(&sb, "%q", child)
			} else {
				sb.WriteString(child)
			}
			sb.WriteString("\n")
		default:
			fmt.Fprintf(&sb, " %v\n", child)
		}
	}

	return []byte(sb.String())
}

func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		switch r {
		case ':', '#', '{', '}', '[', ']', ',', '&', '*', '?', '|', '-', '<', '>', '=', '!', '%', '@', '`', '"', '\'':
			return true
		}
	}
	if s[0] == ' ' || s[len(s)-1] == ' ' {
		return true
	}
	lower := strings.ToLower(s)
	if lower == "true" || lower == "false" || lower == "yes" || lower == "no" || lower == "null" || lower == "~" {
		return true
	}
	return false
}

func sortStrings(ss []string) {
	for i := 0; i < len(ss); i++ {
		for j := i + 1; j < len(ss); j++ {
			if ss[i] > ss[j] {
				ss[i], ss[j] = ss[j], ss[i]
			}
		}
	}
}
