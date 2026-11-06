package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/go-jimu/components/ddd/event"
)

// storeTimeout bounds one Store call the Relay makes for a record, so a
// successful run is recorded even while the Relay shuts down.
const storeTimeout = 10 * time.Second

// Run runs the Relay until ctx ends or Close is called, and returns nil then;
// see "Relay". It returns ErrAlreadyRunning when another Run is active.
func (o *Outbox) Run(ctx context.Context) error {
	if !o.running.CompareAndSwap(false, true) {
		return ErrAlreadyRunning
	}
	execCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	o.mu.Lock()
	select {
	case <-o.closing:
		// Close checks runDone under o.mu after closing, so either it waits
		// for this Run or this Run sees closing here.
		o.mu.Unlock()
		cancel()
		o.running.Store(false)
		return nil
	default:
	}
	o.runDone, o.cancel = done, cancel
	o.mu.Unlock()
	defer func() {
		cancel()
		o.running.Store(false)
		close(done)
	}()

	o.startup(ctx)

	slots := make(chan struct{}, o.cfg.workers)
	var running sync.WaitGroup
	// start waits for a free worker, and returns false when the Relay stops
	// meanwhile.
	start := func(r Record) bool {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return false
		case <-o.closing:
			return false
		}
		running.Add(1)
		go func() {
			defer func() {
				<-slots
				running.Done()
			}()
			o.execute(execCtx, r)
		}()
		return true
	}
	claim := func() {
		free := min(o.cfg.batchSize, cap(slots)-len(slots))
		if free <= 0 {
			return
		}
		now := time.Now()
		records, err := o.store.Claim(ctx, ClaimRequest{
			Owner:      o.cfg.owner,
			Handlers:   o.handlerNames(),
			Now:        now,
			LeaseUntil: now.Add(o.cfg.lease),
			Limit:      free,
		})
		if err != nil {
			if ctx.Err() == nil {
				o.logError(ctx, "outbox claim failed", err)
			}
			return
		}
		for _, r := range records {
			if !start(r) {
				return // the unstarted records run again once their lease expires
			}
		}
	}

	poll := time.NewTicker(o.cfg.pollInterval)
	defer poll.Stop()
	purge := time.NewTicker(min(o.cfg.retention, time.Hour))
	defer purge.Stop()

	claim()
	for {
		select {
		case <-ctx.Done():
			cancel()
			running.Wait()
			return nil
		case <-o.closing:
			running.Wait() // Close cancels execCtx when its own ctx ends
			return nil
		case records := <-o.hints:
			// A hint is a record this Outbox inserted, leased to its owner at
			// insert; one whose lease has expired may be another Relay's now.
			for _, r := range records {
				if time.Now().Before(r.LeaseUntil) && !start(r) {
					break
				}
			}
		case <-poll.C:
			claim()
		case <-purge.C:
			o.purge(ctx)
		}
	}
}

// startup puts failed records back to pending and reports records whose
// handler this Outbox does not run.
func (o *Outbox) startup(ctx context.Context) {
	if n, err := o.store.RetryFailed(ctx, time.Now()); err != nil {
		o.logError(ctx, "outbox retry of failed records failed", err)
	} else if n > 0 {
		o.cfg.logger.LogAttrs(ctx, slog.LevelInfo, "outbox failed records retried", slog.Int("records", n))
	}
	counts, err := o.store.Counts(ctx)
	if err != nil {
		o.logError(ctx, "outbox record count failed", err)
		return
	}
	known := o.handlerNames()
	for _, c := range counts {
		if !slices.Contains(known, c.Handler) {
			o.cfg.logger.LogAttrs(ctx, slog.LevelError, "outbox records of an unsubscribed handler",
				slog.String("handler", c.Handler), slog.String("status", c.Status.String()), slog.Int("records", c.Records))
		}
	}
}

func (o *Outbox) purge(ctx context.Context) {
	if _, err := o.store.Purge(ctx, time.Now().Add(-o.cfg.retention)); err != nil && ctx.Err() == nil {
		o.logError(ctx, "outbox purge failed", err)
	}
}

