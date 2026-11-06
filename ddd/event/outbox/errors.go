package outbox

import "errors"

// Errors returned by this package. Compare with errors.Is.
var (
	// ErrPermanent marks a handler error that retrying cannot fix. A Handler
	// wraps it, as in fmt.Errorf("%w: account closed", outbox.ErrPermanent),
	// and the Relay marks the record failed instead of retrying it. See
	// "Failures".
	ErrPermanent = errors.New("outbox: permanent failure")

	// ErrLeaseLost means a Store refused a record update because the record
	// is no longer leased to that owner and attempt: another Relay took it
	// over after the lease expired. See "Leases".
	ErrLeaseLost = errors.New("outbox: record lease lost")

	// ErrRecordNotFound means a Store has no record with that ID in a state
	// the call applies to, such as Skip of a record whose Progress is not the
	// given index.
	ErrRecordNotFound = errors.New("outbox: record not found")

	// ErrUnregisteredKind means an event's Kind, or its Go type for that Kind,
	// is not in the Registry. See "Events".
	ErrUnregisteredKind = errors.New("outbox: event kind is not registered")

	// ErrInvalidEvent means an event is nil, or its type reports an empty
	// Kind.
	ErrInvalidEvent = errors.New("outbox: event is invalid")

	// ErrDispatchAfterFlush means DispatchAll ran in a transaction whose
	// records this Outbox has already written: a BeforeCommit hook registered
	// after the transaction's first dispatch dispatched again. See
	// "Dispatch".
	ErrDispatchAfterFlush = errors.New("outbox: events dispatched after the transaction's records were written")

	// ErrDuplicateKind means Register was called twice for one Kind.
	ErrDuplicateKind = errors.New("outbox: event kind is already registered")

	// ErrInvalidHandler means Subscribe got a nil Handler, an empty Name, or
	// no Listening kinds.
	ErrInvalidHandler = errors.New("outbox: handler is invalid")

	// ErrDuplicateHandler means Subscribe got a second Handler with the same
	// Name.
	ErrDuplicateHandler = errors.New("outbox: handler name is already subscribed")

	// ErrNilStore means New was given a nil Store.
	ErrNilStore = errors.New("outbox: store is nil")

	// ErrNilRegistry means New was given a nil Registry.
	ErrNilRegistry = errors.New("outbox: registry is nil")

	// ErrAlreadyRunning means Run was called while another Run of the same
	// Outbox is active.
	ErrAlreadyRunning = errors.New("outbox: relay is already running")
)
