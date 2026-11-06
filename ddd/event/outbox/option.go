package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-jimu/components/ddd/event"
)

// Option configures New.
type Option func(*config)

type config struct {
	logger       *slog.Logger
	owner        string
	lease        time.Duration
	pollInterval time.Duration
	batchSize    int
	workers      int
	minBackoff   time.Duration
	maxBackoff   time.Duration
	retention    time.Duration
	bestEffort   event.Dispatcher
	capture      func(context.Context) map[string]string
	restore      func(context.Context, map[string]string) context.Context
}

func defaultConfig() config {
	return config{
		logger:       slog.Default(),
		lease:        time.Minute,
		pollInterval: 5 * time.Second,
		batchSize:    32,
		workers:      4,
		minBackoff:   time.Second,
		maxBackoff:   5 * time.Minute,
		retention:    7 * 24 * time.Hour,
	}
}

// WithLogger sets the logger for relay failures. It defaults to
// slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(c *config) {
		if logger != nil {
			c.logger = logger
		}
	}
}

// WithOwner sets the name this Outbox takes leases under. It defaults to a
// random name per Outbox; set it to a stable, unique instance name, such as
// the pod name, to read lease owners in the store. See "Leases".
func WithOwner(owner string) Option {
	return func(c *config) {
		if owner != "" {
			c.owner = owner
		}
	}
}

// WithLease sets how long a lease lasts, 1 minute by default. Keep it longer
// than the slowest record takes to run. See "Leases".
func WithLease(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.lease = d
		}
	}
}

// WithPollInterval sets how often the Relay claims due records, 5 seconds by
// default. See "Relay".
func WithPollInterval(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.pollInterval = d
		}
	}
}

// WithBatchSize sets how many records one claim takes, 32 by default.
func WithBatchSize(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.batchSize = n
		}
	}
}

// WithWorkers sets how many records the Relay runs at once, 4 by default.
func WithWorkers(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.workers = n
		}
	}
}

// WithBackoff sets the delay before a retry: min after the first attempt,
// doubling per attempt up to max. It defaults to 1 second and 5 minutes. See
// "Failures".
func WithBackoff(minDelay, maxDelay time.Duration) Option {
	return func(c *config) {
		if minDelay > 0 && maxDelay >= minDelay {
			c.minBackoff, c.maxBackoff = minDelay, maxDelay
		}
	}
}

// WithRetention sets how long done records are kept before the Relay purges
// them, 7 days by default.
func WithRetention(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.retention = d
		}
	}
}

// WithBestEffort sets the dispatcher that receives every dispatched event
// after commit, for handlers that may lose events, such as an
// event.InMemoryDispatcher. See "Handlers".
func WithBestEffort(dispatcher event.Dispatcher) Option {
	return func(c *config) {
		c.bestEffort = dispatcher
	}
}

// WithTrace sets how the caller's trace crosses into the Relay: capture
// returns a carrier from the dispatching ctx, and restore derives the ctx
// Handle receives from it. See "Context".
func WithTrace(capture func(context.Context) map[string]string, restore func(context.Context, map[string]string) context.Context) Option {
	return func(c *config) {
		if capture != nil && restore != nil {
			c.capture, c.restore = capture, restore
		}
	}
}
