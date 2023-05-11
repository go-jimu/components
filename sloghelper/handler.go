package sloghelper

import (
	"context"
	"runtime/debug"

	"golang.org/x/exp/slog"
)

type (
	Handler struct {
		handler           slog.Handler
		disableStackTrace bool
		keyStack          string
		funcs             []HandleFunc
	}

	HandlerOption func(*Handler)

	HandleFunc func(context.Context, *slog.Record)
)

var (
	_        slog.Handler = (*Handler)(nil)
	keyStack              = "stack"
)

func NewHandler(hdl slog.Handler, opts ...HandlerOption) slog.Handler {
	nh := &Handler{keyStack: keyStack}
	ch, ok := hdl.(*Handler)
	if ok {
		*nh = *ch
	} else {
		nh.handler = hdl
	}
	for _, opt := range opts {
		nh.Apply(opt)
	}
	return nh
}

func (ch *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return ch.handler.Enabled(ctx, level)
}

func (ch *Handler) Handle(ctx context.Context, r slog.Record) error {
	ch.errorLogWithStackTrack(ctx, &r)
	for _, fn := range ch.funcs {
		fn(ctx, &r)
	}
	return ch.handler.Handle(ctx, r)
}

func (ch *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hdl := ch.handler.WithAttrs(attrs)
	return NewHandler(hdl)
}

func (ch *Handler) WithGroup(name string) slog.Handler {
	hdl := ch.handler.WithGroup(name)
	return NewHandler(hdl)
}

func (ch *Handler) Apply(opt HandlerOption) {
	opt(ch)
}

func (ch *Handler) errorLogWithStackTrack(ctx context.Context, r *slog.Record) {
	if ch.Enabled(ctx, r.Level) && r.Level == slog.LevelError && !ch.disableStackTrace {
		r.AddAttrs(slog.String(ch.keyStack, string(debug.Stack())))
	}
}

func WithDisableStackTrace(disabled bool) HandlerOption {
	return func(ch *Handler) {
		ch.disableStackTrace = disabled
	}
}

func WithStackKey(key string) HandlerOption {
	return func(ch *Handler) {
		if key == "" {
			key = keyStack
		}
		ch.keyStack = keyStack
	}
}

func WithHandleFunc(fn HandleFunc) HandlerOption {
	return func(h *Handler) {
		h.funcs = append(h.funcs, fn)
	}
}
