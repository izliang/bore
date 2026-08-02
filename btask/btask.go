// Package btask provides fixed-size worker pools and a multi-window pool for routing
// work (e.g. LLM calls) by name with independent concurrency limits and optional timeouts.
package btask

import (
	"sync"
	"sync/atomic"
)

// Pool is a worker pool with a fixed number of goroutines. At most parallel jobs run
// concurrently; extra submissions block until a worker is free (with channel backpressure).
type Pool struct {
	jobs      chan func()
	taskWg    sync.WaitGroup
	workerWg  sync.WaitGroup
	closeOnce sync.Once

	parallel  int
	running   atomic.Int64
	submitted atomic.Int64
	completed atomic.Int64
}

// PoolStats is an instantaneous snapshot of a Pool.
type PoolStats struct {
	Parallel      int
	QueueLength   int
	QueueCapacity int
	Running       int64
	Submitted     int64
	Completed     int64
	Pending       int64
}

// NewPool starts parallel worker goroutines. Values less than 1 are treated as 1.
// The job channel buffer size equals parallel to reduce blocking on bursty submits.
func NewPool(parallel int) *Pool {
	if parallel < 1 {
		parallel = 1
	}
	p := &Pool{
		jobs:     make(chan func(), parallel),
		parallel: parallel,
	}
	p.workerWg.Add(parallel)
	for range parallel {
		go func() {
			defer p.workerWg.Done()
			p.workerLoop()
		}()
	}
	return p
}

func (p *Pool) workerLoop() {
	for fn := range p.jobs {
		if fn != nil {
			fn()
		}
	}
}

// Go schedules fn to run asynchronously on the pool. If all workers are busy and the
// buffer is full, Go blocks until a worker accepts a job.
// Nil fn is a no-op.
func (p *Pool) Go(fn func()) {
	if fn == nil {
		return
	}
	p.submitted.Add(1)
	p.taskWg.Add(1)
	p.jobs <- func() {
		p.running.Add(1)
		defer func() {
			p.running.Add(-1)
			p.completed.Add(1)
			p.taskWg.Done()
		}()
		fn()
	}
}

// Stats returns an instantaneous snapshot of pool state.
func (p *Pool) Stats() PoolStats {
	submitted := p.submitted.Load()
	completed := p.completed.Load()
	pending := max(submitted-completed, 0)
	return PoolStats{
		Parallel:      p.parallel,
		QueueLength:   len(p.jobs),
		QueueCapacity: cap(p.jobs),
		Running:       p.running.Load(),
		Submitted:     submitted,
		Completed:     completed,
		Pending:       pending,
	}
}

// Wait blocks until all jobs submitted via Go have finished executing.
// Call Wait after you stop submitting new work (or accept that Wait may return while
// queued wrappers are still pending pickup).
func (p *Pool) Wait() {
	p.taskWg.Wait()
}

// Close closes the job channel. Later calls to Go panic on send.
// Prefer Wait (or let running jobs finish) before Close to avoid dropping in-flight semantics.
// After Close, call Shutdown to wait for worker goroutines to exit.
func (p *Pool) Close() {
	p.closeOnce.Do(func() {
		close(p.jobs)
	})
}

// Shutdown closes the pool and waits for all workers to exit.
func (p *Pool) Shutdown() {
	p.Close()
	p.workerWg.Wait()
}
