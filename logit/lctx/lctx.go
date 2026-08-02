// Package lctx provides context-based propagation of structured logging fields for logit.
// It allows attaching ordered key-value pairs to a context.Context that are automatically
// included in every log line emitted from that context. This is useful for propagating
// request IDs, trace IDs, and other contextual metadata through the call stack.
//
// Usage:
//
//	ctx := lctx.PushPair(context.Background(), myRequestFormatter)
//	logger.Info(ctx, "processing started") // automatically includes the request fields
//
// Callers should treat the slice returned by ListPair as read-only and not modify it.
package lctx

import "context"

// lCtx is an empty type used as a unique key for storing log pairs in context.
// Using a custom type prevents key collisions with other packages.
type lCtx struct{}

// lCtxKey is the singleton key used to store the pairs in context.
var lCtxKey = lCtx{}

// Pair represents a single key-value log entry attached to the context.
type Pair struct {
	Key   string // Field key name
	Value string // Pre-rendered string value
}

// LogFormatter is an interface that supplies a set of Pair values to be added to the context.
// This allows types like request objects to provide their own logging fields declaratively.
// Example: a http.Request can attach request ID, trace ID automatically.
type LogFormatter interface {
	Values() []Pair // Returns the pairs to add to the logging context
}

// PushPair returns a new derived context that includes the pairs from all provided formatters.
// Pairs are appended to any existing pairs already in the context, preserving order.
// If no formatters are provided or all formatters return empty slices, the original context
// is returned unchanged.
func PushPair(ctx context.Context, lfs ...LogFormatter) context.Context {
	if len(lfs) == 0 {
		return ctx
	}
	var pairs []Pair
	for _, lf := range lfs {
		pairs = append(pairs, lf.Values()...)
	}
	if len(pairs) == 0 {
		return ctx
	}
	t, ok := ctx.Value(lCtxKey).([]Pair)
	if ok && t != nil {
		t = append(t, pairs...)
	} else {
		t = pairs
	}
	return context.WithValue(ctx, lCtxKey, t)
}

// ListPair extracts all logging pairs that have been pushed into the context.
// Returns nil if no pairs have been stored. Callers must not modify the returned slice.
func ListPair(ctx context.Context) []Pair {
	t, ok := ctx.Value(lCtxKey).([]Pair)
	if ok && len(t) > 0 {
		return t
	}
	return nil
}

type DefaultPair struct {
	Key   string
	Value string
}

func (p *DefaultPair) Values() []Pair {
	return []Pair{
		{
			Key:   p.Key,
			Value: p.Value,
		},
	}
}
