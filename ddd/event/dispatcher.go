package event

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/go-jimu/components/sloghelper"
)

const (
	defaultBufferSize = 1024
	defaultWorkers    = 5
)

// queued is one accepted event waiting for, or being handled by, a worker.
type queued struct {
	seq    uint64
	key    string
	event  Event
	source context.Context
}

// InMemoryDispatcher handles events with a pool of workers. Events with the
// same key are handled one at a time in the order they were accepted; other
// events are handled concurrently.
type InMemoryDispatcher struct {
	mu                      sync.Mutex
	ready                   *sync.Cond
	spaceFreed              chan struct{}
	queue                   []queued
	waiters                 []uint64
	nextTicket              uint64
	inFlight                map[uint64]queued
	busyKeys                map[string]struct{}
	closing                 bool // Close has started; events are still accepted
	closed                  bool // events are rejected
	forced                  bool // Close's ctx ended; workers stop
	nextSeq                 uint64
	done                    chan struct{}
	handlers                map[Kind][]Handler
	logger                  *slog.Logger
	workers                 int
	keyFunc                 func(Event) string
	handlerTimeout          time.Duration
	contextFactory          func(context.Context, Event) context.Context
	unhandledEventHandler   func(UnhandledContext)
	panicHandler            func(PanicContext)
	closeInterruptedHandler func(CloseInterruptedContext)
	bufferSize              int
	rootCtx                 context.Context
	rootCancel              context.CancelFunc
}

var (
	_ Dispatcher = (*InMemoryDispatcher)(nil)
	_ Subscriber = (*InMemoryDispatcher)(nil)
)

// NewDispatcher creates an in-process dispatcher and starts its workers. The
// workers run until Close returns; call Close once during shutdown.
func NewDispatcher(opts ...Option) *InMemoryDispatcher {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	d := &InMemoryDispatcher{
		spaceFreed: make(chan struct{}),
		inFlight:   make(map[uint64]queued),
		busyKeys:   make(map[string]struct{}),
		done:       make(chan struct{}),
		handlers:   make(map[Kind][]Handler),
		logger:     slog.Default(),
		workers:    defaultWorkers,
		bufferSize: defaultBufferSize,
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
	}
	d.ready = sync.NewCond(&d.mu)

	for _, opt := range opts {
		opt(d)
	}

	var workers sync.WaitGroup
	for range d.workers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			d.run()
		}()
	}
	go func() {
		workers.Wait()
		close(d.done)
	}()
	return d
}

// Subscribe registers handler for each kind returned by handler.Listening.
// Handlers of one kind run in subscription order. A nil handler, and any
// Subscribe after Close has started, is ignored. It is safe for concurrent
// use.
func (d *InMemoryDispatcher) Subscribe(handler Handler) {
	if handler == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closing {
		return
	}
	for _, kind := range handler.Listening() {
		d.handlers[kind] = append(d.handlers[kind], handler)
	}
}

// Dispatch is DispatchAll with one event.
func (d *InMemoryDispatcher) Dispatch(ctx context.Context, event Event) error {
	return d.DispatchAll(ctx, []Event{event})
}

// DispatchAll accepts all events or none of them. When the queue cannot hold
// them, it waits until it can or until ctx is done; ctx does not reject events
// that can be queued immediately. Waiting calls are admitted first come, first
// served, so a later call never overtakes an earlier one. More events than the
// buffer size are accepted once the queue is empty. Nil events are skipped.
//
// It returns nil when the events are accepted, ErrDispatcherClosed once the
// dispatcher rejects events (see Close), and ctx.Err() when ctx ends while
// waiting. Rejected events are logged on the caller's context.
func (d *InMemoryDispatcher) DispatchAll(ctx context.Context, events []Event) error {
	accepted := make([]queued, 0, len(events))
	for _, event := range events {
		if event != nil {
			// Keys run user code, so they are computed before taking the lock.
			accepted = append(accepted, queued{key: d.keyOf(event), event: event})
		}
	}
	if len(accepted) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	source := context.WithoutCancel(ctx)

	var turn uint64 // 0 until this call joins the line for queue space
	for {
		d.mu.Lock()
		if d.closed {
			d.removeWaiterLocked(turn)
			d.mu.Unlock()
			d.logRejectedDispatch(ctx, accepted)
			return ErrDispatcherClosed
		}
		if d.isFirstWaiterLocked(turn) && d.fitsLocked(len(accepted)) {
			d.removeWaiterLocked(turn)
			for _, item := range accepted {
				d.nextSeq++
				item.seq = d.nextSeq
				item.source = source
				d.queue = append(d.queue, item)
			}
			d.ready.Broadcast()
			if len(d.waiters) > 0 {
				// The next waiter may fit in the space that is left.
				d.signalSpaceFreedLocked()
			}
			d.mu.Unlock()
			return nil
		}
		if turn == 0 {
			d.nextTicket++
			turn = d.nextTicket
			d.waiters = append(d.waiters, turn)
		}
		spaceFreed := d.spaceFreed
		d.mu.Unlock()

		select {
		case <-spaceFreed:
		case <-ctx.Done():
			d.mu.Lock()
			d.removeWaiterLocked(turn)
			// The waiter behind this one may now be first.
			d.signalSpaceFreedLocked()
			d.mu.Unlock()
			d.logRejectedDispatch(ctx, accepted)
			return ctx.Err()
		}
	}
}

