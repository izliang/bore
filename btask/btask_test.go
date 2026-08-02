package btask

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewPool_parallelClamped(t *testing.T) {
	tests := []struct {
		name        string
		parallel    int
		wantWorkers int // effective worker count after NewPool normalization
	}{
		{name: "negative", parallel: -10, wantWorkers: 1},
		{name: "zero", parallel: 0, wantWorkers: 1},
		{name: "one", parallel: 1, wantWorkers: 1},
		{name: "four", parallel: 4, wantWorkers: 4},
		{name: "sixteen", parallel: 16, wantWorkers: 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPool(tt.parallel)
			defer p.Shutdown()

			hold := make(chan struct{})
			var started atomic.Int32

			// Submit exactly wantWorkers blocking tasks to avoid deadlock when parallel==1 and buffer==1.
			for range tt.wantWorkers {
				p.Go(func() {
					started.Add(1)
					<-hold
				})
			}

			deadline := time.Now().Add(2 * time.Second)
			for started.Load() < int32(tt.wantWorkers) {
				if time.Now().After(deadline) {
					t.Fatalf("want %d tasks started, got %d", tt.wantWorkers, started.Load())
				}
				time.Sleep(50 * time.Microsecond)
			}

			snap := started.Load()
			time.Sleep(30 * time.Millisecond)
			if started.Load() != snap {
				t.Fatalf("pool not capped at %d: started went %d -> %d", tt.wantWorkers, snap, started.Load())
			}

			close(hold)
			p.Wait()
		})
	}
}

func TestPool_workerLoop_nilJobSafe(t *testing.T) {
	tests := []struct {
		name string
		nilN int
	}{
		{name: "single", nilN: 1},
		{name: "several", nilN: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPool(2)
			defer p.Shutdown()

			for range tt.nilN {
				p.jobs <- nil
			}

			var ran atomic.Int32
			p.Go(func() { ran.Add(1) })
			p.Wait()
			if ran.Load() != 1 {
				t.Fatalf("want 1 task ran after nil jobs, got %d", ran.Load())
			}
		})
	}
}

func TestPool_Go_nilIgnored(t *testing.T) {
	tests := []struct {
		name   string
		nilOps int
	}{
		{name: "single_nil", nilOps: 1},
		{name: "many_nil", nilOps: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPool(2)
			defer p.Shutdown()

			for range tt.nilOps {
				p.Go(nil)
			}
			done := make(chan struct{})
			go func() {
				p.Wait()
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(500 * time.Millisecond):
				t.Fatal("Wait blocked: nil tasks must not increment taskWg")
			}

			stats := p.Stats()
			if stats.Submitted != 0 || stats.Completed != 0 || stats.Running != 0 || stats.Pending != 0 {
				t.Fatalf("nil tasks changed stats: %+v", stats)
			}
		})
	}
}

func TestPool_Stats_parallelAndQueue(t *testing.T) {
	tests := []struct {
		name     string
		parallel int
		want     int
	}{
		{name: "zero", parallel: 0, want: 1},
		{name: "negative", parallel: -1, want: 1},
		{name: "three", parallel: 3, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPool(tt.parallel)
			defer p.Shutdown()

			stats := p.Stats()
			if stats.Parallel != tt.want || stats.QueueCapacity != tt.want {
				t.Fatalf("stats = %+v, want parallel/capacity %d", stats, tt.want)
			}
		})
	}
}

func TestPool_Stats_runningQueuedAndCompleted(t *testing.T) {
	p := NewPool(1)
	defer p.Shutdown()

	started := make(chan struct{})
	release := make(chan struct{})
	p.Go(func() {
		close(started)
		<-release
	})
	<-started

	p.Go(func() {})

	stats := p.Stats()
	if stats.Running != 1 || stats.QueueLength != 1 || stats.Submitted != 2 || stats.Completed != 0 || stats.Pending != 2 {
		t.Fatalf("stats while blocked = %+v", stats)
	}

	close(release)
	p.Wait()
	stats = p.Stats()
	if stats.Running != 0 || stats.QueueLength != 0 || stats.Submitted != 2 || stats.Completed != 2 || stats.Pending != 0 {
		t.Fatalf("stats after wait = %+v", stats)
	}
}

