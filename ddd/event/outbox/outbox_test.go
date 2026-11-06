package outbox_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/event"
	"github.com/go-jimu/components/ddd/event/outbox"
	"github.com/go-jimu/components/ddd/event/outbox/outboxtest"
	"github.com/go-jimu/components/transaction"
	"github.com/go-jimu/components/transaction/transactiontest"
)

type OrderPaid struct {
	OrderID string `json:"order_id"`
}

func (OrderPaid) Kind() event.Kind { return "order.paid" }

type OrderShipped struct {
	OrderID string `json:"order_id"`
}

func (OrderShipped) Kind() event.Kind { return "order.shipped" }

type Unregistered struct{}

func (Unregistered) Kind() event.Kind { return "unregistered" }

// handler records the events it runs and answers with fail.
type handler struct {
	name  string
	kinds []event.Kind

	mu         sync.Mutex
	handled    []event.Event
	deliveries []outbox.Delivery
	ctxs       []context.Context
	fail       func(e event.Event, call int) error
	ran        chan struct{}
}

func newHandler(name string, kinds ...event.Kind) *handler {
	return &handler{name: name, kinds: kinds, ran: make(chan struct{}, 100)}
}

func (h *handler) Name() string            { return h.name }
func (h *handler) Listening() []event.Kind { return h.kinds }

func (h *handler) Handle(ctx context.Context, e event.Event) error {
	h.mu.Lock()
	call := len(h.handled)
	h.handled = append(h.handled, e)
	d, _ := outbox.DeliveryFrom(ctx)
	h.deliveries = append(h.deliveries, d)
	h.ctxs = append(h.ctxs, ctx)
	fail := h.fail
	h.mu.Unlock()
	defer func() { h.ran <- struct{}{} }()
	if fail != nil {
		return fail(e, call)
	}
	return nil
}

func (h *handler) events() []event.Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.handled)
}

func (h *handler) wait(t *testing.T, runs int) {
	t.Helper()
	for range runs {
		select {
		case <-h.ran:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s ran %d events, want %d more", h.name, len(h.events()), runs)
		}
	}
}

type fixture struct {
	t       *testing.T
	driver  *transactiontest.Driver
	manager *transaction.Manager
	store   *outboxtest.Store
	outbox  *outbox.Outbox
}

func newFixture(t *testing.T, opts ...outbox.Option) *fixture {
	t.Helper()
	driver := &transactiontest.Driver{}
	manager, err := transaction.NewManager(driver)
	if err != nil {
		t.Fatal(err)
	}
	registry := outbox.NewRegistry()
	if err := outbox.Register[OrderPaid](registry); err != nil {
		t.Fatal(err)
	}
	if err := outbox.Register[OrderShipped](registry); err != nil {
		t.Fatal(err)
	}
	store := &outboxtest.Store{}
	opts = append([]outbox.Option{
		outbox.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
		outbox.WithPollInterval(5 * time.Millisecond),
		outbox.WithBackoff(time.Millisecond, time.Millisecond),
	}, opts...)
	o, err := outbox.New(store, registry, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, driver: driver, manager: manager, store: store, outbox: o}
}

func (f *fixture) subscribe(handlers ...*handler) {
	f.t.Helper()
	for _, h := range handlers {
		if err := f.outbox.Subscribe(h); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *fixture) dispatch(events ...event.Event) error {
	return f.manager.Within(context.Background(), func(ctx context.Context) error {
		return f.outbox.DispatchAll(ctx, events)
	})
}

// run starts the Relay and stops it when the test ends.
func (f *fixture) run() {
	f.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.outbox.Run(ctx) }()
	f.t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			f.t.Errorf("Run = %v", err)
		}
	})
	f.waitFor("the Relay to start", func() bool { return outbox.Running(f.outbox) })
}

