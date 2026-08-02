package btask

import (
	"context"
	"testing"
	"time"
)

func TestEnsureContext(t *testing.T) {
	tests := []struct {
		name string
		in   context.Context
		want string // tag: expect non-nil context equivalent to Background
	}{
		{name: "nil", in: nil, want: "background"},
		{name: "background", in: context.Background(), want: "background"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnsureContext(tt.in)
			if tt.want == "background" && got != context.Background() {
				if got.Err() != nil {
					t.Fatal(got.Err())
				}
			}
		})
	}
}

func TestWithOptionalTimeout(t *testing.T) {
	tests := []struct {
		name      string
		d         time.Duration
		wantFires bool
	}{
		{name: "zero", d: 0, wantFires: false},
		{name: "negative", d: -time.Second, wantFires: false},
		{name: "short", d: 20 * time.Millisecond, wantFires: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := WithOptionalTimeout(context.Background(), tt.d)
			defer cancel()

			select {
			case <-ctx.Done():
				if !tt.wantFires {
					t.Fatal("unexpected Done")
				}
			case <-time.After(80 * time.Millisecond):
				if tt.wantFires {
					t.Fatal("expected Done")
				}
			}
		})
	}
}

func TestMergeTimeout_respectsParentDeadline(t *testing.T) {
	parent, stop := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer stop()

	ctx, cancel := MergeTimeout(parent, time.Minute)
	defer cancel()

	select {
	case <-ctx.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected parent deadline to win")
	}
}
