# btask

`btask` provides small, explicit **worker pools** for Go: a single `Pool` with a fixed concurrency limit, a **`MultiPool`** that routes work to named **windows** (each with its own pool and optional default timeout), and **context helpers** for deadlines.

Use cases include throttling outbound **LLM** or HTTP calls by separating fast vs slow lanes, prompt size, or business rules—without spawning unbounded goroutines.

## Install

This package lives in the same module as the rest of the repo:

```go
package a

import "github.com/izliang/bore/btask"
```

## `Pool`

- **`NewPool(parallel int) *Pool`** — starts `parallel` workers (`parallel < 1` is treated as `1`). The job channel buffer size equals `parallel`.
- **`Go(fn func())`** — enqueue work; blocks if workers and buffer are saturated.
- **`Wait()`** — blocks until all jobs submitted via `Go` have finished.
- **`Close()`** — closes the job channel; further `Go` calls panic on send.
- **`Shutdown()`** — `Close()` then waits for workers to exit.

Nil `fn` in `Go` is ignored.

## `MultiPool` and `Window`

A **window** is a named lane with its own `Pool` and optional **`DefaultTimeout`** (used only by **`GoCtx`**).

```go
mp, err := btask.NewMultiPool([]btask.Window{
    {Name: btask.WindowFast, Parallel: 16, DefaultTimeout: 30 * time.Second},
    {Name: btask.WindowSlow, Parallel: 2, DefaultTimeout: 5 * time.Minute},
})
if err != nil {
    log.Fatal(err)
}
defer mp.Shutdown()
```

- **`Go(ctx, windowName, fn)`** — schedules `fn()` on that window’s pool. Does **not** apply `DefaultTimeout` (no `context` is passed to `fn`).
- **`GoCtx(ctx, windowName, fn)`** — runs `fn` with a child context from `WithOptionalTimeout(ctx, window.DefaultTimeout)` inside the worker. Use this for LLM calls that should respect deadlines and `ctx.Done()`.
- **`Wait()`** — waits on every window’s pool.
- **`Shutdown()`** — prevents new submissions, drains work, then shuts down each pool. **`Go` / `GoCtx` return an error** after shutdown.

Constants **`WindowFast`**, **`WindowSlow`**, **`WindowNormal`** are suggested names only; any unique non-empty string works.

## Monitoring

Use **`Stats()`** for direct snapshots:

```go
stats := pool.Stats()
fmt.Println(stats.Running, stats.QueueLength, stats.Pending)

for _, window := range mp.Stats() {
    fmt.Println(window.Window.Name, window.Stats.Submitted, window.Stats.Completed)
}
```

Use **`NewPoolMetrics`** or **`NewMultiPoolMetrics`** to expose Prometheus collectors that compose with this repository's `metrics.Provider` interface:

```go
type ServiceMetrics struct {
    identity.DefaultIdentity
    *btask.Metrics
}

sm := &ServiceMetrics{
    Metrics: btask.NewMultiPoolMetrics(mp),
}
metrics.MustInit(sm)
```

The default metric names include **`btask_pool_parallel`**, **`btask_pool_queue_length`**, **`btask_pool_running`**, **`btask_pool_pending`**, **`btask_pool_submitted_total`**, and **`btask_pool_completed_total`**, labeled by **`pool`**.

## Context helpers (`ctxutil.go`)

- **`EnsureContext(parent)`** — returns `context.Background()` if `parent` is nil.
- **`WithOptionalTimeout(parent, d)`** — if `d <= 0`, returns `parent` and a no-op `cancel`; otherwise `context.WithTimeout` (deadline is the **earlier** of parent and `now+d`, per the standard library).
- **`MergeTimeout`** — alias of `WithOptionalTimeout` for readability.

## Lifecycle and safety

1. Stop submitting new work (or call **`Shutdown`** on `MultiPool`).
2. For a bare `Pool`, call **`Wait()`**, then **`Close()`** / **`Shutdown()`** as needed.
3. **`MultiPool.Shutdown()`** waits for queued work before closing underlying pools.

Calling **`Go` after `Close`** on a `Pool` panics (send on closed channel). **`MultiPool`** returns a **`btask: MultiPool has been shut down`** error instead.

## Testing

```bash
go test -race ./btask/...
```

## License

Same as the enclosing repository.
