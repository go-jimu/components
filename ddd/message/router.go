package message

import (
	"context"
	"sync"
)

type Publisher interface {
	Publish(context.Context, Message) error
}

type Handler interface {
	Listening() []Kind
	Handle(context.Context, Message) error
}

type Subscriber interface {
	Subscribe(Handler) error
}

type Router struct {
	mu       sync.RWMutex
	handlers map[Kind][]Handler
}

var _ Subscriber = (*Router)(nil)

func NewRouter() *Router {
	return &Router{
		handlers: make(map[Kind][]Handler),
	}
}

func (r *Router) Subscribe(handler Handler) error {
	if handler == nil {
		return ErrNilHandler
	}

	kinds := handler.Listening()
	if len(kinds) == 0 {
		return ErrNoListening
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, kind := range kinds {
		r.handlers[kind] = append(r.handlers[kind], handler)
	}
	return nil
}

func (r *Router) Handle(ctx context.Context, msg Message) error {
	r.mu.RLock()
	handlers := append([]Handler(nil), r.handlers[msg.Kind()]...)
	r.mu.RUnlock()

	if len(handlers) == 0 {
		return ErrUnhandledKind
	}

	for _, handler := range handlers {
		if err := handler.Handle(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}
