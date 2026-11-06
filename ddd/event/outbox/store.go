package outbox

import (
	"context"
	"time"

	"github.com/go-jimu/components/ddd/event"
)

// Status is the state of a Record: pending until done, or failed; see
// "Failures".
type Status int

const (
	// StatusPending means the record has events left to run.
	StatusPending Status = iota + 1
	// StatusDone means every event ran successfully.
	StatusDone
	// StatusFailed means an event failed permanently; RetryFailed puts the
	// record back to pending.
	StatusFailed
)

func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// EncodedEvent is one event of a Record in its stored form.
type EncodedEvent struct {
	Kind    event.Kind
	Payload []byte
}

// Record is the outbox record of one must-run handler for one transaction:
// the events of that transaction the handler listens to, in dispatch order,
// and how far it has run them. The Relay purges done records after
// WithRetention.
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
	// Trace is the carrier WithTrace captured; see "Context".
	Trace     map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Lease identifies one attempt on a record, which fences the record's
// updates; see "Leases" and "Writing a store".
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
	// Oldest is the CreatedAt of the oldest of those records: how long the
	// handler has been behind.
	Oldest time.Time
}

// Store keeps the records of one database next to its business data.
// Technology adapters implement it by the rules in "Writing a store". Every
// method is safe for concurrent use.
type Store interface {
	// Insert writes records inside the transaction ctx is in, and returns
	// transaction.ErrNoTransaction when ctx is in none.
	Insert(ctx context.Context, records []Record) error
	// Claim atomically leases and returns up to Limit pending records whose
	// handler is in Handlers, whose NextAttemptAt is not after Now, and whose
	// lease has expired, oldest first. It sets LeaseOwner, LeaseUntil, and
	// increments Attempts on each.
	Claim(ctx context.Context, req ClaimRequest) ([]Record, error)
	// Advance sets Progress, fenced by lease. In a transaction of the Store's
	// database it writes in that transaction; for a ctx in another database's
	// transaction it returns transaction.ErrForeignTransaction.
	Advance(ctx context.Context, lease Lease, progress int) error
	// Complete marks the record done, fenced by lease.
	Complete(ctx context.Context, lease Lease) error
	// Retry, fenced by lease, releases the lease and sets NextAttemptAt and
	// LastError; the record stays pending.
	Retry(ctx context.Context, lease Lease, nextAttemptAt time.Time, cause string) error
	// Fail, fenced by lease, releases the lease, sets LastError, and marks the
	// record failed.
	Fail(ctx context.Context, lease Lease, cause string) error
	// RetryFailed puts every failed record back to pending, due at now, and
	// returns how many it reset.
	RetryFailed(ctx context.Context, now time.Time) (int, error)
	// Skip passes the event at index of a pending or failed record whose
	// Progress is index: it sets Progress to index+1, marks the record done
	// when no event is left and pending otherwise, and releases the lease. It
	// returns ErrRecordNotFound when no such record exists.
	Skip(ctx context.Context, id string, index int) error
	// Purge deletes done records last updated before before, and returns how
	// many it deleted.
	Purge(ctx context.Context, before time.Time) (int, error)
	// Counts returns the number of pending and failed records per handler and
	// status, ordered by handler and status.
	Counts(ctx context.Context) ([]Count, error)
	// Failed returns up to limit failed records, oldest first.
	Failed(ctx context.Context, limit int) ([]Record, error)
}
