package outbox_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/go-jimu/components/ddd/event"
	"github.com/go-jimu/components/ddd/event/outbox"
	"github.com/go-jimu/components/ddd/event/outbox/outboxtest"
	"github.com/go-jimu/components/transaction"
	"github.com/go-jimu/components/transaction/transactiontest"
)

// ProjectOrder keeps a read model of orders: a must-run handler, written as a
// reconciler.
type ProjectOrder struct{ done chan string }

func (ProjectOrder) Name() string            { return "order-projection" }
func (ProjectOrder) Listening() []event.Kind { return []event.Kind{OrderPaid{}.Kind()} }

func (p ProjectOrder) Handle(ctx context.Context, e event.Event) error {
	paid, ok := e.(OrderPaid)
	if !ok {
		return nil
	}
	d, _ := outbox.DeliveryFrom(ctx)
	// Reconcile: upsert the projection guarded by the order version, so a
	// rerun or a stale event changes nothing.
	p.done <- fmt.Sprintf("projected %s (attempt %d)", paid.OrderID, d.Attempt)
	return nil
}

func Example() {
	// Composition: the Manager and the Store work on the same database; here
	// both are in memory.
	manager, _ := transaction.NewManager(&transactiontest.Driver{})
	registry := outbox.NewRegistry()
	_ = outbox.Register[OrderPaid](registry)
	dispatcher, _ := outbox.New(&outboxtest.Store{}, registry,
		outbox.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	projector := ProjectOrder{done: make(chan string, 1)}
	_ = dispatcher.Subscribe(projector)
	go func() { _ = dispatcher.Run(context.Background()) }()
	defer func() { _ = dispatcher.Close(context.Background()) }()

	// A use case: save the aggregate and dispatch its events in one
	// transaction, returning the dispatch error.
	err := manager.Within(context.Background(), func(ctx context.Context) error {
		// repo.Save(ctx, order) runs here.
		return dispatcher.Dispatch(ctx, OrderPaid{OrderID: "o-1"})
	})
	fmt.Println("committed:", err == nil)
	fmt.Println(<-projector.done)
	// Output:
	// committed: true
	// projected o-1 (attempt 1)
}
