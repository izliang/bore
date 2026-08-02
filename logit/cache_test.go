package logit

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func resetAppNameCache() {
	appNameCache = nil
}

func TestTimeCache_appendFormattedTimestamp(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, c *TimeCache)
	}{
		{
			name: "two_calls_same_second_hits_cached_prefix",
			run: func(t *testing.T, c *TimeCache) {
				var b1, b2 bytes.Buffer
				c.appendFormattedTimestamp(&b1)
				c.appendFormattedTimestamp(&b2)
				if b1.Len() < 10 || b2.Len() < 10 {
					t.Fatalf("short buffers: %q %q", b1.String(), b2.String())
				}
			},
		},
		{
			name: "forced_new_unix_second_refreshes_prefix",
			run: func(t *testing.T, c *TimeCache) {
				var buf bytes.Buffer
				c.appendFormattedTimestamp(&buf)
				c.mu.Lock()
				c.lastUnix = 0
				c.mu.Unlock()
				buf.Reset()
				c.appendFormattedTimestamp(&buf)
				if buf.Len() < 10 {
					t.Fatalf("short buffer: %q", buf.String())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.run(t, &TimeCache{})
		})
	}
}

func TestCallerSkipPrefixesMatch(t *testing.T) {
	tests := []struct {
		name  string
		fn    string
		items []CallerSkipConfigItem
		want  bool
	}{
		// Prefix mode
		{name: "prefix_exact", fn: "example.com/wrapper", items: []CallerSkipConfigItem{{Value: "example.com/wrapper", Mode: CallerSkipPrefix}}, want: true},
		{name: "prefix_dot", fn: "example.com/wrapper/log.Info", items: []CallerSkipConfigItem{{Value: "example.com/wrapper", Mode: CallerSkipPrefix}}, want: true},
		{name: "prefix_slash_subpkg", fn: "example.com/wrapper/sub/pkg.Foo", items: []CallerSkipConfigItem{{Value: "example.com/wrapper", Mode: CallerSkipPrefix}}, want: true},
		{name: "prefix_no_boundary_false", fn: "example.com/wrapperextra.Foo", items: []CallerSkipConfigItem{{Value: "example.com/wrapper", Mode: CallerSkipPrefix}}, want: false},
		{name: "prefix_unrelated_false", fn: "example.com/pkg.Foo", items: []CallerSkipConfigItem{{Value: "example.com/wrapper", Mode: CallerSkipPrefix}}, want: false},
		{name: "prefix_empty_false", fn: "main.main", items: []CallerSkipConfigItem{{Value: "", Mode: CallerSkipPrefix}}, want: false},
		// Contains mode
		{name: "contains_simple", fn: "example.com/wrapper/log.Info", items: []CallerSkipConfigItem{{Value: "wrapper", Mode: CallerSkipContains}}, want: true},
		{name: "contains_mid_pkg", fn: "github.com/bore.git/logit.baseLogger.Info", items: []CallerSkipConfigItem{{Value: "logit", Mode: CallerSkipContains}}, want: true},
		{name: "contains_short_name", fn: "gorm.io/gorm/logger.logger.Trace", items: []CallerSkipConfigItem{{Value: "gorm", Mode: CallerSkipContains}}, want: true},
		{name: "contains_no_match_false", fn: "example.com/pkg.Foo", items: []CallerSkipConfigItem{{Value: "wrapper", Mode: CallerSkipContains}}, want: false},
		{name: "contains_empty_false", fn: "main.main", items: []CallerSkipConfigItem{{Value: "", Mode: CallerSkipContains}}, want: false},
		// Mixed items
		{name: "mixed_first_prefix", fn: "example.com/a.Foo", items: []CallerSkipConfigItem{{Value: "example.com/a", Mode: CallerSkipPrefix}, {Value: "b", Mode: CallerSkipContains}}, want: true},
		{name: "mixed_second_contains", fn: "x/b/y.Foo", items: []CallerSkipConfigItem{{Value: "example.com/a", Mode: CallerSkipPrefix}, {Value: "b", Mode: CallerSkipContains}}, want: true},
		{name: "mixed_neither_false", fn: "example.com/c.Foo", items: []CallerSkipConfigItem{{Value: "example.com/a", Mode: CallerSkipPrefix}, {Value: "b", Mode: CallerSkipContains}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := callerSkipPrefixesMatch(tt.fn, tt.items); got != tt.want {
				t.Errorf("callerSkipPrefixesMatch(%q, %v) = %v want %v", tt.fn, tt.items, got, tt.want)
			}
		})
	}
}

