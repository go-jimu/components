package taskqueue

import "errors"

// Errors returned by this package and by provider adapters. Compare them with
// errors.Is: functions may wrap them with more context.
var (
	// ErrEmptyType means a Definition, Processor, or Task has an empty
	// TaskType.
	ErrEmptyType = errors.New("task type is empty")
	// ErrNilProcessor means a nil Processor or ProcessorFunc was registered or
	// called.
	ErrNilProcessor = errors.New("task processor is nil")
	// ErrUnhandledType means Router.Process found no processor for the task
	// type. Providers treat it as a failed attempt.
	ErrUnhandledType = errors.New("task type is unhandled")
	// ErrDuplicateProcessor means a Router already has a processor for the
	// task type.
	ErrDuplicateProcessor = errors.New("task processor is already registered")
	// ErrDuplicatePayloadType means a SchemaRegistry already maps the payload
	// Go type to another task type.
	ErrDuplicatePayloadType = errors.New("task payload type is already registered")
	// ErrUnknownType means a SchemaRegistry has no schema for the task type.
	ErrUnknownType = errors.New("task type is unknown")
	// ErrUnknownPayloadType means a SchemaRegistry has no task type for the
	// payload's Go type.
	ErrUnknownPayloadType = errors.New("task payload type is unknown")
	// ErrEmptyPayloadCodec means no codec name was given, or the task records
	// none: raw tasks from New need WithPayloadCodec before DecodePayload.
	ErrEmptyPayloadCodec = errors.New("task payload codec is empty")
	// ErrUnknownPayloadCodec means the codec name is not registered.
	ErrUnknownPayloadCodec = errors.New("task payload codec is unknown")
	// ErrNilPayload means a SchemaRegistry received a nil payload.
	ErrNilPayload = errors.New("task payload is nil")
	// ErrNilDecodeTarget means a decode function received a nil target.
	ErrNilDecodeTarget = errors.New("task decode target is nil")
	// ErrNilPayloadFactory means a payload factory or resolver is nil or
	// returned nil.
	ErrNilPayloadFactory = errors.New("task payload factory is nil or returned nil")
	// ErrInvalidPayloadFactory means a payload factory returned a non-pointer.
	ErrInvalidPayloadFactory = errors.New("task payload factory returned invalid payload")
	// ErrNilPayloadResolver means a nil PayloadResolverFunc was called.
	ErrNilPayloadResolver = errors.New("task payload resolver is nil")
	// ErrPanic wraps a panic recovered by the Recover middleware.
	ErrPanic = errors.New("task processor panicked")
	// ErrInvalidEnqueueOption means EnqueueOptions.Validate rejected the
	// policy.
	ErrInvalidEnqueueOption = errors.New("task enqueue option is invalid")
	// ErrEmptySchedule means a schedule has no cron spec, a non-positive
	// interval, or no kind.
	ErrEmptySchedule = errors.New("task schedule is empty")
	// ErrInvalidSchedule means a cron spec does not have five fields.
	ErrInvalidSchedule = errors.New("task schedule is invalid")
	// ErrInvalidScheduleLocation means WithLocation named an unknown IANA
	// time zone.
	ErrInvalidScheduleLocation = errors.New("task schedule location is invalid")
	// ErrInvalidScheduleOption means an option does not apply to the
	// schedule kind, such as WithLocation on IntervalSchedule.
	ErrInvalidScheduleOption = errors.New("task schedule option is invalid")
	// ErrEmptyPeriodicTaskName means a PeriodicTask has an empty name.
	ErrEmptyPeriodicTaskName = errors.New("periodic task name is empty")
	// ErrInvalidPeriodicTaskPolicy means a PeriodicTask uses an absolute
	// time, WithProcessAt or WithDeadline, that cannot hold on every fire.
	ErrInvalidPeriodicTaskPolicy = errors.New("periodic task enqueue policy is invalid")
	// ErrDuplicatePeriodicTask means a registrar already has a periodic task
	// with the same name.
	ErrDuplicatePeriodicTask = errors.New("periodic task is already registered")
	// ErrPeriodicTaskID means a PeriodicTask's task sets an ID; periodic
	// tasks use WithUnique instead.
	ErrPeriodicTaskID = errors.New("periodic task must not set a task ID")

	// ErrDuplicateTask reports that a task with the same ID is still retained
	// by the provider; the work it names is already scheduled or was kept
	// after it ran. Enqueue returns an error matching it. Provider adapters
	// return it, optionally wrapping their own error, on an ID conflict.
	ErrDuplicateTask = errors.New("task with the same ID already exists")

	// ErrSkipRetry marks a processor failure as permanent. Wrap it in the
	// error returned from Process, as in
	// fmt.Errorf("%w: %w", taskqueue.ErrSkipRetry, err), and the provider
	// stops retrying the task at once. Decode errors already wrap it.
	ErrSkipRetry = errors.New("skip retry for task")
)
