package message

import (
	"context"
	"sync"
)

type Publisher interface {
	Publish(context.Context, Message) error
}

// Handler processes integration messages for the kinds returned by Listening.
//
// Handle returns nil when the message has been accepted and fully handled by
// this handler. A provider may ack, commit, or otherwise mark delivery complete
// after all matching handlers return nil.
//
// A non-nil error means the message was not successfully handled. Providers may
// apply their own retry, redelivery, dead-letter, stop, or failure-recording
// policy. Business failures that should not cause redelivery should be handled
// inside the handler and then return nil.
type Handler interface {
	Listening() []Kind
	Handle(context.Context, Message) error
}

// Subscriber registers message handlers.
//
// Subscribe is a handler registration operation only. It does not imply that a
// broker consumer has started polling, reserved partitions, acknowledged
// records, committed offsets, or joined a consumer group. Providers that own a
// runtime loop should expose that separately, for example by implementing
// Runner.
type Subscriber interface {
	Subscribe(Handler) error
}

// Runner is an optional runtime loop capability for providers that actively
// consume messages.
type Runner interface {
	Run(context.Context) error
}

// Closer is an optional lifecycle capability for providers that own resources.
type Closer interface {
	Close() error
}

// Router registers handlers by Kind and dispatches messages to them
// sequentially.
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

// Subscribe registers handler for each non-empty kind returned by Listening.
//
// Duplicate kinds from the same handler are ignored.
func (r *Router) Subscribe(handler Handler) error {
	if handler == nil {
		return ErrNilHandler
	}

	kinds := handler.Listening()
	if len(kinds) == 0 {
		return ErrNoListening
	}

	seen := make(map[Kind]struct{}, len(kinds))
	deduped := make([]Kind, 0, len(kinds))
	for _, kind := range kinds {
		if kind == "" {
			return ErrEmptyKind
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		deduped = append(deduped, kind)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, kind := range deduped {
		r.handlers[kind] = append(r.handlers[kind], handler)
	}
	return nil
}

// Handle dispatches msg to all handlers registered for msg.Kind().
//
// Handlers run sequentially in subscription order. Routing stops at the first
// handler error and returns that error so the caller can avoid acknowledging a
// partially handled message. ErrUnhandledKind is returned when no handler is
// registered for the message kind.
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
