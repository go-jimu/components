package taskqueue

import (
	"context"
	"time"
)

// Enqueuer hands a task to a task queue provider. Application code depends on
// Enqueuer; runtime composition supplies a provider adapter. Implementations
// are safe for concurrent use.
//
// Enqueue returns nil once the provider has stored the task, an error matching
// ErrDuplicateTask when the provider already holds the work (see "Task ID"),
// or a provider-specific error. The caller validates the policy; see
// "Creating and enqueueing tasks".
type Enqueuer interface {
	Enqueue(context.Context, Task, ...EnqueueOption) error
}

// EnqueueOption configures enqueue policy. Later options override earlier
// ones for the same field.
type EnqueueOption func(*EnqueueOptions)

// EnqueueOptions carries provider-neutral enqueue policy. Callers build it
// with NewEnqueueOptions to validate a policy; adapters build it to read one.
type EnqueueOptions struct {
	delay       time.Duration
	processAt   time.Time
	maxRetry    int
	maxRetrySet bool
	timeout     time.Duration
	deadline    time.Time
	uniqueTTL   time.Duration
}

// NewEnqueueOptions applies opts and returns the resulting enqueue policy.
func NewEnqueueOptions(opts ...EnqueueOption) EnqueueOptions {
	cfg := EnqueueOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// Validate checks whether the enqueue policy is internally consistent. It
// returns ErrInvalidEnqueueOption for a negative delay, max retry, timeout, or
// unique TTL, and when Delay and ProcessAt are both set; otherwise nil. See
// "Creating and enqueueing tasks".
func (o EnqueueOptions) Validate() error {
	if o.delay < 0 {
		return ErrInvalidEnqueueOption
	}
	if !o.processAt.IsZero() && o.delay != 0 {
		return ErrInvalidEnqueueOption
	}
	if o.maxRetrySet && o.maxRetry < 0 {
		return ErrInvalidEnqueueOption
	}
	if o.timeout < 0 {
		return ErrInvalidEnqueueOption
	}
	if o.uniqueTTL < 0 {
		return ErrInvalidEnqueueOption
	}
	return nil
}

// WithDelay asks the provider to process the task after delay.
func WithDelay(delay time.Duration) EnqueueOption {
	return func(cfg *EnqueueOptions) {
		cfg.delay = delay
	}
}

// WithProcessAt asks the provider to process the task at processAt.
func WithProcessAt(processAt time.Time) EnqueueOption {
	return func(cfg *EnqueueOptions) {
		cfg.processAt = processAt
	}
}

// WithMaxRetry sets how many times the provider retries a failed attempt,
// overriding its default. 0 means no retries.
func WithMaxRetry(maxRetry int) EnqueueOption {
	return func(cfg *EnqueueOptions) {
		cfg.maxRetry = maxRetry
		cfg.maxRetrySet = true
	}
}

// WithTimeout asks the provider to bound one handling attempt by timeout.
func WithTimeout(timeout time.Duration) EnqueueOption {
	return func(cfg *EnqueueOptions) {
		cfg.timeout = timeout
	}
}

// WithDeadline asks the provider to stop processing the task after deadline.
func WithDeadline(deadline time.Time) EnqueueOption {
	return func(cfg *EnqueueOptions) {
		cfg.deadline = deadline
	}
}

// WithUnique asks the provider to suppress duplicates of the task's content
// for ttl. The adapter documents which fields define a duplicate. See
// "Task ID".
func WithUnique(ttl time.Duration) EnqueueOption {
	return func(cfg *EnqueueOptions) {
		cfg.uniqueTTL = ttl
	}
}

// Delay returns the relative processing delay.
func (o EnqueueOptions) Delay() time.Duration {
	return o.delay
}

// ProcessAt returns the absolute processing time.
func (o EnqueueOptions) ProcessAt() time.Time {
	return o.processAt
}

// MaxRetry returns the configured retry count and whether it was set; when
// unset, the adapter keeps the provider default.
func (o EnqueueOptions) MaxRetry() (int, bool) {
	return o.maxRetry, o.maxRetrySet
}

// Timeout returns the per-attempt timeout.
func (o EnqueueOptions) Timeout() time.Duration {
	return o.timeout
}

// Deadline returns the absolute processing deadline.
func (o EnqueueOptions) Deadline() time.Time {
	return o.deadline
}

// UniqueTTL returns the provider duplicate suppression window.
func (o EnqueueOptions) UniqueTTL() time.Duration {
	return o.uniqueTTL
}
