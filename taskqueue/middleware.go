package taskqueue

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Middleware wraps a task processor function.
type Middleware func(ProcessorFunc) ProcessorFunc

// Chain wraps processor with middleware in declaration order.
func Chain(processor ProcessorFunc, middleware ...Middleware) ProcessorFunc {
	for i := len(middleware) - 1; i >= 0; i-- {
		if middleware[i] != nil {
			processor = middleware[i](processor)
		}
	}
	return processor
}

// Recover converts processor panics into ErrPanic.
func Recover() Middleware {
	return func(next ProcessorFunc) ProcessorFunc {
		return func(ctx context.Context, task Task) (err error) {
			if next == nil {
				return ErrNilProcessor
			}
			defer func() {
				if recovered := recover(); recovered != nil {
					err = panicAsError(recovered)
				}
			}()
			return next(ctx, task)
		}
	}
}

// Logging records task processor start, success, and failure events with slog.
func Logging(logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next ProcessorFunc) ProcessorFunc {
		return func(ctx context.Context, task Task) error {
			startedAt := time.Now()
			attrs := []any{
				"task_type", task.Type(),
				"queue", task.Queue(),
				"key", task.Key(),
			}
			logger.InfoContext(ctx, "taskqueue processor started", attrs...)

			if next == nil {
				return logTaskFailure(ctx, logger, attrs, startedAt, ErrNilProcessor)
			}
			if err := next(ctx, task); err != nil {
				return logTaskFailure(ctx, logger, attrs, startedAt, err)
			}

			logger.InfoContext(ctx, "taskqueue processor completed",
				append(attrs, "elapsed", time.Since(startedAt).String())...)
			return nil
		}
	}
}

func logTaskFailure(ctx context.Context, logger *slog.Logger, attrs []any, startedAt time.Time, err error) error {
	logger.ErrorContext(ctx, "taskqueue processor failed",
		append(attrs, "elapsed", time.Since(startedAt).String(), "error", err)...)
	return err
}

func panicAsError(recovered any) error {
	if err, ok := recovered.(error); ok {
		return fmt.Errorf("%w: %w", ErrPanic, err)
	}
	return fmt.Errorf("%w: %v", ErrPanic, recovered)
}
