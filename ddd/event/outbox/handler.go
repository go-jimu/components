package outbox

import (
	"context"
	"strconv"

	"github.com/go-jimu/components/ddd/event"
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

// DeliveryFrom returns the Delivery of the event a Handler runs, from the ctx
// the Relay passes to Handle.
func DeliveryFrom(ctx context.Context) (Delivery, bool) {
	d, ok := ctx.Value(deliveryKey{}).(Delivery)
	return d, ok
}