func (d *InMemoryDispatcher) logRejectedDispatch(ctx context.Context, items []queued) {
	for _, item := range items {
		d.logger.LogAttrs(ctx, slog.LevelError, "domain event dispatch rejected", eventAttrs(item)...)
	}
}

// isFirstWaiterLocked reports whether turn may be admitted now: a call that has
// not joined the line goes first only when nobody is waiting.
func (d *InMemoryDispatcher) isFirstWaiterLocked(turn uint64) bool {
	if turn == 0 {
		return len(d.waiters) == 0
	}
	return d.waiters[0] == turn
}

func (d *InMemoryDispatcher) removeWaiterLocked(turn uint64) {
	if turn == 0 {
		return
	}
	if i := slices.Index(d.waiters, turn); i >= 0 {
		d.waiters = slices.Delete(d.waiters, i, i+1)
	}
}

func (d *InMemoryDispatcher) fitsLocked(n int) bool {
	return len(d.queue) == 0 || len(d.queue)+n <= d.bufferSize
}

// keyOf returns the ordering key of event, or "" when it has none. A Keyed
// event takes precedence over the configured key function.
func (d *InMemoryDispatcher) keyOf(event Event) string {
	if keyed, ok := event.(Keyed); ok {
		return keyed.Key()
	}
	if d.keyFunc != nil {
		return d.keyFunc(event)
	}
	return ""
}

// signalSpaceFreedLocked wakes every DispatchAll waiting for queue space or
// for the dispatcher to close.
func (d *InMemoryDispatcher) signalSpaceFreedLocked() {
	close(d.spaceFreed)
	d.spaceFreed = make(chan struct{})
}

// Close shuts the dispatcher down and returns nil once it is idle: the queue
// is empty and no handler is running. Until then it keeps accepting events,
// so the follow-up events that running handlers dispatch are handled too.
// Once idle, it rejects new events with ErrDispatcherClosed.
//
// Close relies on the runtime to stop every caller of Dispatch from outside
// the handlers before calling it; a caller that keeps dispatching keeps Close
// waiting until ctx ends. See the package documentation, "Shutdown".
//
// When ctx ends first, Close rejects new events, cancels the contexts of
// running handlers, drops the events not yet started, reports both (see
// WithCloseInterruptedHandler), and returns ctx.Err(). Close's ctx is the
// whole shutdown budget of the dispatcher, handler chains included.
func (d *InMemoryDispatcher) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	defer d.rootCancel()

	started := d.beginClose()
	if started {
		d.logger.LogAttrs(ctx, slog.LevelInfo, "domain event dispatcher closing started")
	}

	select {
	case <-d.done:
		if started {
			d.logger.LogAttrs(ctx, slog.LevelInfo, "domain event dispatcher closed")
		}
		return nil
	case <-ctx.Done():
		inFlight, pending := d.interruptClose()
		d.reportCloseInterrupted(ctx, ctx.Err(), inFlight, pending)
		return ctx.Err()
	}
}

// beginClose marks the dispatcher closing and reports whether this call
// started it.
func (d *InMemoryDispatcher) beginClose() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closing {
		return false
	}
	d.closing = true
	d.closeIfIdleLocked()
	return true
}

// closeIfIdleLocked rejects new events and lets the workers exit once Close
// has started, the queue is empty, and no handler is running: from then on no
// handler can dispatch a follow-up event.
func (d *InMemoryDispatcher) closeIfIdleLocked() {
	if d.closing && !d.closed && len(d.queue) == 0 && len(d.inFlight) == 0 {
		d.closed = true
		d.ready.Broadcast()
		d.signalSpaceFreedLocked()
	}
}

// interruptClose stops the dispatcher at once and returns the events that
// were running and those not yet started, each in acceptance order.
func (d *InMemoryDispatcher) interruptClose() (inFlight, pending []queued) {
	d.mu.Lock()
	d.closing = true
	d.closed = true
	d.forced = true
	for _, item := range d.inFlight {
		inFlight = append(inFlight, item)
	}
	slices.SortFunc(inFlight, func(a, b queued) int { return compareSeq(a.seq, b.seq) })
	pending = slices.Clone(d.queue)
	d.ready.Broadcast()
	d.signalSpaceFreedLocked()
	d.mu.Unlock()

	d.rootCancel()
	return inFlight, pending
}