func (f *fixture) waitFor(what string, cond func() bool) {
	f.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			f.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func (f *fixture) statuses() []outbox.Status {
	var out []outbox.Status
	for _, r := range f.store.Records() {
		out = append(out, r.Status)
	}
	return out
}

func (f *fixture) settled(status outbox.Status) func() bool {
	return func() bool {
		records := f.store.Records()
		for _, r := range records {
			if r.Status != status {
				return false
			}
		}
		return len(records) > 0
	}
}

func TestDispatchNeedsTransaction(t *testing.T) {
	f := newFixture(t)
	f.subscribe(newHandler("projector", OrderPaid{}.Kind()))
	err := f.outbox.Dispatch(context.Background(), OrderPaid{OrderID: "o-1"})
	if !errors.Is(err, transaction.ErrNoTransaction) {
		t.Fatalf("Dispatch outside a transaction = %v, want ErrNoTransaction", err)
	}
}

func TestDispatchRecordsPerHandler(t *testing.T) {
	f := newFixture(t)
	paid := newHandler("projector", OrderPaid{}.Kind(), OrderShipped{}.Kind())
	shipped := newHandler("notifier", OrderShipped{}.Kind())
	idle := newHandler("idle", event.Kind("order.cancelled"))
	f.subscribe(paid, shipped)
	if err := f.outbox.Subscribe(idle); !errors.Is(err, outbox.ErrUnregisteredKind) {
		t.Fatalf("Subscribe of an unregistered kind = %v, want ErrUnregisteredKind", err)
	}

	err := f.dispatch(OrderPaid{OrderID: "o-1"}, OrderShipped{OrderID: "o-1"}, Unregistered{})
	if err != nil {
		t.Fatalf("dispatch = %v", err)
	}
	records := f.store.Records()
	if len(records) != 2 {
		t.Fatalf("stored %d records, want one per listening handler", len(records))
	}
	byHandler := map[string]outbox.Record{}
	for _, r := range records {
		byHandler[r.Handler] = r
		if r.Status != outbox.StatusPending || r.LeaseOwner != "" || r.Attempts != 0 {
			t.Fatalf("record %+v, want pending and unleased while no Relay runs", r)
		}
	}
	got := byHandler["projector"].Events
	if len(got) != 2 || got[0].Kind != "order.paid" || got[1].Kind != "order.shipped" ||
		string(got[0].Payload) != `{"order_id":"o-1"}` {
		t.Fatalf("projector events = %v, want order.paid then order.shipped", got)
	}
	if got := byHandler["notifier"].Events; len(got) != 1 || got[0].Kind != "order.shipped" {
		t.Fatalf("notifier events = %v, want order.shipped", got)
	}
}

func TestDispatchRolledBack(t *testing.T) {
	f := newFixture(t)
	f.subscribe(newHandler("projector", OrderPaid{}.Kind()))
	boom := errors.New("boom")
	err := f.manager.Within(context.Background(), func(ctx context.Context) error {
		if err := f.outbox.Dispatch(ctx, OrderPaid{OrderID: "o-1"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Within = %v, want boom", err)
	}
	if records := f.store.Records(); len(records) != 0 {
		t.Fatalf("stored %d records of a rolled back transaction", len(records))
	}
}

func TestDispatchRejectsUnregisteredType(t *testing.T) {
	f := newFixture(t)
	f.subscribe(newHandler("projector", OrderPaid{}.Kind()))
	var dispatchErr error
	err := f.manager.Within(context.Background(), func(ctx context.Context) error {
		dispatchErr = f.outbox.DispatchAll(ctx, []event.Event{OrderPaid{OrderID: "o-1"}, &OrderPaid{OrderID: "o-2"}})
		return dispatchErr
	})
	if !errors.Is(dispatchErr, outbox.ErrUnregisteredKind) || !errors.Is(err, outbox.ErrUnregisteredKind) {
		t.Fatalf("DispatchAll of *OrderPaid = %v, Within = %v, want ErrUnregisteredKind from both", dispatchErr, err)
	}
	if last := f.driver.Last(); last.State() != transactiontest.RolledBack {
		t.Fatalf("transaction %s, want rolled back", last.State())
	}
	if records := f.store.Records(); len(records) != 0 {
		t.Fatalf("stored %d records", len(records))
	}
}

func TestDispatchFromBeforeCommitHook(t *testing.T) {
	f := newFixture(t)
	f.subscribe(newHandler("projector", OrderPaid{}.Kind(), OrderShipped{}.Kind()))
	err := f.manager.Within(context.Background(), func(ctx context.Context) error {
		// A hook registered before the first dispatch, such as a unit of work
		// that saves aggregates and dispatches their events, runs before the
		// Outbox writes its records.
		if err := transaction.BeforeCommit(ctx, func(ctx context.Context) error {
			return f.outbox.Dispatch(ctx, OrderShipped{OrderID: "o-1"})
		}); err != nil {
			return err
		}
		return f.outbox.Dispatch(ctx, OrderPaid{OrderID: "o-1"})
	})
	if err != nil {
		t.Fatalf("Within = %v", err)
	}
	records := f.store.Records()
	if len(records) != 1 {
		t.Fatalf("stored %d records, want one for the transaction", len(records))
	}
	var kinds []event.Kind
	for _, e := range records[0].Events {
		kinds = append(kinds, e.Kind)
	}
	if !slices.Equal(kinds, []event.Kind{"order.paid", "order.shipped"}) {
		t.Fatalf("recorded kinds %v, want both events in dispatch order", kinds)
	}
}

func TestDispatchAfterFlush(t *testing.T) {
	f := newFixture(t)
	f.subscribe(newHandler("projector", OrderPaid{}.Kind(), OrderShipped{}.Kind()))
	var lateErr error
	err := f.manager.Within(context.Background(), func(ctx context.Context) error {
		if err := f.outbox.Dispatch(ctx, OrderPaid{OrderID: "o-1"}); err != nil {
			return err
		}
		return transaction.BeforeCommit(ctx, func(ctx context.Context) error {
			lateErr = f.outbox.Dispatch(ctx, OrderShipped{OrderID: "o-1"})
			return lateErr
		})
	})
	if !errors.Is(lateErr, outbox.ErrDispatchAfterFlush) || !errors.Is(err, outbox.ErrDispatchAfterFlush) {
		t.Fatalf("dispatch from a later hook = %v, Within = %v, want ErrDispatchAfterFlush", lateErr, err)
	}
	if records := f.store.Records(); len(records) != 0 {
		t.Fatalf("stored %d records of a rolled back transaction", len(records))
	}
}

func TestDispatchInvalidEvent(t *testing.T) {
	f := newFixture(t)
	if err := f.dispatch(nil); !errors.Is(err, outbox.ErrInvalidEvent) {
		t.Fatalf("dispatch of nil = %v, want ErrInvalidEvent", err)
	}
}

func TestSubscribeDuringCommits(t *testing.T) {
	f := newFixture(t)
	f.subscribe(newHandler("projector", OrderPaid{}.Kind()))
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range 20 {
			_ = f.outbox.Subscribe(newHandler(fmt.Sprintf("late-%d", i), OrderPaid{}.Kind()))
		}
	})
	for range 20 {
		if err := f.dispatch(OrderPaid{OrderID: "o-1"}); err != nil {
			t.Fatalf("dispatch = %v", err)
		}
	}
	wg.Wait()
}

func TestRelayRunsAfterCommit(t *testing.T) {
	f := newFixture(t, outbox.WithPollInterval(time.Hour)) // only the commit hint can start it
	h := newHandler("projector", OrderPaid{}.Kind(), OrderShipped{}.Kind())
	f.subscribe(h)
	f.run()

	if err := f.dispatch(OrderPaid{OrderID: "o-1"}, OrderShipped{OrderID: "o-1"}); err != nil {
		t.Fatalf("dispatch = %v", err)
	}
	h.wait(t, 2)
	f.waitFor("the record to complete", f.settled(outbox.StatusDone))
	if got := h.events(); !slices.Equal(got, []event.Event{OrderPaid{OrderID: "o-1"}, OrderShipped{OrderID: "o-1"}}) {
		t.Fatalf("handled %v, want both events in order", got)
	}
	r := f.store.Records()[0]
	if h.deliveries[0] != (outbox.Delivery{RecordID: r.ID, Index: 0, Attempt: 1}) ||
		h.deliveries[1].Key() != r.ID+"/1" {
		t.Fatalf("deliveries %+v, want record %s at indexes 0 and 1", h.deliveries, r.ID)
	}
}

func TestRelayClaimsUnleasedRecords(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	f.subscribe(h)
	if err := f.dispatch(OrderPaid{OrderID: "o-1"}); err != nil {
		t.Fatalf("dispatch = %v", err)
	}
	f.run()
	h.wait(t, 1)
	f.waitFor("the record to complete", f.settled(outbox.StatusDone))
}

func TestRelayRetriesFromProgress(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind(), OrderShipped{}.Kind())
	h.fail = func(_ event.Event, call int) error {
		switch call {
		case 1:
			return errors.New("database unavailable")
		case 2:
			panic("handler bug fixed by the next deploy")
		default:
			return nil
		}
	}
	f.subscribe(h)
	f.run()
	if err := f.dispatch(OrderPaid{OrderID: "o-1"}, OrderShipped{OrderID: "o-1"}); err != nil {
		t.Fatalf("dispatch = %v", err)
	}
	h.wait(t, 4)
	f.waitFor("the record to complete", f.settled(outbox.StatusDone))
	want := []event.Event{OrderPaid{"o-1"}, OrderShipped{"o-1"}, OrderShipped{"o-1"}, OrderShipped{"o-1"}}
	if got := h.events(); !slices.Equal(got, want) {
		t.Fatalf("handled %v, want the first event once and the second until it succeeds", got)
	}
	r := f.store.Records()[0]
	if r.Attempts != 3 || r.LastError == "" {
		t.Fatalf("record attempts %d, last error %q, want 3 attempts and the last error kept", r.Attempts, r.LastError)
	}
	if key := h.deliveries[1].Key(); key != h.deliveries[3].Key() {
		t.Fatalf("delivery keys %s and %s differ across retries", key, h.deliveries[3].Key())
	}
}

func TestRelayFailsPermanentErrorsAndRetriesThemAtStartup(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	var recovered atomic.Bool
	h.fail = func(event.Event, int) error {
		if !recovered.Load() {
			return fmt.Errorf("%w: account closed", outbox.ErrPermanent)
		}
		return nil
	}
	f.subscribe(h)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.outbox.Run(ctx) }()
	if err := f.dispatch(OrderPaid{OrderID: "o-1"}); err != nil {
		t.Fatalf("dispatch = %v", err)
	}
	h.wait(t, 1)
	f.waitFor("the record to fail", f.settled(outbox.StatusFailed))
	time.Sleep(20 * time.Millisecond) // several polls
	if n := len(h.events()); n != 1 {
		t.Fatalf("ran a failed record %d times, want once", n)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run = %v", err)
	}

	recovered.Store(true)
	f.run()
	h.wait(t, 1)
	f.waitFor("the record to complete after restart", f.settled(outbox.StatusDone))
}

