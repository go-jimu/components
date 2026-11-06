package outbox

import (
	"context"
	"time"

	"github.com/go-jimu/components/ddd/event"
)

// Status is the state of a Record. See "Records".
type Status int

const (
	// StatusPending means the record has events left to run.
	StatusPending Status = iota + 1
	// StatusDone means every event ran successfully.
	StatusDone
	// StatusFailed means an event failed permanently; RetryFailed puts the
	// record back to pending.
	StatusFailed
	// StatusSkipped means an operator skipped the record with Skip.
	StatusSkipped
)

func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	case StatusSkipped:
		return "skipped"
	default:
		return "unknown"
	}
}

// EncodedEvent is one event of a Record in its stored form.
type EncodedEvent struct {
	Kind    event.Kind
	Payload []byte
}

// Record is the outbox record of one Handler for one transaction: the
// events of that transaction the Handler listens to, in dispatch order, and
// how far it has run them. See "Records".
type Record struct {
	ID      string
	Handler string
	Events  []EncodedEvent
	// Progress is the index of the next event to run.
	Progress int
	Status   Status
	// Attempts counts the leases taken on the record, the first included.
	Attempts      int
	LeaseOwner    string
	LeaseUntil    time.Time
	NextAttemptAt time.Time
	LastError     string
	// Trace is the carrier captured at dispatch; see WithTrace.
	Trace     map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Lease identifies one attempt on a record: a Store applies an update only
// while the record is pending and still leased to Owner for Attempt. See
// "Leases".
type Lease struct {
	RecordID string
	Owner    string
	Attempt  int
}

// LeaseOf returns the lease a record holds.
func LeaseOf(r Record) Lease {
	return Lease{RecordID: r.ID, Owner: r.LeaseOwner, Attempt: r.Attempts}
}

// ClaimRequest selects records for Claim.
type ClaimRequest struct {
	// Owner takes the leases.
	Owner string
	// Handlers limits the claim to these handler names: a Relay claims only
	// records it can run.
	Handlers []string
	// Now is the time the claim is evaluated at.
	Now time.Time
	// LeaseUntil is when the new leases expire.
	LeaseUntil time.Time
	// Limit is the maximum number of records to claim.
	Limit int
}

// Count is the number of records of one handler in one status.
type Count struct {
	Handler string
	Status  Status
	Records int
}

// Store keeps the outbox records of one transactional resource, such as one
// database, next to the business data; see "Writing a store". Technology
// adapters implement it, and outboxtest.TestStore checks an implementation.
// Every method is safe for concurrent use. Each update method returns
// ErrLeaseLost when the lease no longer holds.
type Store interface {
	// Insert writes records inside the transaction ctx is in, and returns
	// transaction.ErrNoTransaction when ctx is in none.
	Insert(ctx context.Context, records []Record) error
	// Claim atomically leases and returns up to Limit pending records whose
	// handler is in Handlers, whose NextAttemptAt is not after Now, and whose
	// lease has expired, oldest first. It sets LeaseOwner, LeaseUntil, and
	// increments Attempts on each.
	Claim(ctx context.Context, req ClaimRequest) ([]Record, error)
	// Advance sets Progress.
	Advance(ctx context.Context, lease Lease, progress int) error
	// Complete marks the record done.
	Complete(ctx context.Context, lease Lease) error
	// Retry releases the lease and sets NextAttemptAt and LastError; the
	// record stays pending.
	Retry(ctx context.Context, lease Lease, nextAttemptAt time.Time, cause string) error
	// Fail releases the lease, sets LastError, and marks the record failed.
	Fail(ctx context.Context, lease Lease, cause string) error
	// RetryFailed puts every failed record back to pending, due at now, and
	// returns how many it reset.
	RetryFailed(ctx context.Context, now time.Time) (int, error)
	// Skip marks a pending or failed record skipped, or returns
	// ErrRecordNotFound.
	Skip(ctx context.Context, id string) error
	// Purge deletes done and skipped records last updated before before, and
	// returns how many it deleted.
	Purge(ctx context.Context, before time.Time) (int, error)
	// Counts returns the number of records per handler and status, for
	// pending and failed records.
	Counts(ctx context.Context) ([]Count, error)
}