func TestFingerprintCallerPrefixes_ModeSeparation(t *testing.T) {
	// Same values but different modes → different fingerprints.
	itemsPrefix := []CallerSkipConfigItem{{Value: "a", Mode: CallerSkipPrefix}, {Value: "b", Mode: CallerSkipPrefix}}
	itemsContains := []CallerSkipConfigItem{{Value: "a", Mode: CallerSkipContains}, {Value: "b", Mode: CallerSkipContains}}
	fp1 := fingerprintCallerPrefixes(itemsPrefix)
	fp2 := fingerprintCallerPrefixes(itemsContains)
	if fp1 == fp2 {
		t.Fatal("different modes must produce different fingerprints")
	}
	// Mode per item matters: same values, different mode distribution → different fingerprints.
	itemsMixed := []CallerSkipConfigItem{{Value: "a", Mode: CallerSkipPrefix}, {Value: "b", Mode: CallerSkipContains}}
	fp3 := fingerprintCallerPrefixes(itemsMixed)
	if fp1 == fp3 {
		t.Fatal("mixed modes must produce different fingerprint from all-prefix")
	}
}

func TestAppendCaller_invalidSkip(t *testing.T) {
	var buf bytes.Buffer
	appendCaller(&buf, 1<<20, nil, 0, CallerShort)
	if got := buf.String(); got != "unknown:0" {
		t.Fatalf("got %q want unknown:0", got)
	}
}

func TestAppendCaller_okAndCacheHit(t *testing.T) {
	for range 2 {
		var buf bytes.Buffer
		appendCaller(&buf, 0, nil, 0, CallerShort)
		s := buf.String()
		if !strings.Contains(s, ":") {
			t.Fatalf("expected file:line, got %q", s)
		}
	}
}

func TestAppendCaller_Formats(t *testing.T) {
	tests := []struct {
		name   string
		format CallerFormat
		want   func(s string) bool
	}{
		{
			name:   "short",
			format: CallerShort,
			want: func(s string) bool {
				return strings.Contains(s, "logit/cache.go:")
			},
		},
		{
			name:   "full",
			format: CallerFull,
			want: func(s string) bool {
				return strings.HasPrefix(s, "/") && strings.Contains(s, "logit/cache.go:")
			},
		},
		{
			name:   "func",
			format: CallerFunc,
			want: func(s string) bool {
				return strings.Contains(s, "logit.") && strings.Contains(s, ":")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			appendCaller(&buf, 0, nil, 0, tt.format)
			s := buf.String()
			if !tt.want(s) {
				t.Fatalf("format %d: got %q, unexpected", tt.format, s)
			}
		})
	}
}

func TestGetAppName(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		setEnv   bool
		wantSub  string
	}{
		{name: "default_when_empty", setEnv: true, envValue: "", wantSub: DefaultPodName},
		{name: "from_env", setEnv: true, envValue: "MyService", wantSub: "MyService"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetAppNameCache()
			t.Cleanup(func() {
				resetAppNameCache()
				_ = os.Unsetenv("APP_NAME")
			})
			if tt.setEnv {
				if err := os.Setenv("APP_NAME", tt.envValue); err != nil {
					t.Fatal(err)
				}
			}
			got := string(getAppName())
			if !strings.Contains(got, tt.wantSub) {
				t.Fatalf("got %q want substring %q", got, tt.wantSub)
			}
			// cached path
			got2 := string(getAppName())
			if got2 != got {
				t.Fatalf("cache mismatch %q vs %q", got, got2)
			}
		})
	}
}