func TestPool_GoWait_runsAllTasks(t *testing.T) {
	tests := []struct {
		name     string
		parallel int
		tasks    int
	}{
		{name: "p1_t10", parallel: 1, tasks: 10},
		{name: "p4_t40", parallel: 4, tasks: 40},
		{name: "p8_t100", parallel: 8, tasks: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPool(tt.parallel)
			defer p.Shutdown()

			var ran atomic.Int32
			for range tt.tasks {
				p.Go(func() {
					ran.Add(1)
				})
			}
			p.Wait()

			if got := int(ran.Load()); got != tt.tasks {
				t.Fatalf("ran %d tasks, want %d", got, tt.tasks)
			}
		})
	}
}

func TestPool_Wait_concurrentSubmit(t *testing.T) {
	tests := []struct {
		name        string
		parallel    int
		producers   int
		perProducer int
	}{
		{name: "2x50", parallel: 4, producers: 2, perProducer: 50},
		{name: "8x20", parallel: 8, producers: 8, perProducer: 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPool(tt.parallel)
			defer p.Shutdown()

			var ran atomic.Int32
			want := int32(tt.producers * tt.perProducer)

			var wg sync.WaitGroup
			wg.Add(tt.producers)
			for range tt.producers {
				go func() {
					defer wg.Done()
					for range tt.perProducer {
						p.Go(func() {
							ran.Add(1)
						})
					}
				}()
			}
			wg.Wait()
			p.Wait()

			if ran.Load() != want {
				t.Fatalf("ran %d, want %d", ran.Load(), want)
			}
			stats := p.Stats()
			if stats.Submitted != int64(want) || stats.Completed != int64(want) || stats.Pending != 0 || stats.Running != 0 {
				t.Fatalf("stats = %+v, want submitted/completed %d", stats, want)
			}
		})
	}
}

func TestPool_Close_idempotent(t *testing.T) {
	tests := []struct {
		name      string
		closeOps  int
		shutdowns int
	}{
		{name: "close_twice", closeOps: 2, shutdowns: 0},
		{name: "close_many", closeOps: 5, shutdowns: 0},
		{name: "shutdown_twice", closeOps: 0, shutdowns: 2},
		{name: "close_then_shutdown_twice", closeOps: 1, shutdowns: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPool(2)
			p.Go(func() {})
			p.Wait()

			for range tt.closeOps {
				p.Close()
			}
			for range tt.shutdowns {
				p.Shutdown()
			}
			if tt.shutdowns == 0 && tt.closeOps > 0 {
				p.Shutdown()
			}
		})
	}
}

func TestPool_Shutdown_waitsWorkers(t *testing.T) {
	tests := []struct {
		name     string
		parallel int
		tasks    int
	}{
		{name: "p2", parallel: 2, tasks: 20},
		{name: "p5", parallel: 5, tasks: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPool(tt.parallel)
			var ran atomic.Int32
			for range tt.tasks {
				p.Go(func() {
					ran.Add(1)
				})
			}
			p.Wait()
			p.Shutdown()

			if int(ran.Load()) != tt.tasks {
				t.Fatalf("ran %d, want %d", ran.Load(), tt.tasks)
			}
		})
	}
}

func TestPool_CloseThenGo_panics(t *testing.T) {
	tests := []struct {
		name     string
		parallel int
	}{
		{name: "p1", parallel: 1},
		{name: "p4", parallel: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPool(tt.parallel)
			p.Go(func() {})
			p.Wait()
			p.Close()

			defer func() {
				if r := recover(); r == nil {
					t.Fatal("Go after Close: expected panic on send to closed channel")
				}
			}()
			p.Go(func() {})
		})
	}
}
