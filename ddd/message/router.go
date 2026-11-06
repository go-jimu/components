package message

import (
	"context"
	"sync"
)

// Publisher sends integration messages through a provider. Publish returns
// nil once the provider has accepted msg; an error means msg may not have been
// sent, and each provider documents which errors it returns. Implementations
// are safe for concurrent use. See "Publishing".
type Publisher interface {
	Publish(context.Context, Message) error
}

// Handler processes integration messages for the kinds returned by Listening.
// Handle returns nil when it has fully handled the message; a non-nil error is
// a message-level failure. See "Handlers" and "Failures".
type Handler interface {
	Listening() []Kind
	Handle(context.Context, Message) error
}

// Subscriber registers message handlers. Subscribe returns an error when the
// handler cannot be registered; see Router.Subscribe for the base errors.
// Subscribe only registers; a provider starts consuming in Runner.Run.
type Subscriber interface {
	Subscribe(Handler) error
}

// Runner is the optional runtime loop of a provider that actively consumes
// messages.
//
// Run blocks until the loop terminates. When ctx is cancelled or expires, Run
// stops consuming and returns nil unless shutdown itself fails; ctx.Err() tells
// why ctx ended. A non-nil error is a runtime-level failure, for example
// because polling, acknowledgement, or commit failed. See "Failures".
type Runner interface {
	Run(context.Context) error
}

// Router registers handlers by Kind and dispatches messages to them
// sequentially. Providers use it to implement Subscriber; its methods are
// safe for concurrent use.
type Router struct {
	mu       sync.RWMutex
	handlers map[Kind][]Handler
}

var _ Subscriber = (*Router)(nil)

// NewRouter returns an empty Router.
func NewRouter() *Router {
	return &Router{
		handlers: make(map[Kind][]Handler),
	}
}

// Subscribe registers handler for each kind returned by Listening, after
// which Handle routes messages of those kinds to it.
//
// Duplicate kinds from the same handler are ignored. It returns ErrNilHandler,
// ErrNoListening, or ErrEmptyKind, and registers nothing, when handler is nil,
// listens to no kinds, or lists an empty kind.
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
