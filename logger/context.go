package logger

import "context"

type ctxKey struct{}

func InContext(ctx context.Context, log Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, log)
}

func FromContext(ctx context.Context) Logger {
	if l, ok := ctx.Value(ctxKey{}).(Logger); ok {
		return l
	}
	return Default()
}
