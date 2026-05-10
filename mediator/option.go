package mediator

import (
	"context"
	"log/slog"
	"time"
)

// Option configures an InMemMediator during construction.
//
// Deprecated: use github.com/go-jimu/components/ddd/event options for new
// domain event code.
type Option func(*InMemMediator)

// WithLogger sets the logger for the mediator.
//
// Deprecated: use github.com/go-jimu/components/ddd/event.WithLogger for new
// domain event code.
func WithLogger(logger *slog.Logger) Option {
	return func(m *InMemMediator) {
		if logger != nil {
			m.logger = logger
		}
	}
}

// WithDelayClose sets the delay before the mediator starts rejecting new events during shutdown.
//
// Deprecated: use github.com/go-jimu/components/ddd/event.WithDelayClose for
// new domain event code.
func WithDelayClose(d time.Duration) Option {
	return func(m *InMemMediator) {
		if d >= 0 {
			m.delayClose = d
		}
	}
}

// WithTimeout sets the context timeout for each handler invocation.
//
// Deprecated: use github.com/go-jimu/components/ddd/event.WithHandlerTimeout for
// new domain event code.
func WithTimeout(timeout time.Duration) Option {
	return func(m *InMemMediator) {
		if timeout >= 0 {
			m.timeout = timeout
		}
	}
}

// WithGenContext sets a function to derive a new context for each event dispatch.
//
// Deprecated: use github.com/go-jimu/components/ddd/event.WithContextFactory for
// new domain event code.
func WithGenContext(fn func(ctx context.Context, ev Event) context.Context) Option {
	return func(m *InMemMediator) {
		m.genContextFn = fn
	}
}

// WithOrphanEventHandler sets a fallback handler for events with no registered handler.
//
// Deprecated: use github.com/go-jimu/components/ddd/event.WithUnhandledEventHandler
// for new domain event code.
func WithOrphanEventHandler(fn func(Event) error) Option {
	return func(m *InMemMediator) {
		m.orphanEventHandler = fn
	}
}
