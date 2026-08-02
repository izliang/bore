package lctx

import (
	"context"
	"testing"
)

type fmtPairs []Pair

func (f fmtPairs) Values() []Pair { return f }

type emptyFmt struct{}

func (emptyFmt) Values() []Pair { return nil }

func TestPushPair_table(t *testing.T) {
	base := context.Background()

	tests := []struct {
		name   string
		ctx    context.Context
		lfs    []LogFormatter
		sameAs context.Context
	}{
		{
			name:   "no_formatters",
			ctx:    base,
			lfs:    nil,
			sameAs: base,
		},
		{
			name:   "empty_values_formatter",
			ctx:    base,
			lfs:    []LogFormatter{emptyFmt{}},
			sameAs: base,
		},
		{
			name: "one_batch",
			ctx:  base,
			lfs:  []LogFormatter{fmtPairs{{Key: "a", Value: "1"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PushPair(tt.ctx, tt.lfs...)
			if tt.sameAs != nil && got != tt.sameAs {
				t.Fatalf("expected same context pointer")
			}
			if tt.sameAs == nil {
				ps := ListPair(got)
				if len(ps) == 0 {
					t.Fatal("expected pairs")
				}
			}
		})
	}
}

func TestPushPair_chain_and_ListPair(t *testing.T) {
	ctx := context.Background()
	ctx1 := PushPair(ctx, fmtPairs{{Key: "rid", Value: "r1"}})
	ctx2 := PushPair(ctx1, fmtPairs{{Key: "tid", Value: "t2"}})

	tests := []struct {
		name string
		ctx  context.Context
		want []Pair
	}{
		{
			name: "first",
			ctx:  ctx1,
			want: []Pair{{Key: "rid", Value: "r1"}},
		},
		{
			name: "chained",
			ctx:  ctx2,
			want: []Pair{{Key: "rid", Value: "r1"}, {Key: "tid", Value: "t2"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ListPair(tt.ctx)
			if len(got) != len(tt.want) {
				t.Fatalf("len %d want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("idx %d got %+v want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestListPair_nilOrEmpty(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{name: "background", ctx: context.Background()},
		{name: "after_empty_push", ctx: PushPair(context.Background(), emptyFmt{})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if p := ListPair(tt.ctx); p != nil {
				t.Fatalf("want nil, got %+v", p)
			}
		})
	}
}