func TestRelayFailsUndecodableRecords(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	f.subscribe(h)
	err := f.manager.Within(context.Background(), func(ctx context.Context) error {
		return f.store.Insert(ctx, []outbox.Record{{
			ID: "r-1", Handler: "projector", Status: outbox.StatusPending,
			Events: []outbox.EncodedEvent{{Kind: "order.paid", Payload: []byte(`not json`)}},
		}})
	})
	if err != nil {
		t.Fatal(err)
	}
	f.run()
	f.waitFor("the record to fail", f.settled(outbox.StatusFailed))
	if n := len(h.events()); n != 0 {
		t.Fatalf("handler ran %d times for an undecodable event", n)
	}
}

func TestRelayClaimsOnlySubscribedHandlers(t *testing.T) {
	f := newFixture(t)
	f.subscribe(newHandler("projector", OrderPaid{}.Kind()))
	err := f.manager.Within(context.Background(), func(ctx context.Context) error {
		return f.store.Insert(ctx, []outbox.Record{{
			ID: "r-1", Handler: "retired", Status: outbox.StatusPending,
			Events: []outbox.EncodedEvent{{Kind: "order.paid", Payload: []byte(`{}`)}},
		}})
	})
	if err != nil {
		t.Fatal(err)
	}
	f.run()
	time.Sleep(20 * time.Millisecond)
	if got := f.store.Records()[0]; got.Status != outbox.StatusPending || got.Attempts != 0 {
		t.Fatalf("record of an unsubscribed handler = %+v, want it left pending and unclaimed", got)
	}
}

