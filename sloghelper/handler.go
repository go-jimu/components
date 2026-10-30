package sloghelper

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

type (
	// Handler wraps a slog.Handler and runs HandleFuncs on every record before
	// passing it on. Handlers derived through WithAttrs and WithGroup share the
	// HandleFuncs, so a later Apply reaches them as well.
	Handler struct {
		handler slog.Handler
		hooks   *hooks
	}

	HandlerOption func(*Handler)

	HandleFunc func(context.Context, *slog.Record)
)

// hooks holds HandleFuncs as a copy-on-write slice, so Handle reads them
// without locking while Apply adds to them concurrently.
type hooks struct {
	mu    sync.Mutex
	funcs atomic.Pointer[[]HandleFunc]
}

func newHooks(funcs []HandleFunc) *hooks {
	h := &hooks{}
	copied := append([]HandleFunc(nil), funcs...)
	h.funcs.Store(&copied)
	return h
}

func (h *hooks) load() []HandleFunc {
	if funcs := h.funcs.Load(); funcs != nil {
		return *funcs
	}
	return nil
}

func (h *hooks) add(fn HandleFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	next := append(append([]HandleFunc(nil), h.load()...), fn)
	h.funcs.Store(&next)
}

var _ slog.Handler = (*Handler)(nil)

// NewHandler wraps hdl. When hdl is already a *Handler, the new Handler starts
// with a copy of its HandleFuncs, so opts do not change hdl.
func NewHandler(hdl slog.Handler, opts ...HandlerOption) slog.Handler {
	nh := &Handler{handler: hdl}
	if ch, ok := hdl.(*Handler); ok {
		nh.handler = ch.handler
		nh.hooks = newHooks(ch.hooks.load())
	} else {
		nh.hooks = newHooks(nil)
	}
	for _, opt := range opts {
		nh.Apply(opt)
	}
	return nh
}

func (ch *Handler) clone(hdl slog.Handler) *Handler {
	return &Handler{handler: hdl, hooks: ch.hooks}
}

func (ch *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return ch.handler.Enabled(ctx, level)
}

func (ch *Handler) Handle(ctx context.Context, r slog.Record) error {
	for _, fn := range ch.hooks.load() {
		fn(ctx, &r)
	}
	return ch.handler.Handle(ctx, r)
}

func (ch *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hdl := ch.handler.WithAttrs(attrs)
	return ch.clone(hdl)
}

func (ch *Handler) WithGroup(name string) slog.Handler {
	hdl := ch.handler.WithGroup(name)
	return ch.clone(hdl)
}

// Apply applies opt to the handler. It is safe to call concurrently with
// logging.
func (ch *Handler) Apply(opt HandlerOption) {
	opt(ch)
}

func WithHandleFunc(fn HandleFunc) HandlerOption {
	return func(h *Handler) {
		if fn != nil {
			h.hooks.add(fn)
		}
	}
}
