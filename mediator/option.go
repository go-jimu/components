package mediator

import (
	"context"
	"log/slog"
	"time"
)

// Option configures an InMemMediator during construction.
type Option func(*InMemMediator)

// WithLogger sets the logger for the mediator.
func WithLogger(logger *slog.Logger) Option {
	return func(m *InMemMediator) {
		if logger != nil {
			m.logger = logger
		}
	}
}

// WithDelayClose sets the delay before the mediator starts rejecting new events during shutdown.
func WithDelayClose(d time.Duration) Option {
	return func(m *InMemMediator) {
		if d >= 0 {
			m.delayClose = d
		}
	}
}

// WithTimeout sets the context timeout for each handler invocation.
func WithTimeout(timeout time.Duration) Option {
	return func(m *InMemMediator) {
		if timeout >= 0 {
			m.timeout = timeout
		}
	}
}

// WithGenContext sets a function to derive a new context for each event dispatch.
func WithGenContext(fn func(ctx context.Context, ev Event) context.Context) Option {
	return func(m *InMemMediator) {
		m.genContextFn = fn
	}
}

// WithOrphanEventHandler sets a fallback handler for events with no registered handler.
func WithOrphanEventHandler(fn func(Event) error) Option {
	return func(m *InMemMediator) {
		m.orphanEventHandler = fn
	}
}
