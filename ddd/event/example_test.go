package event_test

import (
	"context"
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

// OrderPaid is a domain event. Its Kind routes it to handlers.
type OrderPaid struct {
	OrderID string
	Amount  int
}

func (OrderPaid) Kind() event.Kind { return "order.paid" }

// legacyOrderPaid reuses the "order.paid" kind with a different Go type.
type legacyOrderPaid struct{}

func (legacyOrderPaid) Kind() event.Kind { return "order.paid" }

// SendReceipt reacts to OrderPaid. Handle type-asserts the concrete event and
// returns early for any other type that shares the kind.
type SendReceipt struct{}

func (SendReceipt) Listening() []event.Kind {
	return []event.Kind{OrderPaid{}.Kind()}
}

func (SendReceipt) Handle(_ context.Context, e event.Event) {
	paid, ok := e.(OrderPaid)
	if !ok {
		return
	}
	// Handle owns failures: retry, enqueue durable work, or log here.
	fmt.Println("receipt for", paid.OrderID, paid.Amount)
}

func ExampleHandler() {
	dispatcher := event.NewDispatcher(event.WithDelayClose(0))
	dispatcher.Subscribe(SendReceipt{})

	err := dispatcher.DispatchAll(context.Background(), []event.Event{
		OrderPaid{OrderID: "order-1", Amount: 42},
		legacyOrderPaid{},
	})
	if err != nil {
		panic(err)
	}

	// Close waits until accepted events are handled.
	if err := dispatcher.Close(context.Background()); err != nil {
		panic(err)
	}

	// Output:
	// receipt for order-1 42
}

// OrderStepped is keyed by its order ID, so the steps of one order are handled
// one at a time in acceptance order, even with several workers.
type OrderStepped struct {
	OrderID string
	Step    string
}

func (OrderStepped) Kind() event.Kind { return "order.stepped" }

func (e OrderStepped) Key() string { return e.OrderID }

type printStep struct{}

func (printStep) Listening() []event.Kind { return []event.Kind{OrderStepped{}.Kind()} }

func (printStep) Handle(_ context.Context, e event.Event) {
	if stepped, ok := e.(OrderStepped); ok {
		fmt.Println(stepped.OrderID, stepped.Step)
	}
}

func ExampleKeyed() {
	dispatcher := event.NewDispatcher(event.WithWorkers(5), event.WithDelayClose(0))
	dispatcher.Subscribe(printStep{})

	err := dispatcher.DispatchAll(context.Background(), []event.Event{
		OrderStepped{OrderID: "order-1", Step: "placed"},
		OrderStepped{OrderID: "order-1", Step: "paid"},
		OrderStepped{OrderID: "order-1", Step: "shipped"},
	})
	if err != nil {
		panic(err)
	}
	if err := dispatcher.Close(context.Background()); err != nil {
		panic(err)
	}

	// Output:
	// order-1 placed
	// order-1 paid
	// order-1 shipped
}

type requestIDKey struct{}

type printRequestID struct{}

func (printRequestID) Listening() []event.Kind { return []event.Kind{OrderPaid{}.Kind()} }

func (printRequestID) Handle(ctx context.Context, _ event.Event) {
	fmt.Println("request id:", ctx.Value(requestIDKey{}))
}

func ExampleWithContextFactory() {
	// Handler contexts do not inherit caller values. Copy the ones handlers
	// need from SourceContext, onto the handler's own ctx.
	copyRequestID := func(ctx context.Context, _ event.Event) context.Context {
		source, ok := event.SourceContext(ctx)
		if !ok {
			return ctx
		}
		return context.WithValue(ctx, requestIDKey{}, source.Value(requestIDKey{}))
	}

	dispatcher := event.NewDispatcher(
		event.WithContextFactory(copyRequestID),
		event.WithDelayClose(0),
	)
	dispatcher.Subscribe(printRequestID{})

	callerCtx := context.WithValue(context.Background(), requestIDKey{}, "req-7")
	if err := dispatcher.Dispatch(callerCtx, OrderPaid{OrderID: "order-1"}); err != nil {
		panic(err)
	}
	if err := dispatcher.Close(context.Background()); err != nil {
		panic(err)
	}

	// Output:
	// request id: req-7
}
