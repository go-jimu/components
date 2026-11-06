package outbox_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/event"
	"github.com/go-jimu/components/ddd/event/outbox"
	"github.com/go-jimu/components/ddd/event/outbox/outboxtest"
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

// markDoneIn makes h write each event's order ID and call MarkDone in one
// transaction of manager. inTx runs last inside the transaction, after
// returns what Handle returns once the transaction committed.
func markDoneIn(h *handler, manager *transaction.Manager, l *ledger, inTx, after func(ctx context.Context, call int) error) {
	h.fail = func(e event.Event, call int) error {
		h.mu.Lock()
		ctx := h.ctxs[call]
		h.mu.Unlock()
		err := manager.Within(ctx, func(ctx context.Context) error {
			if err := l.write(ctx, e.(OrderPaid).OrderID); err != nil {
				return err
			}
			if err := outbox.MarkDone(ctx); err != nil {
				return err
			}
			if inTx != nil {
				return inTx(ctx, call)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if after != nil {
			return after(ctx, call)
		}
		return nil
	}
}

func TestMarkDoneCommitsProgressWithWrites(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	l := &ledger{}
	markDoneIn(h, f.manager, l, func(ctx context.Context, call int) error {
		if call != 1 {
			return nil
		}
		// o-2's first transaction fails at commit time, after MarkDone
		// advanced the progress in it: the rollback undoes both.
		return transaction.BeforeCommit(ctx, func(context.Context) error {
			return errors.New("deadlock at commit")
		})
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

func TestErrorAfterMarkDoneDoesNotRerun(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	l := &ledger{}
	markDoneIn(h, f.manager, l, nil, func(_ context.Context, call int) error {
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
	if r := f.store.Records()[0]; r.Attempts != 1 {
		t.Fatalf("attempts %d, want 1: the committed progress makes the error no reason to retry", r.Attempts)
	}
}

// committingStore stands for a database whose commit succeeds but whose reply
// is lost: Advance applies at once, whatever becomes of the transaction.
type committingStore struct{ *outboxtest.Store }

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
	markDoneIn(h, f.manager, &ledger{}, func(_ context.Context, call int) error {
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

func TestMarkDoneErrors(t *testing.T) {
	f := newFixture(t)
	other, err := transaction.NewManager(&transactiontest.Driver{})
	if err != nil {
		t.Fatal(err)
	}
	h := newHandler("projector", OrderPaid{}.Kind())
	results := make(chan map[string]error, 1)
	var kept context.Context
	h.fail = func(event.Event, int) error {
		h.mu.Lock()
		ctx := h.ctxs[0]
		h.mu.Unlock()
		kept = ctx
		got := map[string]error{"outside a transaction": outbox.MarkDone(ctx)}
		_ = other.Within(ctx, func(ctx context.Context) error {
			got["another database"] = outbox.MarkDone(ctx)
			return nil
		})
		results <- got
		return nil
	}
	f.subscribe(h)
	f.run()
	if err := f.dispatch(OrderPaid{"o-1"}); err != nil {
		t.Fatal(err)
	}
	got := <-results
	f.waitFor("the record to complete", f.settled(outbox.StatusDone))
	for name, want := range map[string]error{
		"outside a transaction": transaction.ErrNoTransaction,
		"another database":      transaction.ErrForeignTransaction,
	} {
		if !errors.Is(got[name], want) {
			t.Errorf("MarkDone %s = %v, want %v", name, got[name], want)
		}
	}
	err = f.manager.Within(kept, outbox.MarkDone)
	if !errors.Is(err, outbox.ErrNoDelivery) {
		t.Errorf("MarkDone after Handle returned = %v, want ErrNoDelivery", err)
	}
	err = f.manager.Within(context.Background(), outbox.MarkDone)
	if !errors.Is(err, outbox.ErrNoDelivery) {
		t.Errorf("MarkDone on a ctx not from Handle = %v, want ErrNoDelivery", err)
	}
}

// TestMarkDoneAfterHandleCannotRewind covers code that outlives Handle: a
// goroutine started by the first event's Handle calls MarkDone after later
// events advanced the record.
func TestMarkDoneAfterHandleCannotRewind(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	l := &ledger{}
	late := make(chan error, 1)
	markDoneIn(h, f.manager, l, nil, func(ctx context.Context, call int) error {
		if call == 0 {
			go func() {
				time.Sleep(20 * time.Millisecond)
				late <- f.manager.Within(context.WithoutCancel(ctx), outbox.MarkDone)
			}()
		}
		return nil
	})
	f.subscribe(h)
	f.run()
	if err := f.dispatch(OrderPaid{"o-1"}, OrderPaid{"o-2"}); err != nil {
		t.Fatal(err)
	}
	if err := <-late; !errors.Is(err, outbox.ErrNoDelivery) {
		t.Fatalf("MarkDone from a goroutine after Handle returned = %v, want ErrNoDelivery", err)
	}
	f.waitFor("the record to complete", f.settled(outbox.StatusDone))
	if got := l.all(); !slices.Equal(got, []string{"o-1", "o-2"}) {
		t.Fatalf("committed writes %v, want each event's once", got)
	}
}

func TestMarkDoneLeaseLostRollsBack(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	l := &ledger{}
	h.fail = func(e event.Event, call int) error {
		h.mu.Lock()
		ctx := h.ctxs[call]
		h.mu.Unlock()
		return f.manager.Within(ctx, func(ctx context.Context) error {
			if err := l.write(ctx, e.(OrderPaid).OrderID); err != nil {
				return err
			}
			// An operator skips the event mid-run, which releases the lease.
			if err := f.store.Skip(context.Background(), f.store.Records()[0].ID, 0); err != nil {
				return err
			}
			return outbox.MarkDone(ctx)
		})
	}
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

// TestWithoutMarkDoneStaysAtLeastOnce covers a handler that writes in two
// transactions and calls no MarkDone: a failure in the second reruns both.
func TestWithoutMarkDoneStaysAtLeastOnce(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	l := &ledger{}
	h.fail = func(_ event.Event, call int) error {
		h.mu.Lock()
		ctx := h.ctxs[call]
		h.mu.Unlock()
		for step, row := range []string{"a", "b"} {
			err := f.manager.Within(ctx, func(ctx context.Context) error {
				if call == 0 && step == 1 {
					return errors.New("second repository failed")
				}
				return l.write(ctx, row)
			})
			if err != nil {
				return err
			}
		}
		return nil
	}
	f.subscribe(h)
	f.run()
	if err := f.dispatch(OrderPaid{"o-1"}); err != nil {
		t.Fatal(err)
	}
	h.wait(t, 2)
	f.waitFor("the record to complete", f.settled(outbox.StatusDone))
	if got := l.all(); !slices.Equal(got, []string{"a", "a", "b"}) {
		t.Fatalf("committed writes %v, want the first rerun and the second once", got)
	}
}

// TestNextEventChecksLeaseAfterMarkDone covers an operator who skips the
// next event while the current one, done through MarkDone, finishes: the
// Relay checks the lease before running it.
func TestNextEventChecksLeaseAfterMarkDone(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	l := &ledger{}
	markDoneIn(h, f.manager, l, nil, func(_ context.Context, call int) error {
		if call == 0 {
			return f.store.Skip(context.Background(), f.store.Records()[0].ID, 1)
		}
		return nil
	})
	f.subscribe(h)
	f.run()
	if err := f.dispatch(OrderPaid{"o-1"}, OrderPaid{"o-2"}, OrderPaid{"o-3"}); err != nil {
		t.Fatal(err)
	}
	f.waitFor("the record to complete", f.settled(outbox.StatusDone))
	if got := h.events(); !slices.Equal(got, []event.Event{OrderPaid{"o-1"}, OrderPaid{"o-3"}}) {
		t.Fatalf("handled %v, want o-1 and o-3: o-2 was skipped before it ran", got)
	}
	if got := l.all(); !slices.Equal(got, []string{"o-1", "o-3"}) {
		t.Fatalf("committed writes %v, want o-1 and o-3", got)
	}
}
