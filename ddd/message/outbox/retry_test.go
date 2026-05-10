package outbox_test

import (
	"errors"
	"testing"
	"time"

	"github.com/go-jimu/components/ddd/message/outbox"
	"github.com/stretchr/testify/require"
)

// NoRetryPolicy must make failures terminal by default so relays do not create
// unbounded retry loops unless callers choose a retry policy.
func TestNoRetryPolicyReturnsTerminalDecision(t *testing.T) {
	decision := outbox.NoRetryPolicy{}.NextAttempt(outbox.Record{}, errors.New("publish failed"), time.Now())

	require.False(t, decision.Retry)
	require.True(t, decision.NextAttemptAt.IsZero())
	require.Equal(t, "publish failed", decision.Reason)
}

// FixedBackoffPolicy must retry below the max attempt count using the configured
// delay from the relay clock.
func TestFixedBackoffPolicyRetriesBelowMaxAttempts(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	policy := outbox.FixedBackoffPolicy{MaxAttempts: 3, Backoff: 2 * time.Minute}

	decision := policy.NextAttempt(outbox.Record{Attempts: 2}, errors.New("temporary"), now)

	require.True(t, decision.Retry)
	require.Equal(t, now.Add(2*time.Minute), decision.NextAttemptAt)
	require.Equal(t, "temporary", decision.Reason)
}

// FixedBackoffPolicy must stop when the current claim already reached the
// maximum total attempt count.
func TestFixedBackoffPolicyStopsAtMaxAttempts(t *testing.T) {
	policy := outbox.FixedBackoffPolicy{MaxAttempts: 3, Backoff: time.Minute}

	decision := policy.NextAttempt(outbox.Record{Attempts: 3}, errors.New("still failing"), time.Now())

	require.False(t, decision.Retry)
	require.True(t, decision.NextAttemptAt.IsZero())
	require.Equal(t, "still failing", decision.Reason)
}

// FixedBackoffPolicy must treat zero max attempts as unlimited so callers can
// opt out of terminal attempt counting without changing the backoff policy.
func TestFixedBackoffPolicyWithZeroMaxAttemptsRetriesAsUnlimited(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	policy := outbox.FixedBackoffPolicy{MaxAttempts: 0, Backoff: time.Minute}

	decision := policy.NextAttempt(outbox.Record{Attempts: 100}, errors.New("temporary"), now)

	require.True(t, decision.Retry)
	require.Equal(t, now.Add(time.Minute), decision.NextAttemptAt)
	require.Equal(t, "temporary", decision.Reason)
}

// FixedBackoffPolicy must treat negative max attempts as unlimited so invalid
// non-positive limits do not make high-attempt records terminal.
func TestFixedBackoffPolicyWithNegativeMaxAttemptsRetriesAsUnlimited(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	policy := outbox.FixedBackoffPolicy{MaxAttempts: -1, Backoff: time.Minute}

	decision := policy.NextAttempt(outbox.Record{Attempts: 100}, errors.New("temporary"), now)

	require.True(t, decision.Retry)
	require.Equal(t, now.Add(time.Minute), decision.NextAttemptAt)
	require.Equal(t, "temporary", decision.Reason)
}

// FixedBackoffPolicy must allow zero backoff so callers can request immediate
// retry scheduling at the relay clock time.
func TestFixedBackoffPolicyWithZeroBackoffSchedulesAtNow(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	policy := outbox.FixedBackoffPolicy{MaxAttempts: 3, Backoff: 0}

	decision := policy.NextAttempt(outbox.Record{Attempts: 1}, errors.New("temporary"), now)

	require.True(t, decision.Retry)
	require.Equal(t, now, decision.NextAttemptAt)
	require.Equal(t, "temporary", decision.Reason)
}

// FixedBackoffPolicy must treat negative backoff as immediate retry scheduling
// at the relay clock time.
func TestFixedBackoffPolicyWithNegativeBackoffSchedulesAtNow(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	backoff := -time.Minute
	policy := outbox.FixedBackoffPolicy{MaxAttempts: 3, Backoff: backoff}

	decision := policy.NextAttempt(outbox.Record{Attempts: 1}, errors.New("temporary"), now)

	require.True(t, decision.Retry)
	require.Equal(t, now, decision.NextAttemptAt)
	require.Equal(t, "temporary", decision.Reason)
}
