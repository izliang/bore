package btask

import (
	"context"
	"time"
)

// EnsureContext returns parent if non-nil, otherwise context.Background().
func EnsureContext(parent context.Context) context.Context {
	if parent == nil {
		return context.Background()
	}
	return parent
}

// WithOptionalTimeout returns a child context with a timeout when d > 0.
// When d <= 0, it returns parent and a no-op cancel function (always safe to call).
//
// When d > 0, this uses context.WithTimeout: if parent already has an earlier deadline,
// the effective deadline is the minimum of the two (standard library behavior).
func WithOptionalTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	parent = EnsureContext(parent)
	if d <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, d)
}

// MergeTimeout is an alias of WithOptionalTimeout for call sites that read better as
// "merge this duration with the parent context".
func MergeTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return WithOptionalTimeout(parent, d)
}
