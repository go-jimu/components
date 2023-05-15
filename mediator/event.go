package mediator

import (
	"context"
	"sync/atomic"
)

type (
	// EventKind 事件类型描述.
	EventKind string

	EventHandler interface {
		Listening() []EventKind
		Handle(context.Context, Event)
	}

	// Event 事件接口.
	Event interface {
		Kind() EventKind
	}

	eventCollection struct {
		events []Event
		raised int32
	}

	EventCollection interface {
		Add(Event)
		Raise(Mediator)
	}
)

func NewEventCollection() EventCollection {
	return &eventCollection{events: make([]Event, 0)}
}

func (es *eventCollection) Add(ev Event) {
	if atomic.LoadInt32(&es.raised) == 0 {
		es.events = append(es.events, ev)
	}
}

func (es *eventCollection) Raise(m Mediator) {
	if atomic.CompareAndSwapInt32(&es.raised, 0, 1) {
		for _, event := range es.events {
			m.Dispatch(event)
		}
	}
}
