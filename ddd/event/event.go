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
	Event Event
}

// PanicContext describes a handler panic recovered by the dispatcher.
type PanicContext struct {
	Event   Event
	Handler Handler
	Panic   any
	Stack   []byte
}

// CloseInterruptedContext describes accepted events that were not confirmed
// as handled before Close was interrupted. InFlight lists the events workers
// were handling and Pending the events not yet started, both in the order
// they were accepted.
type CloseInterruptedContext struct {
	Error    error
	InFlight []Event
	Pending  []Event
}

// Keyed is implemented by events that must be handled in order with other
// events of the same key, usually the ID of the aggregate that raised them.
//
// Events with the same key are handled one at a time in the order they were
// accepted. Events with different keys, and events without a key, are handled
// concurrently. An empty key means no key. Events that do not implement Keyed
// can get a key from WithKeyFunc.
type Keyed interface {
	Key() string
}

// Collection stores pending domain events raised by an aggregate. Drain removes
// the pending events but leaves the collection ready to accept later events.
// Events is the implementation; its zero value is ready to use.
// Its methods are safe for concurrent use, but they do not synchronize other
// aggregate state or mutable values referenced by an Event.
type Collection interface {
	// Add records a pending event. A nil event is ignored.
	Add(Event)
	// Drain returns the pending events in acceptance order and clears them. An
	// Add concurrent with Drain is returned by either that Drain or the next.
	Drain() []Event
	// Len returns a snapshot of the number of pending events.
	Len() int
}

// Handler reacts to a domain event as a follow-up transaction.
type Handler interface {
	Listening() []Kind
	Handle(context.Context, Event)
}

// Dispatcher accepts domain events for asynchronous handling.
//
// DispatchAll accepts all of its events or none of them. Dispatch and
// DispatchAll only bound admission with ctx: when the dispatcher has to wait
// for capacity, it stops waiting and returns ctx.Err() once ctx is done,
// without accepting the events. Accepted events are handled after the
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
