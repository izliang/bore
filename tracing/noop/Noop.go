package noop

import (
	"context"

	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/tracing"
)

const Scope = tracing.Scope("noop")

type Provider struct{}

func NewProvider() *Provider {
	return &Provider{}
}

func (p *Provider) Tracer(scope tracing.Scope) tracing.Tracer {
	return &Tracer{}
}

func (p *Provider) Shutdown(ctx context.Context) error {
	return nil
}

type Tracer struct {
}

func (t *Tracer) Start(ctx context.Context, name string, opts ...tracing.StartOptions) (context.Context, tracing.Span) {
	return ctx, &Span{}
}

type Span struct {
}

func (s *Span) End() {
}

func (s *Span) RecordError(err error) {
}

func (s *Span) SetAttributes(attrs ...logit.FieldFunc) {

}
