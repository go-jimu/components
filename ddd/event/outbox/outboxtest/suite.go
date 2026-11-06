package outboxtest

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/event"
	"github.com/go-jimu/components/ddd/event/outbox"
	"github.com/go-jimu/components/transaction"
)

// Harness is one empty Store under test and the Transactor whose
// transactions its Insert joins.
type Harness struct {
	Store      outbox.Store
	Transactor transaction.Transactor
}

// TestStore checks a Store adapter against the outbox.Store contract; see
// "Writing a store" in package outbox. newHarness returns a Harness over an
// empty Store for each subtest; subtests run one at a time.
func TestStore(t *testing.T, newHarness func(t *testing.T) Harness) {
	t.Helper()
	for _, c := range storeCases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			c.run(t, &suite{t: t, h: h, base: time.Now().Truncate(time.Millisecond)})
		})
	}
}

type suite struct {
	t    *testing.T
	h    Harness
	base time.Time
	n    int
}

var storeCases = []struct {
	name string
	run  func(t *testing.T, s *suite)
}{
	{"InsertNeedsTransaction", func(t *testing.T, s *suite) {
		err := s.h.Store.Insert(context.Background(), []outbox.Record{s.record("a")})
		if !errors.Is(err, transaction.ErrNoTransaction) {
			t.Fatalf("Insert outside a transaction = %v, want ErrNoTransaction", err)
		}
	}},
	{"InsertRolledBack", func(t *testing.T, s *suite) {
		rollback := errors.New("rollback")
		err := s.h.Transactor.Within(context.Background(), func(ctx context.Context) error {
			if err := s.h.Store.Insert(ctx, []outbox.Record{s.record("a")}); err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Fatalf("Within = %v, want the rollback error", err)
		}
		if got := s.claim(10); len(got) != 0 {
			t.Fatalf("claimed %d records of a rolled back transaction", len(got))
		}
	}},
	{"InsertKeepsFields", func(t *testing.T, s *suite) {
		want := s.record("a")
		want.Events = []outbox.EncodedEvent{
			{Kind: "order.paid", Payload: []byte(`{"id":"o-1"}`)},
			{Kind: "order.shipped", Payload: []byte(`{"id":"o-1","carrier":"x"}`)},
		}
		want.Trace = map[string]string{"traceparent": "00-abc-def-01"}
		s.insert(want)
		got := s.claim(10)
		if len(got) != 1 {
			t.Fatalf("claimed %d records, want 1", len(got))
		}
		r := got[0]
		if r.ID != want.ID || r.Handler != want.Handler || r.Progress != 0 || r.Status != outbox.StatusPending {
			t.Fatalf("claimed %+v, want the inserted record", r)
		}
		if !slices.EqualFunc(r.Events, want.Events, func(a, b outbox.EncodedEvent) bool {
			return a.Kind == b.Kind && string(a.Payload) == string(b.Payload)
		}) {
			t.Fatalf("Events = %v, want %v", r.Events, want.Events)
		}
		if !maps.Equal(r.Trace, want.Trace) {
			t.Fatalf("Trace = %v, want %v", r.Trace, want.Trace)
		}
		if !r.CreatedAt.Equal(want.CreatedAt) {
			t.Fatalf("CreatedAt = %v, want %v", r.CreatedAt, want.CreatedAt)
		}
	}},
	{"ClaimLeases", func(t *testing.T, s *suite) {
		s.insert(s.record("a"))
		got := s.claim(10)
		if len(got) != 1 {
			t.Fatalf("claimed %d records, want 1", len(got))
		}
		r := got[0]
		if r.LeaseOwner != "relay-1" || !r.LeaseUntil.Equal(s.at(time.Minute)) || r.Attempts != 1 {
			t.Fatalf("claimed lease %s until %v attempt %d, want relay-1 until %v attempt 1",
				r.LeaseOwner, r.LeaseUntil, r.Attempts, s.at(time.Minute))
		}
		if again := s.claim(10); len(again) != 0 {
			t.Fatalf("claimed a leased record again")
		}
		later := s.claimAt("relay-2", []string{"a"}, s.at(time.Minute), 10)
		if len(later) != 1 || later[0].LeaseOwner != "relay-2" || later[0].Attempts != 2 {
			t.Fatalf("claim after the lease expired = %+v, want it leased to relay-2 at attempt 2", later)
		}
	}},
	{"ClaimInsertedLease", func(t *testing.T, s *suite) {
		r := s.record("a")
		r.Attempts, r.LeaseOwner, r.LeaseUntil = 1, "relay-1", s.at(time.Minute)
		s.insert(r)
		if got := s.claim(10); len(got) != 0 {
			t.Fatalf("claimed a record leased at insert")
		}
		if err := s.h.Store.Advance(context.Background(), outbox.LeaseOf(r), 1); err != nil {
			t.Fatalf("Advance under the inserted lease = %v", err)
		}
		if err := s.h.Store.Complete(context.Background(), outbox.LeaseOf(r)); err != nil {
			t.Fatalf("Complete under the inserted lease = %v", err)
		}
	}},
	{"ClaimSelects", func(t *testing.T, s *suite) {
		first, second, other, later := s.record("a"), s.record("a"), s.record("b"), s.record("a")
		later.NextAttemptAt = s.at(30 * time.Second)
		for _, r := range []outbox.Record{first, second, other, later} {
			s.insert(r)
		}
		got := s.claim(1)
		if len(got) != 1 || got[0].ID != first.ID {
			t.Fatalf("claim of 1 = %v, want the oldest record %s", ids(got), first.ID)
		}
		got = s.claim(10)
		if len(got) != 1 || got[0].ID != second.ID {
			t.Fatalf("claim = %v, want only %s: not other handlers, not records due later", ids(got), second.ID)
		}
		got = s.claimAt("relay-1", []string{"a", "b"}, s.at(30*time.Second), 10)
		if !sameIDs(got, other, later) {
			t.Fatalf("claim of both handlers once due, leases unexpired = %v, want %s and %s", ids(got), other.ID, later.ID)
		}
	}},
	{"ClaimIsExclusive", func(t *testing.T, s *suite) {
		const records, relays = 20, 4
		for range records {
			s.insert(s.record("a"))
		}
		var mu sync.Mutex
		seen := make(map[string]int)
		var wg sync.WaitGroup
		for i := range relays {
			wg.Go(func() {
				// A Store that reclaims leased records never runs dry; the
				// bound turns that into duplicate claims instead of a hang.
				for range records {
					got, err := s.h.Store.Claim(context.Background(), outbox.ClaimRequest{
						Owner: fmt.Sprintf("relay-%d", i), Handlers: []string{"a"},
						Now: s.base, LeaseUntil: s.at(time.Minute), Limit: 3,
					})
					if err != nil {
						t.Errorf("Claim = %v", err)
						return
					}
					if len(got) == 0 {
						return
					}
					mu.Lock()
					for _, r := range got {
						seen[r.ID]++
					}
					mu.Unlock()
				}
			})
		}
		wg.Wait()
		if len(seen) != records {
			t.Fatalf("claimed %d distinct records, want %d", len(seen), records)
		}
		for id, n := range seen {
			if n != 1 {
				t.Fatalf("record %s claimed %d times", id, n)
			}
		}
	}},
	{"UpdatesNeedTheLease", func(t *testing.T, s *suite) {
		s.insert(s.record("a"))
		r := s.claim(10)[0]
		lease := outbox.LeaseOf(r)
		stale := []outbox.Lease{
			{RecordID: r.ID, Owner: "relay-2", Attempt: r.Attempts},
			{RecordID: r.ID, Owner: r.LeaseOwner, Attempt: r.Attempts - 1},
			{RecordID: "missing", Owner: r.LeaseOwner, Attempt: r.Attempts},
		}
		ctx := context.Background()
		for _, l := range stale {
			for name, err := range map[string]error{
				"Advance":  s.h.Store.Advance(ctx, l, 1),
				"Complete": s.h.Store.Complete(ctx, l),
				"Retry":    s.h.Store.Retry(ctx, l, s.base, "x"),
				"Fail":     s.h.Store.Fail(ctx, l, "x"),
			} {
				if !errors.Is(err, outbox.ErrLeaseLost) {
					t.Fatalf("%s under lease %+v = %v, want ErrLeaseLost", name, l, err)
				}
			}
		}
		if err := s.h.Store.Complete(ctx, lease); err != nil {
			t.Fatalf("Complete = %v", err)
		}
		if err := s.h.Store.Complete(ctx, lease); !errors.Is(err, outbox.ErrLeaseLost) {
			t.Fatalf("Complete of a done record = %v, want ErrLeaseLost", err)
		}
	}},
	{"AdvanceKeepsProgress", func(t *testing.T, s *suite) {
		s.insert(s.record("a"))
		r := s.claim(10)[0]
		ctx := context.Background()
		if err := s.h.Store.Advance(ctx, outbox.LeaseOf(r), 1); err != nil {
			t.Fatalf("Advance = %v", err)
		}
		if err := s.h.Store.Retry(ctx, outbox.LeaseOf(r), s.base, "boom"); err != nil {
			t.Fatalf("Retry = %v", err)
		}
		got := s.claim(10)
		if len(got) != 1 || got[0].Progress != 1 || got[0].LastError != "boom" || got[0].Attempts != 2 {
			t.Fatalf("claim after Advance and Retry = %+v, want progress 1, last error boom, attempt 2", got)
		}
	}},
	{"RetryReleases", func(t *testing.T, s *suite) {
		s.insert(s.record("a"))
		r := s.claim(10)[0]
		if err := s.h.Store.Retry(context.Background(), outbox.LeaseOf(r), s.at(time.Hour), "boom"); err != nil {
			t.Fatalf("Retry = %v", err)
		}
		if got := s.claim(10); len(got) != 0 {
			t.Fatalf("claimed a record before its next attempt")
		}
		got := s.claimAt("relay-1", []string{"a"}, s.at(time.Hour), 10)
		if len(got) != 1 {
			t.Fatalf("claimed %d records once due, want 1", len(got))
		}
		counts := s.counts()
		if !countsAre(counts, outbox.Count{Handler: "a", Status: outbox.StatusPending, Records: 1, Oldest: r.CreatedAt}) {
			t.Fatalf("Counts = %+v, want 1 pending record of a", counts)
		}
	}},
	{"FailAndRetryFailed", func(t *testing.T, s *suite) {
		s.insert(s.record("a"))
		r := s.claim(10)[0]
		ctx := context.Background()
		if err := s.h.Store.Fail(ctx, outbox.LeaseOf(r), "bad payload"); err != nil {
			t.Fatalf("Fail = %v", err)
		}
		if got := s.claimAt("relay-1", []string{"a"}, s.at(24*time.Hour), 10); len(got) != 0 {
			t.Fatalf("claimed a failed record")
		}
		counts := s.counts()
		if !countsAre(counts, outbox.Count{Handler: "a", Status: outbox.StatusFailed, Records: 1, Oldest: r.CreatedAt}) {
			t.Fatalf("Counts = %+v, want 1 failed record of a", counts)
		}
		n, err := s.h.Store.RetryFailed(ctx, s.base)
		if err != nil || n != 1 {
			t.Fatalf("RetryFailed = %d, %v, want 1", n, err)
		}
		if n, err := s.h.Store.RetryFailed(ctx, s.base); err != nil || n != 0 {
			t.Fatalf("second RetryFailed = %d, %v, want 0", n, err)
		}
		got := s.claim(10)
		if len(got) != 1 || got[0].LastError != "bad payload" || got[0].Attempts != 2 {
			t.Fatalf("claim after RetryFailed = %+v, want the record at attempt 2", got)
		}
	}},
	{"Skip", func(t *testing.T, s *suite) {
		two, one, leased, done := s.record("a"), s.record("a"), s.record("a"), s.record("a")
		two.Events = append(two.Events, outbox.EncodedEvent{Kind: "test.happened", Payload: []byte(`{"n":2}`)})
		for _, r := range []outbox.Record{two, one, leased, done} {
			s.insert(r)
		}
		ctx := context.Background()
		var leasedNow outbox.Record
		for _, r := range s.claim(10) {
			var err error
			switch r.ID {
			case two.ID, one.ID:
				err = s.h.Store.Fail(ctx, outbox.LeaseOf(r), "poison")
			case done.ID:
				err = s.h.Store.Complete(ctx, outbox.LeaseOf(r))
			default:
				leasedNow = r
			}
			if err != nil {
				t.Fatalf("settling %s = %v", r.ID, err)
			}
		}
		if err := s.h.Store.Skip(ctx, two.ID, 1); !errors.Is(err, outbox.ErrRecordNotFound) {
			t.Fatalf("Skip of an index other than Progress = %v, want ErrRecordNotFound", err)
		}
		for _, id := range []string{two.ID, one.ID, leased.ID} {
			if err := s.h.Store.Skip(ctx, id, 0); err != nil {
				t.Fatalf("Skip(%s, 0) = %v", id, err)
			}
		}
		for _, id := range []string{done.ID, one.ID, "missing"} {
			if err := s.h.Store.Skip(ctx, id, 0); !errors.Is(err, outbox.ErrRecordNotFound) {
				t.Fatalf("Skip(%s, 0) = %v, want ErrRecordNotFound", id, err)
			}
		}
		if err := s.h.Store.Complete(ctx, outbox.LeaseOf(leasedNow)); !errors.Is(err, outbox.ErrLeaseLost) {
			t.Fatalf("Complete under a lease Skip released = %v, want ErrLeaseLost", err)
		}
		got := s.claimAt("relay-1", []string{"a"}, s.at(24*time.Hour), 10)
		if len(got) != 1 || got[0].ID != two.ID || got[0].Progress != 1 || got[0].Status != outbox.StatusPending {
			t.Fatalf("claim after skipping = %+v, want only %s, pending at progress 1", got, two.ID)
		}
		if failed := s.failed(10); len(failed) != 0 {
			t.Fatalf("Failed = %v, want none after skipping", ids(failed))
		}
	}},
	{"FailedAndCounts", func(t *testing.T, s *suite) {
		first, second, third, pending := s.record("a"), s.record("a"), s.record("b"), s.record("b")
		for _, r := range []outbox.Record{first, second, third, pending} {
			s.insert(r)
		}
		ctx := context.Background()
		for _, r := range s.claimAt("relay-1", []string{"a", "b"}, s.base, 10) {
			var err error
			if r.ID == pending.ID {
				err = s.h.Store.Retry(ctx, outbox.LeaseOf(r), s.base, "x")
			} else {
				err = s.h.Store.Fail(ctx, outbox.LeaseOf(r), "poison "+r.ID)
			}
			if err != nil {
				t.Fatalf("settling %s = %v", r.ID, err)
			}
		}
		failed := s.failed(2)
		if len(failed) != 2 || failed[0].ID != first.ID || failed[1].ID != second.ID ||
			failed[0].LastError != "poison "+first.ID || failed[0].Status != outbox.StatusFailed {
			t.Fatalf("Failed(2) = %+v, want %s then %s with their errors", failed, first.ID, second.ID)
		}
		if !countsAre(s.counts(),
			outbox.Count{Handler: "a", Status: outbox.StatusFailed, Records: 2, Oldest: first.CreatedAt},
			outbox.Count{Handler: "b", Status: outbox.StatusPending, Records: 1, Oldest: pending.CreatedAt},
			outbox.Count{Handler: "b", Status: outbox.StatusFailed, Records: 1, Oldest: third.CreatedAt},
		) {
			t.Fatalf("Counts = %+v, want per handler and status with the oldest CreatedAt", s.counts())
		}
	}},
	{"Purge", func(t *testing.T, s *suite) {
		done, skipped, pending, failed := s.record("a"), s.record("a"), s.record("a"), s.record("a")
		failed.Events = append(failed.Events, failed.Events[0])
		for _, r := range []outbox.Record{done, skipped, pending, failed} {
			s.insert(r)
		}
		ctx := context.Background()
		for _, r := range s.claim(10) {
			var err error
			switch r.ID {
			case done.ID:
				err = s.h.Store.Complete(ctx, outbox.LeaseOf(r))
			case failed.ID, skipped.ID:
				err = s.h.Store.Fail(ctx, outbox.LeaseOf(r), "x")
			default:
				err = s.h.Store.Retry(ctx, outbox.LeaseOf(r), s.base, "x")
			}
			if err != nil {
				t.Fatalf("settling %s = %v", r.ID, err)
			}
		}
		if err := s.h.Store.Skip(ctx, skipped.ID, 0); err != nil {
			t.Fatalf("Skip = %v", err)
		}
		if n, err := s.h.Store.Purge(ctx, time.Now().Add(-time.Hour)); err != nil || n != 0 {
			t.Fatalf("Purge before the records settled = %d, %v, want 0", n, err)
		}
		if n, err := s.h.Store.Purge(ctx, time.Now().Add(time.Hour)); err != nil || n != 2 {
			t.Fatalf("Purge = %d, %v, want the done records, one skipped to done", n, err)
		}
		if counts := s.counts(); !countsAre(counts,
			outbox.Count{Handler: "a", Status: outbox.StatusPending, Records: 1, Oldest: pending.CreatedAt},
			outbox.Count{Handler: "a", Status: outbox.StatusFailed, Records: 1, Oldest: failed.CreatedAt},
		) {
			t.Fatalf("Counts after Purge = %+v, want the pending and failed records", counts)
		}
	}},
}

// record returns a pending, unleased record of handler, created after every
// earlier one.
func (s *suite) record(handler string) outbox.Record {
	s.n++
	created := s.base.Add(time.Duration(s.n-100) * time.Millisecond)
	return outbox.Record{
		ID:            fmt.Sprintf("%s-%03d-%d", handler, s.n, s.base.UnixNano()),
		Handler:       handler,
		Events:        []outbox.EncodedEvent{{Kind: event.Kind("test.happened"), Payload: []byte(`{}`)}},
		Status:        outbox.StatusPending,
		NextAttemptAt: created,
		CreatedAt:     created,
		UpdatedAt:     created,
	}
}

func (s *suite) at(d time.Duration) time.Time { return s.base.Add(d) }

func (s *suite) insert(r outbox.Record) {
	s.t.Helper()
	err := s.h.Transactor.Within(context.Background(), func(ctx context.Context) error {
		return s.h.Store.Insert(ctx, []outbox.Record{r})
	})
	if err != nil {
		s.t.Fatalf("inserting %s = %v", r.ID, err)
	}
}

// claim claims records of handler "a" at base.
func (s *suite) claim(limit int) []outbox.Record {
	s.t.Helper()
	return s.claimAt("relay-1", []string{"a"}, s.base, limit)
}

func (s *suite) claimAt(owner string, handlers []string, now time.Time, limit int) []outbox.Record {
	s.t.Helper()
	got, err := s.h.Store.Claim(context.Background(), outbox.ClaimRequest{
		Owner: owner, Handlers: handlers, Now: now, LeaseUntil: now.Add(time.Minute), Limit: limit,
	})
	if err != nil {
		s.t.Fatalf("Claim = %v", err)
	}
	return got
}

func (s *suite) counts() []outbox.Count {
	s.t.Helper()
	counts, err := s.h.Store.Counts(context.Background())
	if err != nil {
		s.t.Fatalf("Counts = %v", err)
	}
	return counts
}

func (s *suite) failed(limit int) []outbox.Record {
	s.t.Helper()
	failed, err := s.h.Store.Failed(context.Background(), limit)
	if err != nil {
		s.t.Fatalf("Failed = %v", err)
	}
	return failed
}

// countsAre reports whether got equals want, in order, comparing Oldest as
// an instant.
func countsAre(got []outbox.Count, want ...outbox.Count) bool {
	return slices.EqualFunc(got, want, func(a, b outbox.Count) bool {
		return a.Handler == b.Handler && a.Status == b.Status && a.Records == b.Records && a.Oldest.Equal(b.Oldest)
	})
}

func ids(records []outbox.Record) []string {
	out := make([]string, len(records))
	for i, r := range records {
		out[i] = r.ID
	}
	return out
}

func sameIDs(got []outbox.Record, want ...outbox.Record) bool {
	g, w := ids(got), ids(want)
	slices.Sort(g)
	slices.Sort(w)
	return slices.Equal(g, w)
}
