package taskqueue

import "errors"

var (
	ErrEmptyType             = errors.New("task type is empty")
	ErrNilProcessor          = errors.New("task processor is nil")
	ErrUnhandledType         = errors.New("task type is unhandled")
	ErrDuplicateProcessor    = errors.New("task processor is already registered")
	ErrDuplicatePayloadType  = errors.New("task payload type is already registered")
	ErrUnknownType           = errors.New("task type is unknown")
	ErrUnknownPayloadType    = errors.New("task payload type is unknown")
	ErrNilPayload            = errors.New("task payload is nil")
	ErrNilDecodeTarget       = errors.New("task decode target is nil")
	ErrNilPayloadFactory     = errors.New("task payload factory is nil or returned nil")
	ErrInvalidPayloadFactory = errors.New("task payload factory returned invalid payload")
	ErrNilPayloadResolver    = errors.New("task payload resolver is nil")
	ErrPanic                 = errors.New("task processor panicked")
	ErrEmptySchedule         = errors.New("task schedule is empty")
	ErrEmptyPeriodicTaskName = errors.New("periodic task name is empty")
	ErrDuplicatePeriodicTask = errors.New("periodic task is already registered")

	// ErrSkipRetry marks a failure as non-retryable for provider adapters.
	ErrSkipRetry = errors.New("skip retry for task")
)
