package event

import (
	"context"
	"log/slog"
	"time"
)

// Option configures a Dispatcher during construction.
type Option func(*InMemoryDispatcher)

// WithLogger sets the logger for dispatcher lifecycle and runtime diagnostics.
func WithLogger(logger *slog.Logger) Option {
	return func(d *InMemoryDispatcher) {
		if logger != nil {
			d.logger = logger
		}
	}
}

// WithDelayClose sets the delay before the dispatcher starts rejecting new events during shutdown.
func WithDelayClose(delay time.Duration) Option {
	return func(d *InMemoryDispatcher) {
		if delay >= 0 {
			d.delayClose = delay
		}
	}
}

// WithHandlerTimeout sets the timeout for each handler invocation.
func WithHandlerTimeout(timeout time.Duration) Option {
	return func(d *InMemoryDispatcher) {
		if timeout >= 0 {
			d.handlerTimeout = timeout
		}
	}
}

// WithContextFactory sets a function to derive a new context for each event dispatch.
func WithContextFactory(fn func(context.Context, Event) context.Context) Option {
	return func(d *InMemoryDispatcher) {
		d.contextFactory = fn
	}
}

// WithUnhandledEventHandler sets a hook for events with no registered handler.
func WithUnhandledEventHandler(fn func(UnhandledContext)) Option {
	return func(d *InMemoryDispatcher) {
		d.unhandledEventHandler = fn
	}
}

// WithPanicHandler sets a hook for recovered handler panics.
func WithPanicHandler(fn func(PanicContext)) Option {
	return func(d *InMemoryDispatcher) {
		d.panicHandler = fn
	}
}

// WithCloseInterruptedHandler sets a hook for close interruptions that leave
// accepted work unconfirmed.
func WithCloseInterruptedHandler(fn func(CloseInterruptedContext)) Option {
	return func(d *InMemoryDispatcher) {
		d.closeInterruptedHandler = fn
	}
}

// WithBufferSize sets the maximum number of queued event batches.
func WithBufferSize(size int) Option {
	return func(d *InMemoryDispatcher) {
		if size > 0 {
			d.bufferSize = size
		}
	}
}
