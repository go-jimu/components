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

// Collection stores domain events raised by an aggregate until the application
// layer drains them after persistence succeeds.
type Collection interface {
	Add(Event) bool
	Drain() []Event
	Len() int
}

// Handler reacts to a domain event as a follow-up transaction.
type Handler interface {
	Listening() []Kind
	Handle(context.Context, Event)
}

// Dispatcher accepts domain event batches for handling.
type Dispatcher interface {
	Subscribe(Handler)
	Dispatch(Event) error
	DispatchAll([]Event) error
	Close(context.Context) error
}
