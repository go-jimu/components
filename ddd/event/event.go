package event

import (
	"context"
	"errors"
)

// Kind identifies the kind of a domain event inside one bounded context and
// routes it to the handlers that listen to it. Use one Kind per event type and
// a stable name such as "order.paid".
type Kind string

// Event is a domain fact raised inside one bounded context.
type Event interface {
	Kind() Kind
}

// ErrDispatcherClosed reports that a dispatcher cannot accept new events
// because it is closing or closed.
var ErrDispatcherClosed = errors.New("domain event dispatcher is closed")

// ErrAdmissionTimeout reports that a dispatcher rejected events because its
// queue stayed full for the whole admission timeout.
var ErrAdmissionTimeout = errors.New("domain event dispatcher queue stayed full")

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

// Keyed is implemented by events that carry an ordering key, usually the ID
// of the aggregate that raised them; an empty key means no key. WithKeyFunc
// keys events that do not implement Keyed. See "Ordering".
type Keyed interface {
	Key() string
}

// Collection stores pending domain events raised by an aggregate; Events
// implements it. Its methods are safe for concurrent use, but they do not
// synchronize other aggregate state or mutable values referenced by an Event.
type Collection interface {
	// Add records a pending event. A nil event is ignored.
	Add(Event)
	// Drain returns the pending events in acceptance order and clears them. An
	// Add concurrent with Drain is returned by either that Drain or the next.
	Drain() []Event
	// Len returns a snapshot of the number of pending events.
	Len() int
}

// Handler reacts to a domain event as a follow-up transaction; see
// "Handlers".
//
// Subscribe reads Listening once. Handle receives each accepted event of
// those kinds, returns early for any concrete type it was not written for,
// and owns its failures: nothing retries it. Handle runs concurrently with
// itself for events of different keys, see "Ordering", on a ctx that
// WithHandlerTimeout and an interrupted Close cancel.
type Handler interface {
	Listening() []Kind
	Handle(context.Context, Event)
}

// Dispatcher accepts domain events for asynchronous handling. Its methods are
// safe for concurrent use.
//
// DispatchAll accepts all of its events or none, and returns nil when it
// accepts them. The ctx of Dispatch and DispatchAll carries values only, such
// as the caller's trace: its cancellation and deadline neither reject events
// nor reach handlers. An error's meaning is the implementation's:
// InMemoryDispatcher reports each rejected event itself, so a use case
// ignores its error, see "Dispatch"; an error from ddd/event/outbox rolls the
// transaction back. Close stops admission, see "Shutdown".
type Dispatcher interface {
	Dispatch(context.Context, Event) error
	DispatchAll(context.Context, []Event) error
	Close(context.Context) error
}

type sourceContextKey struct{}

// SourceContext returns the context passed to Dispatch or DispatchAll for the
// event being handled, detached from its cancellation and deadline. It reports
// false for contexts not created by a dispatcher. Copy observability values,
// such as the trace, from it; see "Dispatch".
func SourceContext(ctx context.Context) (context.Context, bool) {
	source, ok := ctx.Value(sourceContextKey{}).(context.Context)
	return source, ok
}

// Subscriber registers handlers for domain events. Subscribe every handler
// before the first event of its kinds is dispatched: an event that reaches a
// worker before its kind has a handler is reported as unhandled.
type Subscriber interface {
	Subscribe(Handler)
}
