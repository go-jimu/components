package event

import "context"

// Kind identifies the kind of a domain event inside one bounded context.
type Kind string

// Event is a domain fact raised inside one bounded context.
type Event interface {
	Kind() Kind
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

// Dispatcher accepts domain event batches for in-process handling.
type Dispatcher interface {
	Subscribe(Handler)
	Dispatch(Event) bool
	DispatchAll([]Event) bool
	Close(context.Context) error
}
