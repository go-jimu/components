package outbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-jimu/components/ddd/event"
	"github.com/go-jimu/components/sloghelper"
	"github.com/go-jimu/components/transaction"
)

// Outbox records the events of a transaction for the Handlers that must run
// them, and runs them through its Relay after commit. It implements
// event.Dispatcher, so use cases dispatch to it as to any dispatcher, inside
// Within. See "Runtime composition". Its methods are safe for concurrent use.
type Outbox struct {
	store    Store
	registry *Registry
	cfg      config

	mu       sync.RWMutex
	handlers map[string]Handler
	byKind   map[event.Kind][]Handler
	runDone  chan struct{}
	cancel   context.CancelFunc

	hints     chan []Record
	running   atomic.Bool
	closing   chan struct{}
	closeOnce sync.Once
}

var _ event.Dispatcher = (*Outbox)(nil)

// New returns an Outbox that keeps records in store and encodes events with
// registry. It returns ErrNilStore or ErrNilRegistry.
func New(store Store, registry *Registry, opts ...Option) (*Outbox, error) {
	if store == nil {
		return nil, ErrNilStore
	}
	if registry == nil {
		return nil, ErrNilRegistry
	}
	cfg := defaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.owner == "" {
		cfg.owner = "outbox-" + randomHex(4)
	}
	return &Outbox{
		store:    store,
		registry: registry,
		cfg:      cfg,
		handlers: make(map[string]Handler),
		byKind:   make(map[event.Kind][]Handler),
		hints:    make(chan []Record, 1024),
		closing:  make(chan struct{}),
	}, nil
}

// Subscribe adds a Handler that must run. Subscribe every Handler before
// traffic starts. It returns ErrInvalidHandler, ErrDuplicateHandler, or
// ErrUnregisteredKind wrapped when the Registry lacks one of its kinds.
func (o *Outbox) Subscribe(h Handler) error {
	if h == nil || h.Name() == "" || len(h.Listening()) == 0 {
		return ErrInvalidHandler
	}
	for _, kind := range h.Listening() {
		if !o.registry.Registered(kind) {
			return fmt.Errorf("%w: %s, listened to by %s", ErrUnregisteredKind, kind, h.Name())
		}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, found := o.handlers[h.Name()]; found {
		return fmt.Errorf("%w: %s", ErrDuplicateHandler, h.Name())
	}
	o.handlers[h.Name()] = h
	for _, kind := range h.Listening() {
		o.byKind[kind] = append(o.byKind[kind], h)
	}
	return nil
}

// Dispatch is DispatchAll with one event.
func (o *Outbox) Dispatch(ctx context.Context, e event.Event) error {
	return o.DispatchAll(ctx, []event.Event{e})
}

// DispatchAll adds events to the transaction ctx is in; see "Dispatch". It
// returns transaction.ErrNoTransaction or transaction.ErrTransactionDone for
// a ctx outside a live transaction, ErrUnregisteredKind wrapped for an event a
// Handler listens to whose type is not registered, and
// event.ErrDispatcherClosed after Close.
func (o *Outbox) DispatchAll(ctx context.Context, events []event.Event) error {
	select {
	case <-o.closing:
		return event.ErrDispatcherClosed
	default:
	}
	for _, e := range events {
		if e == nil {
			return fmt.Errorf("%w: nil event", ErrUnregisteredKind)
		}
		if o.listened(e.Kind()) {
			if err := o.registry.check(e); err != nil {
				return err
			}
		}
	}
	b, err := transaction.Value(ctx, batchKey{o}, newBatch)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.hooked {
		// A flush takes the events dispatched so far; events dispatched by a
		// later BeforeCommit hook register the next flush.
		if err := transaction.BeforeCommit(ctx, o.flush); err != nil {
			return err
		}
		b.hooked = true
	}
	b.events = append(b.events, events...)
	return nil
}

// Close stops the Relay and closes the best-effort dispatcher; see
// "Shutdown". It waits for running records until ctx ends, then cancels
// their ctx; an interrupted record runs again once its lease expires.
func (o *Outbox) Close(ctx context.Context) error {
	o.closeOnce.Do(func() { close(o.closing) })
	o.mu.RLock()
	done, cancel := o.runDone, o.cancel
	o.mu.RUnlock()
	var err error
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			cancel()
			<-done
			err = ctx.Err()
		}
	}
	if o.cfg.bestEffort != nil {
		err = errors.Join(err, o.cfg.bestEffort.Close(ctx))
	}
	return err
}

