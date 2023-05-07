package sloghelper

import (
	"context"
	"runtime/debug"

	"golang.org/x/exp/slog"
)

type (
	handler struct {
		handler           slog.Handler
		disableStackTrace bool
		keyStack          string
	}

	HandlerOption func(*handler)
)

var (
	_        slog.Handler = (*handler)(nil)
	keyStack              = "stack"
)

func NewHandler(hdl slog.Handler, opts ...HandlerOption) slog.Handler {
	nh := &handler{keyStack: keyStack}
	ch, ok := hdl.(*handler)
	if ok {
		*nh = *ch
	} else {
		nh.handler = hdl
	}
	for _, opt := range opts {
		nh.apply(opt)
	}
	return nh
}

func (ch *handler) Enabled(ctx context.Context, level slog.Level) bool {
	return ch.handler.Enabled(ctx, level)
}

func (ch *handler) Handle(ctx context.Context, r slog.Record) error {
	if ch.Enabled(ctx, r.Level) && r.Level == slog.LevelError && !ch.disableStackTrace {
		r.AddAttrs(slog.String(ch.keyStack, string(debug.Stack())))
	}
	return ch.handler.Handle(ctx, r)
}

func (ch *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	hdl := ch.handler.WithAttrs(attrs)
	return NewHandler(hdl)
}

func (ch *handler) WithGroup(name string) slog.Handler {
	hdl := ch.handler.WithGroup(name)
	return NewHandler(hdl)
}

func (ch *handler) apply(opt HandlerOption) {
	opt(ch)
}

func WithDisableStackTrace(disabled bool) HandlerOption {
	return func(ch *handler) {
		ch.disableStackTrace = disabled
	}
}

func WithStackKey(key string) HandlerOption {
	return func(ch *handler) {
		if key == "" {
			key = keyStack
		}
		ch.keyStack = keyStack
	}
}
