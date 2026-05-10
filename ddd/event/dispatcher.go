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
	events []Event
}

type dispatcher struct {
	mu                    sync.Mutex
	notEmpty              *sync.Cond
	notFull               *sync.Cond
	queue                 []batch
	closed                bool
	done                  chan struct{}
	handlers              map[Kind][]Handler
	logger                *slog.Logger
	delayClose            time.Duration
	handlerTimeout        time.Duration
	contextFactory        func(context.Context, Event) context.Context
	unhandledEventHandler func(Event)
	panicHandler          func(Event, any, []byte)
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
	defer d.mu.Unlock()

	for len(d.queue) >= d.bufferSize && !d.closed {
		d.notFull.Wait()
	}
	if d.closed {
		return false
	}

	d.queue = append(d.queue, batch{events: copied})
	d.notEmpty.Signal()
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
			return ctx.Err()
		}
	}

	d.beginClose()

	select {
	case <-d.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *dispatcher) beginClose() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return
	}
	d.closed = true
	d.notEmpty.Broadcast()
	d.notFull.Broadcast()
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
		d.handleEvent(event)
	}
}

func (d *dispatcher) handleEvent(event Event) {
	if event == nil {
		return
	}

	d.mu.Lock()
	handlers := append([]Handler(nil), d.handlers[event.Kind()]...)
	d.mu.Unlock()

	if len(handlers) == 0 {
		if d.unhandledEventHandler != nil {
			d.unhandledEventHandler(event)
		}
		return
	}

	for _, handler := range handlers {
		d.handleOne(handler, event)
	}
}

func (d *dispatcher) handleOne(handler Handler, event Event) {
	defer func() {
		if recovered := recover(); recovered != nil {
			stack := debug.Stack()
			if d.panicHandler != nil {
				d.panicHandler(event, recovered, stack)
				return
			}
			d.logger.Error("panic occurred while handling event",
				slog.Any("event", event),
				slog.Any("panic", recovered),
				slog.Any("stack_trace", string(stack)))
		}
	}()

	ctx, cancel := d.handlerContext(event)
	defer cancel()
	handler.Handle(ctx, event)
}

func (d *dispatcher) handlerContext(event Event) (context.Context, context.CancelFunc) {
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
		}
	}
	return ctx, cancel
}