type batchKey struct{ outbox *Outbox }

type batch struct {
	mu     sync.Mutex
	hooked bool
	events []event.Event
}

// take returns the buffered events and empties the batch.
func (b *batch) take() []event.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	events := b.events
	b.events, b.hooked = nil, false
	return events
}

func newBatch() *batch { return &batch{} }

func (o *Outbox) listened(kind event.Kind) bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return len(o.byKind[kind]) > 0
}

// flush runs before commit: it writes one record per Handler and hands the
// records to the Relay, and the events to the best-effort dispatcher, after
// commit.
func (o *Outbox) flush(ctx context.Context) error {
	b, err := transaction.Value(ctx, batchKey{o}, newBatch)
	if err != nil {
		return err
	}
	events := b.take()
	records, err := o.records(ctx, events)
	if err != nil {
		return err
	}
	if len(records) > 0 {
		if err := o.store.Insert(ctx, records); err != nil {
			return err
		}
	}
	return transaction.AfterCommit(ctx, func(ctx context.Context) {
		if len(records) > 0 && records[0].LeaseOwner != "" {
			select {
			case o.hints <- records:
			default: // the Relay claims them once their lease expires
			}
		}
		if o.cfg.bestEffort != nil {
			_ = o.cfg.bestEffort.DispatchAll(ctx, events) // the dispatcher logs what it rejects
		}
	})
}

func (o *Outbox) records(ctx context.Context, events []event.Event) ([]Record, error) {
	o.mu.RLock()
	names := make([]string, 0, len(o.handlers))
	for name := range o.handlers {
		names = append(names, name)
	}
	handlers := o.handlers
	o.mu.RUnlock()
	slices.Sort(names)

	var trace map[string]string
	if o.cfg.capture != nil {
		trace = o.cfg.capture(ctx)
	}
	// While this Outbox's Relay runs, a record is leased to it at insert and
	// handed to it after commit; otherwise it waits unleased for any Relay.
	now := time.Now()
	leased := o.running.Load()
	var records []Record
	for _, name := range names {
		kinds := handlers[name].Listening()
		var encoded []EncodedEvent
		for _, e := range events {
			if !slices.Contains(kinds, e.Kind()) {
				continue
			}
			enc, err := o.registry.Encode(e)
			if err != nil {
				return nil, err
			}
			encoded = append(encoded, enc)
		}
		if len(encoded) == 0 {
			continue
		}
		r := Record{
			ID:            randomHex(16),
			Handler:       name,
			Events:        encoded,
			Status:        StatusPending,
			NextAttemptAt: now,
			Trace:         trace,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if leased {
			r.Attempts, r.LeaseOwner, r.LeaseUntil = 1, o.cfg.owner, now.Add(o.cfg.lease)
		}
		records = append(records, r)
	}
	return records, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand.Read never fails
	return hex.EncodeToString(b)
}

func (o *Outbox) handler(name string) (Handler, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	h, ok := o.handlers[name]
	return h, ok
}

func (o *Outbox) handlerNames() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	names := make([]string, 0, len(o.handlers))
	for name := range o.handlers {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (o *Outbox) logError(ctx context.Context, msg string, err error, attrs ...slog.Attr) {
	o.cfg.logger.LogAttrs(ctx, slog.LevelError, msg, append(attrs, sloghelper.Error(err))...)
}
