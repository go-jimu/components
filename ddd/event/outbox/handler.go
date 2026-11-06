package outbox

import (
	"context"
	"strconv"

	"github.com/go-jimu/components/ddd/event"
)

// Handler is a handler that must run: the Outbox records its events in the
// business transaction and the Relay runs them until they succeed. See
// "Handlers".
type Handler interface {
	// Name identifies the handler in stored records. It stays the same
	// across versions and refactorings.
	Name() string
	// Listening returns the kinds the handler runs for.
	Listening() []event.Kind
	// Handle runs one event. It returns nil when done, an error to retry the
	// event later, or an error wrapping ErrPermanent to stop retrying.
	Handle(ctx context.Context, e event.Event) error
}

// Delivery identifies one run of one event: Key is the same on every retry
// of that event, so a Handler deduplicates its effects by it. See "Handlers".
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

// DeliveryFrom returns the Delivery of the event a Handler runs, from the ctx
// the Relay passes to Handle.
func DeliveryFrom(ctx context.Context) (Delivery, bool) {
	d, ok := ctx.Value(deliveryKey{}).(Delivery)
	return d, ok
}