func TestRunTwice(t *testing.T) {
	f := newFixture(t)
	f.run()
	if err := f.outbox.Run(context.Background()); !errors.Is(err, outbox.ErrAlreadyRunning) {
		t.Fatalf("second Run = %v, want ErrAlreadyRunning", err)
	}
}

func TestCloseWaitsForRunningRecords(t *testing.T) {
	f := newFixture(t)
	h := newHandler("projector", OrderPaid{}.Kind())
	release := make(chan struct{})
	started := make(chan struct{})
	h.fail = func(event.Event, int) error {
		close(started)
		<-release
		return nil
	}
	f.subscribe(h)
	done := make(chan error, 1)
	go func() { done <- f.outbox.Run(context.Background()) }()
	if err := f.dispatch(OrderPaid{OrderID: "o-1"}); err != nil {
		t.Fatalf("dispatch = %v", err)
	}
	<-started

	closed := make(chan error, 1)
	go func() { closed <- f.outbox.Close(context.Background()) }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned %v while a record ran", err)
	case <-time.After(20 * time.Millisecond):
	}
	if err := f.dispatch(OrderPaid{OrderID: "o-2"}); !errors.Is(err, event.ErrDispatcherClosed) {
		t.Fatalf("dispatch after Close = %v, want ErrDispatcherClosed", err)
	}
	close(release)
	if err := <-closed; err != nil {
		t.Fatalf("Close = %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Run = %v", err)
	}
	if got := f.statuses(); !slices.Equal(got, []outbox.Status{outbox.StatusDone}) {
		t.Fatalf("statuses %v, want the running record done", got)
	}
	if err := f.outbox.Run(context.Background()); err != nil {
		t.Fatalf("Run after Close = %v, want nil", err)
	}
}

