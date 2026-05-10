package mediator

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"
)

type (
	// Mediator is the interface that wraps the methods of a mediator.
	//
	// Deprecated: use github.com/go-jimu/components/ddd/event.Dispatcher for new
	// domain event code.
	Mediator interface {
		Dispatch(Event) error
		Subscribe(EventHandler)
	}

	// InMemMediator is a simple in-memory mediator implementation.
	//
	// Deprecated: use github.com/go-jimu/components/ddd/event.NewDispatcher for
	// new domain event code.
	InMemMediator struct {
		timeout            time.Duration
		handlers           map[EventKind][]EventHandler
		concurrent         chan struct{}
		orphanEventHandler func(Event) error
		genContextFn       func(ctx context.Context, ev Event) context.Context
		logger             *slog.Logger
		mu                 sync.RWMutex
		closed             bool
		wg                 sync.WaitGroup
		delayClose         time.Duration
		rootCtx            context.Context
		rootCancel         context.CancelFunc
	}

	// Options is the options for the mediator.
	//
	// Deprecated: use github.com/go-jimu/components/ddd/event options for new
	// domain event code.
	Options struct {
		Timeout    string `json:"timeout" yaml:"timeout" toml:"timeout"`
		Concurrent int    `json:"concurrent" yaml:"concurrent" toml:"concurrent"`
	}
)

var (
	_ Mediator = (*InMemMediator)(nil)
	// Deprecated: use github.com/go-jimu/components/ddd/event.Dispatcher return
	// values for new domain event code.
	ErrMediatorClosed = errors.New("mediator is closed")
	// Deprecated: use github.com/go-jimu/components/ddd/event.WithUnhandledEventHandler
	// for new domain event code.
	ErrNoHandlerMatched = errors.New("no matching event handler found")
)

// Deprecated: use github.com/go-jimu/components/ddd/event.NewDispatcher for new
// domain event code.
func NewInMemMediator(opt Options, opts ...Option) Mediator {
	if opt.Concurrent < 1 {
		opt.Concurrent = 1
	}

	d, err := time.ParseDuration(opt.Timeout)
	if err != nil {
		d = 0
	}
	if d < 0 {
		d = 0
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &InMemMediator{
		handlers:   make(map[EventKind][]EventHandler),
		concurrent: make(chan struct{}, opt.Concurrent),
		timeout:    d,
		logger:     slog.Default(),
		delayClose: 5 * time.Second,
		rootCtx:    ctx,
		rootCancel: cancel,
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Subscribe registers an event handler to the mediator.
func (m *InMemMediator) Subscribe(hdl EventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, kind := range hdl.Listening() {
		m.handlers[kind] = append(m.handlers[kind], hdl)
	}
}

// Dispatch dispatches an event to the mediator.
func (m *InMemMediator) Dispatch(ev Event) error {
	// RLock guarantees that closed check, handlers lookup, and wg.Add(1)
	// are atomic with respect to GracefulShutdown's write-lock + wg.Wait()
	// and Subscribe's write-lock + map write.
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		m.logger.Error("mediator is closed, drop the event", slog.Any("event", ev))
		return ErrMediatorClosed
	}
	handlers, ok := m.handlers[ev.Kind()]
	if ok {
		m.wg.Add(1)
	}
	m.mu.RUnlock()

	if !ok {
		if m.orphanEventHandler != nil {
			return m.orphanEventHandler(ev)
		}
		m.logger.Error("no handler found for event", slog.Any("event", ev))
		return ErrNoHandlerMatched
	}

	m.concurrent <- struct{}{}
	go func(ev Event, handlers ...EventHandler) {
		defer func() {
			if recv := recover(); recv != nil {
				logger := slog.Default()
				if m.logger != nil {
					logger = m.logger
				}
				logger.Error("panic occurred while handling event",
					slog.Any("event", ev),
					slog.Any("panic", recv),
					slog.Any("stack_trace", string(debug.Stack())))
			}
			<-m.concurrent
			m.wg.Done()
		}()

		var ctx context.Context
		var cancel context.CancelFunc
		if m.timeout > 0 {
			ctx, cancel = context.WithTimeout(m.rootCtx, m.timeout)
		} else {
			ctx, cancel = context.WithCancel(m.rootCtx)
		}
		defer cancel()

		if m.genContextFn != nil {
			ctx = m.genContextFn(ctx, ev)
		}
		for _, handler := range handlers {
			handler.Handle(ctx, ev)
		}
	}(ev, handlers...)
	return nil
}

// GracefulShutdown waits for all the events to be processed and then closes the mediator.
// The caller controls the maximum wait time via ctx.
func (m *InMemMediator) GracefulShutdown(ctx context.Context) error {
	defer m.rootCancel()

	// Respect external context during the delay phase.
	if m.delayClose > 0 {
		select {
		case <-time.After(m.delayClose):
		case <-ctx.Done():
		}
	}

	// Stop accepting new events under exclusive lock, so that no Dispatch
	// can sneak in a wg.Add(1) after we call wg.Wait().
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()

	// Wait for all in-flight handlers to finish.
	waitCh := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(waitCh)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-waitCh:
		return nil
	}
}

// Deprecated: Use WithOrphanEventHandler Option in NewInMemMediator instead.
func (m *InMemMediator) WithOrphanEventHandler(fn func(Event) error) {
	m.orphanEventHandler = fn
}

// Deprecated: Use WithGenContext Option in NewInMemMediator instead.
func (m *InMemMediator) WithGenContext(fn func(ctx context.Context, ev Event) context.Context) {
	m.genContextFn = fn
}

// Deprecated: Use WithTimeout Option in NewInMemMediator instead.
func (m *InMemMediator) WithTimeout(timeout time.Duration) {
	m.timeout = timeout
}

// Deprecated: Use WithDelayClose Option in NewInMemMediator instead.
func (m *InMemMediator) WithDelayClose(d time.Duration) {
	if d < 0 {
		d = 0
	}
	m.delayClose = d
}

// Deprecated: Use WithLogger Option in NewInMemMediator instead.
func (m *InMemMediator) WithLogger(logger *slog.Logger) {
	if logger == nil {
		return
	}
	m.logger = logger
}
