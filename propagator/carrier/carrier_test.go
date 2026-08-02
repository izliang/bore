package carrier

import (
	"net/http"
	"testing"

	"google.golang.org/grpc/metadata"
)

// ──────────────────────────── HTTPHeader ────────────────────────────

func TestHTTPHeader_GetSetKeys(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(h *HTTPHeader)
		getKey   string
		wantVal  string
		wantKeys []string
	}{
		{
			name:    "空 header",
			setup:   func(h *HTTPHeader) {},
			getKey:  "X-Any",
			wantVal: "",
			wantKeys: nil,
		},
		{
			name: "单 key 读写",
			setup: func(h *HTTPHeader) {
				h.Set("X-Request-Id", "req-123")
			},
			getKey:  "X-Request-Id",
			wantVal: "req-123",
			wantKeys: []string{"X-Request-Id"},
		},
		{
			name: "覆盖写入",
			setup: func(h *HTTPHeader) {
				h.Set("X-Key", "old")
				h.Set("X-Key", "new")
			},
			getKey:   "X-Key",
			wantVal:  "new",
			wantKeys: []string{"X-Key"},
		},
		{
			name: "多个 key",
			setup: func(h *HTTPHeader) {
				h.Set("X-A", "1")
				h.Set("X-B", "2")
				h.Set("X-C", "3")
			},
			getKey:  "X-B",
			wantVal: "2",
		},
		{
			name: "http.Header 大小写不敏感读",
			setup: func(h *HTTPHeader) {
				h.Header.Set("X-Custom-Header", "value")
			},
			getKey:  "x-custom-header",
			wantVal: "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &HTTPHeader{Header: http.Header{}}
			tt.setup(h)

			got := h.Get(tt.getKey)
			if got != tt.wantVal {
				t.Errorf("Get(%q) = %q, want %q", tt.getKey, got, tt.wantVal)
			}

			if tt.wantKeys != nil {
				keys := h.Keys()
				if !stringSetEqual(keys, tt.wantKeys) {
					t.Errorf("Keys() = %v, want %v", keys, tt.wantKeys)
				}
			}
		})
	}
}

// ──────────────────────────── GRPCMetadata ──────────────────────────

func TestGRPCMetadata_GetSetKeys(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(g *GRPCMetadata)
		getKey  string
		wantVal string
	}{
		{
			name:    "空 metadata",
			setup:   func(g *GRPCMetadata) {},
			getKey:  "any",
			wantVal: "",
		},
		{
			name: "单 key 读写",
			setup: func(g *GRPCMetadata) {
				g.Set("x-request-id", "req-123")
			},
			getKey:  "x-request-id",
			wantVal: "req-123",
		},
		{
			name: "多值 key 取首个",
			setup: func(g *GRPCMetadata) {
				g.MD.Append("x-key", "v1")
				g.MD.Append("x-key", "v2")
			},
			getKey:  "x-key",
			wantVal: "v1",
		},
		{
			name: "覆盖写入",
			setup: func(g *GRPCMetadata) {
				g.Set("x-key", "old")
				g.Set("x-key", "new")
			},
			getKey:  "x-key",
			wantVal: "new",
		},
		{
			name: "多个 key",
			setup: func(g *GRPCMetadata) {
				g.Set("a", "1")
				g.Set("b", "2")
			},
			getKey:  "b",
			wantVal: "2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GRPCMetadata{MD: metadata.New(nil)}
			tt.setup(g)

			got := g.Get(tt.getKey)
			if got != tt.wantVal {
				t.Errorf("Get(%q) = %q, want %q", tt.getKey, got, tt.wantVal)
			}
		})
	}
}

func TestGRPCMetadata_Keys(t *testing.T) {
	g := &GRPCMetadata{MD: metadata.New(nil)}
	g.Set("a", "1")
	g.Set("b", "2")
	g.Set("c", "3")

	keys := g.Keys()
	if len(keys) != 3 {
		t.Fatalf("Keys() len = %d, want 3", len(keys))
	}
	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !keySet[want] {
			t.Errorf("Keys() missing %q", want)
		}
	}
}

// ──────────────────────────── MQHeaders ─────────────────────────────

func TestMQHeaders_GetSetKeys(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(m *MQHeaders)
		getKey  string
		wantVal string
	}{
		{
			name:    "空 headers",
			setup:   func(m *MQHeaders) {},
			getKey:  "any",
			wantVal: "",
		},
		{
			name: "单 key 读写",
			setup: func(m *MQHeaders) {
				m.Set("x-request-id", "req-123")
			},
			getKey:  "x-request-id",
			wantVal: "req-123",
		},
		{
			name: "大小写不敏感读（精确匹配优先）",
			setup: func(m *MQHeaders) {
				m.Headers["X-Request-Id"] = "exact"
				m.Headers["x-request-id"] = "lower"
			},
			getKey:  "x-request-id",
			wantVal: "lower",
		},
		{
			name: "大小写不敏感读（仅有大写 key）",
			setup: func(m *MQHeaders) {
				m.Headers["X-Custom-Header"] = "value"
			},
			getKey:  "x-custom-header",
			wantVal: "value",
		},
		{
			name: "覆盖写入",
			setup: func(m *MQHeaders) {
				m.Set("key", "old")
				m.Set("key", "new")
			},
			getKey:  "key",
			wantVal: "new",
		},
		{
			name: "空值",
			setup: func(m *MQHeaders) {
				m.Headers["empty"] = ""
			},
			getKey:  "empty",
			wantVal: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MQHeaders{Headers: make(map[string]string)}
			tt.setup(m)

			got := m.Get(tt.getKey)
			if got != tt.wantVal {
				t.Errorf("Get(%q) = %q, want %q", tt.getKey, got, tt.wantVal)
			}
		})
	}
}

func TestMQHeaders_Keys(t *testing.T) {
	m := &MQHeaders{Headers: map[string]string{"a": "1", "b": "2"}}
	keys := m.Keys()
	if len(keys) != 2 {
		t.Fatalf("Keys() len = %d, want 2", len(keys))
	}
}

// ──────────────────────────── 辅助函数 ──────────────────────────────

func stringSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[string]bool, len(a))
	for _, s := range a {
		am[s] = true
	}
	for _, s := range b {
		if !am[s] {
			return false
		}
	}
	return true
}

func TestMQHeaders_GetFromNilMap(t *testing.T) {
	mq := &MQHeaders{Headers: nil}
	if got := mq.Get("any"); got != "" {
		t.Errorf("nil map Get = %q, want empty string", got)
	}
}