// execute runs the events of one leased record from its Progress on, and
// records the outcome.
func (o *Outbox) execute(ctx context.Context, r Record) {
	lease := LeaseOf(r)
	attrs := []slog.Attr{
		slog.String("record_id", r.ID),
		slog.String("handler", r.Handler),
		slog.Int("attempt", r.Attempts),
	}
	h, ok := o.handler(r.Handler)
	if !ok {
		o.fail(ctx, lease, fmt.Errorf("handler %q is not subscribed", r.Handler), attrs)
		return
	}
	if o.cfg.restore != nil && r.Trace != nil {
		ctx = o.cfg.restore(ctx, r.Trace)
	}
	for i := r.Progress; i < len(r.Events); i++ {
		eventAttrs := append(slices.Clone(attrs), slog.String("event_kind", string(r.Events[i].Kind)), slog.Int("event_index", i))
		e, err := o.registry.Decode(r.Events[i])
		if err != nil {
			o.fail(ctx, lease, err, eventAttrs)
			return
		}
		deliveryCtx := context.WithValue(ctx, deliveryKey{}, Delivery{RecordID: r.ID, Index: i, Attempt: r.Attempts})
		if err := handle(deliveryCtx, h, e); err != nil {
			if errors.Is(err, ErrPermanent) {
				o.fail(ctx, lease, err, eventAttrs)
				return
			}
			o.retry(ctx, lease, err, eventAttrs)
			return
		}
		if i+1 < len(r.Events) {
			if !o.record(ctx, "outbox progress update failed", func(ctx context.Context) error {
				return o.store.Advance(ctx, lease, i+1)
			}, eventAttrs) {
				return
			}
		}
	}
	o.record(ctx, "outbox completion failed", func(ctx context.Context) error {
		return o.store.Complete(ctx, lease)
	}, attrs)
}

func handle(ctx context.Context, h Handler, e event.Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("outbox: handler panicked: %v", r)
		}
	}()
	return h.Handle(ctx, e)
}

func (o *Outbox) retry(ctx context.Context, lease Lease, cause error, attrs []slog.Attr) {
	delay := o.backoff(lease.Attempt)
	if ctx.Err() != nil {
		delay = 0 // interrupted by shutdown, not failed: due again at once
	}
	o.cfg.logger.LogAttrs(ctx, slog.LevelWarn, "outbox handler failed, retrying",
		append(attrs, slog.Duration("retry_in", delay), slog.String("error", cause.Error()))...)
	o.record(ctx, "outbox retry update failed", func(ctx context.Context) error {
		return o.store.Retry(ctx, lease, time.Now().Add(delay), cause.Error())
	}, attrs)
}

func (o *Outbox) fail(ctx context.Context, lease Lease, cause error, attrs []slog.Attr) {
	o.logError(ctx, "outbox record failed permanently", cause, attrs...)
	o.record(ctx, "outbox failure update failed", func(ctx context.Context) error {
		return o.store.Fail(ctx, lease, cause.Error())
	}, attrs)
}

// record runs one Store update on a ctx that outlives shutdown, and reports
// whether it applied.
func (o *Outbox) record(ctx context.Context, msg string, update func(context.Context) error, attrs []slog.Attr) bool {
	storeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), storeTimeout)
	defer cancel()
	err := update(storeCtx)
	switch {
	case err == nil:
		return true
	case errors.Is(err, ErrLeaseLost):
		o.cfg.logger.LogAttrs(ctx, slog.LevelWarn, "outbox lease lost; another relay runs the record", attrs...)
	default:
		o.logError(ctx, msg, err, attrs...)
	}
	return false
}

func (o *Outbox) backoff(attempt int) time.Duration {
	delay := o.cfg.minBackoff
	for i := 1; i < attempt && delay < o.cfg.maxBackoff; i++ {
		delay *= 2
	}
	return min(delay, o.cfg.maxBackoff)
}
