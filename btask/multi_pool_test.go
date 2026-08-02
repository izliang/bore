package btask

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewMultiPool_errors(t *testing.T) {
	tests := []struct {
		name    string
		windows []Window
		wantErr bool
	}{
		{name: "empty", windows: nil, wantErr: true},
		{name: "dup_name", windows: []Window{{Name: "a", Parallel: 1}, {Name: "a", Parallel: 2}}, wantErr: true},
		{name: "empty_name", windows: []Window{{Name: "", Parallel: 1}}, wantErr: true},
		{name: "ok", windows: []Window{{Name: WindowFast, Parallel: 2}, {Name: WindowSlow, Parallel: 1}}, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewMultiPool(tt.windows)
			if tt.wantErr && err == nil {
				t.Fatal("want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMultiPool_Go_GoCtx_Wait(t *testing.T) {
	tests := []struct {
		name     string
		fastN    int
		slowN    int
		parallel int
	}{
		{name: "small", fastN: 5, slowN: 3, parallel: 2},
		{name: "more", fastN: 20, slowN: 10, parallel: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewMultiPool([]Window{
				{Name: WindowFast, Parallel: tt.parallel},
				{Name: WindowSlow, Parallel: 1},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer m.Shutdown()

			var fastRan, slowRan atomic.Int32
			for range tt.fastN {
				_ = m.Go(context.Background(), WindowFast, func() { fastRan.Add(1) })
			}
			for range tt.slowN {
				_ = m.GoCtx(context.Background(), WindowSlow, func(ctx context.Context) {
					_ = ctx
					slowRan.Add(1)
				})
			}
			m.Wait()
			if int(fastRan.Load()) != tt.fastN {
				t.Fatalf("fast: got %d want %d", fastRan.Load(), tt.fastN)
			}
			if int(slowRan.Load()) != tt.slowN {
				t.Fatalf("slow: got %d want %d", slowRan.Load(), tt.slowN)
			}
		})
	}
}

func TestMultiPool_unknownWindow(t *testing.T) {
	m := MustNewMultiPool([]Window{{Name: "only", Parallel: 1}})
	defer m.Shutdown()

	err := m.Go(context.Background(), "nope", func() {})
	if err == nil || err.Error() == "" {
		t.Fatal("want unknown window error")
	}
	if m.Pool("nope") != nil {
		t.Fatal("Pool should be nil")
	}
	_, ok := m.Window("only")
	if !ok {
		t.Fatal("want window only")
	}
}

func TestMultiPool_GoCtx_defaultTimeout(t *testing.T) {
	m := MustNewMultiPool([]Window{
		{Name: "tight", Parallel: 2, DefaultTimeout: 25 * time.Millisecond},
	})
	defer m.Shutdown()

	var sawDeadline atomic.Bool
	_ = m.GoCtx(context.Background(), "tight", func(ctx context.Context) {
		<-ctx.Done()
		sawDeadline.Store(true)
	})
	m.Wait()
	if !sawDeadline.Load() {
		t.Fatal("expected ctx cancelled by window timeout")
	}
}

func TestMultiPool_Go_cancelledParent(t *testing.T) {
	m := MustNewMultiPool([]Window{{Name: WindowFast, Parallel: 1}})
	defer m.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := m.Go(ctx, WindowFast, func() {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want Canceled, got %v", err)
	}
}

func TestMultiPool_Shutdown_rejectsGo(t *testing.T) {
	m := MustNewMultiPool([]Window{{Name: WindowFast, Parallel: 1}})
	_ = m.Go(context.Background(), WindowFast, func() {})
	m.Shutdown()

	err := m.Go(context.Background(), WindowFast, func() {})
	if err == nil {
		t.Fatal("want error after shutdown")
	}
}

func TestMultiPool_Names_order(t *testing.T) {
	m := MustNewMultiPool([]Window{
		{Name: "z", Parallel: 1},
		{Name: "a", Parallel: 1},
	})
	defer m.Shutdown()
	names := m.Names()
	if len(names) != 2 || names[0] != "z" || names[1] != "a" {
		t.Fatalf("names: %v", names)
	}
}

func TestMultiPool_Stats(t *testing.T) {
	m := MustNewMultiPool([]Window{
		{Name: "z", Parallel: 2, DefaultTimeout: time.Second},
		{Name: "a", Parallel: 1},
	})
	defer m.Shutdown()

	_ = m.Go(context.Background(), "z", func() {})
	_ = m.Go(context.Background(), "z", func() {})
	_ = m.Go(context.Background(), "a", func() {})
	m.Wait()

	stats := m.Stats()
	if len(stats) != 2 {
		t.Fatalf("stats len = %d, want 2", len(stats))
	}
	if stats[0].Window.Name != "z" || stats[1].Window.Name != "a" {
		t.Fatalf("stats order = %#v", stats)
	}
	if stats[0].Window.DefaultTimeout != time.Second || stats[0].Stats.Parallel != 2 {
		t.Fatalf("first window stats = %+v", stats[0])
	}
	if stats[0].Stats.Submitted != 2 || stats[0].Stats.Completed != 2 {
		t.Fatalf("z stats = %+v", stats[0].Stats)
	}
	if stats[1].Stats.Submitted != 1 || stats[1].Stats.Completed != 1 {
		t.Fatalf("a stats = %+v", stats[1].Stats)
	}
}