func compareSeq(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// reportCloseInterrupted logs the interruption and hands the dropped events
// to WithCloseInterruptedHandler. Without the hook, nothing recovers them, so
// each one is logged at Error with its kind and key, on the ctx of the
// Dispatch call that queued it.
func (d *InMemoryDispatcher) reportCloseInterrupted(ctx context.Context, err error, inFlight, pending []queued) {
	level := slog.LevelError
	if d.closeInterruptedHandler != nil {
		level = slog.LevelWarn
	}
	d.logger.LogAttrs(ctx, level, "domain event dispatcher close interrupted",
		sloghelper.Error(err),
		slog.Int("in_flight_event_count", len(inFlight)),
		slog.Int("pending_event_count", len(pending)))

	if d.closeInterruptedHandler != nil {
		d.closeInterruptedHandler(CloseInterruptedContext{
			Error:    err,
			InFlight: eventsOf(inFlight),
			Pending:  eventsOf(pending),
		})
		return
	}
	for _, item := range inFlight {
		d.logger.LogAttrs(item.logContext(), slog.LevelError, "domain event dropped",
			append(eventAttrs(item), slog.String("event_state", "in_flight"))...)
	}
	for _, item := range pending {
		d.logger.LogAttrs(item.logContext(), slog.LevelError, "domain event dropped",
			append(eventAttrs(item), slog.String("event_state", "pending"))...)
	}
}

func eventsOf(items []queued) []Event {
	if len(items) == 0 {
		return nil
	}
	events := make([]Event, 0, len(items))
	for _, item := range items {
		events = append(events, item.event)
	}
	return events
}

func (d *InMemoryDispatcher) run() {
	for {
		next, ok := d.next()
		if !ok {
			return
		}
		d.handleEvent(next)
		d.finish(next)
	}
}

// next takes the oldest queued event whose key is not being handled by another
// worker and marks it in flight. Events without a key are always eligible.
func (d *InMemoryDispatcher) next() (queued, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for {
		if d.forced {
			return queued{}, false
		}
		if i := d.nextEligibleLocked(); i >= 0 {
			item := d.queue[i]
			d.queue = slices.Delete(d.queue, i, i+1)
			if item.key != "" {
				d.busyKeys[item.key] = struct{}{}
			}
			d.inFlight[item.seq] = item
			d.signalSpaceFreedLocked()
			return item, true
		}
		if d.closed {
			return queued{}, false
		}
		d.ready.Wait()
	}
}

func (d *InMemoryDispatcher) nextEligibleLocked() int {
	for i, item := range d.queue {
		if item.key == "" {
			return i
		}
		if _, busy := d.busyKeys[item.key]; !busy {
			return i
		}
	}
	return -1
}

// finish releases the event's key and wakes workers that may be waiting for
// it, or for the dispatcher to become idle during Close.
func (d *InMemoryDispatcher) finish(item queued) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.inFlight, item.seq)
	if item.key != "" {
		delete(d.busyKeys, item.key)
		d.ready.Broadcast()
	}
	d.closeIfIdleLocked()
}

func (d *InMemoryDispatcher) handleEvent(item queued) {
	event := item.event

	d.mu.Lock()
	handlers := append([]Handler(nil), d.handlers[event.Kind()]...)
	d.mu.Unlock()

	if len(handlers) == 0 {
		if d.unhandledEventHandler != nil {
			d.unhandledEventHandler(UnhandledContext{Event: event})
		} else {
			d.logger.LogAttrs(item.logContext(), slog.LevelWarn, "domain event has no handler", eventAttrs(item)...)
		}
		return
	}

	for _, handler := range handlers {
		d.handleOne(item, handler)
	}
}

func (d *InMemoryDispatcher) handleOne(item queued, handler Handler) {
	event := item.event
	defer func() {
		if recovered := recover(); recovered != nil {
			stack := debug.Stack()
			if d.panicHandler != nil {
				d.panicHandler(PanicContext{
					Event:   event,
					Handler: handler,
					Panic:   recovered,
					Stack:   stack,
				})
				return
			}
			d.logger.LogAttrs(item.logContext(), slog.LevelError, "domain event handler panicked", append(eventAttrs(item),
				slog.String("event_handler", fmt.Sprintf("%T", handler)),
				slog.Any("event", event),
				slog.Any("panic", recovered),
				slog.String("stack_trace", string(stack)))...)
		}
	}()

	ctx, cancel := d.handlerContext(item)
	defer cancel()
	handler.Handle(ctx, event)
}

func (d *InMemoryDispatcher) handlerContext(item queued) (context.Context, context.CancelFunc) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	if d.handlerTimeout > 0 {
		ctx, cancel = context.WithTimeout(d.rootCtx, d.handlerTimeout)
	} else {
		ctx, cancel = context.WithCancel(d.rootCtx)
	}
	ctx = context.WithValue(ctx, sourceContextKey{}, item.source)

	if d.contextFactory != nil {
		if derived := d.contextFactory(ctx, item.event); derived != nil {
			ctx = derived
		} else {
			d.logger.LogAttrs(item.logContext(), slog.LevelWarn,
				"domain event context factory returned nil", eventAttrs(item)...)
		}
	}
	return ctx, cancel
}

// logContext is the ctx of the Dispatch call that queued item, without its
// cancellation, so logs about item carry its trace attributes.
func (item queued) logContext() context.Context {
	if item.source == nil {
		return context.Background()
	}
	return item.source
}

func eventAttrs(item queued) []slog.Attr {
	attrs := []slog.Attr{slog.String("event_kind", string(item.event.Kind()))}
	if item.key != "" {
		attrs = append(attrs, slog.String("event_key", item.key))
	}
	return attrs
}
