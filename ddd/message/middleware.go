package message

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-jimu/components/sloghelper"
)

// HandleFunc is the signature of Handler.Handle.
type HandleFunc func(context.Context, Message) error

// Middleware wraps Handle, for logging, metrics, or tracing around a Handler.
// It returns next's error unless it means to change the failure the provider
// sees.
type Middleware func(next HandleFunc) HandleFunc

// Chain returns a Handler that listens to the kinds of handler and runs its
// Handle through middleware in declaration order: the first middleware is the
// outermost, so Chain(h, a, b) runs a, then b, then h. Nil middleware is
// skipped. Chain of a nil handler is nil, which Subscribe rejects with
// ErrNilHandler. Subscribe the result in place of handler.
func Chain(handler Handler, middleware ...Middleware) Handler {
	if handler == nil {
		return nil
	}
	handle := handler.Handle
	for i := len(middleware) - 1; i >= 0; i-- {
		if middleware[i] != nil {
			handle = middleware[i](handle)
		}
	}
	return chained{Handler: handler, handle: handle}
}

type chained struct {
	Handler
	handle HandleFunc
}

func (c chained) Handle(ctx context.Context, msg Message) error {
	return c.handle(ctx, msg)
}

// Logging logs each Handle call at Debug, with ctx: "message handled" or
// "message handler failed" with the error under sloghelper.Error. Both carry
// message_kind, message_id, message_key when set, and elapsed_ms. A failure
// is Debug because the provider's failure policy decides whether it is
// retried or lost, and the provider logs it at the level that follows. A nil
// logger means slog.Default().
func Logging(logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next HandleFunc) HandleFunc {
		return func(ctx context.Context, msg Message) error {
			startedAt := time.Now()
			err := next(ctx, msg)
			attrs := make([]slog.Attr, 0, 5) //nolint:mnd // the attributes below
			attrs = append(attrs,
				slog.String("message_kind", string(msg.Kind())),
				slog.String("message_id", msg.ID()),
			)
			if msg.Key() != "" {
				attrs = append(attrs, slog.String("message_key", msg.Key()))
			}
			attrs = append(attrs, slog.Int64("elapsed_ms", time.Since(startedAt).Milliseconds()))
			if err != nil {
				logger.LogAttrs(ctx, slog.LevelDebug, "message handler failed", append(attrs, sloghelper.Error(err))...)
				return err
			}
			logger.LogAttrs(ctx, slog.LevelDebug, "message handled", attrs...)
			return nil
		}
	}
}
