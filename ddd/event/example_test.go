package event_test

import (
	"fmt"

	"github.com/go-jimu/components/ddd/event"
)

type exampleDomainEvent struct {
	kind event.Kind
}

func (e exampleDomainEvent) Kind() event.Kind {
	return e.kind
}

func ExampleNewCollection() {
	events := event.NewCollection()

	events.Add(exampleDomainEvent{kind: "order.paid"})
	events.Add(exampleDomainEvent{kind: "order.confirmed"})

	fmt.Println("pending", events.Len())

	drained := events.Drain()
	fmt.Println(drained[0].Kind(), drained[1].Kind(), events.Len())

	// Output:
	// pending 2
	// order.paid order.confirmed 0
}
