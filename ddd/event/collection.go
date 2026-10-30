package event

import "sync"

// Events is a Collection whose zero value is ready to use, so an aggregate
// can hold it as a plain field:
//
//	type Order struct {
//		Events event.Events
//	}
//
//	order.Events.Add(OrderPaid{OrderID: order.ID})
//	dispatcher.DispatchAll(ctx, order.Events.Drain())
//
// Events holds a mutex, so share an aggregate by pointer rather than copying
// it.
type Events struct {
	mu     sync.Mutex
	events []Event
}

var _ Collection = (*Events)(nil)

// NewCollection returns an empty Events as a Collection.
func NewCollection() Collection {
	return &Events{}
}

// Add records a pending event. A nil event is ignored.
func (c *Events) Add(event Event) {
	if event == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.events = append(c.events, event)
}

// Drain returns the pending events in acceptance order and clears them.
func (c *Events) Drain() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	events := c.events
	c.events = nil
	return events
}

// Len returns the number of pending events.
func (c *Events) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.events)
}
