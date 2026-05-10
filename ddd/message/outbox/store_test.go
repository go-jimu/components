package outbox_test

import (
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/message/outbox"
	"github.com/stretchr/testify/require"
)

// Claim options must produce a safe lock window before a relay asks the store
// to claim records.
func TestClaimOptionsNormalize(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	opts := outbox.ClaimOptions{
		Limit:       10,
		LockedUntil: now.Add(time.Minute),
		ClaimedBy:   "worker-1",
	}

	normalized, err := outbox.NormalizeClaimOptions(opts, func() time.Time { return now })

	require.NoError(t, err)
	require.Equal(t, now, normalized.Now)
}

// Invalid claim options must fail before a store attempts to lock records.
func TestClaimOptionsNormalizeRejectsUnsafeOptions(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	_, err := outbox.NormalizeClaimOptions(outbox.ClaimOptions{Limit: 0, LockedUntil: now.Add(time.Minute), ClaimedBy: "worker-1"}, func() time.Time { return now })
	require.ErrorIs(t, err, outbox.ErrInvalidClaimOptions)

	_, err = outbox.NormalizeClaimOptions(outbox.ClaimOptions{Limit: 1, LockedUntil: now.Add(time.Minute)}, func() time.Time { return now })
	require.ErrorIs(t, err, outbox.ErrInvalidClaimOptions)

	_, err = outbox.NormalizeClaimOptions(outbox.ClaimOptions{Limit: 1, LockedUntil: now, ClaimedBy: "worker-1"}, func() time.Time { return now })
	require.ErrorIs(t, err, outbox.ErrInvalidClaimOptions)
}
