package sloghelper

import (
	"context"
	"log/slog"
)

var ctxKey = &struct{ key string }{"context"}

// NewContext returns a copy of ctx that carries logger, for FromContext.
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey, logger)
}

// FromContext returns the logger stored by NewContext, or slog.Default() when
// ctx carries none.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(ctxKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
