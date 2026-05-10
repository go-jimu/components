package outbox

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Claim options must produce a safe lock window before a relay asks the store
// to claim records.
func TestClaimOptionsNormalize(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	opts := ClaimOptions{
		Limit:       10,
		LockedUntil: now.Add(time.Minute),
		ClaimedBy:   "worker-1",
	}

	normalized, err := opts.normalize(func() time.Time { return now })

	require.NoError(t, err)
	require.Equal(t, now, normalized.Now)
}

// Invalid claim options must fail before a store attempts to lock records.
func TestClaimOptionsNormalizeRejectsUnsafeOptions(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	_, err := (ClaimOptions{Limit: 0, LockedUntil: now.Add(time.Minute), ClaimedBy: "worker-1"}).normalize(func() time.Time { return now })
	require.ErrorIs(t, err, ErrInvalidClaimOptions)

	_, err = (ClaimOptions{Limit: 1, LockedUntil: now.Add(time.Minute)}).normalize(func() time.Time { return now })
	require.ErrorIs(t, err, ErrInvalidClaimOptions)

	_, err = (ClaimOptions{Limit: 1, LockedUntil: now, ClaimedBy: "worker-1"}).normalize(func() time.Time { return now })
	require.ErrorIs(t, err, ErrInvalidClaimOptions)
}
