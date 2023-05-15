package mediator

import (
	"context"
	"time"
)

type (
	Mediator interface {
		Dispatch(Event)
		Subscribe(EventHandler)
	}

	InMemMediator struct {
		timeout            time.Duration
		handlers           map[EventKind][]EventHandler
		concurrent         chan struct{}
		orphanEventHandler func(Event)
	}

	Options struct {
		Timeout    string `json:"timeout" yaml:"timeout" toml:"timeout"`
		Concurrent int    `json:"concurrent" yaml:"concurrent" toml:"concurrent"`
	}
)

var _ Mediator = (*InMemMediator)(nil)

func NewInMemMediator(opt Options) (Mediator, error) {
	if opt.Concurrent < 1 {
		opt.Concurrent = 1
	}

	if opt.Timeout == "" {
		opt.Timeout = "0s"
	}
	d, err := time.ParseDuration(opt.Timeout)
	if err != nil {
		return nil, err
	}
	if d < 0 {
		d = 0
	}

	m := &InMemMediator{
		handlers:   make(map[EventKind][]EventHandler),
		concurrent: make(chan struct{}, opt.Concurrent),
		timeout:    d,
	}
	return m, nil
}

func (m *InMemMediator) Subscribe(hdl EventHandler) {
	for _, kind := range hdl.Listening() {
		if _, ok := m.handlers[kind]; !ok {
			m.handlers[kind] = make([]EventHandler, 0)
		}
		m.handlers[kind] = append(m.handlers[kind], hdl)
	}
}

func (m *InMemMediator) Dispatch(ev Event) {
	if _, ok := m.handlers[ev.Kind()]; !ok {
		if m.orphanEventHandler != nil {
			m.orphanEventHandler(ev)
			return
		}
		return
	}

	m.concurrent <- struct{}{}
	go func(ev Event, handlers ...EventHandler) { // 确保event的多个handler处理的顺序以及时效性
		defer func() {
			<-m.concurrent
		}()
		var ctx = context.Background()
		var cancel context.CancelFunc
		if m.timeout > 0 {
			ctx, cancel = context.WithTimeout(context.Background(), m.timeout)
			defer cancel()
		}
		for _, handler := range handlers {
			handler.Handle(ctx, ev) // 在handler内部处理ctx.Done()
		}
	}(ev, m.handlers[ev.Kind()]...)
}

func (m *InMemMediator) WithOrphanEventHandler(fn func(Event)) {
	m.orphanEventHandler = fn
}
