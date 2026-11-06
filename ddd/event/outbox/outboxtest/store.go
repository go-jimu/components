// Package outboxtest provides Store, an in-memory outbox.Store for unit tests
// of use cases and handlers, and TestStore, the conformance suite a Store
// adapter passes. Pair Store with a transactiontest.Driver: it keeps what a
// transaction inserts once that transaction commits.
package outboxtest

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/go-jimu/components/ddd/event/outbox"
	"github.com/go-jimu/components/transaction"
)

// Store is an in-memory outbox.Store. It joins any transaction.Manager's
// transaction: Insert keeps its records once that transaction commits. The
// zero value is ready to use. Its methods are safe for concurrent use.
type Store struct {
	mu      sync.Mutex
	seq     int
	records map[string]*stored
}

type stored struct {
	seq    int
	record outbox.Record
}

var _ outbox.Store = (*Store)(nil)

// Insert keeps records once the transaction ctx is in commits. It returns
// transaction.ErrNoTransaction or transaction.ErrTransactionDone.
func (s *Store) Insert(ctx context.Context, records []outbox.Record) error {
	copied := make([]outbox.Record, len(records))
	for i, r := range records {
		copied[i] = clone(r)
	}
	return transaction.AfterCommit(ctx, func(context.Context) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.records == nil {
			s.records = make(map[string]*stored)
		}
		for _, r := range copied {
			s.seq++
			s.records[r.ID] = &stored{seq: s.seq, record: r}
		}
	})
}

// Claim implements outbox.Store.
func (s *Store) Claim(_ context.Context, req outbox.ClaimRequest) ([]outbox.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var due []*stored
	for _, st := range s.records {
		r := st.record
		if r.Status == outbox.StatusPending &&
			slices.Contains(req.Handlers, r.Handler) &&
			!r.NextAttemptAt.After(req.Now) &&
			!r.LeaseUntil.After(req.Now) {
			due = append(due, st)
		}
	}
	slices.SortFunc(due, func(a, b *stored) int { return a.seq - b.seq })
	if len(due) > req.Limit {
		due = due[:max(req.Limit, 0)]
	}
	claimed := make([]outbox.Record, 0, len(due))
	for _, st := range due {
		st.record.LeaseOwner = req.Owner
		st.record.LeaseUntil = req.LeaseUntil
		st.record.Attempts++
		st.record.UpdatedAt = time.Now()
		claimed = append(claimed, clone(st.record))
	}
	return claimed, nil
}

// Advance implements outbox.Store.
func (s *Store) Advance(_ context.Context, lease outbox.Lease, progress int) error {
	return s.update(lease, func(r *outbox.Record) { r.Progress = progress })
}

// Complete implements outbox.Store.
func (s *Store) Complete(_ context.Context, lease outbox.Lease) error {
	return s.update(lease, func(r *outbox.Record) {
		r.Status = outbox.StatusDone
		release(r)
	})
}

// Retry implements outbox.Store.
func (s *Store) Retry(_ context.Context, lease outbox.Lease, nextAttemptAt time.Time, cause string) error {
	return s.update(lease, func(r *outbox.Record) {
		r.NextAttemptAt, r.LastError = nextAttemptAt, cause
		release(r)
	})
}

// Fail implements outbox.Store.
func (s *Store) Fail(_ context.Context, lease outbox.Lease, cause string) error {
	return s.update(lease, func(r *outbox.Record) {
		r.Status, r.LastError = outbox.StatusFailed, cause
		release(r)
	})
}

// RetryFailed implements outbox.Store.
func (s *Store) RetryFailed(_ context.Context, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, st := range s.records {
		if st.record.Status == outbox.StatusFailed {
			st.record.Status, st.record.NextAttemptAt, st.record.UpdatedAt = outbox.StatusPending, now, time.Now()
			n++
		}
	}
	return n, nil
}

// Skip implements outbox.Store.
func (s *Store) Skip(_ context.Context, id string, index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, found := s.records[id]
	if !found || st.record.Progress != index ||
		(st.record.Status != outbox.StatusPending && st.record.Status != outbox.StatusFailed) {
		return outbox.ErrRecordNotFound
	}
	r := &st.record
	r.Progress = index + 1
	r.Status = outbox.StatusPending
	if r.Progress >= len(r.Events) {
		r.Status = outbox.StatusDone
	}
	release(r)
	r.UpdatedAt = time.Now()
	return nil
}

// Failed implements outbox.Store.
func (s *Store) Failed(_ context.Context, limit int) ([]outbox.Record, error) {
	var failed []outbox.Record
	for _, r := range s.Records() {
		if r.Status == outbox.StatusFailed && len(failed) < limit {
			failed = append(failed, r)
		}
	}
	return failed, nil
}

// Purge implements outbox.Store.
func (s *Store) Purge(_ context.Context, before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for id, st := range s.records {
		r := st.record
		if r.Status == outbox.StatusDone && r.UpdatedAt.Before(before) {
			delete(s.records, id)
			n++
		}
	}
	return n, nil
}

// Counts implements outbox.Store.
func (s *Store) Counts(context.Context) ([]outbox.Count, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	type key struct {
		handler string
		status  outbox.Status
	}
	counts := make(map[key]*outbox.Count)
	for _, st := range s.records {
		r := st.record
		if r.Status != outbox.StatusPending && r.Status != outbox.StatusFailed {
			continue
		}
		c, found := counts[key{r.Handler, r.Status}]
		if !found {
			c = &outbox.Count{Handler: r.Handler, Status: r.Status, Oldest: r.CreatedAt}
			counts[key{r.Handler, r.Status}] = c
		}
		c.Records++
		if r.CreatedAt.Before(c.Oldest) {
			c.Oldest = r.CreatedAt
		}
	}
	result := make([]outbox.Count, 0, len(counts))
	for _, c := range counts {
		result = append(result, *c)
	}
	slices.SortFunc(result, func(a, b outbox.Count) int {
		if a.Handler != b.Handler {
			if a.Handler < b.Handler {
				return -1
			}
			return 1
		}
		return int(a.Status) - int(b.Status)
	})
	return result, nil
}

// Records returns every record the Store keeps, oldest first.
func (s *Store) Records() []outbox.Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := slices.Collect(maps.Values(s.records))
	slices.SortFunc(all, func(a, b *stored) int { return a.seq - b.seq })
	records := make([]outbox.Record, len(all))
	for i, st := range all {
		records[i] = clone(st.record)
	}
	return records
}

func (s *Store) update(lease outbox.Lease, apply func(*outbox.Record)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, found := s.records[lease.RecordID]
	if !found || st.record.Status != outbox.StatusPending ||
		st.record.LeaseOwner != lease.Owner || st.record.Attempts != lease.Attempt {
		return outbox.ErrLeaseLost
	}
	apply(&st.record)
	st.record.UpdatedAt = time.Now()
	return nil
}

func release(r *outbox.Record) {
	r.LeaseOwner, r.LeaseUntil = "", time.Time{}
}

func clone(r outbox.Record) outbox.Record {
	r.Events = slices.Clone(r.Events)
	for i := range r.Events {
		r.Events[i].Payload = slices.Clone(r.Events[i].Payload)
	}
	r.Trace = maps.Clone(r.Trace)
	return r
}
