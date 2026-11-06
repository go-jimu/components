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

// storeTimeout bounds one Store update the Relay makes for a record, so the
// outcome of a run is stored even while the Relay shuts down.
const storeTimeout = 10 * time.Second

// Run runs the Relay; see "Relay". It returns nil when ctx ends or after
// Close, and ErrAlreadyRunning while another Run of this Outbox is active.
func (o *Outbox) Run(ctx context.Context) error {
	if !o.running.CompareAndSwap(false, true) {
		return ErrAlreadyRunning
	}
	// loopCtx bounds the Relay's own Store calls; execCtx is the ctx records
	// run on, which outlives ctx until the Relay stops them. Close cancels
	// both once its own ctx ends.
	loopCtx, cancelLoop := context.WithCancel(ctx)
	execCtx, cancelExec := context.WithCancel(context.WithoutCancel(ctx))
	cancel := func() {
		cancelLoop()
		cancelExec()
	}
	done := make(chan struct{})
	o.mu.Lock()
	if o.closed() {
		// Close checks runDone under o.mu after closing, so either it waits
		// for this Run or this Run sees closing here.
		o.mu.Unlock()
		cancel()
		o.running.Store(false)
		return nil
	}
	o.runDone, o.cancel = done, cancel
	o.mu.Unlock()

	r := &relay{
		outbox: o, loopCtx: loopCtx, execCtx: execCtx, cancelExec: cancelExec,
		slots: make(chan struct{}, o.cfg.workers),
	}
	defer func() {
		o.running.Store(false)
		r.releaseHints() // hints sent while running records finished
		cancel()
		close(done)
	}()
	r.run()
	return nil
}

// relay is one Run of the Relay.
type relay struct {
	outbox  *Outbox
	loopCtx context.Context
	execCtx context.Context
	// cancelExec cancels execCtx.
	cancelExec context.CancelFunc
	slots      chan struct{}
	running    sync.WaitGroup
}

func (r *relay) run() {
	o := r.outbox
	o.taking.Store(true)
	defer func() {
		// Stop leasing at insert and release the handed-over records before
		// waiting for running ones, which may take until their handlers
		// return.
		o.taking.Store(false)
		r.releaseHints()
		r.running.Wait()
	}()
	r.startup()
	poll := time.NewTicker(o.cfg.pollInterval)
	defer poll.Stop()
	purge := time.NewTicker(min(o.cfg.retention, time.Hour))
	defer purge.Stop()

	r.claim()
	for {
		select {
		case <-r.loopCtx.Done():
			r.stopRecords()
			return
		case <-o.closing:
			return
		case records := <-o.hints:
			r.startAll(records)
		case <-poll.C:
			r.claim()
		case <-purge.C:
			r.purge()
		}
	}
}

// stopRecords cancels the running records once ctx of Run ends.
func (r *relay) stopRecords() { r.cancelExec() }

// startup puts failed records back to pending and reports records whose
// handler this Outbox does not run.
func (r *relay) startup() {
	o, ctx := r.outbox, r.loopCtx
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
	known := handlerNames(o.subscribed())
	for _, c := range counts {
		if !slices.Contains(known, c.Handler) {
			o.cfg.logger.LogAttrs(ctx, slog.LevelError, "outbox records of an unsubscribed handler",
				slog.String("handler", c.Handler), slog.String("status", c.Status.String()), slog.Int("records", c.Records))
		}
	}
}

func (r *relay) claim() {
	o, ctx := r.outbox, r.loopCtx
	free := min(o.cfg.batchSize, cap(r.slots)-len(r.slots))
	if free <= 0 || o.closed() {
		return
	}
	now := time.Now()
	records, err := o.store.Claim(ctx, ClaimRequest{
		Owner:      o.cfg.owner,
		Handlers:   handlerNames(o.subscribed()),
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
	r.startAll(records)
}

// startAll runs leased records as workers free up. When the Relay stops
// meanwhile, it releases the records not started, so another Relay claims
// them without waiting for their lease to expire.
func (r *relay) startAll(records []Record) {
	for i, rec := range records {
		// select picks at random among ready cases, so check for a stop first:
		// a free worker must not win over Close.
		if r.loopCtx.Err() != nil || r.outbox.closed() {
			r.release(records[i:])
			return
		}
		select {
		case r.slots <- struct{}{}:
		case <-r.loopCtx.Done():
			r.release(records[i:])
			return
		case <-r.outbox.closing:
			r.release(records[i:])
			return
		}
		r.running.Add(1)
		go func() {
			defer func() {
				<-r.slots
				r.running.Done()
			}()
			r.outbox.execute(r.execCtx, rec)
		}()
	}
}

// releaseHints releases the records handed over at commit that the Relay
// did not start.
func (r *relay) releaseHints() {
	for {
		select {
		case records := <-r.outbox.hints:
			r.release(records)
		default:
			return
		}
	}
}

func (r *relay) release(records []Record) {
	for _, rec := range records {
		r.outbox.record(r.execCtx, "outbox lease release failed", func(ctx context.Context) error {
			return r.outbox.store.Retry(ctx, LeaseOf(rec), rec.NextAttemptAt, rec.LastError)
		}, recordAttrs(rec))
	}
}

func (r *relay) purge() {
	o, ctx := r.outbox, r.loopCtx
	if _, err := o.store.Purge(ctx, time.Now().Add(-o.cfg.retention)); err != nil && ctx.Err() == nil {
		o.logError(ctx, "outbox purge failed", err)
	}
}

func recordAttrs(r Record) []slog.Attr {
	return []slog.Attr{
		slog.String("record_id", r.ID),
		slog.String("handler", r.Handler),
		slog.Int("attempt", r.Attempts),
	}
}

// execute runs the events of one leased record from its Progress on, and
// stores the outcome.
func (o *Outbox) execute(ctx context.Context, r Record) {
	lease := LeaseOf(r)
	attrs := recordAttrs(r)
	if !time.Now().Before(r.LeaseUntil) {
		// It waited for a worker past its lease; another Relay may run it.
		o.cfg.logger.LogAttrs(ctx, slog.LevelWarn, "outbox lease expired before the record ran", attrs...)
		return
	}
	h, ok := o.handler(r.Handler)
	if !ok {
		// Unreachable: the Relay leases only records of subscribed handlers.
		o.logError(ctx, "outbox record of an unsubscribed handler", ErrInvalidHandler, attrs...)
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

func (o *Outbox) handler(name string) (Handler, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	h, ok := o.handlers[name]
	return h, ok
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
