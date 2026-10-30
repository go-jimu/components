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

// WithPanicHandler sets a hook for recovered handler panics. Keep fn to
// reporting, such as logging or sending to an error tracker: it runs inside the
// recovery, so a panic in fn ends the process.
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

// WithWorkers sets how many events the dispatcher may handle concurrently. The
// default is 5, so handlers and hooks run on several goroutines at once. Size it
// against the resources handlers share, such as the database connection pool.
// With 1 worker, every event is handled in the order it was accepted.
func WithWorkers(n int) Option {
	return func(d *InMemoryDispatcher) {
		if n > 0 {
			d.workers = n
		}
	}
}

// WithKeyFunc sets the ordering key of events that do not implement Keyed, for
// example to key existing events by aggregate ID without changing them. An
// empty key means the event has no ordering requirement.
func WithKeyFunc(fn func(Event) string) Option {
	return func(d *InMemoryDispatcher) {
		d.keyFunc = fn
	}
}

// WithBufferSize sets how many events the queue holds before DispatchAll waits.
// A single call with more events than size is accepted once the queue is empty,
// so the queue can briefly hold more.
func WithBufferSize(size int) Option {
	return func(d *InMemoryDispatcher) {
		if size > 0 {
			d.bufferSize = size
		}
	}
}
