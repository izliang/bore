package btask

import (
	"context"
	"fmt"
	"sync"
)

// MultiPool owns one Pool per named Window (e.g. fast vs slow) so you can route work by
// prompt or policy with separate concurrency limits and optional per-window timeouts.
type MultiPool struct {
	mu     sync.RWMutex
	slots  map[string]*multiSlot
	order  []string // registration order (stable iteration for Wait / Shutdown)
	shut   sync.RWMutex
	closed bool
}

type multiSlot struct {
	win  Window
	pool *Pool
}

// WindowPoolStats is an instantaneous snapshot for one MultiPool window.
type WindowPoolStats struct {
	Window Window
	Stats  PoolStats
}

// NewMultiPool builds one Pool per window. Window names must be unique and non-empty.
// Window.Parallel is passed to NewPool (values < 1 become 1).
func NewMultiPool(windows []Window) (*MultiPool, error) {
	if len(windows) == 0 {
		return nil, fmt.Errorf("btask: at least one Window is required")
	}
	m := &MultiPool{
		slots: make(map[string]*multiSlot, len(windows)),
		order: make([]string, 0, len(windows)),
	}
	seen := make(map[string]struct{}, len(windows))
	for _, w := range windows {
		if w.Name == "" {
			return nil, fmt.Errorf("btask: Window.Name must be non-empty")
		}
		if _, ok := seen[w.Name]; ok {
			return nil, fmt.Errorf("btask: duplicate window name %q", w.Name)
		}
		seen[w.Name] = struct{}{}
		p := NewPool(w.Parallel)
		m.slots[w.Name] = &multiSlot{win: w, pool: p}
		m.order = append(m.order, w.Name)
	}
	return m, nil
}

// MustNewMultiPool is like NewMultiPool but panics on error (tests, package init).
func MustNewMultiPool(windows []Window) *MultiPool {
	m, err := NewMultiPool(windows)
	if err != nil {
		panic(err)
	}
	return m
}

// Go runs fn on the named window's pool. Window.DefaultTimeout is not applied (fn has no
// context); use GoCtx when you need cooperative cancellation or a per-window timeout.
func (m *MultiPool) Go(ctx context.Context, windowName string, fn func()) error {
	if fn == nil {
		return nil
	}
	slot, err := m.slot(windowName)
	if err != nil {
		return err
	}
	return m.withSubmitLock(ctx, func() {
		slot.pool.Go(fn)
	})
}

// GoCtx runs fn with a context derived from ctx and the window’s DefaultTimeout.
// The derived context is created inside the worker; cancel runs after fn returns.
func (m *MultiPool) GoCtx(ctx context.Context, windowName string, fn func(context.Context)) error {
	if fn == nil {
		return nil
	}
	slot, err := m.slot(windowName)
	if err != nil {
		return err
	}
	return m.withSubmitLock(ctx, func() {
		slot.pool.Go(func() {
			base := EnsureContext(ctx)
			cctx, cancel := WithOptionalTimeout(base, slot.win.DefaultTimeout)
			defer cancel()
			if cctx.Err() != nil {
				return
			}
			fn(cctx)
		})
	})
}

// withSubmitLock serializes shutdown vs enqueue: Shutdown holds the write lock until
// all pools are drained and stopped; Go/GoCtx hold the read lock for the enqueue only.
func (m *MultiPool) withSubmitLock(ctx context.Context, enqueue func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.shut.RLock()
	defer m.shut.RUnlock()
	if m.closed {
		return fmt.Errorf("btask: MultiPool has been shut down")
	}
	enqueue()
	return nil
}

// Pool returns the underlying Pool for a window, or nil if the name is unknown.
func (m *MultiPool) Pool(windowName string) *Pool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.slots[windowName]; ok {
		return s.pool
	}
	return nil
}

// Window returns a copy of the window configuration and reports whether it exists.
func (m *MultiPool) Window(windowName string) (Window, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s, ok := m.slots[windowName]; ok {
		return s.win, true
	}
	return Window{}, false
}

// Names returns window names in registration order.
func (m *MultiPool) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, len(m.order))
	copy(out, m.order)
	return out
}

// Stats returns instantaneous snapshots for all windows in registration order.
func (m *MultiPool) Stats() []WindowPoolStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]WindowPoolStats, 0, len(m.order))
	for _, name := range m.order {
		slot := m.slots[name]
		out = append(out, WindowPoolStats{Window: slot.win, Stats: slot.pool.Stats()})
	}
	return out
}

// Wait waits until all jobs submitted via Go/GoCtx have completed on every window.
func (m *MultiPool) Wait() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, name := range m.order {
		m.slots[name].pool.Wait()
	}
}

// Shutdown marks the pool as closed, waits for outstanding work, then shuts down each
// Pool. Afterward, Go and GoCtx return an error. Shutdown is idempotent.
func (m *MultiPool) Shutdown() {
	m.shut.Lock()
	defer m.shut.Unlock()
	if m.closed {
		return
	}
	m.closed = true

	m.waitPoolsLocked()
	m.shutdownPoolsLocked()
}

func (m *MultiPool) waitPoolsLocked() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, name := range m.order {
		m.slots[name].pool.Wait()
	}
}

func (m *MultiPool) shutdownPoolsLocked() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, name := range m.order {
		m.slots[name].pool.Shutdown()
	}
}

func (m *MultiPool) slot(name string) (*multiSlot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.slots[name]
	if !ok {
		return nil, fmt.Errorf("btask: unknown window %q", name)
	}
	return s, nil
}
