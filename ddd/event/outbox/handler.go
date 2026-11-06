package outbox

import (
	"context"
	"strconv"
	"sync/atomic"

	"github.com/go-jimu/components/ddd/event"
	"github.com/go-jimu/components/transaction"
)

// Handler is a must-run handler. See "Handlers".
type Handler interface {
	// Name keys the handler's records; keep it stable, see "Handlers".
	Name() string
	// Listening returns the kinds the handler runs for.
	Listening() []event.Kind
	// Handle runs one event. Its error decides what follows: nil moves on,
	// ErrPermanent fails the record, anything else retries; see "Failures".
	Handle(ctx context.Context, e event.Event) error
}

// Delivery identifies one run of one event. Key is the same on every retry
// of that event; see "Handlers".
type Delivery struct {
	RecordID string
	// Index is the event's position in the record.
	Index int
	// Attempt counts the leases taken on the record, the first included.
	Attempt int
}

// Key returns RecordID and Index joined, stable across retries.
func (d Delivery) Key() string {
	return d.RecordID + "/" + strconv.Itoa(d.Index)
}

type deliveryKey struct{}

// running is one run of one event by Handle, carried in Handle's ctx.
type running struct {
	outbox   *Outbox
	lease    Lease
	delivery Delivery
	// progress is the record's progress once this event is done.
	progress int
	// active is true until Handle returns.
	active atomic.Bool
	// stored is set once a transaction that called MarkDone commits.
	stored atomic.Bool
}

// DeliveryFrom returns the Delivery of the event a Handler runs, from the ctx
// the Relay passes to Handle.
func DeliveryFrom(ctx context.Context) (Delivery, bool) {
	r, ok := ctx.Value(deliveryKey{}).(*running)
	if !ok {
		return Delivery{}, false
	}
	return r.delivery, true
}

// ContextWithDelivery returns ctx carrying d, for a unit test of a Handler
// that reads DeliveryFrom. MarkDone on it returns ErrNoDelivery: only the
// Relay runs a delivery.
func ContextWithDelivery(ctx context.Context, d Delivery) context.Context {
	return context.WithValue(ctx, deliveryKey{}, &running{delivery: d})
}

type markKey struct{ run *running }

// MarkDone stores, in the transaction ctx is in, that the event Handle runs
// is done, so that transaction's writes take effect exactly once; see
// "Handlers". Call it inside that transaction, on a ctx derived from the one
// Handle received; a second call in the same transaction does nothing. It
// returns:
//   - ErrNoDelivery when ctx does not come from a Handle that is running;
//   - transaction.ErrNoTransaction or transaction.ErrTransactionDone outside
//     a live transaction;
//   - transaction.ErrForeignTransaction in a transaction of another database
//     than the Store's;
//   - ErrLeaseLost when another Relay has taken the record over.
func MarkDone(ctx context.Context) error {
	r, ok := ctx.Value(deliveryKey{}).(*running)
	if !ok || !r.active.Load() {
		return ErrNoDelivery
	}
	marked, err := transaction.Value(ctx, markKey{r}, func() *atomic.Bool { return new(atomic.Bool) })
	if err != nil {
		return err
	}
	if marked.Load() {
		return nil
	}
	if err := r.outbox.store.Advance(ctx, r.lease, r.progress); err != nil {
		return err
	}
	marked.Store(true)
	return transaction.AfterCommit(ctx, func(context.Context) { r.stored.Store(true) })
}
