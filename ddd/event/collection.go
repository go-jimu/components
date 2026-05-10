package event

type collection struct {
	events  []Event
	drained bool
}

// NewCollection creates an empty aggregate event collection.
func NewCollection() Collection {
	return &collection{}
}

func (c *collection) Add(event Event) bool {
	if c.drained {
		return false
	}
	c.events = append(c.events, event)
	return true
}

func (c *collection) Drain() []Event {
	if c.drained {
		return nil
	}
	c.drained = true
	events := c.events
	c.events = nil
	return events
}

func (c *collection) Len() int {
	if c.drained {
		return 0
	}
	return len(c.events)
}
