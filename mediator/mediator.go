package mediator

import (
	"context"
	"log/slog"
	"time"
)

type (
	// Mediator is the interface that wraps the methods of a mediator.
	Mediator interface {
		Dispatch(Event)
		Subscribe(EventHandler)
	}

	// InMemMediator is a simple in-memory mediator implementation.
	InMemMediator struct {
		timeout            time.Duration
		handlers           map[EventKind][]EventHandler
		concurrent         chan struct{}
		orphanEventHandler func(Event)
		genContextFn       func(ctx context.Context, ev Event) context.Context
		logger             *slog.Logger
	}

	// Options is the options for the mediator.
	Options struct {
		Timeout    string `json:"timeout" yaml:"timeout" toml:"timeout"`
		Concurrent int    `json:"concurrent" yaml:"concurrent" toml:"concurrent"`
	}
)

var _ Mediator = (*InMemMediator)(nil)

func NewInMemMediator(opt Options) Mediator {
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

	m := &InMemMediator{
		handlers:   make(map[EventKind][]EventHandler),
		concurrent: make(chan struct{}, opt.Concurrent),
		timeout:    d,
	}
	return m
}

// Subscribe registers an event handler to the mediator.
func (m *InMemMediator) Subscribe(hdl EventHandler) {
	for _, kind := range hdl.Listening() {
		if _, ok := m.handlers[kind]; !ok {
			m.handlers[kind] = make([]EventHandler, 0)
		}
		m.handlers[kind] = append(m.handlers[kind], hdl)
	}
}

// Dispatch dispatches an event to the mediator.
func (m *InMemMediator) Dispatch(ev Event) {
	if _, ok := m.handlers[ev.Kind()]; !ok {
		if m.orphanEventHandler != nil {
			m.orphanEventHandler(ev)
			return
		}
		return
	}

	m.concurrent <- struct{}{}
	go func(ev Event, handlers ...EventHandler) { // make sure the order of event's multiple handlers and the timeliness
		defer func() {
			if recv := recover(); recv != nil {
				logger := slog.Default()
				if m.logger != nil {
					logger = m.logger
				}
				logger.Error("panic occurred while handling event", slog.Any("panic", recv))
			}
			<-m.concurrent
		}()

		var ctx = context.Background()
		var cancel context.CancelFunc
		if m.timeout > 0 {
			ctx, cancel = context.WithTimeout(context.Background(), m.timeout)
			defer cancel()
		}
		if m.genContextFn != nil {
			ctx = m.genContextFn(ctx, ev)
		}
		for _, handler := range handlers {
			handler.Handle(ctx, ev)
		}
	}(ev, m.handlers[ev.Kind()]...)
}

// WithOrphanEventHandler present a function to handle the event when no handler is found.
func (m *InMemMediator) WithOrphanEventHandler(fn func(Event)) {
	m.orphanEventHandler = fn
}

// WithGenContext present a function to generate a new context for each handler.
func (m *InMemMediator) WithGenContext(fn func(ctx context.Context, ev Event) context.Context) {
	m.genContextFn = fn
}

// WithTimeout present a timeout for each handler.
func (m *InMemMediator) WithTimeout(timeout time.Duration) {
	m.timeout = timeout
}

func (m *InMemMediator) WithLogger(logger *slog.Logger) {
	m.logger = logger
}
