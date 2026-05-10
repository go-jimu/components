package outbox

import "time"

type RetryPolicy interface {
	NextAttempt(record Record, err error, now time.Time) RetryDecision
}

type RetryDecision struct {
	Retry         bool
	NextAttemptAt time.Time
	Reason        string
}

type NoRetryPolicy struct{}

func (NoRetryPolicy) NextAttempt(_ Record, err error, _ time.Time) RetryDecision {
	return RetryDecision{Reason: errorReason(err)}
}

type FixedBackoffPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
}

func (p FixedBackoffPolicy) NextAttempt(record Record, err error, now time.Time) RetryDecision {
	decision := RetryDecision{Reason: errorReason(err)}
	if p.MaxAttempts > 0 && record.Attempts >= p.MaxAttempts {
		return decision
	}
	decision.Retry = true
	decision.NextAttemptAt = now.Add(p.Backoff)
	return decision
}

func errorReason(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
