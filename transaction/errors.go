package transaction

import "errors"

var (
	// ErrNoTransaction means ctx is in no transaction. See "Repositories".
	ErrNoTransaction = errors.New("transaction: ctx is in no transaction")

	// ErrTransactionDone means ctx is in a transaction that has committed or
	// rolled back: a ctx of fn kept past Within. See rule 4 in "Running a use
	// case".
	ErrTransactionDone = errors.New("transaction: transaction has ended")

	// ErrForeignTransaction means ctx is in a transaction of another Manager,
	// that is, of another resource. See rule 5 in "Running a use case".
	ErrForeignTransaction = errors.New("transaction: ctx is in a transaction of another resource")

	// ErrNested means Within was called with a ctx that is already in a
	// transaction. See rule 3 in "Running a use case".
	ErrNested = errors.New("transaction: nested transaction is not supported")

	// ErrConflict means the transaction lost a concurrency conflict and was
	// rolled back. See "Conflict and unknown outcome".
	ErrConflict = errors.New("transaction: transaction rolled back on a conflict")

	// ErrOutcomeUnknown means Commit failed and the transaction may or may not
	// have committed. See "Conflict and unknown outcome".
	ErrOutcomeUnknown = errors.New("transaction: commit outcome is unknown")

	// ErrNilFunc means a nil function was passed to Within, WithinOrJoin,
	// BeforeCommit, or AfterCommit.
	ErrNilFunc = errors.New("transaction: function is nil")

	// ErrNilDriver means NewManager was given a nil Driver.
	ErrNilDriver = errors.New("transaction: driver is nil")
)
