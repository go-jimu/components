package mediator

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// NOTE: EventCollection is NOT safe for concurrent use.
// Callers must ensure that Add and Raise/AsyncRaise are not called concurrently.
// This aligns with the typical DDD usage where events are collected within
// a single aggregate method call.

type (
	// EventKind 事件类型描述.
	//
	// Deprecated: use github.com/go-jimu/components/ddd/event.Kind for new
	// domain event code.
	EventKind string

	// Deprecated: use github.com/go-jimu/components/ddd/event.Handler for new
	// domain event code.
	EventHandler interface {
		Listening() []EventKind
		Handle(context.Context, Event)
	}

	// Event 事件接口.
	//
	// Deprecated: use github.com/go-jimu/components/ddd/event.Event for new
	// domain event code.
	Event interface {
		Kind() EventKind
	}

	eventCollection struct {
		events []Event
		raised atomic.Bool
	}

	// Deprecated: use github.com/go-jimu/components/ddd/event.Collection for new
	// domain event code.
	EventCollection interface {
		Add(Event)
		Raise(Mediator)
		AsyncRaise(Mediator)
	}
)

var _ EventCollection = (*eventCollection)(nil)

// Deprecated: use github.com/go-jimu/components/ddd/event.NewCollection for new
// domain event code.
func NewEventCollection() EventCollection {
	return &eventCollection{events: make([]Event, 0)}
}

func (es *eventCollection) Add(ev Event) {
	if es.raised.Load() {
		slog.Error("failed to add event, already raised", slog.Any("dropped_event", ev))
		return
	}
	es.events = append(es.events, ev)
}

// Raise raises the event collection synchronously.
func (es *eventCollection) Raise(m Mediator) {
	if es.raised.CompareAndSwap(false, true) {
		for _, event := range es.events {
			m.Dispatch(event)
		}
		return
	}
	slog.Error("failed to raise event, already raised", slog.Any("events", es.events))
}

// AsyncRaise raises the event collection asynchronously.
func (es *eventCollection) AsyncRaise(m Mediator) {
	go es.Raise(m)
}
