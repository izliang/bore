package btask

import "time"

// Well-known window names for conventions such as fast vs slow LLM lanes.
// Any non-empty string may be used as a window name when constructing a MultiPool.
const (
	WindowFast   = "fast"
	WindowSlow   = "slow"
	WindowNormal = "normal"
)

// Window describes one logical lane: its own worker pool size and an optional default
// timeout merged into the context passed to MultiPool.GoCtx (see WithOptionalTimeout).
type Window struct {
	Name           string
	Parallel       int
	DefaultTimeout time.Duration
}
