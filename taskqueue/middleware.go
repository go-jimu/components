package taskqueue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-jimu/components/sloghelper"
)

// Middleware wraps a task processor function.
type Middleware func(ProcessorFunc) ProcessorFunc

// Chain wraps processor with middleware in declaration order: the first
// middleware is the outermost, so Chain(p, a, b) runs a, then b, then p. Nil
// middleware is skipped.
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

// Logging logs each processor call: "taskqueue processor started" and
// "taskqueue processor completed" at Debug, and "taskqueue processor failed"
// at Warn when the provider retries the task, or at Error when it does not,
// when the retry budget from ExecutionInfo is unknown, or when the processor
// panicked. Every log names the task with task_type, queue, and task_id, adds
// the ExecutionInfo the provider supplied, and is logged with the processor's
// ctx. A failure also has elapsed_ms, final when the retry budget is known,
// and the error under sloghelper.Error. A nil logger means slog.Default().
//
// Place Logging before Recover in Chain, so it logs a recovered panic.
func Logging(logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next ProcessorFunc) ProcessorFunc {
		return func(ctx context.Context, task Task) error {
			startedAt := time.Now()
			info, _ := ExecutionInfoFromContext(ctx)
			attrs := taskAttrs(task, info)
			logger.LogAttrs(ctx, slog.LevelDebug, "taskqueue processor started", attrs...)

			err := ErrNilProcessor
			if next != nil {
				err = next(ctx, task)
			}
			attrs = append(attrs, slog.Int64("elapsed_ms", time.Since(startedAt).Milliseconds()))
			if err == nil {
				logger.LogAttrs(ctx, slog.LevelDebug, "taskqueue processor completed", attrs...)
				return nil
			}

			level := slog.LevelError
			final, known := finalAttempt(info, err)
			if known {
				attrs = append(attrs, slog.Bool("final", final))
				if !final && !errors.Is(err, ErrPanic) {
					level = slog.LevelWarn
				}
			}
			logger.LogAttrs(ctx, level, "taskqueue processor failed", append(attrs, sloghelper.Error(err))...)
			return err
		}
	}
}

// finalAttempt reports whether the provider stops retrying after err, and
// whether that is known: ErrSkipRetry stops retries, otherwise the retry
// budget of info decides.
func finalAttempt(info ExecutionInfo, err error) (bool, bool) {
	if errors.Is(err, ErrSkipRetry) {
		return true, true
	}
	retryCount, countSet := info.RetryCount()
	maxRetry, maxSet := info.MaxRetry()
	if !countSet || !maxSet {
		return false, false
	}
	return retryCount >= maxRetry, true
}

func taskAttrs(task Task, info ExecutionInfo) []slog.Attr {
	taskID := task.ID()
	if taskID == "" {
		taskID = info.TaskID()
	}
	attrs := []slog.Attr{
		slog.String("task_type", string(task.Type())),
		slog.String("queue", task.Queue()),
		slog.String("task_id", taskID),
	}
	if info.Queue() != "" {
		attrs = append(attrs, slog.String("execution_queue", info.Queue()))
	}
	if retryCount, ok := info.RetryCount(); ok {
		attrs = append(attrs, slog.Int("retry_count", retryCount))
	}
	if maxRetry, ok := info.MaxRetry(); ok {
		attrs = append(attrs, slog.Int("max_retry", maxRetry))
	}
	return attrs
}

func panicAsError(recovered any) error {
	if err, ok := recovered.(error); ok {
		return fmt.Errorf("%w: %w", ErrPanic, err)
	}
	return fmt.Errorf("%w: %v", ErrPanic, recovered)
}
