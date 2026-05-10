package outbox

import (
	"context"
	"time"
)

type Store interface {
	Append(ctx context.Context, records ...Record) error
	Claim(ctx context.Context, opts ClaimOptions) ([]Record, error)
	MarkPublished(ctx context.Context, records ...Record) error
	MarkFailed(ctx context.Context, record Record, reason string, nextAttemptAt time.Time) error
}

type ClaimOptions struct {
	Limit       int
	Now         time.Time
	LockedUntil time.Time
	ClaimedBy   string
}

// NormalizeClaimOptions fills default claim timestamps and validates the claim
// window before a store attempts to lock records.
func NormalizeClaimOptions(opts ClaimOptions, now func() time.Time) (ClaimOptions, error) {
	return opts.normalize(now)
}

func (o ClaimOptions) normalize(now func() time.Time) (ClaimOptions, error) {
	if o.Limit <= 0 || o.ClaimedBy == "" {
		return ClaimOptions{}, ErrInvalidClaimOptions
	}
	if o.Now.IsZero() {
		if now == nil {
			o.Now = time.Now()
		} else {
			o.Now = now()
		}
	}
	if !o.LockedUntil.After(o.Now) {
		return ClaimOptions{}, ErrInvalidClaimOptions
	}
	return o, nil
}
