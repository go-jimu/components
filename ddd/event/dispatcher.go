package event

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"sync"
	"time"
)

const (
	defaultBufferSize = 1024
	defaultDelayClose = 5 * time.Second
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
	inFlight                map[uint64]queued
	busyKeys                map[string]struct{}
	closed                  bool
	forced                  bool
	nextSeq                 uint64
	done                    chan struct{}
	handlers                map[Kind][]Handler
	logger                  *slog.Logger
	workers                 int
	keyFunc                 func(Event) string
	delayClose              time.Duration
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

// NewDispatcher creates an in-process dispatcher and starts its workers.
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
		delayClose: defaultDelayClose,
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

func (d *InMemoryDispatcher) Subscribe(handler Handler) {
	if handler == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return
	}
	for _, kind := range handler.Listening() {
		d.handlers[kind] = append(d.handlers[kind], handler)
	}
}

func (d *InMemoryDispatcher) Dispatch(ctx context.Context, event Event) error {
	return d.DispatchAll(ctx, []Event{event})
}

// DispatchAll accepts all events or none of them. When the queue cannot hold
// them, it waits until it can or until ctx is done; ctx does not reject events
// that can be queued immediately. More events than the buffer size are
// accepted once the queue is empty. Nil events are skipped.
func (d *InMemoryDispatcher) DispatchAll(ctx context.Context, events []Event) error {
	accepted := make([]Event, 0, len(events))
	for _, event := range events {
		if event != nil {
			accepted = append(accepted, event)
		}
	}
	if len(accepted) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	source := context.WithoutCancel(ctx)

	for {
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			d.logger.Warn("domain event dispatch rejected", slog.Int("event_count", len(accepted)))
			return ErrDispatcherClosed
		}
		if len(d.queue) == 0 || len(d.queue)+len(accepted) <= d.bufferSize {
			for _, event := range accepted {
				d.nextSeq++
				d.queue = append(d.queue, queued{
					seq:    d.nextSeq,
					key:    d.keyOf(event),
					event:  event,
					source: source,
				})
			}
			d.ready.Broadcast()
			d.mu.Unlock()
			return nil
		}
		spaceFreed := d.spaceFreed
		d.mu.Unlock()

		select {
		case <-spaceFreed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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

func (d *InMemoryDispatcher) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	defer d.rootCancel()

	if d.delayClose > 0 {
		timer := time.NewTimer(d.delayClose)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			snapshot, started := d.interruptClose(ctx.Err())
			if started {
				d.logger.Info("domain event dispatcher closing started")
			}
			d.reportCloseInterrupted(snapshot)
			return ctx.Err()
		}
	}

	closed := d.beginClose()
	if closed {
		d.logger.Info("domain event dispatcher closing started")
	}

	select {
	case <-d.done:
		if closed {
			d.logger.Info("domain event dispatcher closed")
		}
		return nil
	case <-ctx.Done():
		snapshot, _ := d.interruptClose(ctx.Err())
		d.reportCloseInterrupted(snapshot)
		return ctx.Err()
	}
}

func (d *InMemoryDispatcher) beginClose() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return false
	}
	d.closed = true
	d.ready.Broadcast()
	d.signalSpaceFreedLocked()
	return true
}

func (d *InMemoryDispatcher) interruptClose(err error) (CloseInterruptedContext, bool) {
	d.mu.Lock()
	started := !d.closed
	d.closed = true
	d.forced = true
	snapshot := d.closeInterruptedSnapshotLocked(err)
	d.ready.Broadcast()
	d.signalSpaceFreedLocked()
	d.mu.Unlock()

	d.rootCancel()
	return snapshot, started
}

func (d *InMemoryDispatcher) closeInterruptedSnapshotLocked(err error) CloseInterruptedContext {
	snapshot := CloseInterruptedContext{Error: err}

	inFlight := make([]queued, 0, len(d.inFlight))
	for _, item := range d.inFlight {
		inFlight = append(inFlight, item)
	}
	slices.SortFunc(inFlight, func(a, b queued) int { return compareSeq(a.seq, b.seq) })
	for _, item := range inFlight {
		snapshot.InFlight = append(snapshot.InFlight, item.event)
	}
	for _, item := range d.queue {
		snapshot.Pending = append(snapshot.Pending, item.event)
	}
	return snapshot
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

func (d *InMemoryDispatcher) reportCloseInterrupted(snapshot CloseInterruptedContext) {
	d.logger.Warn("domain event dispatcher close interrupted",
		slog.Any("error", snapshot.Error),
		slog.Int("in_flight_event_count", len(snapshot.InFlight)),
		slog.Int("pending_event_count", len(snapshot.Pending)),
		slog.Any("pending_event_kinds", eventKinds(snapshot.Pending)))
	if d.closeInterruptedHandler != nil {
		d.closeInterruptedHandler(snapshot)
	}
}

func eventKinds(events []Event) []Kind {
	if len(events) == 0 {
		return nil
	}
	kinds := make([]Kind, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind())
	}
	return kinds
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
		if d.closed && len(d.queue) == 0 {
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
// it.
func (d *InMemoryDispatcher) finish(item queued) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.inFlight, item.seq)
	if item.key != "" {
		delete(d.busyKeys, item.key)
		d.ready.Broadcast()
	}
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
			d.logger.Warn("domain event has no handler", eventAttrs(item)...)
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
			d.logger.Error("panic occurred while handling event", append(eventAttrs(item),
				slog.String("handler", fmt.Sprintf("%T", handler)),
				slog.Any("event", event),
				slog.Any("panic", recovered),
				slog.Any("stack_trace", string(stack)))...)
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
			d.logger.Warn("domain event context factory returned nil", eventAttrs(item)...)
		}
	}
	return ctx, cancel
}

func eventAttrs(item queued) []any {
	attrs := []any{slog.Any("event_kind", item.event.Kind())}
	if item.key != "" {
		attrs = append(attrs, slog.String("event_key", item.key))
	}
	return attrs
}
