package event

import "sync"

type collection struct {
	mu     sync.Mutex
	events []Event
}

// NewCollection creates a concurrency-safe aggregate event collection that
// remains open across Drain calls.
func NewCollection() Collection {
	return &collection{}
}

func (c *collection) Add(event Event) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.events = append(c.events, event)
	return true
}

func (c *collection) Drain() []Event {
	c.mu.Lock()
	defer c.mu.Unlock()

	events := c.events
	c.events = nil
	return events
}

func (c *collection) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.events)
}
