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

type dispatcher struct {
	mu                    sync.Mutex
	notEmpty              *sync.Cond
	notFull               *sync.Cond
	queue                 []batch
	closed                bool
	nextBatchID           uint64
	done                  chan struct{}
	handlers              map[Kind][]Handler
	logger                *slog.Logger
	delayClose            time.Duration
	handlerTimeout        time.Duration
	contextFactory        func(context.Context, Event) context.Context
	unhandledEventHandler func(UnhandledContext)
	panicHandler          func(PanicContext)
	bufferSize            int
	rootCtx               context.Context
	rootCancel            context.CancelFunc
}

var _ Dispatcher = (*dispatcher)(nil)

// NewDispatcher creates an in-process dispatcher with one background worker.
func NewDispatcher(opts ...Option) Dispatcher {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	d := &dispatcher{
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

func (d *dispatcher) Subscribe(handler Handler) {
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

func (d *dispatcher) Dispatch(event Event) bool {
	if event == nil {
		return true
	}
	return d.DispatchAll([]Event{event})
}

func (d *dispatcher) DispatchAll(events []Event) bool {
	if len(events) == 0 {
		return true
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
		return false
	}

	d.nextBatchID++
	d.queue = append(d.queue, batch{id: d.nextBatchID, events: copied})
	d.notEmpty.Signal()
	d.mu.Unlock()
	return true
}

func (d *dispatcher) Close(ctx context.Context) error {
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
			closed := d.beginClose()
			if closed {
				d.logger.Info("domain event dispatcher closing started")
			}
			d.logger.Warn("domain event dispatcher close interrupted", slog.Any("error", ctx.Err()))
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
		d.logger.Warn("domain event dispatcher close interrupted", slog.Any("error", ctx.Err()))
		return ctx.Err()
	}
}

func (d *dispatcher) beginClose() bool {
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

func (d *dispatcher) run() {
	defer close(d.done)

	for {
		next, ok := d.nextBatch()
		if !ok {
			return
		}
		d.handleBatch(next)
	}
}

func (d *dispatcher) nextBatch() (batch, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for len(d.queue) == 0 && !d.closed {
		d.notEmpty.Wait()
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

func (d *dispatcher) handleBatch(next batch) {
	for _, event := range next.events {
		d.handleEvent(next.id, event)
	}
}

func (d *dispatcher) handleEvent(batchID uint64, event Event) {
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

func (d *dispatcher) handleOne(batchID uint64, handler Handler, event Event) {
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

func (d *dispatcher) handlerContext(batchID uint64, event Event) (context.Context, context.CancelFunc) {
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
