package event

import "context"

type (
	Mediator interface {
		Dispatch(context.Context, Event)
		Subscribe(Kind, HandleFunc)
	}

	inMemMediator struct {
		handlers           map[Kind][]HandleFunc
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
		handlers:   make(map[Kind][]HandleFunc),
		concurrent: make(chan struct{}, concurrent),
	}

	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *inMemMediator) Subscribe(et Kind, hdl HandleFunc) {
	if _, ok := m.handlers[et]; !ok {
		m.handlers[et] = make([]HandleFunc, 0)
	}
	m.handlers[et] = append(m.handlers[et], hdl)
}

func (m *inMemMediator) Dispatch(ctx context.Context, ev Event) {
	if _, ok := m.handlers[ev.Kind()]; !ok {
		if m.orphanEventHandler != nil {
			m.orphanEventHandler(ev)
			return
		}
		return
	}

	m.concurrent <- struct{}{}
	go func(ctx context.Context, ev Event, handlers ...HandleFunc) { // 确保event的多个handler处理的顺序以及时效性
		defer func() {
			<-m.concurrent
		}()
		for _, handler := range handlers {
			handler(ctx, ev) // 在handler内部处理ctx.Done()
		}
	}(ctx, ev, m.handlers[ev.Kind()]...)
}

func WithOrphanEventHandler(fn func(Event)) Option {
	return func(m *inMemMediator) {
		m.orphanEventHandler = fn
	}
}
