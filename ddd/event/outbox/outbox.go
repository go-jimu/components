package outbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
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
	return nil
}

// Dispatch is DispatchAll with one event.
func (o *Outbox) Dispatch(ctx context.Context, e event.Event) error {
	return o.DispatchAll(ctx, []event.Event{e})
}

// DispatchAll adds events to the transaction ctx is in, all or none; see
// "Dispatch" for when it does and the errors it returns.
func (o *Outbox) DispatchAll(ctx context.Context, events []event.Event) error {
	if o.closed() {
		return event.ErrDispatcherClosed
	}
	subscribed := o.subscribed()
	for _, e := range events {
		if e == nil {
			return fmt.Errorf("%w: nil event", ErrInvalidEvent)
		}
		if listenedBy(subscribed, e.Kind()) {
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
	if b.flushed {
		return ErrDispatchAfterFlush
	}
	if !b.hooked {
		if err := transaction.BeforeCommit(ctx, o.flush); err != nil {
			return err
		}
		b.hooked = true
		if o.cfg.capture != nil {
			b.trace = o.cfg.capture(ctx)
		}
	}
	b.events = append(b.events, events...)
	return nil
}

// Close stops the Relay and closes the best-effort dispatcher, waiting until
// ctx ends; see "Shutdown".
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
			err = ctx.Err()
		}
	}
	if o.cfg.bestEffort != nil {
		err = errors.Join(err, o.cfg.bestEffort.Close(ctx))
	}
	return err
}

func (o *Outbox) closed() bool {
	select {
	case <-o.closing:
		return true
	default:
		return false
	}
}

// relaying reports whether this Outbox's Relay takes new records: it runs and
// is not shutting down.
func (o *Outbox) relaying() bool {
	return o.running.Load() && !o.closed()
}

type batchKey struct{ outbox *Outbox }

// batch is the events one transaction dispatched to one Outbox.
type batch struct {
	mu      sync.Mutex
	hooked  bool
	flushed bool
	trace   map[string]string
	events  []event.Event
}

func newBatch() *batch { return &batch{} }

// flush runs before commit: it writes one record per Handler and, after
// commit, hands the records to the Relay and the events to the best-effort
// dispatcher.
func (o *Outbox) flush(ctx context.Context) error {
	b, err := transaction.Value(ctx, batchKey{o}, newBatch)
	if err != nil {
		return err
	}
	b.mu.Lock()
	b.flushed = true
	events, trace := b.events, b.trace
	b.mu.Unlock()

	records, err := o.records(events, trace)
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
			default: // the records wait for their lease to expire; see "Leases"
			}
		}
		if o.cfg.bestEffort != nil {
			_ = o.cfg.bestEffort.DispatchAll(ctx, events) // the dispatcher logs what it rejects
		}
	})
}

// records returns one record per subscribed Handler that listens to any of
// events, holding those events in dispatch order.
func (o *Outbox) records(events []event.Event, trace map[string]string) ([]Record, error) {
	// While this Outbox's Relay takes records, a record is leased to it at
	// insert and handed to it after commit; otherwise it waits unleased for
	// any Relay.
	now := time.Now()
	leased := o.relaying()
	var records []Record
	for _, h := range o.subscribed() {
		kinds := h.Listening()
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
			Handler:       h.Name(),
			Events:        encoded,
			Status:        StatusPending,
			NextAttemptAt: now,
			Trace:         maps.Clone(trace),
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

// subscribed returns the subscribed Handlers ordered by name.
func (o *Outbox) subscribed() []Handler {
	o.mu.RLock()
	handlers := slices.Collect(maps.Values(o.handlers))
	o.mu.RUnlock()
	slices.SortFunc(handlers, func(a, b Handler) int { return strings.Compare(a.Name(), b.Name()) })
	return handlers
}

func listenedBy(handlers []Handler, kind event.Kind) bool {
	return slices.ContainsFunc(handlers, func(h Handler) bool { return slices.Contains(h.Listening(), kind) })
}

func handlerNames(handlers []Handler) []string {
	names := make([]string, len(handlers))
	for i, h := range handlers {
		names[i] = h.Name()
	}
	return names
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b) // crypto/rand.Read never fails
	return hex.EncodeToString(b)
}

func (o *Outbox) logError(ctx context.Context, msg string, err error, attrs ...slog.Attr) {
	o.cfg.logger.LogAttrs(ctx, slog.LevelError, msg, append(attrs, sloghelper.Error(err))...)
}