func TestCloseDeadlineCancelsRunningRecords(t *testing.T) {
	f := newFixture(t, outbox.WithBackoff(time.Hour, time.Hour))
	h := newHandler("projector", OrderPaid{}.Kind())
	started := make(chan struct{})
	h.fail = func(event.Event, int) error {
		close(started)
		h.mu.Lock()
		ctx := h.ctxs[0]
		h.mu.Unlock()
		<-ctx.Done()
		return ctx.Err()
	}
	f.subscribe(h)
	done := make(chan error, 1)
	go func() { done <- f.outbox.Run(context.Background()) }()
	if err := f.dispatch(OrderPaid{OrderID: "o-1"}); err != nil {
		t.Fatalf("dispatch = %v", err)
	}
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := f.outbox.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close = %v, want DeadlineExceeded", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Run = %v", err)
	}
	r := f.store.Records()[0]
	if r.Status != outbox.StatusPending || r.LeaseOwner != "" || r.NextAttemptAt.After(time.Now()) {
		t.Fatalf("interrupted record %+v, want pending, released, and due now", r)
	}
}

func TestCloseReturnsAtDeadline(t *testing.T) {
	t.Run("handler ignores ctx", func(t *testing.T) {
		f := newFixture(t)
		h := newHandler("projector", OrderPaid{}.Kind())
		release, started := make(chan struct{}), make(chan struct{})
		h.fail = func(event.Event, int) error {
			close(started)
			<-release
			return nil
		}
		f.subscribe(h)
		done := make(chan error, 1)
		go func() { done <- f.outbox.Run(context.Background()) }()
		if err := f.dispatch(OrderPaid{OrderID: "o-1"}); err != nil {
			t.Fatalf("dispatch = %v", err)
		}
		<-started
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		if err := f.outbox.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Close = %v, want DeadlineExceeded", err)
		}
		close(release)
		if err := <-done; err != nil {
			t.Fatalf("Run = %v", err)
		}
	})
	t.Run("store hangs", func(t *testing.T) {
		f := newFixture(t)
		f.subscribe(newHandler("projector", OrderPaid{}.Kind()))
		hanging := &hangingStore{Store: f.store, claiming: make(chan struct{}, 1)}
		registry := outbox.NewRegistry()
		_ = outbox.Register[OrderPaid](registry)
		o, err := outbox.New(hanging, registry, outbox.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- o.Run(context.Background()) }()
		<-hanging.claiming
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		if err := o.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Close = %v, want DeadlineExceeded", err)
		}
		if err := <-done; err != nil {
			t.Fatalf("Run = %v", err)
		}
	})
}

