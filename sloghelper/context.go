package sloghelper

import (
	"context"
	"log/slog"
)

var ctxKey = &struct{}{}

func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey, logger)
}

func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(ctxKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
