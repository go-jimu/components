package taskqueue

import (
	"context"
	"sync"
)

// Handler processes tasks for the types returned by Listening.
type Handler interface {
	Listening() []string
	Handle(context.Context, Task) error
}

// HandlerFunc adapts a function to task handling middleware.
type HandlerFunc func(context.Context, Task) error

// Subscriber registers task handlers.
//
// Subscribe is a handler registration operation only. It does not imply that a
// provider worker has started polling, acknowledged tasks, scheduled retries,
// or joined a runtime process.
type Subscriber interface {
	Subscribe(Handler) error
}

// Runner is an optional runtime loop capability for providers that actively
// consume tasks.
type Runner interface {
	Run(context.Context) error
}

type functionHandler struct {
	fn    HandlerFunc
	types []string
}

// NewHandlerFunc constructs a Handler from fn and listened task types.
func NewHandlerFunc(fn HandlerFunc, types ...string) Handler {
	return functionHandler{fn: fn, types: append([]string(nil), types...)}
}

func (h functionHandler) Listening() []string {
	return append([]string(nil), h.types...)
}

func (h functionHandler) Handle(ctx context.Context, task Task) error {
	if h.fn == nil {
		return ErrNilHandler
	}
	return h.fn(ctx, task)
}

// Router registers handlers by task type and dispatches tasks sequentially.
type Router struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

var _ Subscriber = (*Router)(nil)

// NewRouter constructs an empty task router.
func NewRouter() *Router {
	return &Router{handlers: make(map[string][]Handler)}
}

// Subscribe registers handler for each non-empty task type returned by Listening.
func (r *Router) Subscribe(handler Handler) error {
	if handler == nil {
		return ErrNilHandler
	}
	types := handler.Listening()
	if len(types) == 0 {
		return ErrNoListening
	}
	seen := make(map[string]struct{}, len(types))
	deduped := make([]string, 0, len(types))
	for _, taskType := range types {
		if taskType == "" {
			return ErrEmptyType
		}
		if _, ok := seen[taskType]; ok {
			continue
		}
		seen[taskType] = struct{}{}
		deduped = append(deduped, taskType)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, taskType := range deduped {
		r.handlers[taskType] = append(r.handlers[taskType], handler)
	}
	return nil
}

// Handle dispatches task to all handlers registered for task.Type().
func (r *Router) Handle(ctx context.Context, task Task) error {
	r.mu.RLock()
	handlers := append([]Handler(nil), r.handlers[task.Type()]...)
	r.mu.RUnlock()
	if len(handlers) == 0 {
		return ErrUnhandledType
	}
	for _, handler := range handlers {
		if err := handler.Handle(ctx, task); err != nil {
			return err
		}
	}
	return nil
}
