package taskqueue

import "errors"

var (
	ErrEmptyType                 = errors.New("task type is empty")
	ErrNilProcessor              = errors.New("task processor is nil")
	ErrUnhandledType             = errors.New("task type is unhandled")
	ErrDuplicateProcessor        = errors.New("task processor is already registered")
	ErrDuplicatePayloadType      = errors.New("task payload type is already registered")
	ErrUnknownType               = errors.New("task type is unknown")
	ErrUnknownPayloadType        = errors.New("task payload type is unknown")
	ErrEmptyPayloadCodec         = errors.New("task payload codec is empty")
	ErrUnknownPayloadCodec       = errors.New("task payload codec is unknown")
	ErrNilPayload                = errors.New("task payload is nil")
	ErrNilDecodeTarget           = errors.New("task decode target is nil")
	ErrNilPayloadFactory         = errors.New("task payload factory is nil or returned nil")
	ErrInvalidPayloadFactory     = errors.New("task payload factory returned invalid payload")
	ErrNilPayloadResolver        = errors.New("task payload resolver is nil")
	ErrPanic                     = errors.New("task processor panicked")
	ErrInvalidEnqueueOption      = errors.New("task enqueue option is invalid")
	ErrEmptySchedule             = errors.New("task schedule is empty")
	ErrInvalidSchedule           = errors.New("task schedule is invalid")
	ErrInvalidScheduleLocation   = errors.New("task schedule location is invalid")
	ErrInvalidScheduleOption     = errors.New("task schedule option is invalid")
	ErrEmptyPeriodicTaskName     = errors.New("periodic task name is empty")
	ErrInvalidPeriodicTaskPolicy = errors.New("periodic task enqueue policy is invalid")
	ErrDuplicatePeriodicTask     = errors.New("periodic task is already registered")
	ErrPeriodicTaskID            = errors.New("periodic task must not set a task ID")

	// ErrDuplicateTask reports that a task with the same ID is still retained
	// by the provider. Provider adapters should return it, optionally wrapping
	// their own error, when Enqueue hits an ID conflict.
	ErrDuplicateTask = errors.New("task with the same ID already exists")

	// ErrSkipRetry marks a failure as non-retryable for provider adapters.
	ErrSkipRetry = errors.New("skip retry for task")
)
