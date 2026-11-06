package event

import (
	"context"
	"log/slog"
	"time"
)

// Option configures an InMemoryDispatcher; pass options to NewDispatcher.
type Option func(*InMemoryDispatcher)

// WithLogger sets the logger for dispatcher lifecycle and runtime diagnostics:
// rejected dispatches, events without a handler, handler panics without
// WithPanicHandler, and close progress. Logs about an event carry the ctx
// passed to Dispatch, and close logs the ctx passed to Close. The default is
// slog.Default(); a nil logger keeps the default. Attribute names follow the
// conventions in github.com/go-jimu/components/sloghelper.
func WithLogger(logger *slog.Logger) Option {
	return func(d *InMemoryDispatcher) {
		if logger != nil {
			d.logger = logger
		}
	}
}

// WithHandlerTimeout bounds each Handle call: its context is canceled after
// timeout. The default, 0, sets no timeout; a negative timeout is ignored. The
// handler must watch ctx for the timeout to free its worker.
func WithHandlerTimeout(timeout time.Duration) Option {
	return func(d *InMemoryDispatcher) {
		if timeout >= 0 {
			d.handlerTimeout = timeout
		}
	}
}

// WithContextFactory sets a function that derives the context of each Handle
// call. fn receives the dispatcher-owned handler context and the event; derive
// the result from that ctx so cancellation and timeouts still apply, and copy
// caller values, such as a trace or request ID, from SourceContext(ctx). fn
// runs on worker goroutines, once per handler call. When fn returns nil, the
// handler gets the original context and a warning is logged.
func WithContextFactory(fn func(context.Context, Event) context.Context) Option {
	return func(d *InMemoryDispatcher) {
		d.contextFactory = fn
	}
}

// WithUnhandledEventHandler sets a hook for accepted events whose Kind has no
// subscribed handler. Without it, such events are logged as warnings. fn runs
// on worker goroutines.
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

// WithCloseInterruptedHandler sets a hook that Close calls when its ctx ends
// before every accepted event was handled. The hook receives the events that
// were running and those never started, for example to persist them for
// compensation. With the hook set, the dispatcher logs only a Warn summary
// and the hook owns the events; without it, each event is logged at Error.
// See the package documentation, "Shutdown".
func WithCloseInterruptedHandler(fn func(CloseInterruptedContext)) Option {
	return func(d *InMemoryDispatcher) {
		d.closeInterruptedHandler = fn
	}
}

// WithWorkers sets how many events the dispatcher may handle concurrently. The
// default is 5, so handlers and hooks run on several goroutines at once. Size it
// against the resources handlers share, such as the database connection pool.
// With 1 worker, every event is handled in the order it was accepted. A value
// below 1 keeps the default.
func WithWorkers(n int) Option {
	return func(d *InMemoryDispatcher) {
		if n > 0 {
			d.workers = n
		}
	}
}

// WithKeyFunc sets the ordering key of events that do not implement Keyed, for
// example to key existing events by aggregate ID without changing them. An
// empty key means the event has no ordering requirement. Keyed takes
// precedence over fn. fn runs on the goroutine calling DispatchAll.
func WithKeyFunc(fn func(Event) string) Option {
	return func(d *InMemoryDispatcher) {
		d.keyFunc = fn
	}
}

// WithBufferSize sets how many events the queue holds before DispatchAll waits.
// The default is 1024; a size below 1 keeps the default.
// A single call with more events than size is accepted once the queue is empty,
// so the queue can briefly hold more.
func WithBufferSize(size int) Option {
	return func(d *InMemoryDispatcher) {
		if size > 0 {
			d.bufferSize = size
		}
	}
}
