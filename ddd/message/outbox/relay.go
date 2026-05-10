package outbox

import (
	"context"
	"time"

	"github.com/go-jimu/components/ddd/message"
)

type Relay struct {
	store     Store
	codec     Codec
	publisher message.Publisher
	retry     RetryPolicy
	now       func() time.Time
}

type relayConfig struct {
	retry RetryPolicy
	now   func() time.Time
}

type RelayOption func(*relayConfig)

func WithRetryPolicy(policy RetryPolicy) RelayOption {
	return func(cfg *relayConfig) {
		if policy != nil {
			cfg.retry = policy
		}
	}
}

func WithClock(now func() time.Time) RelayOption {
	return func(cfg *relayConfig) {
		if now != nil {
			cfg.now = now
		}
	}
}

func NewRelay(store Store, codec Codec, publisher message.Publisher, opts ...RelayOption) (*Relay, error) {
	if store == nil {
		return nil, ErrNilStore
	}
	if codec == nil {
		return nil, ErrNilCodec
	}
	if publisher == nil {
		return nil, ErrNilPublisher
	}
	cfg := relayConfig{retry: NoRetryPolicy{}, now: time.Now}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &Relay{store: store, codec: codec, publisher: publisher, retry: cfg.retry, now: cfg.now}, nil
}

type RunResult struct {
	Claimed   int
	Published int
	Failed    int
	Errors    []error
}

func (r *Relay) RunOnce(ctx context.Context, opts ClaimOptions) RunResult {
	opts, err := opts.normalize(r.now)
	if err != nil {
		return RunResult{Errors: []error{err}}
	}
	records, err := r.store.Claim(ctx, opts)
	if err != nil {
		return RunResult{Errors: []error{err}}
	}
	result := RunResult{Claimed: len(records)}
	for _, record := range records {
		msg, err := r.codec.Decode(record)
		if err != nil {
			r.markFailed(ctx, &result, record, err)
			continue
		}
		if err := r.publisher.Publish(ctx, msg); err != nil {
			r.markFailed(ctx, &result, record, err)
			continue
		}
		if err := r.store.MarkPublished(ctx, record.ID); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		result.Published++
	}
	return result
}

func (r *Relay) markFailed(ctx context.Context, result *RunResult, record Record, cause error) {
	decision := r.retry.NextAttempt(record, cause, r.now())
	nextAttemptAt := time.Time{}
	if decision.Retry {
		nextAttemptAt = decision.NextAttemptAt
	}
	if err := r.store.MarkFailed(ctx, record.ID, decision.Reason, nextAttemptAt); err != nil {
		result.Errors = append(result.Errors, err)
		return
	}
	result.Failed++
}
