package event

import (
	"context"
	"errors"
)

// Kind identifies the kind of a domain event inside one bounded context.
type Kind string

// Event is a domain fact raised inside one bounded context.
type Event interface {
	Kind() Kind
}

// ErrDispatcherClosed reports that a dispatcher cannot accept new events
// because it is closing or closed.
var ErrDispatcherClosed = errors.New("domain event dispatcher is closed")

// UnhandledContext describes an event that has no registered handler.
type UnhandledContext struct {
	BatchID uint64
	Event   Event
}

// PanicContext describes a recovered handler panic.
type PanicContext struct {
	BatchID uint64
	Event   Event
	Panic   any
	Stack   []byte
}

// PendingBatch describes an accepted event batch that was not started before
// Close was interrupted.
type PendingBatch struct {
	BatchID uint64
	Events  []Event
}

// CloseInterruptedContext describes accepted work that was not confirmed as
// handled before Close was interrupted.
type CloseInterruptedContext struct {
	Error           error
	InFlightBatchID uint64
	PendingBatches  []PendingBatch
}

// Collection stores pending domain events raised by an aggregate. Drain removes
// the current batch but leaves the collection ready to accept later events.
// Its methods are safe for concurrent use, but they do not synchronize other
// aggregate state or mutable values referenced by an Event.
type Collection interface {
	// Add records an event in the current batch. A nil event is ignored.
	Add(Event)
	// Drain returns the pending events in acceptance order and clears the current
	// batch. An Add concurrent with Drain belongs to either that returned batch or
	// the next batch.
	Drain() []Event
	// Len returns a snapshot of the number of events pending in the current batch.
	Len() int
}

// Handler reacts to a domain event as a follow-up transaction.
type Handler interface {
	Listening() []Kind
	Handle(context.Context, Event)
}

// Dispatcher accepts domain event batches for handling.
//
// Dispatch and DispatchAll only bound admission with ctx: when the dispatcher
// has to wait for capacity, it stops waiting and returns ctx.Err() once ctx is
// done, without accepting the batch. Accepted events are handled after the
// caller returns, so handlers never inherit the cancellation, deadline, or
// values of ctx. Use SourceContext to read caller values explicitly.
type Dispatcher interface {
	Dispatch(context.Context, Event) error
	DispatchAll(context.Context, []Event) error
	Close(context.Context) error
}

type sourceContextKey struct{}

// SourceContext returns the context passed to Dispatch or DispatchAll for the
// event being handled, detached from its cancellation and deadline. It reports
// false for contexts not created by a dispatcher.
//
// Handler contexts deliberately do not inherit caller values, because some of
// them, such as an open transaction, are only valid during the caller's
// request. Copy the values a handler needs, such as trace or request
// identifiers, from SourceContext, typically inside WithContextFactory.
func SourceContext(ctx context.Context) (context.Context, bool) {
	source, ok := ctx.Value(sourceContextKey{}).(context.Context)
	return source, ok
}

// Subscriber registers handlers for domain events.
type Subscriber interface {
	Subscribe(Handler)
}
