package sloghelper

import (
	"context"
	"log/slog"
)

type (
	Handler struct {
		handler slog.Handler
		funcs   []HandleFunc
	}

	HandlerOption func(*Handler)

	HandleFunc func(context.Context, *slog.Record)
)

var _ slog.Handler = (*Handler)(nil)

func NewHandler(hdl slog.Handler, opts ...HandlerOption) slog.Handler {
	nh := &Handler{}
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

func (ch *Handler) clone(hdl slog.Handler) *Handler {
	cloned := new(Handler)
	*cloned = *ch
	cloned.handler = hdl
	return cloned
}

func (ch *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return ch.handler.Enabled(ctx, level)
}

func (ch *Handler) Handle(ctx context.Context, r slog.Record) error {
	for _, fn := range ch.funcs {
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

func (ch *Handler) Apply(opt HandlerOption) {
	opt(ch)
}

func WithHandleFunc(fn HandleFunc) HandlerOption {
	return func(h *Handler) {
		h.funcs = append(h.funcs, fn)
	}
}
