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

	firstBatch := events.Drain()
	fmt.Println(firstBatch[0].Kind(), firstBatch[1].Kind(), events.Len())

	events.Add(exampleDomainEvent{kind: "order.shipped"})
	secondBatch := events.Drain()
	fmt.Println(secondBatch[0].Kind(), events.Len())

	// Output:
	// pending 2
	// order.paid order.confirmed 0
	// order.shipped 0
}