// TestCloseStartsNothingQueued repeats its scenario because select picks
// among ready cases at random: a regression shows in some runs only.
func TestCloseStartsNothingQueued(t *testing.T) {
	for i := range 10 {
		t.Run(fmt.Sprint(i), closeStartsNothingQueued)
	}
}

func closeStartsNothingQueued(t *testing.T) {
	f := newFixture(t, outbox.WithWorkers(8))
	h := newHandler("projector", OrderPaid{}.Kind())
	f.subscribe(h)
	gate := &gatedStore{Store: f.store, entered: make(chan struct{}), open: make(chan struct{})}
	registry := outbox.NewRegistry()
	_ = outbox.Register[OrderPaid](registry)
	o, err := outbox.New(gate, registry, outbox.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Subscribe(h); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- o.Run(context.Background()) }()
	<-gate.entered // the Relay takes records but its loop has not started
	for i := range 20 {
		err := f.manager.Within(context.Background(), func(ctx context.Context) error {
			return o.Dispatch(ctx, OrderPaid{OrderID: fmt.Sprintf("o-%d", i)})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	closed := make(chan error, 1)
	go func() { closed <- o.Close(context.Background()) }()
	f.waitFor("Close to start", func() bool {
		return errors.Is(f.manager.Within(context.Background(), func(ctx context.Context) error {
			return o.Dispatch(ctx, OrderPaid{OrderID: "probe"})
		}), event.ErrDispatcherClosed)
	})
	close(gate.open)
	if err := <-closed; err != nil {
		t.Fatalf("Close = %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Run = %v", err)
	}
	if n := len(h.events()); n != 0 {
		t.Fatalf("started %d records after Close", n)
	}
	for _, r := range f.store.Records() {
		if r.LeaseOwner != "" {
			t.Fatalf("record %s still leased after Close", r.ID)
		}
	}
}

// gatedStore blocks RetryFailed, the Relay's first call, until open closes.
type gatedStore struct {
	*outboxtest.Store
	entered, open chan struct{}
}

func (s *gatedStore) RetryFailed(ctx context.Context, now time.Time) (int, error) {
	close(s.entered)
	<-s.open
	return s.Store.RetryFailed(ctx, now)
}

// hangingStore blocks RetryFailed until ctx ends, as a database that stopped
// answering.
type hangingStore struct {
	*outboxtest.Store
	claiming chan struct{}
}

func (s *hangingStore) RetryFailed(ctx context.Context, _ time.Time) (int, error) {
	s.claiming <- struct{}{}
	<-ctx.Done()
	return 0, ctx.Err()
}

// blockFirst makes h block on its first event until release closes or its
// ctx ends, and closes started once it blocks.
func blockFirst(h *handler, started, release chan struct{}) {
	h.fail = func(_ event.Event, call int) error {
		if call > 0 {
			return nil
		}
		h.mu.Lock()
		ctx := h.ctxs[0]
		h.mu.Unlock()
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func TestRelaySkipsHandedOverRecordsWhoseLeaseExpired(t *testing.T) {
	f := newFixture(t, outbox.WithWorkers(1), outbox.WithLease(50*time.Millisecond))
	h := newHandler("projector", OrderPaid{}.Kind())
	started, release := make(chan struct{}), make(chan struct{})
	blockFirst(h, started, release)
	f.subscribe(h)
	f.run()
	if err := f.dispatch(OrderPaid{OrderID: "o-1"}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := f.dispatch(OrderPaid{OrderID: "o-2"}); err != nil { // waits for the one worker
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // past o-2's lease
	close(release)
	h.wait(t, 2)
	f.waitFor("both records to complete", f.settled(outbox.StatusDone))
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.deliveries) != 2 || h.deliveries[1].Attempt != 2 {
		t.Fatalf("deliveries %+v, want o-2 run once, under the lease of a claim (attempt 2)", h.deliveries)
	}
}

func TestCloseReleasesRecordsNotStarted(t *testing.T) {
	f := newFixture(t, outbox.WithWorkers(1), outbox.WithPollInterval(time.Hour))
	h := newHandler("projector", OrderPaid{}.Kind())
	started := make(chan struct{})
	blockFirst(h, started, make(chan struct{}))
	f.subscribe(h)
	done := make(chan error, 1)
	go func() { done <- f.outbox.Run(context.Background()) }()
	f.waitFor("the Relay to start", func() bool { return outbox.Running(f.outbox) })
	for _, id := range []string{"o-1", "o-2", "o-3"} {
		if err := f.dispatch(OrderPaid{OrderID: id}); err != nil {
			t.Fatal(err)
		}
		if id == "o-1" {
			<-started
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_ = f.outbox.Close(ctx)
	if err := <-done; err != nil {
		t.Fatalf("Run = %v", err)
	}
	for _, r := range f.store.Records() {
		if r.Status != outbox.StatusPending || r.LeaseOwner != "" {
			t.Fatalf("record %+v after Close, want pending and released", r)
		}
	}
	if n := len(h.events()); n != 1 {
		t.Fatalf("handler ran %d events, want only o-1", n)
	}
}

// TestStoppingRelayLeasesNothing covers a Relay whose ctx has ended while a
// handler that ignores ctx still runs: it takes no new records, and the ones
// handed over to it are released without waiting for that handler.
func TestStoppingRelayLeasesNothing(t *testing.T) {
	f := newFixture(t, outbox.WithWorkers(1), outbox.WithPollInterval(time.Hour), outbox.WithLease(time.Hour))
	h := newHandler("projector", OrderPaid{}.Kind())
	started, release := make(chan struct{}), make(chan struct{})
	h.fail = func(_ event.Event, call int) error {
		if call == 0 {
			close(started)
			<-release // ignores its ctx
		}
		return nil
	}
	f.subscribe(h)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.outbox.Run(ctx) }()
	f.waitFor("the Relay to start", func() bool { return outbox.Running(f.outbox) })
	for _, id := range []string{"o-1", "o-2", "o-3"} { // o-2 waits for the worker, o-3 in the hand-over queue
		if err := f.dispatch(OrderPaid{OrderID: id}); err != nil {
			t.Fatal(err)
		}
		if id == "o-1" {
			<-started
		}
	}
	cancel()
	f.waitFor("the loop to stop taking records", func() bool { return !outbox.Running(f.outbox) })
	if err := f.dispatch(OrderPaid{OrderID: "o-4"}); err != nil {
		t.Fatal(err)
	}
	f.waitFor("o-2 and o-3 to be released while o-1 runs", func() bool {
		released := 0
		for _, r := range f.store.Records() {
			if r.LeaseOwner == "" {
				released++
			}
		}
		return released == 3 // o-2, o-3, and o-4 inserted unleased
	})
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Run = %v", err)
	}
}

func TestBestEffortGetsCommittedEvents(t *testing.T) {
	best := &recordingDispatcher{}
	f := newFixture(t, outbox.WithBestEffort(best))
	f.subscribe(newHandler("projector", OrderPaid{}.Kind()))
	if err := f.dispatch(OrderPaid{OrderID: "o-1"}, Unregistered{}); err != nil {
		t.Fatalf("dispatch = %v", err)
	}
	_ = f.manager.Within(context.Background(), func(ctx context.Context) error {
		_ = f.outbox.Dispatch(ctx, OrderPaid{OrderID: "o-2"})
		return errors.New("rollback")
	})
	if got := best.all(); !slices.Equal(got, []event.Event{OrderPaid{OrderID: "o-1"}, Unregistered{}}) {
		t.Fatalf("best-effort dispatcher got %v, want the committed events only", got)
	}
	if err := f.outbox.Close(context.Background()); err != nil || !best.closed {
		t.Fatalf("Close = %v, best-effort closed %v, want it closed", err, best.closed)
	}
}

func TestTraceCrossesIntoRelay(t *testing.T) {
	type traceKey struct{}
	f := newFixture(t, outbox.WithTrace(
		func(ctx context.Context) map[string]string {
			id, _ := ctx.Value(traceKey{}).(string)
			return map[string]string{"trace": id}
		},
		func(ctx context.Context, carrier map[string]string) context.Context {
			return context.WithValue(ctx, traceKey{}, carrier["trace"])
		},
	))
	h := newHandler("projector", OrderPaid{}.Kind())
	f.subscribe(h)
	f.run()
	err := f.manager.Within(context.Background(), func(ctx context.Context) error {
		// A span started inside the use case is the one captured.
		ctx = context.WithValue(ctx, traceKey{}, "trace-1")
		return f.outbox.Dispatch(ctx, OrderPaid{OrderID: "o-1"})
	})
	if err != nil {
		t.Fatal(err)
	}
	h.wait(t, 1)
	h.mu.Lock()
	got := h.ctxs[0].Value(traceKey{})
	h.mu.Unlock()
	if got != "trace-1" {
		t.Fatalf("handler trace = %v, want trace-1", got)
	}
}

func TestSubscribe(t *testing.T) {
	f := newFixture(t)
	for name, h := range map[string]outbox.Handler{
		"nil":          nil,
		"no name":      newHandler("", OrderPaid{}.Kind()),
		"no listening": newHandler("projector"),
	} {
		if err := f.outbox.Subscribe(h); !errors.Is(err, outbox.ErrInvalidHandler) {
			t.Errorf("Subscribe(%s) = %v, want ErrInvalidHandler", name, err)
		}
	}
	f.subscribe(newHandler("projector", OrderPaid{}.Kind()))
	if err := f.outbox.Subscribe(newHandler("projector", OrderShipped{}.Kind())); !errors.Is(err, outbox.ErrDuplicateHandler) {
		t.Fatalf("Subscribe of a taken name = %v, want ErrDuplicateHandler", err)
	}
}

func TestNew(t *testing.T) {
	if _, err := outbox.New(nil, outbox.NewRegistry()); !errors.Is(err, outbox.ErrNilStore) {
		t.Errorf("New(nil store) = %v, want ErrNilStore", err)
	}
	if _, err := outbox.New(&outboxtest.Store{}, nil); !errors.Is(err, outbox.ErrNilRegistry) {
		t.Errorf("New(nil registry) = %v, want ErrNilRegistry", err)
	}
}

type recordingDispatcher struct {
	mu     sync.Mutex
	events []event.Event
	closed bool
}

func (d *recordingDispatcher) Dispatch(ctx context.Context, e event.Event) error {
	return d.DispatchAll(ctx, []event.Event{e})
}

func (d *recordingDispatcher) DispatchAll(_ context.Context, events []event.Event) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, events...)
	return nil
}

func (d *recordingDispatcher) Close(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return nil
}

func (d *recordingDispatcher) all() []event.Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.events)
}
