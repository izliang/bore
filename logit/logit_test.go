package logit

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/izliang/bore/logit/lctx"
)

func TestMergeConfig(t *testing.T) {
	tests := []struct {
		name       string
		in         *Config
		wantLevel  Level
		wantCaller bool
	}{
		{name: "nil_defaults", in: nil, wantLevel: Debug, wantCaller: true},
		{name: "override_level_only", in: &Config{Level: Warn}, wantLevel: Warn, wantCaller: false},
		{name: "level_and_caller", in: &Config{Level: Info}, wantLevel: Info, wantCaller: false},
		{name: "empty_level_keeps_default", in: &Config{}, wantLevel: Debug, wantCaller: false},
		{name: "unknown_config_level", in: &Config{Level: "CUSTOM"}, wantLevel: "CUSTOM", wantCaller: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeConfig(tt.in)
			if got.Level != tt.wantLevel {
				t.Fatalf("mergeConfig(%v) = %+v want Level=%q ShowCaller=%v", tt.in, got, tt.wantLevel, tt.wantCaller)
			}
		})
	}
}

func TestMergeConfig_callerSkip(t *testing.T) {
	t.Run("nil_config_has_builtin_items", func(t *testing.T) {
		got := mergeConfig(nil)
		if len(got.CallerSkip.List) != 2 {
			t.Fatalf("want 2 builtin items, got %d: %v", len(got.CallerSkip.List), got.CallerSkip.List)
		}
		if got.CallerSkip.List[0].Value != PackageImportPath || got.CallerSkip.List[0].Mode != CallerSkipPrefix {
			t.Fatalf("builtin[0] = %+v want {PackageImportPath, CallerSkipPrefix}", got.CallerSkip.List[0])
		}
		if got.CallerSkip.List[1].Value != AppCtxImportPath || got.CallerSkip.List[1].Mode != CallerSkipPrefix {
			t.Fatalf("builtin[1] = %+v want {AppCtxImportPath, CallerSkipPrefix}", got.CallerSkip.List[1])
		}
		if got.callerSkipFP != fingerprintCallerPrefixes(got.CallerSkip.List) {
			t.Fatal("callerSkipFP out of sync")
		}
		if got.CallerSkip.Format != CallerShort {
			t.Fatalf("default format = %v want CallerShort", got.CallerSkip.Format)
		}
	})

	t.Run("user_items_merged_after_builtin", func(t *testing.T) {
		got := mergeConfig(&Config{
			CallerSkip: CallerSkipConfig{
				List: []CallerSkipConfigItem{
					{Value: "example.com/mywrap", Mode: CallerSkipContains},
					{Value: "example.com/other", Mode: CallerSkipPrefix},
				},
			},
		})
		if got.CallerSkip.List[0].Value != PackageImportPath {
			t.Fatalf("builtin must be first: %v", got.CallerSkip.List)
		}
		var seenWrap, seenOther bool
		for _, item := range got.CallerSkip.List {
			switch item.Value {
			case "example.com/mywrap":
				seenWrap = true
			case "example.com/other":
				seenOther = true
			}
		}
		if !seenWrap || !seenOther {
			t.Fatalf("missing user items: %v", got.CallerSkip.List)
		}
	})

	t.Run("dedup_user_repeat_builtin", func(t *testing.T) {
		got := mergeConfig(&Config{
			CallerSkip: CallerSkipConfig{
				List: []CallerSkipConfigItem{
					{Value: PackageImportPath, Mode: CallerSkipPrefix},
					{Value: "z", Mode: CallerSkipContains},
				},
			},
		})
		n := 0
		for _, item := range got.CallerSkip.List {
			if item.Value == PackageImportPath {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("duplicate builtin: %v", got.CallerSkip.List)
		}
		var seenZ bool
		for _, item := range got.CallerSkip.List {
			if item.Value == "z" {
				seenZ = true
			}
		}
		if !seenZ {
			t.Fatalf("missing user item 'z': %v", got.CallerSkip.List)
		}
	})

	t.Run("format_preserved", func(t *testing.T) {
		got := mergeConfig(&Config{
			CallerSkip: CallerSkipConfig{Format: CallerFunc},
		})
		if got.CallerSkip.Format != CallerFunc {
			t.Fatalf("format = %v want CallerFunc", got.CallerSkip.Format)
		}
	})

	t.Run("zero_value_still_has_builtins", func(t *testing.T) {
		got := mergeConfig(&Config{Level: Warn})
		if len(got.CallerSkip.List) != 2 {
			t.Fatalf("want 2 builtin items, got %d", len(got.CallerSkip.List))
		}
	})
}

type fmtPairs []lctx.Pair

func (f fmtPairs) Values() []lctx.Pair { return f }

func TestSimpleLogger_table(t *testing.T) {
	ctx := context.Background()
	errBoom := errors.New("boom")
	ctxTagged := lctx.PushPair(ctx, fmtPairs{{Key: "rid", Value: "r1"}})
	ctxChain := lctx.PushPair(ctxTagged, fmtPairs{{Key: "tid", Value: "t2"}})

	tests := []struct {
		name        string
		cfg         *Config
		act         func(Logger)
		wantEmpty   bool
		wantSubs    []string
		wantNotSubs []string
	}{
		{
			name:      "info_dropped_when_min_warn",
			cfg:       &Config{Level: Warn},
			act:       func(l Logger) { l.Info(ctx, "hidden") },
			wantEmpty: true,
		},
		{
			name:      "debug_dropped_when_min_info",
			cfg:       &Config{Level: Info},
			act:       func(l Logger) { l.Debug(ctx, "hidden") },
			wantEmpty: true,
		},
		{
			name:     "warn_kept_at_warn",
			cfg:      &Config{Level: Warn},
			act:      func(l Logger) { l.Warn(ctx, "seen") },
			wantSubs: []string{"WARN", "msg[seen]"},
		},
		{
			name:     "info_with_fields_and_nil_field_func",
			cfg:      &Config{Level: Debug},
			act:      func(l Logger) { l.Info(ctx, "hi", nil, StringField("k", "v")) },
			wantSubs: []string{"INFO", "msg[hi]", "k[v]"},
		},
		{
			name:     "lctx_pairs_in_line",
			cfg:      &Config{Level: Info},
			act:      func(l Logger) { l.Info(ctxTagged, "withctx") },
			wantSubs: []string{"rid[r1]", "msg[withctx]"},
		},
		{
			name:     "lctx_chained_pairs",
			cfg:      &Config{Level: Info},
			act:      func(l Logger) { l.Info(ctxChain, "chained") },
			wantSubs: []string{"rid[r1]", "tid[t2]", "msg[chained]"},
		},
		{
			name:      "error_nil_skipped",
			cfg:       &Config{Level: Error},
			act:       func(l Logger) { l.Error(ctx, nil) },
			wantEmpty: true,
		},
		{
			name:     "error_msg_empty_uses_err_text",
			cfg:      &Config{Level: Error},
			act:      func(l Logger) { l.Error(ctx, errBoom) },
			wantSubs: []string{"ERROR", "msg[] error[boom"},
		},
		{
			name:     "error_custom_msg_adds_error_field",
			cfg:      &Config{Level: Error},
			act:      func(l Logger) { l.Error(ctx, errBoom) },
			wantSubs: []string{"ERROR", "msg[]", "error[boom]"},
		},
		{
			name:      "info_dropped_when_min_error",
			cfg:       &Config{Level: Error},
			act:       func(l Logger) { l.Info(ctx, "nope") },
			wantEmpty: true,
		},
		{
			name:     "error_kept_when_min_error",
			cfg:      &Config{Level: Error},
			act:      func(l Logger) { l.Error(ctx, errBoom) },
			wantSubs: []string{"ERROR", "error[boom]"},
		},
		{
			name:     "caller_in_output_by_default",
			cfg:      &Config{Level: Info},
			act:      func(l Logger) { l.Info(ctx, "plain") },
			wantSubs: []string{"msg[plain]", "[Thread]", ".go:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := NewSimpleWithConfig(&buf, tt.cfg)
			tt.act(log)
			out := buf.String()
			if tt.wantEmpty {
				if out != "" {
					t.Fatalf("want no output, got %q", out)
				}
				return
			}
			for _, s := range tt.wantSubs {
				if !strings.Contains(out, s) {
					t.Fatalf("output %q missing %q", out, s)
				}
			}
			for _, s := range tt.wantNotSubs {
				if strings.Contains(out, s) {
					t.Fatalf("output %q should not contain %q", out, s)
				}
			}
		})
	}
}

func TestWithLevel_SimpleLoggerScopedDebug(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	base := NewSimpleWithConfig(&buf, &Config{Level: Info})

	base.Debug(ctx, "hidden")
	if got := buf.String(); got != "" {
		t.Fatalf("base logger should drop debug, got %q", got)
	}

	debugLogger := WithLevel(base, Debug)
	debugLogger.Debug(ctx, "visible")
	if got := buf.String(); !strings.Contains(got, "DEBUG") || !strings.Contains(got, "msg[visible]") {
		t.Fatalf("scoped debug logger should emit debug, got %q", got)
	}

	buf.Reset()
	base.Debug(ctx, "still_hidden")
	if got := buf.String(); got != "" {
		t.Fatalf("base logger should remain info level, got %q", got)
	}
}

func TestNewSimple_nilWriterUsesDiscard(t *testing.T) {
	l := NewSimple(nil)
	if l == nil {
		t.Fatal("nil logger")
	}
	ctx := context.Background()
	l.Info(ctx, "to_discard", StringField("a", "b")) // must not panic
}

func TestSimpleLogger_Write_nilUnderlying(t *testing.T) {
	var buf bytes.Buffer
	log := NewSimpleWithConfig(&buf, &Config{Level: Info})
	sl := log.(*SimpleLogger)
	sl.w = nil
	n, err := sl.Write([]byte("raw"))
	if err != nil || n != 0 {
		t.Fatalf("Write = (%d, %v) want (0, nil)", n, err)
	}
}

func TestSimpleLogger_Write_forwards(t *testing.T) {
	var buf bytes.Buffer
	log := NewSimpleWithConfig(&buf, nil)
	n, err := log.Write([]byte("ping"))
	if err != nil || n != 4 {
		t.Fatalf("Write = (%d, %v)", n, err)
	}
	if got := buf.String(); got != "ping" {
		t.Fatalf("got %q", got)
	}
}
