package mediator

import "context"

type (
	Mediator interface {
		Dispatch(Event)
		Subscribe(EventHandler)
	}

	inMemMediator struct {
		handlers           map[EventKind][]EventHandler
		concurrent         chan struct{}
		orphanEventHandler func(Event)
	}

	Option func(*inMemMediator)
)

var (
	_ Mediator = (*inMemMediator)(nil)
)

func NewInMemMediator(concurrent int, opts ...Option) Mediator {
	if concurrent < 1 {
		concurrent = 1
	}

	m := &inMemMediator{
		handlers:   make(map[EventKind][]EventHandler),
		concurrent: make(chan struct{}, concurrent),
	}

	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *inMemMediator) Subscribe(hdl EventHandler) {
	for _, kind := range hdl.Listening() {
		if _, ok := m.handlers[kind]; !ok {
			m.handlers[kind] = make([]EventHandler, 0)
		}
		m.handlers[kind] = append(m.handlers[kind], hdl)
	}
}

func (m *inMemMediator) Dispatch(ev Event) {
	if _, ok := m.handlers[ev.Kind()]; !ok {
		if m.orphanEventHandler != nil {
			m.orphanEventHandler(ev)
			return
		}
		return
	}

	m.concurrent <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	go func(ctx context.Context, ev Event, handlers ...EventHandler) { // 确保event的多个handler处理的顺序以及时效性
		defer func() {
			<-m.concurrent
			cancel()
		}()
		for _, handler := range handlers {
			handler.Handle(ctx, ev) // 在handler内部处理ctx.Done()
		}
	}(ctx, ev, m.handlers[ev.Kind()]...)
}

func WithOrphanEventHandler(fn func(Event)) Option {
	return func(m *inMemMediator) {
		m.orphanEventHandler = fn
	}
}
