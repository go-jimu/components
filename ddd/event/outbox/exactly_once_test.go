package outbox_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/go-jimu/components/ddd/event"
	"github.com/go-jimu/components/ddd/event/outbox"
	"github.com/go-jimu/components/transaction"
	"github.com/go-jimu/components/transaction/transactiontest"
)

// ledger stands for the handler's table in the Store's database: a write
// takes effect when the transaction it ran in commits.
type ledger struct {
	mu   sync.Mutex
	rows []string
}

func (l *ledger) write(ctx context.Context, row string) error {
	return transaction.AfterCommit(ctx, func(context.Context) {
		l.mu.Lock()
		l.rows = append(l.rows, row)
		l.mu.Unlock()
	})
}

func (l *ledger) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.rows)
}

// writeIn makes h write each event's order ID in a transaction of manager,
// then return what after returns.
func writeIn(h *handler, manager *transaction.Manager, l *ledger, before func(call int) error, after func(call int) error) {
	h.fail = func(e event.Event, call int) error {
		h.mu.Lock()
		ctx := h.ctxs[call]
		h.mu.Unlock()
		err := manager.Within(ctx, func(ctx context.Context) error {
			if err := l.write(ctx, e.(OrderPaid).OrderID); err != nil {
				return err
			}
			if before != nil {
				return before(call)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if after != nil {
			return after(call)
		}
		return nil
	}
}

func TestHandlerTransactionStoresProgress(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	l := &ledger{}
	writeIn(h, f.manager, l, func(call int) error {
		if call == 1 {
			return errors.New("constraint violated") // o-2's first transaction rolls back
		}
		return nil
	}, nil)
	f.subscribe(h)
	f.run()
	if err := f.dispatch(OrderPaid{"o-1"}, OrderPaid{"o-2"}); err != nil {
		t.Fatal(err)
	}
	h.wait(t, 3)
	f.waitFor("the record to complete", f.settled(outbox.StatusDone))
	if got := l.all(); !slices.Equal(got, []string{"o-1", "o-2"}) {
		t.Fatalf("committed writes %v, want each event's once", got)
	}
	if got := h.events(); len(got) != 3 {
		t.Fatalf("handled %v, want o-1 once and o-2 twice", got)
	}
}

func TestHandlerErrorAfterCommitDoesNotRerun(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	l := &ledger{}
	writeIn(h, f.manager, l, nil, func(call int) error {
		if call == 0 {
			return errors.New("reply lost after commit")
		}
		return nil
	})
	f.subscribe(h)
	f.run()
	if err := f.dispatch(OrderPaid{"o-1"}, OrderPaid{"o-2"}); err != nil {
		t.Fatal(err)
	}
	h.wait(t, 2)
	f.waitFor("the record to complete", f.settled(outbox.StatusDone))
	if got := h.events(); !slices.Equal(got, []event.Event{OrderPaid{"o-1"}, OrderPaid{"o-2"}}) {
		t.Fatalf("handled %v, want each event once", got)
	}
	if got := l.all(); !slices.Equal(got, []string{"o-1", "o-2"}) {
		t.Fatalf("committed writes %v, want each event's once", got)
	}
	if r := f.store.Records()[0]; r.Attempts != 1 {
		t.Fatalf("attempts %d, want 1: the committed progress makes the error no reason to retry", r.Attempts)
	}
}

// committingStore stands for a database whose commit succeeds but whose reply
// is lost: Advance applies at once, whatever becomes of the transaction.
type committingStore struct{ outbox.Store }

func (s committingStore) Advance(_ context.Context, lease outbox.Lease, progress int) error {
	return s.Store.Advance(context.Background(), lease, progress)
}

func TestUnknownOutcomeDoesNotRerun(t *testing.T) {
	f := newFixture(t)
	registry := outbox.NewRegistry()
	_ = outbox.Register[OrderPaid](registry)
	o, err := outbox.New(committingStore{f.store}, registry, fixtureOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	f.outbox = o
	h := newHandler("projector", OrderPaid{}.Kind())
	writeIn(h, f.manager, &ledger{}, func(call int) error {
		if call == 0 {
			f.driver.FailNextCommit(errors.New("connection reset"))
		}
		return nil
	}, nil)
	f.subscribe(h)
	f.run()
	if err := f.dispatch(OrderPaid{"o-1"}, OrderPaid{"o-2"}); err != nil {
		t.Fatal(err)
	}
	h.wait(t, 2)
	f.waitFor("the record to complete", f.settled(outbox.StatusDone))
	if got := h.events(); !slices.Equal(got, []event.Event{OrderPaid{"o-1"}, OrderPaid{"o-2"}}) {
		t.Fatalf("handled %v, want o-1 once: its progress committed though the outcome was unknown", got)
	}
	if r := f.store.Records()[0]; r.Attempts != 2 {
		t.Fatalf("attempts %d, want a retry after the unknown outcome", r.Attempts)
	}
}

func TestForeignTransactionProgressStoredByRelay(t *testing.T) {
	f := newFixture(t)
	other, err := transaction.NewManager(&transactiontest.Driver{})
	if err != nil {
		t.Fatal(err)
	}
	h := newHandler("projector", OrderPaid{}.Kind())
	l := &ledger{}
	writeIn(h, other, l, nil, nil)
	f.subscribe(h)
	f.run()
	if err := f.dispatch(OrderPaid{"o-1"}, OrderPaid{"o-2"}); err != nil {
		t.Fatal(err)
	}
	h.wait(t, 2)
	f.waitFor("the record to complete", f.settled(outbox.StatusDone))
	if got := l.all(); !slices.Equal(got, []string{"o-1", "o-2"}) {
		t.Fatalf("committed writes %v, want each event's once", got)
	}
}

func TestLeaseLostInHandlerTransactionRollsBack(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	l := &ledger{}
	writeIn(h, f.manager, l, func(int) error {
		// An operator skips the event mid-run, which releases the lease.
		return f.store.Skip(context.Background(), f.store.Records()[0].ID, 0)
	}, nil)
	f.subscribe(h)
	f.run()
	if err := f.dispatch(OrderPaid{"o-1"}); err != nil {
		t.Fatal(err)
	}
	h.wait(t, 1)
	f.waitFor("the skip to settle the record", f.settled(outbox.StatusDone))
	if got := l.all(); len(got) != 0 {
		t.Fatalf("committed writes %v, want none: the lease was lost inside the transaction", got)
	}
}
