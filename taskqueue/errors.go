package taskqueue

import "errors"

var (
	ErrEmptyType       = errors.New("task type is empty")
	ErrNilHandler      = errors.New("task handler is nil")
	ErrNoListening     = errors.New("task handler listens to no task types")
	ErrUnhandledType   = errors.New("task type is unhandled")
	ErrNilDecodeTarget = errors.New("task decode target is nil")
	ErrPanic           = errors.New("task handler panicked")

	// ErrSkipRetry marks a failure as non-retryable for provider adapters.
	ErrSkipRetry = errors.New("skip retry for task")
)
