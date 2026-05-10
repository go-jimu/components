package event

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

const (
	defaultBufferSize = 1024
	defaultDelayClose = 5 * time.Second
)

type batch struct {
	id     uint64
	events []Event
}

type InMemoryDispatcher struct {
	mu                      sync.Mutex
	notEmpty                *sync.Cond
	notFull                 *sync.Cond
	queue                   []batch
	closed                  bool
	forced                  bool
	nextBatchID             uint64
	inFlightBatchID         uint64
	done                    chan struct{}
	handlers                map[Kind][]Handler
	logger                  *slog.Logger
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

// NewDispatcher creates an in-process dispatcher with one background worker.
func NewDispatcher(opts ...Option) *InMemoryDispatcher {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	d := &InMemoryDispatcher{
		done:       make(chan struct{}),
		handlers:   make(map[Kind][]Handler),
		logger:     slog.Default(),
		delayClose: defaultDelayClose,
		bufferSize: defaultBufferSize,
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
	}
	d.notEmpty = sync.NewCond(&d.mu)
	d.notFull = sync.NewCond(&d.mu)

	for _, opt := range opts {
		opt(d)
	}

	go d.run()
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

func (d *InMemoryDispatcher) Dispatch(event Event) error {
	if event == nil {
		return nil
	}
	return d.DispatchAll([]Event{event})
}

func (d *InMemoryDispatcher) DispatchAll(events []Event) error {
	if len(events) == 0 {
		return nil
	}

	copied := make([]Event, len(events))
	copy(copied, events)

	d.mu.Lock()
	for len(d.queue) >= d.bufferSize && !d.closed {
		d.notFull.Wait()
	}
	if d.closed {
		d.mu.Unlock()
		d.logger.Warn("domain event dispatch rejected", slog.Int("event_count", len(copied)))
		return ErrDispatcherClosed
	}

	d.nextBatchID++
	d.queue = append(d.queue, batch{id: d.nextBatchID, events: copied})
	d.notEmpty.Signal()
	d.mu.Unlock()
	return nil
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
	d.notEmpty.Broadcast()
	d.notFull.Broadcast()
	return true
}

func (d *InMemoryDispatcher) interruptClose(err error) (CloseInterruptedContext, bool) {
	d.mu.Lock()
	started := !d.closed
	d.closed = true
	d.forced = true
	snapshot := d.closeInterruptedSnapshotLocked(err)
	d.notEmpty.Broadcast()
	d.notFull.Broadcast()
	d.mu.Unlock()

	d.rootCancel()
	return snapshot, started
}

func (d *InMemoryDispatcher) closeInterruptedSnapshotLocked(err error) CloseInterruptedContext {
	snapshot := CloseInterruptedContext{
		Error:           err,
		InFlightBatchID: d.inFlightBatchID,
	}
	for _, queued := range d.queue {
		events := make([]Event, len(queued.events))
		copy(events, queued.events)
		snapshot.PendingBatches = append(snapshot.PendingBatches, PendingBatch{
			BatchID: queued.id,
			Events:  events,
		})
	}
	return snapshot
}

func (d *InMemoryDispatcher) reportCloseInterrupted(snapshot CloseInterruptedContext) {
	pendingBatchIDs, pendingEventKinds, pendingEventCount := closeInterruptedSummary(snapshot)
	d.logger.Warn("domain event dispatcher close interrupted",
		slog.Any("error", snapshot.Error),
		slog.Int("pending_batch_count", len(snapshot.PendingBatches)),
		slog.Int("pending_event_count", pendingEventCount),
		slog.Uint64("in_flight_batch_id", snapshot.InFlightBatchID),
		slog.Any("pending_batch_ids", pendingBatchIDs),
		slog.Any("pending_event_kinds", pendingEventKinds))
	if d.closeInterruptedHandler != nil {
		d.closeInterruptedHandler(snapshot)
	}
}

func closeInterruptedSummary(snapshot CloseInterruptedContext) ([]uint64, []Kind, int) {
	var (
		batchIDs   []uint64
		eventKinds []Kind
		eventCount int
	)
	if len(snapshot.PendingBatches) > 0 {
		batchIDs = make([]uint64, 0, len(snapshot.PendingBatches))
	}
	for _, pending := range snapshot.PendingBatches {
		batchIDs = append(batchIDs, pending.BatchID)
		eventCount += len(pending.Events)
		for _, event := range pending.Events {
			if event == nil {
				continue
			}
			eventKinds = append(eventKinds, event.Kind())
		}
	}
	return batchIDs, eventKinds, eventCount
}

func (d *InMemoryDispatcher) run() {
	defer close(d.done)

	for {
		next, ok := d.nextBatch()
		if !ok {
			return
		}
		d.setInFlight(next.id)
		d.handleBatch(next)
		d.clearInFlight(next.id)
	}
}

func (d *InMemoryDispatcher) nextBatch() (batch, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for len(d.queue) == 0 && !d.closed {
		d.notEmpty.Wait()
	}
	if d.forced {
		return batch{}, false
	}
	if len(d.queue) == 0 && d.closed {
		return batch{}, false
	}

	next := d.queue[0]
	copy(d.queue, d.queue[1:])
	d.queue[len(d.queue)-1] = batch{}
	d.queue = d.queue[:len(d.queue)-1]
	d.notFull.Signal()
	return next, true
}

func (d *InMemoryDispatcher) setInFlight(batchID uint64) {
	d.mu.Lock()
	d.inFlightBatchID = batchID
	d.mu.Unlock()
}

func (d *InMemoryDispatcher) clearInFlight(batchID uint64) {
	d.mu.Lock()
	if d.inFlightBatchID == batchID {
		d.inFlightBatchID = 0
	}
	d.mu.Unlock()
}

func (d *InMemoryDispatcher) handleBatch(next batch) {
	for _, event := range next.events {
		d.handleEvent(next.id, event)
	}
}

func (d *InMemoryDispatcher) handleEvent(batchID uint64, event Event) {
	if event == nil {
		return
	}

	d.mu.Lock()
	handlers := append([]Handler(nil), d.handlers[event.Kind()]...)
	d.mu.Unlock()

	if len(handlers) == 0 {
		if d.unhandledEventHandler != nil {
			d.unhandledEventHandler(UnhandledContext{BatchID: batchID, Event: event})
		} else {
			d.logger.Warn("domain event has no handler",
				slog.Uint64("batch_id", batchID),
				slog.Any("event_kind", event.Kind()))
		}
		return
	}

	for _, handler := range handlers {
		d.handleOne(batchID, handler, event)
	}
}

func (d *InMemoryDispatcher) handleOne(batchID uint64, handler Handler, event Event) {
	defer func() {
		if recovered := recover(); recovered != nil {
			stack := debug.Stack()
			if d.panicHandler != nil {
				d.panicHandler(PanicContext{
					BatchID: batchID,
					Event:   event,
					Panic:   recovered,
					Stack:   stack,
				})
				return
			}
			d.logger.Error("panic occurred while handling event",
				slog.Uint64("batch_id", batchID),
				slog.Any("event_kind", event.Kind()),
				slog.Any("event", event),
				slog.Any("panic", recovered),
				slog.Any("stack_trace", string(stack)))
		}
	}()

	ctx, cancel := d.handlerContext(batchID, event)
	defer cancel()
	handler.Handle(ctx, event)
}

func (d *InMemoryDispatcher) handlerContext(batchID uint64, event Event) (context.Context, context.CancelFunc) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	if d.handlerTimeout > 0 {
		ctx, cancel = context.WithTimeout(d.rootCtx, d.handlerTimeout)
	} else {
		ctx, cancel = context.WithCancel(d.rootCtx)
	}

	if d.contextFactory != nil {
		if derived := d.contextFactory(ctx, event); derived != nil {
			ctx = derived
		} else {
			d.logger.Warn("domain event context factory returned nil",
				slog.Uint64("batch_id", batchID),
				slog.Any("event_kind", event.Kind()))
		}
	}
	return ctx, cancel
}
